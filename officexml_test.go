package gofast

import (
	"bytes"
	"context"
	"strings"
	"testing"

	gofastcore "github.com/AOShei/go-fast-core"
)

const (
	nsFlatOPC       = "http://schemas.microsoft.com/office/2006/xmlPackage"
	nsWordML        = "http://schemas.microsoft.com/office/word/2003/wordml"
	nsSpreadsheetML = "urn:schemas-microsoft-com:office:spreadsheet"
)

const wordML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<?mso-application progid="Word.Document"?>
<w:wordDocument xmlns:w="` + nsWordML + `" xmlns:wx="http://schemas.microsoft.com/office/word/2003/auxHint"><w:body><wx:sect>
<w:p><w:r><w:t>Hello from Word 2003.</w:t></w:r></w:p>
</wx:sect></w:body></w:wordDocument>`

const spreadsheetML = `<?xml version="1.0"?>
<?mso-application progid="Excel.Sheet"?>
<Workbook xmlns="` + nsSpreadsheetML + `" xmlns:ss="` + nsSpreadsheetML + `">
<Worksheet ss:Name="Sheet1"><Table>
<Row><Cell><Data ss:Type="String">Region</Data></Cell><Cell><Data ss:Type="String">Sales</Data></Cell></Row>
<Row><Cell><Data ss:Type="String">North</Data></Cell><Cell><Data ss:Type="Number">42</Data></Cell></Row>
</Table></Worksheet></Workbook>`

const flatOPC = `<?xml version="1.0" standalone="yes"?>
<?mso-application progid="Word.Document"?>
<pkg:package xmlns:pkg="` + nsFlatOPC + `">
<pkg:part pkg:name="/_rels/.rels" pkg:contentType="application/vnd.openxmlformats-package.relationships+xml"><pkg:xmlData>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>
</pkg:xmlData></pkg:part>
<pkg:part pkg:name="/word/document.xml" pkg:contentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"><pkg:xmlData>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Hello from Flat OPC.</w:t></w:r></w:p></w:body></w:document>
</pkg:xmlData></pkg:part>
</pkg:package>`

// utf16 writes ASCII text as UTF-16, with a byte order mark when bom.
func utf16(s string, bigEndian, bom bool) []byte {
	var b []byte
	if bom {
		if bigEndian {
			b = []byte{0xFE, 0xFF}
		} else {
			b = []byte{0xFF, 0xFE}
		}
	}
	for _, c := range []byte(s) {
		if bigEndian {
			b = append(b, 0, c)
		} else {
			b = append(b, c, 0)
		}
	}
	return b
}

// TestDetectOfficeXML: an Office document saved as XML goes to go-fast-ooxml
// by its root element and namespace, whatever its name or media type says,
// in the encodings Office writes, after a document type declaration that
// declares nothing; other XML stays with go-fast-xml.
func TestDetectOfficeXML(t *testing.T) {
	ooxml, xml, html := Kind{LibOOXML, ""}, Kind{LibXML, ""}, Kind{LibHTML, "html"}
	workbook := func(decl, before string) []byte {
		return []byte(decl + before + `<Workbook xmlns="` + nsSpreadsheetML + `"><Worksheet/></Workbook>`)
	}
	for _, c := range []struct {
		what, name, mime string
		body             []byte
		want             Kind
	}{
		{"WordprocessingML 2003", "doc.xml", "", []byte(wordML), ooxml},
		{"SpreadsheetML 2003", "book.xml", "", []byte(spreadsheetML), ooxml},
		{"Flat OPC", "doc.xml", "", []byte(flatOPC), ooxml},
		{"no name", "", "", []byte(wordML), ooxml},
		{"other name", "notes.txt.bak", "", []byte(spreadsheetML), ooxml},
		{"application/xml", "x", "application/xml", []byte(flatOPC), ooxml},
		{"text/xml", "x", "text/xml; charset=utf-8", []byte(spreadsheetML), ooxml},
		{"a +xml type", "x", "application/rss+xml", []byte(wordML), ooxml},
		{"Tika's type", "x", "application/vnd.ms-spreadsheetml", nil, ooxml},
		{"Tika's type", "x", "application/vnd.ms-wordml", nil, ooxml},
		{"other root prefix", "x.xml", "", []byte(`<?xml version="1.0"?><word:wordDocument xmlns:word="` + nsWordML + `"/>`), ooxml},
		{"windows-1251", "x.xml", "", workbook(`<?xml version="1.0" encoding="windows-1251"?>`, "<!-- \xcf\xf0\xe8\xe2\xe5\xf2 -->"), ooxml},
		{"Shift JIS", "x.xml", "", workbook(`<?xml version="1.0" encoding="Shift_JIS"?>`, "<!-- \x93\xfa\x96\x7b\x8c\xea -->"), ooxml},
		{"UTF-8 BOM", "x.xml", "", workbook("\xEF\xBB\xBF<?xml version=\"1.0\"?>\n", ""), ooxml},
		{"UTF-16LE", "", "", utf16(`<?xml version="1.0" encoding="UTF-16"?><Workbook xmlns="`+nsSpreadsheetML+`"/>`, false, true), ooxml},
		{"UTF-16BE", "", "", utf16(`<?xml version="1.0" encoding="UTF-16"?><Workbook xmlns="`+nsSpreadsheetML+`"/>`, true, true), ooxml},
		{"UTF-16LE, no BOM", "x.xml", "", utf16(`<?xml version="1.0" encoding="UTF-16"?><Workbook xmlns="`+nsSpreadsheetML+`"/>`, false, false), ooxml},
		{"root past 4 KiB", "x.xml", "", workbook(`<?xml version="1.0"?>`, "<!--"+strings.Repeat("x", 6000)+"-->"), ooxml},
		{"a declaration that declares nothing", "x.xml", "", workbook(`<?xml version="1.0"?>`, `<!DOCTYPE Workbook>`), ooxml},
		{"one with an external DTD", "x.xml", "", workbook(`<?xml version="1.0"?>`, `<!DOCTYPE Workbook SYSTEM "a[1]>.dtd">`), ooxml},
		{"one in Word 2003", "x.xml", "", []byte(`<?xml version="1.0"?><!DOCTYPE w:wordDocument><w:wordDocument xmlns:w="` + nsWordML + `"/>`), ooxml},
		{"root tag past 4 KiB", "x.xml", "", []byte(`<?xml version="1.0"?><Workbook` + strings.Repeat(` a="1"`, 1000) + ` xmlns="` + nsSpreadsheetML + `"/>`), ooxml},
		// Not Office.
		{"root past 64 KiB", "x.xml", "", workbook(`<?xml version="1.0"?>`, "<!--"+strings.Repeat("x", 70000)+"-->"), xml},
		{"a declaration with an internal subset", "x.xml", "", workbook(`<?xml version="1.0"?>`, `<!DOCTYPE Workbook [<!ENTITY a "b">]>`), xml},
		{"two declarations", "x.xml", "", workbook(`<?xml version="1.0"?>`, `<!DOCTYPE Workbook><!DOCTYPE Workbook>`), xml},
		{"other namespace", "x.xml", "", []byte(`<?xml version="1.0"?><Workbook xmlns="urn:example"/>`), xml},
		{"no namespace", "x.xml", "", []byte(`<?xml version="1.0"?><Workbook/>`), xml},
		{"prefix not bound", "x.xml", "", []byte(`<?xml version="1.0"?><wordDocument xmlns:w="` + nsWordML + `"/>`), xml},
		{"other local name", "x.xml", "", []byte(`<?xml version="1.0"?><w:document xmlns:w="` + nsWordML + `"/>`), xml},
		{"a feed", "x", "application/rss+xml", []byte(`<?xml version="1.0"?><rss version="2.0"/>`), xml},
		{"no mark of XML", "noext", "", []byte(`<Workbook><Worksheet/></Workbook>`), Kind{LibHTML, "txt"}},
		{"XHTML type", "x", "application/xhtml+xml", []byte(spreadsheetML), html},
		{"a .docx name", "x.docx", "", []byte(flatOPC), ooxml},
	} {
		got := Detect(c.name, c.mime, bytes.NewReader(c.body), int64(len(c.body)))
		if got != c.want {
			t.Errorf("%s: Detect(%q, %q) = %+v, want %+v", c.what, c.name, c.mime, got, c.want)
		}
	}
}

// TestOfficeXMLExtract: each of the three is read by go-fast-ooxml through
// the umbrella, on its own and as a member of an archive.
func TestOfficeXMLExtract(t *testing.T) {
	for _, c := range []struct {
		name, body, format, packaging, text string
	}{
		{"doc.xml", wordML, "wordml", "", "Hello from Word 2003."},
		{"book.xml", spreadsheetML, "spreadsheetml", "", "North"},
		{"flat.xml", flatOPC, "docx", "flat_opc", "Hello from Flat OPC."},
		{"doctype.xml", strings.Replace(spreadsheetML, "<Workbook", "<!DOCTYPE Workbook>\n<Workbook", 1), "spreadsheetml", "", "North"},
	} {
		doc, err := extractNamed(&Extractor{}, c.name, c.body)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if doc.Source.Format != c.format || !strings.Contains(doc.Text(), c.text) {
			t.Errorf("%s: format %q, text %q", c.name, doc.Source.Format, doc.Text())
		}
		if got := doc.Metadata.Extra["packaging"]; got != c.packaging {
			t.Errorf("%s: packaging %q, want %q", c.name, got, c.packaging)
		}
	}
	z := zipOf(t, file{name: "a/book.xml", body: spreadsheetML}, file{name: "a/report.xml", body: xmlDoc})
	doc, err := (&Extractor{}).Extract(context.Background(), bytes.NewReader(z), int64(len(z)), &gofastcore.Options{Name: "x.zip"})
	if err != nil || len(doc.Children) != 2 {
		t.Fatalf("zip document: %v", err)
	}
	if f := []string{doc.Children[0].Source.Format, doc.Children[1].Source.Format}; f[0] != "spreadsheetml" || f[1] != "xml" {
		t.Errorf("zip members: %v", f)
	}
	for _, f := range []string{"wordml", "spreadsheetml"} {
		if !contains((&Extractor{}).Formats(), f) {
			t.Errorf("Formats lacks %s: %v", f, (&Extractor{}).Formats())
		}
	}
}
