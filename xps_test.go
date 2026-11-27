package gofast

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

const (
	nsXPS  = "http://schemas.microsoft.com/xps/2005/06"
	nsOXPS = "http://schemas.openxps.org/oxps/v1.0"
)

// xpsPackage is a one-page XPS document in the namespace ns, with a
// content-types table, its text in a Glyphs element whose font the
// package does not have (go-fast-pdf reads the text and warns of the
// font).
func xpsPackage(t testing.TB, ns, text string) []byte {
	t.Helper()
	return zipOf(t,
		file{name: "[Content_Types].xml", body: `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="fdseq" ContentType="application/vnd.ms-package.xps-fixeddocumentsequence+xml"/>` +
			`<Default Extension="fdoc" ContentType="application/vnd.ms-package.xps-fixeddocument+xml"/>` +
			`<Default Extension="fpage" ContentType="application/vnd.ms-package.xps-fixedpage+xml"/></Types>`},
		file{name: "_rels/.rels", body: fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
			`<Relationship Id="R1" Type="%s/fixedrepresentation" Target="/FixedDocSeq.fdseq"/></Relationships>`, ns)},
		file{name: "FixedDocSeq.fdseq", body: fmt.Sprintf(`<FixedDocumentSequence xmlns="%s"><DocumentReference Source="/Documents/1/FixedDoc.fdoc"/></FixedDocumentSequence>`, ns)},
		file{name: "Documents/1/FixedDoc.fdoc", body: fmt.Sprintf(`<FixedDocument xmlns="%s"><PageContent Source="Pages/1.fpage"/></FixedDocument>`, ns)},
		file{name: "Documents/1/Pages/1.fpage", body: fmt.Sprintf(`<FixedPage xmlns="%s" Width="816" Height="1056" xml:lang="en-US">`+
			`<Glyphs Fill="#FF000000" FontUri="/Resources/f.odttf" FontRenderingEmSize="16" OriginX="96" OriginY="120" UnicodeString="%s"/></FixedPage>`, ns, text)},
	)
}

// TestDetectXPS: XPS and OpenXPS go to go-fast-pdf by name, media type
// or bytes. By its bytes a package is XPS when a package relationship
// leads to a fixed document sequence, though it has a content-types
// table as an Office file does.
func TestDetectXPS(t *testing.T) {
	pdf, ooxml := Kind{LibPDF, ""}, Kind{LibOOXML, ""}
	xps, oxps := xpsPackage(t, nsXPS, "x"), xpsPackage(t, nsOXPS, "x")
	office := zipOf(t, file{name: "[Content_Types].xml", body: "<Types/>"},
		file{name: "_rels/.rels", body: `<Relationships><Relationship Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`})
	noTypes := zipOf(t, file{name: "_rels/.rels", body: fmt.Sprintf(`<Relationships><Relationship Type='%s/fixedrepresentation' Target='/s.fdseq'/></Relationships>`, nsOXPS)})
	other := zipOf(t, file{name: "_rels/.rels", body: `<Relationships><Relationship Type="` + nsXPS + `/fixedrepresentationx" Target="/s"/></Relationships>`})
	for _, c := range []struct {
		what, name, mime string
		body             []byte
		want             Kind
	}{
		{"XPS by its bytes", "", "", xps, pdf},
		{"OpenXPS by its bytes", "", "", oxps, pdf},
		{"no content types", "", "", noTypes, pdf},
		{"as application/octet-stream", "x", "application/octet-stream", xps, pdf},
		{".xps", "a.xps", "", nil, pdf},
		{".oxps", "a.OXPS", "", nil, pdf},
		{"XPS media type", "x", "application/vnd.ms-xpsdocument", nil, pdf},
		{"OpenXPS media type", "x", "application/oxps", nil, pdf},
		{"a compound file named .xps", "a.xps", "", []byte("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1"), pdf},
		{"Office", "", "", office, ooxml},
		{"another relationship type", "", "", other, Kind{LibArchive, "zip"}},
	} {
		var size int64
		if c.body != nil {
			size = int64(len(c.body))
		}
		if got := Detect(c.name, c.mime, bytes.NewReader(c.body), size); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.what, got, c.want)
		}
	}
	if !KnownExtension("a.xps") || !KnownExtension("b.oxps") {
		t.Error("a directory does not take .xps and .oxps")
	}
}

// TestXPSExtract: go-fast-pdf reads XPS and OpenXPS through the umbrella,
// named or not.
func TestXPSExtract(t *testing.T) {
	for _, c := range []struct{ ns, format string }{{nsXPS, "xps"}, {nsOXPS, "oxps"}} {
		body := xpsPackage(t, c.ns, "Hello from XPS")
		for _, name := range []string{"", "doc." + c.format} {
			doc, err := extractNamed(&Extractor{}, name, string(body))
			if err != nil {
				t.Fatalf("%s %q: %v", c.format, name, err)
			}
			if doc.Source.Format != c.format || !strings.Contains(doc.Text(), "Hello from XPS") {
				t.Errorf("%s %q: format %q, text %q", c.format, name, doc.Source.Format, doc.Text())
			}
		}
	}
}
