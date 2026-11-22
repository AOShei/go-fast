package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gofastcore "github.com/AOShei/go-fast-core"
)

func runCmd(t *testing.T, stdin string, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	status := run(context.Background(), args, strings.NewReader(stdin), &out, &errb)
	return out.String(), errb.String(), status
}

func writeZip(t *testing.T, p string, files ...[2]string) {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, f := range files {
		w, err := z.Create(f[0])
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(f[1]))
	}
	z.Close()
	if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// docs decodes JSON Lines and checks every document.
func docs(t *testing.T, out string) []*gofastcore.Document {
	t.Helper()
	var ds []*gofastcore.Document
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(nil, 1<<26)
	for sc.Scan() {
		var d gofastcore.Document
		if err := json.Unmarshal(sc.Bytes(), &d); err != nil {
			t.Fatalf("%v: %q", err, sc.Text())
		}
		if err := d.Validate(); err != nil {
			t.Fatalf("Validate %s: %v", d.Source.Name, err)
		}
		ds = append(ds, &d)
	}
	return ds
}

// TestOrder reads many files with many workers: the documents come out
// in the order of the inputs and of the members.
func TestOrder(t *testing.T) {
	dir := t.TempDir()
	var want []string
	for i := range 40 {
		name := fmt.Sprintf("f%02d.md", i)
		body := fmt.Sprintf("# Doc %d\n\n%s", i, strings.Repeat("words ", (40-i)*200))
		os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
		want = append(want, name)
		if i == 20 {
			writeZip(t, filepath.Join(dir, "f20z.zip"), [2]string{"x/one.txt", "one"}, [2]string{"two.html", "<p>two"})
			want = append(want, "f20z.zip/x/one.txt", "f20z.zip/two.html")
		}
	}
	os.WriteFile(filepath.Join(dir, "skip.bin"), []byte{0, 1, 2}, 0o644)
	out, errs, status := runCmd(t, "", "-j", "8", "-quiet", dir)
	if status != 0 {
		t.Fatalf("status %d: %s", status, errs)
	}
	var got []string
	for _, d := range docs(t, out) {
		got = append(got, d.Source.Name)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order:\n got %q\nwant %q", got, want)
	}
}

func TestSingle(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	os.WriteFile(p, []byte("# A"), 0o644)
	out, _, status := runCmd(t, "", p)
	if status != 0 || !strings.HasPrefix(out, "{\n  \"schema_version\"") {
		t.Errorf("single input is not indented: %d %.60q", status, out)
	}
	z := filepath.Join(dir, "one.zip")
	writeZip(t, z, [2]string{"a.md", "# A"})
	out, _, _ = runCmd(t, "", "-quiet", z)
	if strings.Count(out, "\n") != 1 || len(docs(t, out)) != 1 {
		t.Errorf("an archive gives JSON Lines: %.80q", out)
	}
	out, _, status = runCmd(t, "Subject: hi\nFrom: a@b\n\nbody\n", "-quiet", "-")
	if status != 0 || len(docs(t, out)) != 1 || !strings.Contains(out, `"format": "eml"`) && !strings.Contains(out, `"format":"eml"`) {
		t.Errorf("stdin: %d %.120q", status, out)
	}
}

func TestDir(t *testing.T) {
	in, out := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(in, "sub"), 0o755)
	os.WriteFile(filepath.Join(in, "sub", "a.txt"), []byte("alpha"), 0o644)
	writeZip(t, filepath.Join(in, "b.zip"), [2]string{"inner/c.md", "# C"}, [2]string{`..\..\evil.txt`, "e"}, [2]string{"weird:name?.txt", "w"})
	_, errs, status := runCmd(t, "", "-quiet", "-format", "markdown", "-dir", out, in)
	if status != 1 || !strings.Contains(errs, "not read: its name is not a path inside the archive") {
		t.Errorf("status %d: %s", status, errs)
	}
	var files []string
	filepath.Walk(out, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(out, p)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if strings.Join(files, ",") != "b.zip/inner/c.md.md,b.zip/weird_name_.txt.md,sub/a.txt.md" {
		t.Errorf("files: %q", files)
	}
	b, _ := os.ReadFile(filepath.Join(out, "b.zip", "inner", "c.md.md"))
	if strings.TrimSpace(string(b)) != "# C" {
		t.Errorf("content: %q", b)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(out), "evil.txt.md")); err == nil {
		t.Error("wrote outside -dir")
	}
}

func TestOutputPath(t *testing.T) {
	dir := t.TempDir()
	for rel, want := range map[string]string{
		"a/b.pdf":       "a/b.pdf.json",
		"../../x":       "_/_/x.json",
		"/abs/./y":      "abs/y.json",
		"c:/z":          "c_/z.json",
		"":              "document.json",
		"q\x00r":        "q_r.json",
		"dir/../../esc": "dir/_/_/esc.json",
	} {
		p, err := outputPath(dir, rel, ".json")
		if err != nil {
			t.Errorf("%q: %v", rel, err)
			continue
		}
		got, _ := filepath.Rel(dir, p)
		if filepath.ToSlash(got) != want {
			t.Errorf("outputPath(%q) = %q, want %q", rel, got, want)
		}
	}
}

func TestFailures(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.md")
	os.WriteFile(good, []byte("# Good"), 0o644)
	bad := filepath.Join(dir, "bad.docx")
	os.WriteFile(bad, []byte("not a zip"), 0o644)
	out, errs, status := runCmd(t, "", good, bad, filepath.Join(dir, "missing.pdf"))
	if status != 1 || len(docs(t, out)) != 1 || !strings.Contains(errs, "bad.docx: ") || !strings.Contains(errs, "missing.pdf") ||
		!strings.Contains(errs, "gofast: 1 documents, 1 not read") {
		t.Errorf("status %d\n%s", status, errs)
	}
	for _, args := range [][]string{{"-format", "xml", good}, {"-j", "0", good}, {"-o", "x", "-dir", "y", good}, {"-images", "all", good}, {}} {
		if _, _, status := runCmd(t, "", args...); status != 2 {
			t.Errorf("%q: status %d", args, status)
		}
	}
	if _, _, status := runCmd(t, "", "-version"); status != 0 {
		t.Errorf("-version: %d", status)
	}
}

func TestOutputFile(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	os.WriteFile(a, []byte("alpha"), 0o644)
	os.WriteFile(b, []byte("beta"), 0o644)
	o := filepath.Join(dir, "out.txt")
	stdout, _, status := runCmd(t, "", "-quiet", "-format", "text", "-o", o, a, b)
	got, _ := os.ReadFile(o)
	if status != 0 || stdout != "" || !strings.Contains(string(got), "==> "+a+" <==\nalpha") || !strings.Contains(string(got), "==> "+b+" <==\nbeta") {
		t.Errorf("%d %q", status, got)
	}
}

// TestNoHang: a gzip member inside a zip, with a memory bound smaller
// than the zip and the member together, is read (the walk does not wait
// for memory it holds itself); a cancelled run ends.
func TestNoHang(t *testing.T) {
	dir := t.TempDir()
	var gzb bytes.Buffer
	zw := gzip.NewWriter(&gzb)
	zw.Write([]byte(strings.Repeat("notes ", 2000)))
	zw.Close()
	p := filepath.Join(dir, "gzinzip.zip")
	writeZip(t, p, [2]string{"notes.txt.gz", gzb.String()})
	done := make(chan int, 1)
	var out string
	go func() {
		o, _, status := runCmd(t, "", "-quiet", "-max-memory-bytes", "1000", "-max-member-bytes", "5368709120", p)
		out = o
		done <- status
	}()
	select {
	case status := <-done:
		if status != 0 || len(docs(t, out)) != 1 {
			t.Errorf("status %d: %.100q", status, out)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the run did not end")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var o, e bytes.Buffer
	if status := run(ctx, []string{p}, strings.NewReader(""), &o, &e); status != 1 {
		t.Errorf("cancelled run: status %d", status)
	}
}

func TestDirRootAndDevices(t *testing.T) {
	if filepath.Separator == '/' {
		if p, err := outputPath("/", "a.pdf", ".json"); err != nil || p != "/a.pdf.json" {
			t.Errorf("-dir /: %q %v", p, err)
		}
	}
	dir := t.TempDir()
	for rel, want := range map[string]string{"CON.txt": "_CON.txt.json", "x./nul": "x_/_nul.json", "lpt1": "_lpt1.json", "a. ": "a_.json", "COM10": "COM10.json"} {
		p, err := outputPath(dir, rel, ".json")
		got, _ := filepath.Rel(dir, p)
		if err != nil || filepath.ToSlash(got) != want {
			t.Errorf("outputPath(%q) = %q %v, want %q", rel, got, err, want)
		}
	}
}

// TestHardLinkNote: a tar hard link is a note, not a failure.
func TestHardLinkNote(t *testing.T) {
	dir := t.TempDir()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	w.WriteHeader(&tar.Header{Name: "a.txt", Typeflag: tar.TypeReg, Size: 1})
	w.Write([]byte("a"))
	w.WriteHeader(&tar.Header{Name: "b.txt", Typeflag: tar.TypeLink, Linkname: "a.txt"})
	w.Close()
	p := filepath.Join(dir, "h.tar")
	os.WriteFile(p, b.Bytes(), 0o644)
	out, errs, status := runCmd(t, "", p)
	if status != 0 || len(docs(t, out)) != 1 || !strings.Contains(errs, "h.tar/b.txt: a hard link to a.txt") {
		t.Errorf("status %d\n%s", status, errs)
	}
}

func TestStdinBound(t *testing.T) {
	_, errs, status := runCmd(t, strings.Repeat("x", 2000), "-max-input-bytes", "1000", "-")
	if status != 1 || !strings.Contains(errs, "larger than 1000 bytes") {
		t.Errorf("status %d: %s", status, errs)
	}
}

// TestCaseNames: two members whose names differ only in case are both
// written; where the file system ignores case the second gets a number.
func TestCaseNames(t *testing.T) {
	in, out := t.TempDir(), t.TempDir()
	writeZip(t, filepath.Join(in, "case.zip"), [2]string{"README", "upper"}, [2]string{"readme", "lower"})
	_, errs, status := runCmd(t, "", "-quiet", "-dir", out, filepath.Join(in, "case.zip"))
	if status != 0 {
		t.Fatalf("status %d: %s", status, errs)
	}
	entries, _ := os.ReadDir(filepath.Join(out, "case.zip"))
	if len(entries) != 2 {
		t.Errorf("files: %v\n%s", entries, errs)
	}
}
