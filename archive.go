package gofast

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Defaults of the bounds on archives.
const (
	// DefaultMaxMemberBytes bounds the decompressed size of one archive
	// member, which is held in memory to be read (a member stored
	// uncompressed in a zip or plain tar file is read in place).
	DefaultMaxMemberBytes = 1 << 30
	// DefaultMaxArchiveBytes bounds the bytes decompressed from one
	// archive and the archives inside it, the defence against an archive
	// that decompresses to far more than any real one.
	DefaultMaxArchiveBytes = 64 << 30
	// DefaultMaxArchiveMembers bounds the members of an archive read as
	// one document with the members as its children (an archive embedded
	// in a document, or Extract). The command reads each member of an
	// archive it is given as a document of its own, with no such bound.
	DefaultMaxArchiveMembers = 10000
)

// Member is one file in an archive.
type Member struct {
	// Path is the member's name in the archive, slash-separated, with no
	// leading slash, "." or ".." element, and no control character.
	Path string
	// Size is the member's size, decompressed.
	Size int64
	// R reads the member's bytes.
	R io.ReaderAt
	// Release, when not nil, is called once the member's bytes are no
	// longer needed: it gives back what WalkOptions.Hold accounted for
	// them.
	Release func()
}

// Skipped is a member that was not read, and why.
type Skipped struct {
	Path   string
	Reason string
	// Limit says that a bound of this package left it out (as opposed to
	// a member that cannot be read: encrypted, compressed with a method
	// not supported, damaged).
	Limit bool
}

// ErrArchive wraps an archive that cannot be read at all.
var ErrArchive = errors.New("gofast: archive cannot be read")

// archiveWalker reads the members of archives within bounds that hold
// for the whole walk, nested archives included.
type archiveWalker struct {
	ctx context.Context
	// maxMember bounds one member decompressed; left is what may still be
	// decompressed in this walk.
	maxMember int64
	left      int64
	// hold is called before a member's bytes are held in memory, and its
	// result after they are no longer needed (nil: no accounting).
	hold func(n int64) func()
}

// errBudget stops a walk whose decompression bound is reached.
var errBudget = errors.New("the bound on bytes decompressed from the archive was reached")

// walk calls fn for every regular file of the archive r, in the order the
// archive stores them, and skip for every member left out. A member held
// in memory is released when fn returns. A nested archive is a member like
// any other: the caller decides whether to open it.
func (w *archiveWalker) walk(format string, r io.ReaderAt, size int64, gzName string,
	fn func(Member) error, skip func(Skipped)) error {
	switch format {
	case "zip":
		return w.walkZip(r, size, fn, skip)
	case "tar":
		return w.walkTar(io.NewSectionReader(r, 0, size), true, fn, skip)
	case "gzip":
		return w.walkGzip(r, size, gzName, fn, skip)
	}
	return fmt.Errorf("gofast: archive format %q: %w", format, ErrArchive)
}

func (w *archiveWalker) walkZip(r io.ReaderAt, size int64, fn func(Member) error, skip func(Skipped)) error {
	z, err := zip.NewReader(r, size)
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return fmt.Errorf("gofast: zip: %v: %w", err, ErrArchive)
	}
	if err := overlapping(z.File); err != nil {
		return err
	}
	for _, f := range z.File {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		mode := f.Mode()
		if f.FileInfo().IsDir() || mode&^0o777 != 0 && !mode.IsRegular() {
			continue
		}
		name, ok := memberPath(f.Name)
		if !ok {
			skip(Skipped{Path: cleanName(f.Name), Reason: "its name is not a path inside the archive"})
			continue
		}
		if f.Flags&1 != 0 {
			skip(Skipped{Path: name, Reason: "encrypted"})
			continue
		}
		var m Member
		switch {
		case f.Method == zip.Store && f.CompressedSize64 == f.UncompressedSize64:
			off, err := f.DataOffset()
			if err != nil || off < 0 || off > size || int64(f.UncompressedSize64) > size-off {
				skip(Skipped{Path: name, Reason: "damaged"})
				continue
			}
			m = Member{Path: name, Size: int64(f.UncompressedSize64), R: io.NewSectionReader(r, off, int64(f.UncompressedSize64))}
		default:
			if int64(f.UncompressedSize64) > w.maxMember || f.UncompressedSize64 > 1<<62 {
				skip(Skipped{Path: name, Reason: fmt.Sprintf("larger than %d bytes decompressed", w.maxMember), Limit: true})
				continue
			}
			rc, err := f.Open()
			if err != nil {
				reason := "damaged"
				if errors.Is(err, zip.ErrAlgorithm) {
					reason = fmt.Sprintf("compressed with a method not supported (%d)", f.Method)
				}
				skip(Skipped{Path: name, Reason: reason})
				continue
			}
			data, rel, err := w.read(rc, int64(f.UncompressedSize64), true)
			rc.Close()
			if errors.Is(err, errBudget) {
				return err
			}
			if err != nil {
				skip(Skipped{Path: name, Reason: err.Error(), Limit: errors.Is(err, errMember)})
				continue
			}
			m = Member{Path: name, Size: int64(len(data)), R: bytesReaderAt(data), Release: rel}
		}
		if err := fn(m); err != nil {
			return err
		}
	}
	return nil
}

// overlapping refuses a zip archive whose members overlap, the
// construction of a zip bomb that decompresses one stream many times
// over (a real archive never shares bytes between members).
func overlapping(files []*zip.File) error {
	type span struct{ start, end int64 }
	spans := make([]span, 0, len(files))
	for _, f := range files {
		off, err := f.DataOffset()
		if err != nil {
			continue
		}
		start := off - 30 - int64(len(f.Name)) // the local header, at least
		spans = append(spans, span{max(start, 0), off + int64(f.CompressedSize64)})
	}
	slices.SortFunc(spans, func(a, b span) int {
		switch {
		case a.start < b.start:
			return -1
		case a.start > b.start:
			return 1
		}
		return 0
	})
	for i := 1; i < len(spans); i++ {
		if spans[i].start < spans[i-1].end {
			return fmt.Errorf("gofast: zip: members overlap, which only a zip bomb does: %w", ErrArchive)
		}
	}
	return nil
}

// walkTar reads a tar stream. inPlace says that src is the archive file
// itself (not decompressed), whose members can be read where they are.
func (w *archiveWalker) walkTar(src io.Reader, inPlace bool, fn func(Member) error, skip func(Skipped)) error {
	tr := tar.NewReader(src)
	sr, _ := src.(*io.SectionReader)
	for {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			if errors.Is(err, errBudget) {
				return err
			}
			return fmt.Errorf("gofast: tar: %v: %w", err, ErrArchive)
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			if h.Typeflag == tar.TypeGNUSparse {
				skip(Skipped{Path: cleanName(h.Name), Reason: "a sparse file"})
			}
			continue
		}
		name, ok := memberPath(h.Name)
		if !ok {
			skip(Skipped{Path: cleanName(h.Name), Reason: "its name is not a path inside the archive"})
			continue
		}
		if h.Size < 0 {
			skip(Skipped{Path: name, Reason: "damaged"})
			continue
		}
		var m Member
		if inPlace && sr != nil {
			off, err := sr.Seek(0, io.SeekCurrent)
			if err != nil || h.Size > sr.Size()-off {
				skip(Skipped{Path: name, Reason: "damaged"})
				continue
			}
			m = Member{Path: name, Size: h.Size, R: io.NewSectionReader(sr, off, h.Size)}
		} else {
			if h.Size > w.maxMember {
				skip(Skipped{Path: name, Reason: fmt.Sprintf("larger than %d bytes decompressed", w.maxMember), Limit: true})
				continue
			}
			data, rel, err := w.read(tr, h.Size, false)
			if errors.Is(err, errBudget) {
				return err
			}
			if err != nil {
				return fmt.Errorf("gofast: tar: %v: %w", err, ErrArchive)
			}
			m = Member{Path: name, Size: int64(len(data)), R: bytesReaderAt(data), Release: rel}
		}
		if err := fn(m); err != nil {
			return err
		}
	}
}

// walkGzip reads a gzip file: a tar archive inside it is walked as one,
// and anything else is its one member, named by the gzip header or by the
// file's name without .gz.
func (w *archiveWalker) walkGzip(r io.ReaderAt, size int64, name string, fn func(Member) error, skip func(Skipped)) error {
	zr, err := gzip.NewReader(io.NewSectionReader(r, 0, size))
	if err != nil {
		return fmt.Errorf("gofast: gzip: %v: %w", err, ErrArchive)
	}
	defer zr.Close()
	counted := &budgetReader{r: zr, w: w}
	br := bufio.NewReaderSize(counted, 64<<10)
	head, _ := br.Peek(262)
	inner := innerName(name, zr.Header.Name)
	if len(head) >= 262 && string(head[257:262]) == "ustar" || strings.HasSuffix(strings.ToLower(inner), ".tar") {
		return w.walkTar(br, false, fn, skip)
	}
	data, rel, err := w.read(br, -1, false)
	if errors.Is(err, errBudget) {
		return err
	}
	if err != nil {
		if errors.Is(err, errMember) {
			skip(Skipped{Path: inner, Reason: err.Error(), Limit: true})
			return nil
		}
		return fmt.Errorf("gofast: gzip: %v: %w", err, ErrArchive)
	}
	return fn(Member{Path: inner, Size: int64(len(data)), R: bytesReaderAt(data), Release: rel})
}

// innerName is the name of the file a gzip file holds: the name its
// header records, else the gzip file's own name without .gz (.tgz is
// .tar).
func innerName(outer, header string) string {
	if n, ok := memberPath(path.Base(strings.ReplaceAll(header, `\`, "/"))); ok && header != "" {
		return n
	}
	base := path.Base(strings.ReplaceAll(outer, `\`, "/"))
	lower := strings.ToLower(base)
	switch {
	case strings.HasSuffix(lower, ".tgz"):
		base = base[:len(base)-4] + ".tar"
	case strings.HasSuffix(lower, ".gz"):
		base = base[:len(base)-3]
	}
	if n, ok := memberPath(base); ok {
		return n
	}
	return "data"
}

// errMember is a member larger than the bound on one member.
var errMember = errors.New("larger than the bound on a member decompressed")

// read reads a member into memory: at most w.maxMember bytes, taken from
// the walk's decompression bound when budget is set (otherwise src
// already takes them, as the stream of a gzip file does). declared is the
// size the archive gives it, -1 if none. The function returned releases
// what hold accounted.
func (w *archiveWalker) read(src io.Reader, declared int64, budget bool) ([]byte, func(), error) {
	want := w.maxMember
	if declared >= 0 {
		want = min(declared, w.maxMember)
	}
	release := func() {}
	if w.hold != nil {
		n := declared
		if n < 0 {
			n = w.maxMember // a gzip stream does not say
		}
		release = w.hold(min(n, w.maxMember))
	}
	var buf []byte
	if declared >= 0 {
		buf = make([]byte, 0, declared)
	}
	b := bytesWriter{buf: buf}
	n, err := io.Copy(&b, io.LimitReader(&budgetReader{r: src, w: w, unbudgeted: !budget}, want+1))
	if err != nil {
		release()
		return nil, nil, err
	}
	if n > w.maxMember {
		release()
		return nil, nil, fmt.Errorf("%w (%d bytes)", errMember, w.maxMember)
	}
	if declared >= 0 && n != declared {
		release()
		return nil, nil, errors.New("damaged: its size is not the size the archive gives")
	}
	return b.buf, release, nil
}

// budgetReader takes what it reads from the walk's decompression bound,
// unless unbudgeted (when the source already does, as a gzip stream does).
type budgetReader struct {
	r          io.Reader
	w          *archiveWalker
	unbudgeted bool
}

func (b *budgetReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if !b.unbudgeted {
		if b.w.left -= int64(n); b.w.left < 0 {
			return n, errBudget
		}
	}
	return n, err
}

type bytesWriter struct{ buf []byte }

func (b *bytesWriter) Write(p []byte) (int, error) { b.buf = append(b.buf, p...); return len(p), nil }

// bytesReaderAt reads from a byte slice.
type bytesReaderAt []byte

func (b bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("gofast: negative offset")
	}
	if off >= int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// memberPath cleans a member's name: slash-separated, without a leading
// slash or drive, "." and empty elements, and control characters. A name
// with a ".." element, or nothing left, is not a path inside the archive.
func memberPath(name string) (string, bool) {
	name = cleanName(strings.ReplaceAll(name, `\`, "/"))
	if len(name) >= 2 && name[1] == ':' {
		name = name[2:]
	}
	var out []string
	for _, el := range strings.Split(name, "/") {
		switch el {
		case "", ".":
			continue
		case "..":
			return "", false
		}
		out = append(out, el)
	}
	if len(out) == 0 {
		return "", false
	}
	return strings.Join(out, "/"), true
}

// cleanName makes a name valid UTF-8 without control characters.
func cleanName(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "\uFFFD")
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// WalkOptions bounds a walk of an archive.
type WalkOptions struct {
	// MaxDepth bounds how deeply archives inside the archive are opened:
	// 1 opens none. Zero means gofastcore.DefaultMaxDepth.
	MaxDepth int
	// Hold, when not nil, is called with the number of bytes a member
	// will take in memory before they are read, and returns the function
	// that gives them back (Member.Release). It may block, to bound the
	// memory the members being read take together.
	Hold func(n int64) (release func())
}

// Walk calls fn for every file of the archive r (whose Kind is k, as
// Detect gives it, and whose name is name), in the order the archive
// stores them, and skip for every member left out. An archive inside it
// is opened in turn, its members' paths then beginning with its own
// ("a.zip/b.tar/c.pdf"), down to opts.MaxDepth; a deeper one is skipped.
// The bounds of e (MaxMemberBytes, MaxArchiveBytes) hold for the whole
// walk. fn owns each Member: it calls Release, when set, once it no longer
// needs the bytes, and may do so after Walk returns. An error from fn
// stops the walk and is returned; an archive that cannot be read gives an
// error wrapping ErrArchive, after the members before the damage.
func (e *Extractor) Walk(ctx context.Context, k Kind, name string, r io.ReaderAt, size int64, opts WalkOptions,
	fn func(Member) error, skip func(Skipped)) error {
	if !k.IsArchive() {
		return fmt.Errorf("gofast: %s is not an archive: %w", name, ErrArchive)
	}
	w := &archiveWalker{ctx: ctx, maxMember: resolve(e.MaxMemberBytes, DefaultMaxMemberBytes),
		left: resolve(e.MaxArchiveBytes, DefaultMaxArchiveBytes), hold: opts.Hold}
	maxDepth := int(resolve(int64(opts.MaxDepth), 4))
	var walk func(k Kind, name, prefix string, r io.ReaderAt, size int64, depth int) error
	walk = func(k Kind, name, prefix string, r io.ReaderAt, size int64, depth int) error {
		err := w.walk(k.Format, r, size, name, func(m Member) error {
			m.Path = prefix + m.Path
			mk := Detect(m.Path, "", m.R, m.Size)
			if !mk.IsArchive() {
				return fn(m)
			}
			if depth+1 >= maxDepth {
				if m.Release != nil {
					m.Release()
				}
				skip(Skipped{Path: m.Path, Reason: fmt.Sprintf("an archive nested more than %d deep", maxDepth), Limit: true})
				return nil
			}
			err := walk(mk, m.Path, m.Path+"/", m.R, m.Size, depth+1)
			if m.Release != nil {
				m.Release()
			}
			if errors.Is(err, ErrArchive) {
				// A damaged archive inside: what was read of it stands.
				skip(Skipped{Path: m.Path, Reason: err.Error()})
				return nil
			}
			return err
		}, func(s Skipped) {
			s.Path = prefix + s.Path
			skip(s)
		})
		if errors.Is(err, errBudget) {
			return fmt.Errorf("gofast: %s: %v (%d bytes): %w", name, errBudget, resolve(e.MaxArchiveBytes, DefaultMaxArchiveBytes), ErrLimit)
		}
		return err
	}
	return walk(k, name, "", r, size, 0)
}

// ErrLimit wraps a walk stopped by MaxArchiveBytes.
var ErrLimit = errors.New("gofast: archive bound reached")

// resolve applies the Limits convention: zero is def, negative no bound.
func resolve(v, def int64) int64 {
	switch {
	case v == 0:
		return def
	case v < 0:
		return 1<<63 - 1
	}
	return v
}
