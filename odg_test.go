package gofast

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/AOShei/go-fast-core"
)

// OpenDocument drawings go to go-fast-odf by name, media type and bytes.
func TestDrawings(t *testing.T) {
	flat := `<?xml version="1.0" encoding="UTF-8"?><office:document xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"` +
		` xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"` +
		` office:version="1.3" office:mimetype="application/vnd.oasis.opendocument.graphics"><office:body><office:drawing>` +
		`<draw:page draw:name="page1"><draw:custom-shape><text:p>Pump</text:p></draw:custom-shape></draw:page>` +
		`</office:drawing></office:body></office:document>`
	odf := Kind{LibODF, ""}
	for _, c := range []struct{ name, mime string }{
		{"a.odg", ""}, {"a.OTG", ""}, {"a.fodg", ""},
		{"", "application/vnd.oasis.opendocument.graphics"},
		{"", "application/vnd.oasis.opendocument.graphics-template"},
		{"", ""},
	} {
		if got := Detect(c.name, c.mime, strings.NewReader(flat), int64(len(flat))); got != odf {
			t.Errorf("Detect(%q, %q) = %+v", c.name, c.mime, got)
		}
	}
	doc, err := (&Extractor{}).Extract(context.Background(), bytes.NewReader([]byte(flat)), int64(len(flat)), &gofastcore.Options{Name: "a.fodg"})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Source.Format != "fodg" || len(doc.Parts) != 1 || doc.Parts[0].Kind != gofastcore.PartPage || doc.Parts[0].Blocks[0].Text != "Pump" {
		t.Errorf("%s %+v", doc.Source.Format, doc.Parts)
	}
}
