package gofast

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	utf16enc "unicode/utf16"

	gofastcore "github.com/AOShei/go-fast-core"
)

// compound builds an OLE compound file (version 3, 512-byte sectors)
// whose root storage holds the named streams, each of 4 KiB of zeros, so
// that none is in the mini stream: enough for go-fast-msbin's Detect,
// which reads the directory only.
func compound(names ...string) []byte {
	const sector, streamSectors = 512, 8
	const endOfChain, fatSect, free, noStream = 0xFFFFFFFE, 0xFFFFFFFD, 0xFFFFFFFF, 0xFFFFFFFF
	le := binary.LittleEndian
	entries := 1 + len(names)
	dirSectors := (entries*128 + sector - 1) / sector
	first := 1 + dirSectors // the first stream's sector
	total := first + len(names)*streamSectors
	b := make([]byte, sector*(1+total))
	h := b[:sector]
	copy(h, "\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1")
	le.PutUint16(h[24:], 0x3E)
	le.PutUint16(h[26:], 3)
	le.PutUint16(h[28:], 0xFFFE)
	le.PutUint16(h[30:], 9)
	le.PutUint16(h[32:], 6)
	le.PutUint32(h[44:], 1)          // FAT sectors
	le.PutUint32(h[48:], 1)          // first directory sector
	le.PutUint32(h[56:], 4096)       // mini stream cutoff
	le.PutUint32(h[60:], endOfChain) // no mini FAT
	le.PutUint32(h[68:], endOfChain) // no DIFAT sectors
	for i := 76; i < sector; i += 4 {
		le.PutUint32(h[i:], free)
	}
	le.PutUint32(h[76:], 0) // the FAT is sector 0
	fat := b[sector : 2*sector]
	for i := 0; i < sector/4; i++ {
		le.PutUint32(fat[4*i:], free)
	}
	le.PutUint32(fat[0:], fatSect)
	for s := 1; s < first; s++ {
		next := uint32(s + 1)
		if s == first-1 {
			next = endOfChain
		}
		le.PutUint32(fat[4*s:], next)
	}
	for n := range names {
		for k := 0; k < streamSectors; k++ {
			s := first + n*streamSectors + k
			next := uint32(s + 1)
			if k == streamSectors-1 {
				next = endOfChain
			}
			le.PutUint32(fat[4*s:], next)
		}
	}
	dir := b[2*sector : (2+dirSectors)*sector]
	entry := func(i int, name string, kind byte, child, right, start uint32, size uint64) {
		e := dir[128*i : 128*(i+1)]
		u := utf16enc.Encode([]rune(name))
		for j, c := range u {
			le.PutUint16(e[2*j:], c)
		}
		le.PutUint16(e[64:], uint16(2*(len(u)+1)))
		e[66], e[67] = kind, 1
		le.PutUint32(e[68:], noStream)
		le.PutUint32(e[72:], right)
		le.PutUint32(e[76:], child)
		le.PutUint32(e[116:], start)
		le.PutUint64(e[120:], size)
	}
	for i := 0; i < len(dir)/128; i++ {
		e := dir[128*i:]
		le.PutUint32(e[68:], noStream)
		le.PutUint32(e[72:], noStream)
		le.PutUint32(e[76:], noStream)
	}
	entry(0, "Root Entry", 5, 1, noStream, endOfChain, 0)
	for n, name := range names {
		right := uint32(noStream)
		if n+1 < len(names) {
			right = uint32(n + 2)
		}
		entry(n+1, name, 2, noStream, right, uint32(first+n*streamSectors), streamSectors*sector)
	}
	return b
}

// TestDetectLegacy: Office 97-2003 files go to go-fast-msbin, by their
// name, their media type or their compound file's streams; a file named
// as one that is something else goes where its bytes say.
func TestDetectLegacy(t *testing.T) {
	doc, xls, ppt := compound("WordDocument", "1Table"), compound("Workbook"), compound("PowerPoint Document", "Current User")
	msg := compound("__properties_version1.0")
	other := compound("Contents") // an object another program stored
	msbin := func(f string) Kind { return Kind{LibMSBin, f} }
	for _, c := range []struct {
		what, name, mime string
		body             []byte
		want             Kind
	}{
		{"Word", "a.doc", "", doc, msbin("doc")},
		{"Excel", "a.xls", "", xls, msbin("xls")},
		{"PowerPoint", "a.ppt", "", ppt, msbin("ppt")},
		{"a template", "a.dot", "", doc, msbin("doc")},
		{"a show", "a.pps", "", ppt, msbin("ppt")},
		{"no name", "", "", xls, msbin("xls")},
		{"a wrong name", "notes.bin", "", doc, msbin("doc")},
		{"the name decides nothing", "a.doc", "", xls, msbin("xls")},
		{"a media type", "x", "application/msword", doc, msbin("doc")},
		{"a media type", "x", "application/vnd.ms-excel", xls, msbin("xls")},
		{"a media type", "x", "application/vnd.ms-powerpoint", ppt, msbin("ppt")},
		{"a generic type", "x", "application/octet-stream", ppt, msbin("ppt")},
		{"a generic type, Office name", "a.doc", "application/octet-stream", doc, msbin("doc")},
		{"Outlook message", "", "", msg, Kind{LibEmail, ""}},
		{"Outlook message", "m.msg", "", msg, Kind{LibEmail, ""}},
		{"another compound file", "", "", other, Kind{LibEmail, ""}},
		// Named as Office, but something else.
		{"RTF saved as .doc", "a.doc", "", []byte(`{\rtf1\ansi Hello}`), Kind{LibRTF, ""}},
		{"HTML saved as .doc", "a.doc", "", []byte(`<html><body><p>Hello</p></body></html>`), Kind{LibHTML, "html"}},
		{"HTML saved as .xls", "x", "application/vnd.ms-excel", []byte(`<html><table><tr><td>1</td></tr></table></html>`), Kind{LibHTML, "html"}},
		{"a zip named .xls", "a.xls", "", []byte("PK\x03\x04"), Kind{LibArchive, "zip"}},
		{"bytes none reads", "a.xls", "", []byte("\x09\x00\x04\x00\x02\x00\x10\x00\x00\x00"), msbin("")}, // an Excel 2 workbook's start
		{"no bytes", "a.ppt", "", nil, msbin("")},
		{"cut in the header", "a.doc", "", doc[:100], Kind{LibEmail, ""}},
	} {
		got := Detect(c.name, c.mime, bytes.NewReader(c.body), int64(len(c.body)))
		if got != c.want {
			t.Errorf("%s: Detect(%q, %q) = %+v, want %+v", c.what, c.name, c.mime, got, c.want)
		}
	}
	for _, n := range []string{"a.doc", "B.XLS", "c.ppt", "d.dot", "e.xlt", "f.xla", "g.pps", "h.pot"} {
		if !KnownExtension(n) {
			t.Errorf("KnownExtension(%q) is false", n)
		}
	}
}

// TestLegacyExtract: go-fast-msbin reads what Detect sends it, through
// Extract and as an archive member, and says what a compound file named
// as Office is when it is not one it reads.
func TestLegacyExtract(t *testing.T) {
	ctx := context.Background()
	extract := func(name string, body []byte) error {
		_, err := (&Extractor{}).Extract(ctx, bytes.NewReader(body), int64(len(body)), &gofastcore.Options{Name: name})
		return err
	}
	// A compound file with the streams but not their content: go-fast-msbin
	// is reached, and refuses it.
	for _, c := range []struct {
		name string
		body []byte
	}{
		{"a.doc", compound("WordDocument", "1Table")},
		{"", compound("Workbook")},
		{"a.doc", compound("WordDocument")[:100]},             // cut: go-fast-email refuses it, go-fast-msbin says why
		{"a.ppt", []byte("\x09\x00\x04\x00\x02\x00\x10\x00")}, // no library reads it: go-fast-msbin says what it is
	} {
		err := extract(c.name, c.body)
		if err == nil || !strings.HasPrefix(err.Error(), "gofastmsbin: ") {
			t.Errorf("%q: %v, want go-fast-msbin's error", c.name, err)
		}
	}
	// An Outlook message named .xls is read as a message.
	if err := extract("a.xls", compound("__properties_version1.0")); err != nil {
		t.Errorf("a message named .xls: %v", err)
	}
	// A compound file of neither kind, named nothing: go-fast-email
	// refuses it, and the umbrella says what it is not.
	if err := extract("", compound("Contents")); !errors.Is(err, gofastcore.ErrUnsupported) ||
		!strings.Contains(err.Error(), "neither an Outlook message nor a Word, Excel or PowerPoint 97-2003 file") {
		t.Errorf("another compound file: %v", err)
	}
	for _, f := range []string{"doc", "xls", "ppt"} {
		if !contains((&Extractor{}).Formats(), f) {
			t.Errorf("Formats lacks %s", f)
		}
	}
}
