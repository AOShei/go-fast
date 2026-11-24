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
	"math"
	"path"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/AOShei/go-fast-core/inflate"
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

// maxGzipHeld bounds a gzip file that is read into memory to be
// decompressed with inflate, about twice as fast as compress/gzip, which
// reads a larger one. It is held while the file is walked, and counted
// by WalkOptions.Hold as a member is. A gzip file inside an archive is in
// memory already, and is decompressed with inflate whatever its size.
const maxGzipHeld = 1 << 30

// deflateRatio bounds how many bytes deflate can make of one: 1,032 at
// most. A member said to be larger than its compressed size allows is
// not believed, and no more than the size allowed is set aside for it.
const deflateRatio = 1032

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
	// them, and for an archive the member is in that was held for it.
	Release func()

	held int64 // what Hold accounted for the bytes
}

// Skipped is a member that was not read, and why.
type Skipped struct {
	Path   string
	Reason string
	// Limit says that a bound of this package left it out (as opposed to
	// a member that cannot be read: encrypted, compressed with a method
	// not supported, damaged).
	Limit bool
	// Note says that nothing is missing: the member is another name for
	// one that is read (a tar hard link).
	Note bool
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
	// hold accounts for bytes about to be held in memory; own is what
	// the walk itself holds at the moment (the archives it is inside).
	hold func(n, own int64) (func(), error)
	own  int64
	// counting: members are given to fn without their bytes (R nil),
	// to be counted, and none is decompressed but as the walk must.
	counting bool
}

// errBudget stops a walk whose decompression bound is reached.
var errBudget = errors.New("the bound on bytes decompressed from the archive was reached")

// errHold wraps an error from WalkOptions.Hold, which stops the walk.
var errHold = errors.New("gofast: holding a member's bytes")

// stops reports whether an error from reading a member ends the walk:
// the decompression bound, Hold refusing, or the walk cancelled.
func (w *archiveWalker) stops(err error) bool {
	return errors.Is(err, errBudget) || errors.Is(err, errHold) || w.ctx.Err() != nil
}

// walk calls fn for every regular file of the archive r, in the order the
// archive stores them, and skip for every member left out. fn owns each
// member and calls its Release. A nested archive is a member like any
// other: the caller decides whether to open it.
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
	// Deflated members are read with go-fast-core's inflate, about twice
	// as fast as compress/flate.
	z.RegisterDecompressor(zip.Deflate, inflate.Decompressor)
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
		case w.counting:
			m = Member{Path: name, Size: int64(min(f.UncompressedSize64, math.MaxInt64))}
		case f.Method == zip.Store && f.CompressedSize64 == f.UncompressedSize64:
			off, err := f.DataOffset()
			if err != nil || off < 0 || off > size || f.UncompressedSize64 > uint64(size-off) {
				skip(Skipped{Path: name, Reason: "damaged"})
				continue
			}
			m = Member{Path: name, Size: int64(f.UncompressedSize64), R: io.NewSectionReader(r, off, int64(f.UncompressedSize64))}
		default:
			if f.CompressedSize64 > uint64(size) {
				skip(Skipped{Path: name, Reason: "damaged"})
				continue
			}
			if f.UncompressedSize64 > uint64(w.maxMember) {
				skip(Skipped{Path: name, Reason: fmt.Sprintf("larger than %d bytes decompressed", w.maxMember), Limit: true})
				continue
			}
			// No more than deflate can make of the compressed bytes is
			// believed: a member said to be larger is damaged.
			if f.Method == zip.Deflate && f.UncompressedSize64 > f.CompressedSize64*deflateRatio+1024 {
				skip(Skipped{Path: name, Reason: "damaged: its size is more than its compressed size allows"})
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
			var rerr error
			m, rerr = w.read(name, rc, int64(f.UncompressedSize64), int64(f.UncompressedSize64), true)
			rc.Close()
			if w.stops(rerr) {
				return rerr
			}
			if rerr != nil {
				skip(Skipped{Path: name, Reason: rerr.Error(), Limit: errors.Is(rerr, errMember)})
				continue
			}
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
		spans = append(spans, span{max(start, 0), off + int64(min(f.CompressedSize64, math.MaxInt64/2))})
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

// sparse reports whether a tar member is a sparse file, in any of the GNU
// forms: its bytes in the archive are not its content, which has holes
// the archive does not store.
func sparse(h *tar.Header) bool {
	if h.Typeflag == tar.TypeGNUSparse {
		return true
	}
	for k := range h.PAXRecords {
		if strings.HasPrefix(k, "GNU.sparse.") {
			return true
		}
	}
	return false
}

// walkTar reads a tar stream. inPlace says that src is the archive file
// itself (not decompressed), whose members can be read where they are.
func (w *archiveWalker) walkTar(src io.Reader, inPlace bool, fn func(Member) error, skip func(Skipped)) error {
	tr := tar.NewReader(src)
	sr, _ := src.(*io.SectionReader)
	seen := map[string]bool{} // regular members, for hard links to them
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
		if h.Typeflag == tar.TypeLink {
			if name, ok := memberPath(h.Name); ok {
				target, _ := memberPath(h.Linkname)
				if seen[target] {
					skip(Skipped{Path: name, Reason: fmt.Sprintf("a hard link to %s, which is read under that name", target), Note: true})
				} else {
					skip(Skipped{Path: name, Reason: fmt.Sprintf("a hard link to %s, which is not in the archive before it", target)})
				}
			}
			continue
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA && h.Typeflag != tar.TypeGNUSparse {
			continue
		}
		name, ok := memberPath(h.Name)
		if !ok {
			skip(Skipped{Path: cleanName(h.Name), Reason: "its name is not a path inside the archive"})
			continue
		}
		seen[name] = true
		if sparse(h) {
			skip(Skipped{Path: name, Reason: "a sparse file, which is not read"})
			continue
		}
		if h.Size < 0 {
			skip(Skipped{Path: name, Reason: "damaged"})
			continue
		}
		var m Member
		switch {
		case w.counting:
			m = Member{Path: name, Size: h.Size}
		case inPlace && sr != nil:
			off, err := sr.Seek(0, io.SeekCurrent)
			if err != nil || h.Size > sr.Size()-off {
				skip(Skipped{Path: name, Reason: "damaged"})
				continue
			}
			m = Member{Path: name, Size: h.Size, R: io.NewSectionReader(sr, off, h.Size)}
		default:
			if h.Size > w.maxMember {
				skip(Skipped{Path: name, Reason: fmt.Sprintf("larger than %d bytes decompressed", w.maxMember), Limit: true})
				continue
			}
			// The stream is taken from the bound as it is read.
			var rerr error
			m, rerr = w.read(name, tr, h.Size, h.Size, false)
			if w.stops(rerr) {
				return rerr
			}
			if rerr != nil {
				return fmt.Errorf("gofast: tar: %v: %w", rerr, ErrArchive)
			}
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
	// zr has read the header, for the name. The data is decompressed
	// with go-fast-core's inflate when it is in memory, or small enough
	// to be read into it (maxGzipHeld).
	var stream io.Reader = zr
	if b, ok := r.(bytesReaderAt); ok && int64(len(b)) >= size {
		stream = inflate.NewGzipReader(b[:size], 1<<62)
	} else if size <= maxGzipHeld {
		if w.hold != nil {
			release, err := w.hold(size, w.own)
			if err != nil {
				return fmt.Errorf("%w: %w", errHold, err)
			}
			defer release()
		}
		// The walk holds it until it is out of the file: what it
		// waits for must not wait for this.
		w.own += size
		defer func() { w.own -= size }()
		src := make([]byte, size)
		if n, _ := r.ReadAt(src, 0); int64(n) == size {
			stream = inflate.NewGzipReader(src, 1<<62)
		}
	}
	br := bufio.NewReaderSize(&budgetReader{r: stream, w: w}, 64<<10)
	head, _ := br.Peek(262)
	inner := innerName(name, zr.Header.Name)
	if len(head) >= 262 && string(head[257:262]) == "ustar" || strings.HasSuffix(strings.ToLower(inner), ".tar") {
		return w.walkTar(br, false, fn, skip)
	}
	if w.counting {
		return fn(Member{Path: inner})
	}
	// The stream says nothing of its size; deflate bounds it.
	m, err := w.read(inner, br, -1, size*deflateRatio+1024, false)
	if w.stops(err) {
		return err
	}
	if err != nil {
		if errors.Is(err, errMember) {
			skip(Skipped{Path: inner, Reason: err.Error(), Limit: true})
			return nil
		}
		return fmt.Errorf("gofast: gzip: %v: %w", err, ErrArchive)
	}
	return fn(m)
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
// already takes them, as the stream of a gzip file does). declared is
// the size the archive gives it, -1 if none; most is the most it can
// plausibly be, which is what is held in memory for it and set aside.
func (w *archiveWalker) read(name string, src io.Reader, declared, most int64, budget bool) (Member, error) {
	limit := min(w.maxMember, math.MaxInt64-1)
	held := min(limit, max(most, 0))
	release := func() {}
	if w.hold != nil {
		r, err := w.hold(held, w.own)
		if err != nil {
			return Member{}, fmt.Errorf("%w: %w", errHold, err)
		}
		release = r
	}
	buf := make([]byte, 0, min(held, 64<<20))
	b := bytesWriter{buf: buf}
	n, err := io.Copy(&b, io.LimitReader(&budgetReader{r: src, w: w, unbudgeted: !budget}, limit+1))
	switch {
	case err != nil:
	case n > limit:
		err = fmt.Errorf("%w (%d bytes)", errMember, limit)
	case declared >= 0 && n != declared:
		err = errors.New("damaged: its size is not the size the archive gives")
	case w.ctx.Err() != nil:
		err = w.ctx.Err()
	}
	if err != nil {
		release()
		return Member{}, err
	}
	return Member{Path: name, Size: n, R: bytesReaderAt(b.buf), Release: release, held: held}, nil
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
		s = strings.ToValidUTF8(s, string(utf8.RuneError))
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
	// (or a gzip file read into memory to be decompressed) will take in
	// memory before they are read, and with own, the bytes
	// the walk itself holds at the moment (the archives it is inside,
	// which it cannot give back before it is out of them). It returns
	// the function that gives the bytes back (part of Member.Release).
	// It may wait, to bound the memory the members being read take
	// together, but not for own; an error stops the walk.
	Hold func(n, own int64) (release func(), err error)
}

// nest ties the bytes of an archive held in memory to its members: they
// are given back when the walk is out of it and every member read in
// place from them is released.
type nest struct {
	mu      sync.Mutex
	n       int
	release func()
}

func (r *nest) add() {
	r.mu.Lock()
	r.n++
	r.mu.Unlock()
}

func (r *nest) done() {
	r.mu.Lock()
	r.n--
	last := r.n == 0
	r.mu.Unlock()
	if last && r.release != nil {
		r.release()
	}
}

// wrap makes a member's release also release its share of the archive
// it is in.
func (r *nest) wrap(release func()) func() {
	r.add()
	var once sync.Once
	return func() {
		once.Do(func() {
			if release != nil {
				release()
			}
			r.done()
		})
	}
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
// error wrapping ErrArchive, after the members before the damage, and one
// stopped by MaxArchiveBytes an error wrapping ErrLimit.
func (e *Extractor) Walk(ctx context.Context, k Kind, name string, r io.ReaderAt, size int64, opts WalkOptions,
	fn func(Member) error, skip func(Skipped)) error {
	if !k.IsArchive() {
		return fmt.Errorf("gofast: %s is not an archive: %w", name, ErrArchive)
	}
	w := &archiveWalker{ctx: ctx, maxMember: resolve(e.MaxMemberBytes, DefaultMaxMemberBytes),
		left: resolve(e.MaxArchiveBytes, DefaultMaxArchiveBytes), hold: opts.Hold}
	maxDepth := int(resolve(int64(opts.MaxDepth), 4))
	var walk func(k Kind, name, prefix string, r io.ReaderAt, size int64, depth int, parent *nest) error
	walk = func(k Kind, name, prefix string, r io.ReaderAt, size int64, depth int, parent *nest) error {
		err := w.walk(k.Format, r, size, name, func(m Member) error {
			m.Path = prefix + m.Path
			if parent != nil {
				m.Release = parent.wrap(m.Release)
			}
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
			inner := &nest{n: 1, release: m.Release}
			w.own += m.held
			err := walk(mk, m.Path, m.Path+"/", m.R, m.Size, depth+1, inner)
			w.own -= m.held
			inner.done()
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
	return walk(k, name, "", r, size, 0, nil)
}

// ErrLimit wraps a walk stopped by MaxArchiveBytes.
var ErrLimit = errors.New("gofast: archive bound reached")

// resolve applies the Limits convention: zero is def, negative no bound.
func resolve(v, def int64) int64 {
	switch {
	case v == 0:
		return def
	case v < 0:
		return math.MaxInt64
	}
	return v
}
