package gofast

import (
	"bytes"
	"context"
	"strings"
	"testing"

	gofastcore "github.com/AOShei/go-fast-core"
)

// visioPackage is a Visio drawing of one page with one shape whose text
// is text, with or without its [Content_Types].xml.
func visioPackage(t *testing.T, text string, types bool) []byte {
	const (
		ns  = `xmlns="http://schemas.microsoft.com/office/visio/2012/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
		rel = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`
	)
	files := []file{
		{name: "_rels/.rels", body: rel + `<Relationship Id="rId1" Type="` + visioRelType + `" Target="visio/document.xml"/></Relationships>`},
		{name: "visio/document.xml", body: `<VisioDocument ` + ns + `/>`},
		{name: "visio/_rels/document.xml.rels", body: rel + `<Relationship Id="rId1" Type="http://schemas.microsoft.com/visio/2010/relationships/pages" Target="pages/pages.xml"/></Relationships>`},
		{name: "visio/pages/pages.xml", body: `<Pages ` + ns + `><Page ID="0" Name="Page-1"><PageSheet><Cell N="PageWidth" V="8.5"/><Cell N="PageHeight" V="11"/></PageSheet><Rel r:id="rId1"/></Page></Pages>`},
		{name: "visio/pages/_rels/pages.xml.rels", body: rel + `<Relationship Id="rId1" Type="http://schemas.microsoft.com/visio/2010/relationships/page" Target="page1.xml"/></Relationships>`},
		{name: "visio/pages/page1.xml", body: `<PageContents ` + ns + `><Shapes><Shape ID="1" Type="Shape"><Cell N="PinX" V="2"/><Cell N="PinY" V="2"/><Cell N="Width" V="2"/><Cell N="Height" V="1"/><Text>` + text + `</Text></Shape></Shapes></PageContents>`},
	}
	if types {
		ct := `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/visio/document.xml" ContentType="application/vnd.ms-visio.drawing.main+xml"/>` +
			`<Override PartName="/visio/pages/pages.xml" ContentType="application/vnd.ms-visio.pages+xml"/>` +
			`<Override PartName="/visio/pages/page1.xml" ContentType="application/vnd.ms-visio.page+xml"/></Types>`
		files = append([]file{{name: "[Content_Types].xml", body: ct}}, files...)
	}
	return zipOf(t, files...)
}

// Visio 2013 and later go to go-fast-ooxml by name, media type and bytes,
// also a package with no [Content_Types].xml; the binary and the
// 2003-2010 XML formats are not routed by name.
func TestVisio(t *testing.T) {
	ooxml := Kind{LibOOXML, ""}
	typed, untyped := visioPackage(t, "Pump", true), visioPackage(t, "Valve", false)
	for _, c := range []struct {
		name, mime string
		b          []byte
	}{
		{"a.vsdx", "", nil}, {"a.VSDM", "", nil}, {"a.vstx", "", nil}, {"a.vstm", "", nil}, {"a.vssx", "", nil}, {"a.vssm", "", nil},
		{"", "application/vnd.ms-visio.drawing", nil},
		{"", "application/vnd.ms-visio.drawing.macroEnabled.12", nil},
		{"", "application/vnd.ms-visio.template", nil},
		{"", "application/vnd.ms-visio.template.macroEnabled.12", nil},
		{"", "application/vnd.ms-visio.stencil", nil},
		{"", "application/vnd.ms-visio.stencil.macroEnabled.12", nil},
		{"", "", typed}, {"", "", untyped}, {"drawing", "application/octet-stream", untyped},
	} {
		if got := Detect(c.name, c.mime, bytes.NewReader(c.b), int64(len(c.b))); got != ooxml {
			t.Errorf("Detect(%q, %q) = %+v", c.name, c.mime, got)
		}
		if strings.HasPrefix(c.name, "a.") && !KnownExtension(c.name) {
			t.Errorf("KnownExtension(%q) = false", c.name)
		}
	}
	for _, name := range []string{"a.vsd", "a.vst", "a.vss", "a.vdx", "a.vtx", "a.vsx"} {
		if KnownExtension(name) {
			t.Errorf("KnownExtension(%q) = true", name)
		}
	}
	// A zip whose package relationships lead to no Visio document is an
	// archive.
	other := zipOf(t, file{name: "_rels/.rels", body: `<Relationships/>`}, file{name: "a.txt", body: "a"})
	if got := Detect("", "", bytes.NewReader(other), int64(len(other))); got != (Kind{LibArchive, "zip"}) {
		t.Errorf("zip with other relationships: %+v", got)
	}

	for _, c := range []struct {
		b        []byte
		text     string
		repaired bool
	}{{typed, "Pump", false}, {untyped, "Valve", true}} {
		doc, err := (&Extractor{}).Extract(context.Background(), bytes.NewReader(c.b), int64(len(c.b)), nil)
		if err != nil {
			t.Fatal(err)
		}
		if doc.Source.Format != "vsdx" || doc.Source.MIME != "application/vnd.ms-visio.drawing" || !strings.Contains(doc.Text(), c.text) {
			t.Errorf("%s: %s %s %q", c.text, doc.Source.Format, doc.Source.MIME, doc.Text())
		}
		repaired := false
		for _, w := range doc.Warnings {
			repaired = repaired || w.Code == gofastcore.WarnInputRepaired
		}
		if repaired != c.repaired {
			t.Errorf("%s: input.repaired %v, want %v (%+v)", c.text, repaired, c.repaired, doc.Warnings)
		}
	}
}
