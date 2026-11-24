package gofast

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	gofastcore "github.com/AOShei/go-fast-core"
)

type file struct {
	name, body string
	store      bool
}

func zipOf(t testing.TB, files ...file) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, f := range files {
		h := &zip.FileHeader{Name: f.name, Method: zip.Deflate}
		if f.store {
			h.Method = zip.Store
		}
		w, err := z.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(f.body))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func tarOf(t testing.TB, files ...file) []byte {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for _, f := range files {
		if err := w.WriteHeader(&tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(f.body))
	}
	w.Close()
	return b.Bytes()
}

func gz(b []byte, name string) []byte {
	var out bytes.Buffer
	w := gzip.NewWriter(&out)
	w.Name = name
	w.Write(b)
	w.Close()
	return out.Bytes()
}

func TestDetect(t *testing.T) {
	docx := zipOf(t, file{name: "[Content_Types].xml", body: "<Types/>"})
	odt := zipOf(t, file{name: "mimetype", body: "application/vnd.oasis.opendocument.text", store: true})
	epub := zipOf(t, file{name: "mimetype", body: "application/epub+zip", store: true})
	plain := zipOf(t, file{name: "a.txt", body: "x"})
	for _, c := range []struct {
		name, mime string
		body       []byte
		want       Kind
	}{
		{"a.PDF", "", nil, Kind{LibPDF, ""}},
		{"x", "application/pdf; name=x", nil, Kind{LibPDF, ""}},
		{"x.bin", "", []byte("junk %PDF-1.7\n"), Kind{LibPDF, ""}},
		{"report.docx", "", nil, Kind{LibOOXML, ""}},
		{"noext", "", docx, Kind{LibOOXML, ""}},
		{"noext", "", odt, Kind{LibODF, ""}},
		{"noext", "", epub, Kind{LibHTML, "epub"}},
		{"noext", "", plain, Kind{LibArchive, "zip"}},
		{"noext", "application/octet-stream", plain, Kind{LibArchive, "zip"}},
		{"a.tar.gz", "", nil, Kind{LibArchive, "gzip"}},
		{"a.tgz", "", nil, Kind{LibArchive, "gzip"}},
		{"noext", "", gz([]byte("x"), ""), Kind{LibArchive, "gzip"}},
		{"noext", "", tarOf(t, file{name: "a", body: "b"}), Kind{LibArchive, "tar"}},
		{"noext", "", []byte(`{\rtf1 x}`), Kind{LibRTF, ""}},
		{"noext", "", []byte("From a@b Mon Jan  1 00:00:00 2024\n"), Kind{LibEmail, ""}},
		{"noext", "", []byte("Received: x\nSubject: hi\n\nbody"), Kind{LibEmail, ""}},
		{"WINMAIL.DAT", "", nil, Kind{LibEmail, ""}},
		{"noext", "", []byte("<!doctype html><p>x"), Kind{LibHTML, "html"}},
		{"noext", "", []byte("Name: value\nOther: thing\n"), Kind{LibHTML, "txt"}},
		{"noext", "", []byte("plain words"), Kind{LibHTML, "txt"}},
		{"x.csv", "", nil, Kind{LibHTML, "csv"}},
		{"x", "text/csv; header=present", nil, Kind{LibHTML, "csv"}},
		{"nb.ipynb", "", nil, Kind{LibHTML, "ipynb"}},
		{"data.JSON", "", nil, Kind{LibHTML, "json"}},
		{"log.ndjson", "", nil, Kind{LibHTML, "json"}},
		{"map.geojson", "", nil, Kind{LibHTML, "json"}},
		{"x", "application/json; charset=utf-8", nil, Kind{LibHTML, "json"}},
		{"conf.yml", "", nil, Kind{LibHTML, "yaml"}},
		{"x", "application/yaml", nil, Kind{LibHTML, "yaml"}},
		{"noext", "", []byte(`{"a": 1}`), Kind{LibHTML, "txt"}}, // never by content
		{"old.doc", "", []byte("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1"), Kind{}},
		{"photo.jpg", "", []byte("\xFF\xD8\xFF\xE0\x00\x10JFIF\x00"), Kind{}},
		{"noext", "", []byte("\xFF\xFEa\x00b\x00"), Kind{LibHTML, "txt"}},
		{"empty", "", nil, Kind{LibHTML, "txt"}},
	} {
		got := Detect(c.name, c.mime, bytes.NewReader(c.body), int64(len(c.body)))
		if got != c.want {
			t.Errorf("Detect(%q, %q) = %+v, want %+v", c.name, c.mime, got, c.want)
		}
	}
}

// walkAll walks an archive and returns its members' paths and bodies, and
// the skipped.
func walkAll(t *testing.T, e *Extractor, k Kind, name string, b []byte, opts WalkOptions) ([]string, []Skipped, error) {
	t.Helper()
	var got []string
	var skipped []Skipped
	err := e.Walk(context.Background(), k, name, bytes.NewReader(b), int64(len(b)), opts, func(m Member) error {
		data := make([]byte, m.Size)
		if _, err := m.R.ReadAt(data, 0); err != nil && m.Size > 0 {
			t.Fatalf("%s: %v", m.Path, err)
		}
		got = append(got, m.Path+"="+string(data))
		if m.Release != nil {
			m.Release()
		}
		return nil
	}, func(s Skipped) { skipped = append(skipped, s) })
	return got, skipped, err
}

func TestWalkFormats(t *testing.T) {
	files := []file{{name: "a.txt", body: "one"}, {name: "dir/b.md", body: "two", store: true}, {name: "dir/", body: ""}}
	want := "a.txt=one,dir/b.md=two"
	inner := tarOf(t, file{name: "x.txt", body: "deep"})
	for name, c := range map[string]struct {
		k    Kind
		file string
		b    []byte
		want string
	}{
		"zip":        {Kind{LibArchive, "zip"}, "a.zip", zipOf(t, files...), want},
		"tar":        {Kind{LibArchive, "tar"}, "a.tar", tarOf(t, files[:2]...), want},
		"tar.gz":     {Kind{LibArchive, "gzip"}, "a.tar.gz", gz(tarOf(t, files[:2]...), ""), want},
		"tgz":        {Kind{LibArchive, "gzip"}, "a.tgz", gz(tarOf(t, files[:2]...), ""), want},
		"gz":         {Kind{LibArchive, "gzip"}, "notes.txt.gz", gz([]byte("text"), ""), "notes.txt=text"},
		"gz header":  {Kind{LibArchive, "gzip"}, "x.gz", gz([]byte("text"), "orig.md"), "orig.md=text"},
		"nested":     {Kind{LibArchive, "zip"}, "o.zip", zipOf(t, file{name: "in.tar", body: string(inner)}, file{name: "z.txt", body: "z"}), "in.tar/x.txt=deep,z.txt=z"},
		"traversal":  {Kind{LibArchive, "zip"}, "t.zip", zipOf(t, file{name: "/abs/./a.txt", body: "a"}, file{name: `C:\win\b.txt`, body: "b"}), "abs/a.txt=a,win/b.txt=b"},
		"empty zip":  {Kind{LibArchive, "zip"}, "e.zip", zipOf(t), ""},
		"empty tar":  {Kind{LibArchive, "tar"}, "e.tar", tarOf(t), ""},
		"control ch": {Kind{LibArchive, "zip"}, "c.zip", zipOf(t, file{name: "a\x01b.txt", body: "c"}), "ab.txt=c"},
	} {
		got, skipped, err := walkAll(t, &Extractor{}, c.k, c.file, c.b, WalkOptions{})
		if err != nil || strings.Join(got, ",") != c.want || len(skipped) != 0 {
			t.Errorf("%s: %q %+v %v", name, got, skipped, err)
		}
	}
}

func TestWalkSkips(t *testing.T) {
	// ".." is not a path inside the archive.
	_, skipped, err := walkAll(t, &Extractor{}, Kind{LibArchive, "tar"}, "t.tar", tarOf(t, file{name: "../evil", body: "x"}), WalkOptions{})
	if err != nil || len(skipped) != 1 || skipped[0].Limit {
		t.Errorf("traversal: %+v %v", skipped, err)
	}
	// A member larger than the bound.
	big := zipOf(t, file{name: "big.txt", body: strings.Repeat("a", 5000)}, file{name: "s.txt", body: "s"})
	got, skipped, err := walkAll(t, &Extractor{MaxMemberBytes: 1000}, Kind{LibArchive, "zip"}, "b.zip", big, WalkOptions{})
	if err != nil || strings.Join(got, ",") != "s.txt=s" || len(skipped) != 1 || !skipped[0].Limit {
		t.Errorf("member bound: %q %+v %v", got, skipped, err)
	}
	// The bound on bytes decompressed from the archive.
	_, _, err = walkAll(t, &Extractor{MaxArchiveBytes: 1000}, Kind{LibArchive, "gzip"}, "x.gz", gz([]byte(strings.Repeat("a", 1<<20)), ""), WalkOptions{})
	if !errors.Is(err, ErrLimit) {
		t.Errorf("archive bound: %v", err)
	}
	// Depth.
	level := zipOf(t, file{name: "leaf.txt", body: "leaf"})
	for range 5 {
		level = zipOf(t, file{name: "n.zip", body: string(level)})
	}
	got, skipped, err = walkAll(t, &Extractor{}, Kind{LibArchive, "zip"}, "d.zip", level, WalkOptions{MaxDepth: 3})
	if err != nil || len(got) != 0 || len(skipped) != 1 || !skipped[0].Limit || skipped[0].Path != "n.zip/n.zip/n.zip" {
		t.Errorf("depth: %q %+v %v", got, skipped, err)
	}
	// An encrypted member.
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, _ := z.CreateHeader(&zip.FileHeader{Name: "secret.txt", Method: zip.Store, Flags: 1})
	w.Write([]byte("x"))
	z.Close()
	_, skipped, err = walkAll(t, &Extractor{}, Kind{LibArchive, "zip"}, "e.zip", b.Bytes(), WalkOptions{})
	if err != nil || len(skipped) != 1 || skipped[0].Reason != "encrypted" {
		t.Errorf("encrypted: %+v %v", skipped, err)
	}
	// Damaged archives.
	for name, data := range map[string][]byte{"zip": []byte("PK\x03\x04junk"), "gzip": []byte{0x1f, 0x8b, 0, 0}, "tar": bytes.Repeat([]byte{'x'}, 1024)} {
		if _, _, err := walkAll(t, &Extractor{}, Kind{LibArchive, name}, "x", data, WalkOptions{}); !errors.Is(err, ErrArchive) {
			t.Errorf("damaged %s: %v", name, err)
		}
	}
}

// TestWalkOverlap refuses a zip whose central directory names one stream
// twice: the construction of the zip bombs that reach petabytes.
func TestWalkOverlap(t *testing.T) {
	b := zipOf(t, file{name: "a.txt", body: strings.Repeat("a", 1000)})
	eocd := bytes.LastIndex(b, []byte("PK\x05\x06"))
	cdOff := int(binary.LittleEndian.Uint32(b[eocd+16:]))
	cd := append([]byte{}, b[cdOff:eocd]...)
	dup := append([]byte{}, cd...)
	// Same local header offset, another name of the same length.
	copy(dup[46:], "b.txt")
	out := append(append(append([]byte{}, b[:cdOff]...), cd...), dup...)
	end := append([]byte{}, b[eocd:]...)
	binary.LittleEndian.PutUint16(end[8:], 2)
	binary.LittleEndian.PutUint16(end[10:], 2)
	binary.LittleEndian.PutUint32(end[12:], uint32(2*len(cd)))
	out = append(out, end...)
	if _, _, err := walkAll(t, &Extractor{}, Kind{LibArchive, "zip"}, "bomb.zip", out, WalkOptions{}); !errors.Is(err, ErrArchive) {
		t.Errorf("overlap: %v", err)
	}
}

func TestHold(t *testing.T) {
	held, max := int64(0), int64(0)
	hold := func(n, own int64) (func(), error) {
		held += n
		if held > max {
			max = held
		}
		return func() { held -= n }, nil
	}
	b := zipOf(t, file{name: "a.txt", body: strings.Repeat("a", 100)}, file{name: "b.txt", body: strings.Repeat("b", 200)})
	if _, _, err := walkAll(t, &Extractor{}, Kind{LibArchive, "zip"}, "h.zip", b, WalkOptions{Hold: hold}); err != nil {
		t.Fatal(err)
	}
	if held != 0 || max != 200 {
		t.Errorf("held %d, max %d", held, max)
	}
}

// TestHoldNested: the bytes of an archive inside an archive stay held
// until the members read in place from them are released, and the walk
// tells Hold what it holds itself, so that a bound smaller than the
// archives it is inside does not make it wait for ever.
func TestHoldNested(t *testing.T) {
	inner := zipOf(t, file{name: "big.txt", body: strings.Repeat("x", 1<<16), store: true})
	outer := zipOf(t, file{name: "inner.zip", body: string(inner)}, file{name: "n.txt.gz", body: string(gz([]byte("gz"), ""))})
	held := int64(0)
	var owns []int64
	hold := func(n, own int64) (func(), error) {
		owns = append(owns, own)
		held += n
		return func() { held -= n }, nil
	}
	var pending []Member
	err := (&Extractor{}).Walk(context.Background(), Kind{LibArchive, "zip"}, "o.zip", bytes.NewReader(outer), int64(len(outer)),
		WalkOptions{Hold: hold}, func(m Member) error { pending = append(pending, m); return nil }, func(s Skipped) { t.Errorf("skipped %+v", s) })
	if err != nil || len(pending) != 2 || pending[0].Path != "inner.zip/big.txt" || pending[1].Path != "n.txt.gz/n.txt" {
		t.Fatalf("%v %+v", err, pending)
	}
	if held < int64(len(inner)) {
		t.Errorf("inner.zip given back while its member is unread: held %d", held)
	}
	for _, m := range pending {
		m.Release()
	}
	if held != 0 {
		t.Errorf("held %d after all released", held)
	}
	// Holding n.txt.gz's member, the walk is inside n.txt.gz.
	if len(owns) != 3 || owns[0] != 0 || owns[2] == 0 {
		t.Errorf("own: %v", owns)
	}
}

// TestHoldGzip: a gzip file read into memory to be decompressed is held
// while it is walked, and the walk counts it as its own, so that its
// members do not wait for it.
func TestHoldGzip(t *testing.T) {
	b := gz(tarOf(t, file{name: "a.txt", body: strings.Repeat("a", 3000)}, file{name: "b.txt", body: "b"}), "")
	held := int64(0)
	type call struct{ n, own int64 }
	var calls []call
	hold := func(n, own int64) (func(), error) {
		calls = append(calls, call{n, own})
		held += n
		return func() { held -= n }, nil
	}
	var pending []Member
	err := (&Extractor{}).Walk(context.Background(), Kind{LibArchive, "gzip"}, "h.tar.gz", bytes.NewReader(b), int64(len(b)),
		WalkOptions{Hold: hold}, func(m Member) error { pending = append(pending, m); return nil }, func(s Skipped) { t.Errorf("skipped %+v", s) })
	if err != nil || len(pending) != 2 {
		t.Fatalf("%v %+v", err, pending)
	}
	size := int64(len(b))
	if len(calls) != 3 || calls[0] != (call{size, 0}) || calls[1] != (call{3000, size}) || calls[2] != (call{1, size}) {
		t.Errorf("calls %v", calls)
	}
	// The file is given back when the walk is out of it; the members are
	// copies of its output.
	if held != 3001 {
		t.Errorf("held %d after the walk", held)
	}
	if got, _ := io.ReadAll(io.NewSectionReader(pending[0].R, 0, pending[0].Size)); string(got) != strings.Repeat("a", 3000) {
		t.Errorf("a.txt: %q", got)
	}
	for _, m := range pending {
		m.Release()
	}
	if held != 0 {
		t.Errorf("held %d after all released", held)
	}
}

// paxSparse is a tar archive of a GNU 1.0 sparse member "s" (its PAX
// records written by hand, as Go's tar writer does not write them) and a
// regular "next.txt".
func paxSparse(t *testing.T) []byte {
	t.Helper()
	records := ""
	for _, kv := range [][2]string{{"GNU.sparse.major", "1"}, {"GNU.sparse.minor", "0"}, {"GNU.sparse.name", "s"}, {"GNU.sparse.realsize", "100000000"}} {
		rec := " " + kv[0] + "=" + kv[1] + "\n"
		n := len(rec) + 2
		if len(fmt.Sprint(n)+rec) != n {
			n++
		}
		records += fmt.Sprint(n) + rec
	}
	block := func(name string, size int, flag byte) []byte {
		var b bytes.Buffer
		w := tar.NewWriter(&b)
		w.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Size: int64(size), Format: tar.FormatUSTAR})
		h := append([]byte{}, b.Bytes()[:512]...)
		h[156] = flag
		copy(h[148:156], "        ")
		sum := 0
		for _, c := range h {
			sum += int(c)
		}
		copy(h[148:156], fmt.Sprintf("%06o\x00 ", sum))
		return h
	}
	pad := func(b []byte) []byte { return append(b, make([]byte, (512-len(b)%512)%512)...) }
	data := "1\n0\n3\n" + strings.Repeat("\x00", 512-6) + "abc"
	var out []byte
	out = append(out, block("PaxHeaders/s", len(records), 'x')...)
	out = append(out, pad([]byte(records))...)
	out = append(out, block("s", len(data), '0')...)
	out = append(out, pad([]byte(data))...)
	out = append(out, block("next.txt", 4, '0')...)
	out = append(out, pad([]byte("next"))...)
	return append(out, make([]byte, 1024)...)
}

// TestSparse: a sparse tar member is skipped, neither read raw in place
// nor expanded past the bound.
func TestSparse(t *testing.T) {
	for _, k := range []Kind{{LibArchive, "tar"}, {LibArchive, "gzip"}} {
		in := paxSparse(t)
		if k.Format == "gzip" {
			in = gz(in, "")
		}
		got, skipped, err := walkAll(t, &Extractor{MaxArchiveBytes: 1 << 20}, k, "s.tar", in, WalkOptions{})
		if err != nil || strings.Join(got, ",") != "next.txt=next" || len(skipped) != 1 || skipped[0].Path != "s" {
			t.Errorf("%s: %.80q %+v %v", k.Format, got, skipped, err)
		}
	}
}

func TestHardLink(t *testing.T) {
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	w.WriteHeader(&tar.Header{Name: "a.txt", Typeflag: tar.TypeReg, Size: 1})
	w.Write([]byte("a"))
	w.WriteHeader(&tar.Header{Name: "b.txt", Typeflag: tar.TypeLink, Linkname: "a.txt"})
	w.Close()
	got, skipped, err := walkAll(t, &Extractor{}, Kind{LibArchive, "tar"}, "h.tar", b.Bytes(), WalkOptions{})
	if err != nil || strings.Join(got, ",") != "a.txt=a" || len(skipped) != 1 || !skipped[0].Note || skipped[0].Path != "b.txt" {
		t.Errorf("%q %+v %v", got, skipped, err)
	}
}

// TestNoMemberBound: with no bound on a member, a gzip member is read
// whole, not cut to nothing.
func TestNoMemberBound(t *testing.T) {
	got, _, err := walkAll(t, &Extractor{MaxMemberBytes: -1}, Kind{LibArchive, "gzip"}, "n.txt.gz", gz([]byte("notes"), ""), WalkOptions{})
	if err != nil || strings.Join(got, ",") != "n.txt=notes" {
		t.Errorf("%q %v", got, err)
	}
}

// TestLyingSize: a zip member that says it is far larger than its
// compressed bytes allow is skipped at once, without setting memory
// aside for what it says.
func TestLyingSize(t *testing.T) {
	var files []file
	for i := range 300 {
		files = append(files, file{name: fmt.Sprintf("m%03d.txt", i), body: "x"})
	}
	b := zipOf(t, files...)
	// Each central directory and local header says 1 GiB uncompressed.
	for off := 0; ; {
		i := bytes.Index(b[off:], []byte("PK\x01\x02"))
		if i < 0 {
			break
		}
		binary.LittleEndian.PutUint32(b[off+i+24:], 1<<30)
		off += i + 4
	}
	for off := 0; ; {
		i := bytes.Index(b[off:], []byte("PK\x03\x04"))
		if i < 0 {
			break
		}
		binary.LittleEndian.PutUint32(b[off+i+22:], 1<<30)
		off += i + 4
	}
	start := time.Now()
	got, skipped, err := walkAll(t, &Extractor{}, Kind{LibArchive, "zip"}, "l.zip", b, WalkOptions{})
	if err != nil || len(got) != 0 || len(skipped) != 300 || !strings.Contains(skipped[0].Reason, "compressed size allows") {
		t.Errorf("%d %d %v %+v", len(got), len(skipped), err, skipped[:1])
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v", d)
	}
}

// TestDecodeSettled: an archive's children take from the decode budget
// what the walk left, so nested archives do not multiply it; members
// past MaxArchiveMembers are not decompressed.
func TestDecodeSettled(t *testing.T) {
	inner := zipOf(t, file{name: "b.txt", body: strings.Repeat("b", 600_000)})
	outer := zipOf(t, file{name: "a.txt", body: strings.Repeat("a", 600_000)}, file{name: "inner.zip", body: string(inner)})
	e := &Extractor{}
	doc, err := e.Extract(context.Background(), bytes.NewReader(outer), int64(len(outer)),
		&gofastcore.Options{Name: "o.zip", Limits: gofastcore.Limits{MaxDecodedBytes: 1 << 20}})
	if err != nil {
		t.Fatal(err)
	}
	limited := false
	var walk func(d *gofastcore.Document)
	walk = func(d *gofastcore.Document) {
		for _, w := range d.AllWarnings() {
			limited = limited || w.Code == gofastcore.WarnLimitExceeded
		}
		for _, c := range d.Children {
			walk(c)
		}
	}
	walk(doc)
	if !limited {
		t.Error("1.2 MB decompressed under a 1 MiB budget, with no warning")
	}
	two := zipOf(t, file{name: "a.txt", body: "a"}, file{name: "b.txt", body: strings.Repeat("b", 100_000)})
	doc, err = (&Extractor{MaxArchiveMembers: 1}).Extract(context.Background(), bytes.NewReader(two), int64(len(two)),
		&gofastcore.Options{Name: "t.zip", Limits: gofastcore.Limits{MaxDecodedBytes: 1000}})
	if err != nil || len(doc.Warnings) != 1 || doc.Warnings[0].Message != "members past 1 were not read (1 members)" {
		t.Errorf("%v %+v", err, doc.Warnings)
	}
}

func TestDetectGeneric(t *testing.T) {
	docx := zipOf(t, file{name: "[Content_Types].xml", body: "<Types/>"})
	pdf := []byte("%PDF-1.7\n")
	for _, c := range []struct {
		name, mime string
		body       []byte
		want       Kind
	}{
		{"report.docx", "application/zip", docx, Kind{LibOOXML, ""}},
		{"noext", "application/x-zip-compressed", docx, Kind{LibOOXML, ""}},
		{"a.pdf", "text/plain", pdf, Kind{LibPDF, ""}},
		{"", "text/plain; charset=utf-8", []byte("<html><p>x"), Kind{LibHTML, "txt"}},
		{"README", "", []byte("From the desk of the editor, 2024\nhello"), Kind{LibHTML, "txt"}},
		{"notes", "", []byte("Date: 2024-01-01\nAuthor: me\n\nnotes"), Kind{LibHTML, "txt"}},
		{"front", "", []byte("title: x\ndate: 2024\n"), Kind{LibHTML, "txt"}},
		{"log", "", []byte("printing %PDF- header check\n"), Kind{LibHTML, "txt"}},
		{"m", "", []byte("From: a@b\nTo: c@d\nSubject: s\n\nbody"), Kind{LibEmail, ""}},
	} {
		if got := Detect(c.name, c.mime, bytes.NewReader(c.body), int64(len(c.body))); got != c.want {
			t.Errorf("Detect(%q, %q) = %+v, want %+v", c.name, c.mime, got, c.want)
		}
	}
}

// TestExtractArchive reads an archive as one document whose children are
// its members, also embedded in another document.
func TestExtractArchive(t *testing.T) {
	inner := zipOf(t, file{name: "deep.md", body: "# Deep"})
	b := zipOf(t, file{name: "a.txt", body: "alpha"}, file{name: "page.html", body: "<p>Page</p>"},
		file{name: "in.zip", body: string(inner)}, file{name: "img.png", body: "\x89PNG\r\n\x1a\n\x00\x00"})
	doc, err := (&Extractor{}).Extract(context.Background(), bytes.NewReader(b), int64(len(b)), &gofastcore.Options{Name: "x.zip"})
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	var got []string
	for _, c := range doc.Children {
		s := c.Source.Name + ":" + c.Source.Format
		for _, cc := range c.Children {
			s += "[" + cc.Source.Name + ":" + cc.Source.Format + "]"
		}
		got = append(got, s)
	}
	if strings.Join(got, " ") != "a.txt:txt page.html:html in.zip:zip[deep.md:md] img.png:" || doc.Source.Format != "zip" {
		t.Errorf("children: %q", got)
	}
	if w := doc.Children[3].Warnings; len(w) != 1 || w[0].Code != gofastcore.WarnChildUnsupported {
		t.Errorf("png: %+v", w)
	}
	doc, err = (&Extractor{MaxArchiveMembers: 2}).Extract(context.Background(), bytes.NewReader(b), int64(len(b)), &gofastcore.Options{Name: "x.zip"})
	if err != nil || len(doc.Children) != 2 || len(doc.Warnings) != 1 || doc.Warnings[0].Message != "members past 2 were not read (2 members)" {
		t.Errorf("members bound: %v %+v", err, doc.Warnings)
	}
	if _, err := (&Extractor{}).Extract(context.Background(), strings.NewReader("PK\x03\x04junk"), 8, &gofastcore.Options{Name: "x.zip"}); !errors.Is(err, gofastcore.ErrMalformed) {
		t.Errorf("damaged: %v", err)
	}
}

func TestExtractUnsupported(t *testing.T) {
	for _, name := range []string{"old.doc", "photo.jpg"} {
		body := "\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1\x00\x00"
		if _, err := (&Extractor{}).Extract(context.Background(), strings.NewReader(body), int64(len(body)), &gofastcore.Options{Name: name}); !errors.Is(err, gofastcore.ErrUnsupported) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestMemberPath(t *testing.T) {
	for in, want := range map[string]string{
		"a/b.txt": "a/b.txt", "/a//./b": "a/b", `x\y.txt`: "x/y.txt", "C:/z": "z", "a/../b": "", "..": "", "./": "",
	} {
		got, ok := memberPath(in)
		if got != want || ok != (want != "") {
			t.Errorf("memberPath(%q) = %q %v", in, got, ok)
		}
	}
}

func FuzzArchive(f *testing.F) {
	f.Add(zipOf(f, file{name: "a.txt", body: "x"}, file{name: "b.zip", body: string(zipOf(f, file{name: "c.md", body: "# c"}))}), uint8(0))
	f.Add(tarOf(f, file{name: "a.txt", body: "x"}), uint8(1))
	f.Add(gz(tarOf(f, file{name: "a.html", body: "<p>x"}), ""), uint8(2))
	f.Fuzz(func(t *testing.T, b []byte, which uint8) {
		format := []string{"zip", "tar", "gzip"}[int(which)%3]
		e := &Extractor{MaxMemberBytes: 1 << 20, MaxArchiveBytes: 4 << 20, MaxArchiveMembers: 100}
		doc, err := e.ExtractKind(context.Background(), Kind{LibArchive, format}, "", bytes.NewReader(b), int64(len(b)),
			&gofastcore.Options{Name: "f." + format, Child: e.Child, Limits: gofastcore.Limits{MaxTextBytes: 1 << 20}})
		if err != nil {
			if !errors.Is(err, gofastcore.ErrMalformed) {
				t.Fatal(err)
			}
			return
		}
		if err := doc.Validate(); err != nil {
			t.Fatalf("Validate: %v", err)
		}
	})
}

func FuzzDetect(f *testing.F) {
	f.Add("x.zip", "", []byte("PK\x03\x04"))
	f.Add("", "text/csv; header=present", []byte("a,b"))
	f.Add("m", "", []byte("Received: x\nSubject: y\n\n"))
	f.Fuzz(func(t *testing.T, name, mimeType string, b []byte) {
		k := Detect(name, mimeType, bytes.NewReader(b), int64(len(b)))
		if k.Library == "" && k.Format != "" {
			t.Fatalf("%+v", k)
		}
	})
}

// TestCancelDuringRead: a walk cancelled while a member is read ends with
// the context's error and gives back what it held.
func TestCancelDuringRead(t *testing.T) {
	inputs := map[string][]byte{
		"zip": zipOf(t, file{name: "a.txt", body: "aaaa"}, file{name: "b.txt", body: "b"}),
		"tgz": gz(tarOf(t, file{name: "a.txt", body: "aaaa"}, file{name: "b.txt", body: "b"}), ""),
		"gz":  gz([]byte(strings.Repeat("x", 30000)), ""),
	}
	for name, b := range inputs {
		ctx, cancel := context.WithCancel(context.Background())
		held := int64(0)
		hold := func(n, own int64) (func(), error) {
			held += n
			cancel()
			return func() { held -= n }, nil
		}
		k := Kind{LibArchive, "zip"}
		if name != "zip" {
			k.Format = "gzip"
		}
		var got []string
		err := (&Extractor{}).Walk(ctx, k, "x."+name, bytes.NewReader(b), int64(len(b)), WalkOptions{Hold: hold},
			func(m Member) error { got = append(got, m.Path); m.Release(); return nil }, func(Skipped) {})
		if !errors.Is(err, context.Canceled) || held != 0 {
			t.Errorf("%s: err %v, members %q, held %d", name, err, got, held)
		}
	}
}

// TestHoldError: an error from Hold stops the walk.
func TestHoldError(t *testing.T) {
	quota := errors.New("quota exceeded")
	b := zipOf(t, file{name: "a.txt", body: "a"}, file{name: "b.txt", body: "b"})
	err := (&Extractor{}).Walk(context.Background(), Kind{LibArchive, "zip"}, "q.zip", bytes.NewReader(b), int64(len(b)),
		WalkOptions{Hold: func(n, own int64) (func(), error) { return nil, quota }},
		func(m Member) error { t.Errorf("member %s", m.Path); return nil }, func(s Skipped) { t.Errorf("skipped %+v", s) })
	if !errors.Is(err, quota) {
		t.Errorf("%v", err)
	}
}

// TestLongHeaders: a message whose header block is longer than what is
// sniffed is still email.
func TestLongHeaders(t *testing.T) {
	var h strings.Builder
	h.WriteString("Delivered-To: a@example.com\r\n")
	for i := range 6 {
		fmt.Fprintf(&h, "Received: from mx%d.example.com\r\n        by relay.example.com; Mon, 1 Jan 2024 00:00:00 +0000\r\n", i)
	}
	h.WriteString("ARC-Seal: i=1; a=rsa-sha256;")
	for h.Len() < sniffBytes-20 {
		h.WriteString("\r\n        b=abcdefghijklmnopqrstuvwxyz0123456789")
	}
	for h.Len() < sniffBytes-5 {
		h.WriteString("x")
	}
	h.WriteString("\r\nContent-Type: text/plain\r\n\r\nbody")
	b := []byte(h.String())
	if k := Detect("", "", bytes.NewReader(b), int64(len(b))); k != (Kind{LibEmail, ""}) {
		t.Errorf("%+v", k)
	}
}

// TestOtherMethod: a zip member compressed with a method Go does not
// decode is reported as such, whatever its size says.
func TestOtherMethod(t *testing.T) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, _ := z.CreateRaw(&zip.FileHeader{Name: "a.txt", Method: 12, CompressedSize64: 4, UncompressedSize64: 10 << 20, CRC32: 1})
	w.Write([]byte("BZh9"))
	z.Close()
	_, skipped, err := walkAll(t, &Extractor{}, Kind{LibArchive, "zip"}, "b.zip", b.Bytes(), WalkOptions{})
	if err != nil || len(skipped) != 1 || skipped[0].Reason != "compressed with a method not supported (12)" {
		t.Errorf("%+v %v", skipped, err)
	}
}

// TestDanglingLink: a hard link to a file not in the archive is a member
// not read, not a note.
func TestDanglingLink(t *testing.T) {
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	w.WriteHeader(&tar.Header{Name: "report.txt", Typeflag: tar.TypeLink, Linkname: "elsewhere/report.txt"})
	w.Close()
	_, skipped, err := walkAll(t, &Extractor{}, Kind{LibArchive, "tar"}, "d.tar", b.Bytes(), WalkOptions{})
	if err != nil || len(skipped) != 1 || skipped[0].Note || !strings.Contains(skipped[0].Reason, "not in the archive") {
		t.Errorf("%+v %v", skipped, err)
	}
}

// TestZipCutShort: a zip archive cut short, its directory lost, gives the
// members found by their own headers, skips the one it breaks off in, and
// says so, as a damaged archive does.
func TestZipCutShort(t *testing.T) {
	long := strings.Repeat("the third member, ", 200)
	b := zipOf(t, file{name: "a.txt", body: "first"}, file{name: "s.txt", body: "stored", store: true}, file{name: "c.txt", body: long})
	dir := bytes.Index(b, []byte("PK\x01\x02"))
	k := Kind{LibArchive, "zip"}
	got, skipped, err := walkAll(t, &Extractor{}, k, "cut.zip", b[:dir], WalkOptions{})
	if !errors.Is(err, ErrArchive) || !strings.Contains(err.Error(), "3 members were found by their own headers; any that followed are lost") ||
		strings.Join(got, "|") != "a.txt=first|s.txt=stored|c.txt="+long || len(skipped) != 0 {
		t.Errorf("cut before the directory: %q %+v %v", got, skipped, err)
	}
	// Cut in the third member: it is skipped, damaged.
	at := bytes.Index(b, []byte("c.txt")) + 40
	got, skipped, err = walkAll(t, &Extractor{}, k, "cut.zip", b[:at], WalkOptions{})
	if !errors.Is(err, ErrArchive) || !strings.Contains(err.Error(), "the last, c.txt, breaks off") ||
		strings.Join(got, "|") != "a.txt=first|s.txt=stored" || len(skipped) != 1 || skipped[0].Path != "c.txt" {
		t.Errorf("cut in a member: %q %+v %v", got, skipped, err)
	}
	// As one document: its members, and input.repaired.
	doc, err := (&Extractor{}).Extract(context.Background(), bytes.NewReader(b[:dir]), int64(dir), nil)
	if err != nil {
		t.Fatal(err)
	}
	repaired := false
	for _, w := range doc.Warnings {
		repaired = repaired || w.Code == gofastcore.WarnInputRepaired && strings.Contains(w.Message, "members were found by their own headers")
	}
	if len(doc.Children) != 3 || !repaired {
		t.Errorf("as a document: %d children, repaired %v", len(doc.Children), repaired)
	}
	// A bomb without its directory meets the bound on what the walk may
	// decompress (when recovery finds where it ends, in core, or when it
	// is read).
	bomb := zipOf(t, file{name: "zeros", body: string(make([]byte, 8<<20))})
	cut := bomb[:bytes.Index(bomb, []byte("PK\x01\x02"))]
	doc, err = (&Extractor{MaxArchiveBytes: 1 << 20}).Extract(context.Background(), bytes.NewReader(cut), int64(len(cut)), nil)
	limited := false
	for _, w := range doc.Warnings {
		limited = limited || w.Code == gofastcore.WarnLimitExceeded
	}
	if err != nil || !limited || len(doc.Children) != 0 {
		t.Errorf("a bomb without its directory, over the bound: %v, limit warning %v, %d children", err, limited, len(doc.Children))
	}
}
