package gofast

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

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
	hold := func(n int64) func() {
		held += n
		if held > max {
			max = held
		}
		return func() { held -= n }
	}
	b := zipOf(t, file{name: "a.txt", body: strings.Repeat("a", 100)}, file{name: "b.txt", body: strings.Repeat("b", 200)})
	if _, _, err := walkAll(t, &Extractor{}, Kind{LibArchive, "zip"}, "h.zip", b, WalkOptions{Hold: hold}); err != nil {
		t.Fatal(err)
	}
	if held != 0 || max != 200 {
		t.Errorf("held %d, max %d", held, max)
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
