package gofast

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	gofastcore "github.com/AOShei/go-fast-core"
	gofastxml "github.com/AOShei/go-fast-xml"
)

// TestDetectXML: XML goes to go-fast-xml by its name, its media type or
// its bytes; XHTML, SVG and OpenDocument flat files stay where they were.
func TestDetectXML(t *testing.T) {
	xml, html, odf := Kind{LibXML, ""}, Kind{LibHTML, "html"}, Kind{LibODF, ""}
	utf16le := func(s string) []byte {
		b := []byte{0xFF, 0xFE}
		for _, c := range []byte(s) {
			b = append(b, c, 0)
		}
		return b
	}
	for _, c := range []struct {
		name, mime string
		body       []byte
		want       Kind
	}{
		{"law.XML", "", nil, xml},
		{"topic.dita", "", nil, xml},
		{"map.ditamap", "", nil, xml},
		{"book.docbook", "", nil, xml},
		{"text.tei", "", nil, xml},
		{"x", "application/xml", nil, xml},
		{"x", "text/xml; charset=iso-8859-1", nil, xml},
		{"x", "application/atom+xml", nil, xml},
		{"x", "application/rss+xml", nil, xml},
		{"page.xhtml", "", nil, html},
		{"x", "application/xhtml+xml", []byte(`<?xml version="1.0"?><root/>`), html},
		{"x", "image/svg+xml", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`), html},
		{"noext", "", []byte(`<?xml version="1.0" encoding="UTF-8"?><akomaNtoso/>`), xml},
		{"noext", "", []byte("\xEF\xBB\xBF\n <?xml version='1.0'?>\n<!-- c --><!DOCTYPE doc [<!ENTITY a \"]>\">]><doc>x</doc>"), xml},
		{"noext", "", []byte(`<Legislation xmlns="http://www.legislation.gov.uk/namespaces/legislation" xml:lang="en">`), xml},
		{"noext", "", []byte(`<a:doc xmlns:a="urn:a"><a:p>x</a:p></a:doc>`), xml},
		{"noext", "", utf16le(`<?xml version="1.0" encoding="UTF-16"?><doc/>`), xml},
		{"noext", "", []byte(`<?xml version="1.0"?>` + strings.Repeat(" ", 5000)), xml}, // no element within what is looked at
		{"noext", "", []byte("<!DOCTYPE article PUBLIC \"-//NLM//DTD JATS (Z39.96) Journal Archiving\" \"a.dtd\">\n<article xml:lang=\"en\">"), xml},
		{"noext", "", []byte(`<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 3.2 Final//EN"><title>x</title>`), html},
		{"noext", "", []byte(`<!doctype html>`), html},
		{"noext", "", []byte(`<appendix id="a"><title>No mark of XML</title></appendix>`), html}, // nothing says it is XML
		// XHTML, SVG and HTML stay with go-fast-html.
		{"noext", "", []byte(`<?xml version="1.0"?><!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "x"><html xmlns="http://www.w3.org/1999/xhtml"><p>x</p></html>`), html},
		{"noext", "", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><text>x</text></svg>`), html},
		{"noext", "", []byte(`<div xmlns="http://www.w3.org/1999/xhtml"><p>x</p></div>`), html},
		{"noext", "", []byte(`<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml" lang="en"><p>x`), html},
		{"noext", "", []byte(`<html><body><p>x</p></body></html>`), html},
		{"noext", "", []byte(`<p>no namespace, no declaration</p>`), html},
		{"noext", "", []byte(`<?xml-stylesheet href="a"?>`), html}, // not a declaration, and no element
		// OpenDocument flat XML stays with go-fast-odf.
		{"noext", "", []byte(`<?xml version="1.0"?><office:document xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0">`), odf},
	} {
		got := Detect(c.name, c.mime, bytes.NewReader(c.body), int64(len(c.body)))
		if got != c.want {
			t.Errorf("Detect(%q, %q, %.40q) = %+v, want %+v", c.name, c.mime, c.body, got, c.want)
		}
	}
}

const xmlDoc = `<?xml version="1.0" encoding="UTF-8"?>
<report xmlns="urn:example:report" id="r1"><title>Annual report</title>
<section><heading>Income</heading><para>Income rose by <b>4%</b>.</para></section></report>
`

// TestXMLExtract: an XML file read through the umbrella, on its own and as
// a member of an archive, is what go-fast-xml makes of it.
func TestXMLExtract(t *testing.T) {
	ctx := context.Background()
	want, err := (&gofastxml.Extractor{}).Extract(ctx, strings.NewReader(xmlDoc), int64(len(xmlDoc)), &gofastcore.Options{Name: "report.xml"})
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, _ := json.Marshal(want)
	got, err := extractNamed(&Extractor{}, "report.xml", xmlDoc)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(got); !bytes.Equal(b, wantJSON) {
		t.Errorf("through the umbrella:\n%s\nwant\n%s", b, wantJSON)
	}
	if got.Source.Format != "xml" || !strings.Contains(got.Text(), "Income rose by 4%.") {
		t.Errorf("format %q, text %q", got.Source.Format, got.Text())
	}
	// Named nothing, it is known by its bytes.
	if got, err := extractNamed(&Extractor{}, "", xmlDoc); err != nil || got.Source.Format != "xml" {
		t.Errorf("no name: %v", err)
	}
	// A member of a zip archive.
	z := zipOf(t, file{name: "a/report.xml", body: xmlDoc})
	paths, _, err := walkAll(t, &Extractor{}, Kind{LibArchive, "zip"}, "x.zip", z, WalkOptions{})
	if err != nil || len(paths) != 1 {
		t.Fatalf("zip: %v %v", paths, err)
	}
	doc, err := (&Extractor{}).Extract(ctx, bytes.NewReader(z), int64(len(z)), &gofastcore.Options{Name: "x.zip"})
	if err != nil || len(doc.Children) != 1 {
		t.Fatalf("zip document: %v", err)
	}
	if c := doc.Children[0]; c.Source.Format != "xml" || !strings.Contains(c.Text(), "Income rose") {
		t.Errorf("zip member: %q %q", c.Source.Format, c.Text())
	}
	if !contains((&Extractor{}).Formats(), "xml") {
		t.Errorf("Formats: %v", (&Extractor{}).Formats())
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// TestXMLCharset: XML that declares no encoding and whose encoding is not
// certain is refused without Charset and read in it with; an encoding the
// file declares is not overridden.
func TestXMLCharset(t *testing.T) {
	src := "<doc>" + czech + "</doc>"
	if _, err := extractNamed(&Extractor{}, "a.xml", src); !errors.Is(err, gofastcore.ErrUnsupported) {
		t.Fatalf("without Charset: %v", err)
	}
	doc, err := extractNamed(&Extractor{Charset: "windows-1250"}, "a.xml", src)
	if err != nil || !strings.Contains(doc.Text(), "Vltava") || !strings.Contains(doc.Text(), "řeky") {
		t.Fatalf("with Charset: %v", err)
	}
	declared := `<?xml version="1.0" encoding="windows-1252"?><doc>caf` + "\xe9</doc>"
	doc, err = extractNamed(&Extractor{Charset: "windows-1250"}, "a.xml", declared)
	if err != nil || !strings.Contains(doc.Text(), "café") {
		t.Errorf("declared: %v %q", err, doc.Text())
	}
}

// TestXMLOptions: XMLProfiles and XMLAttributes reach XML read anywhere,
// a member of an archive too.
func TestXMLOptions(t *testing.T) {
	p, err := gofastxml.LoadProfile(strings.NewReader(`{"profile": "memo", "title": "Memos", "format": 1, "match": [{"root": "memo"}],
  "rules": [{"match": "subject", "as": "heading", "level": 1}]}`))
	if err != nil {
		t.Fatal(err)
	}
	z := zipOf(t, file{name: "m.xml", body: `<memo><subject>Budget</subject></memo>`}, file{name: "d.xml", body: `<?xml version="1.0"?><data code="X9">12</data>`})
	e := &Extractor{XMLProfiles: []*gofastxml.Profile{p}, XMLAttributes: gofastxml.AttributesNone}
	doc, err := e.Extract(context.Background(), bytes.NewReader(z), int64(len(z)), &gofastcore.Options{Name: "x.zip"})
	if err != nil || len(doc.Children) != 2 {
		t.Fatalf("%v", err)
	}
	if m := doc.Children[0]; m.Metadata.Extra["xml.profile"] != "memo" {
		t.Errorf("profile: %v", m.Metadata.Extra)
	}
	for _, b := range doc.Children[1].Parts[0].Blocks {
		if b.Kind == gofastcore.KindNote {
			t.Errorf("attribute note with AttributesNone: %+v", b)
		}
	}
}
