package gofast

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	gofastcore "github.com/AOShei/go-fast-core"
	gofasthtml "github.com/AOShei/go-fast-html"
)

// czech is Czech in windows-1250, which is not certain from its bytes.
const czech = "Voda pokr\xfdv\xe1 p\xf8ibli\x9en\xec sedmdes\xe1t procent zemsk\xe9ho povrchu a je nezbytn\xe1 pro v\x9aechny \x9eiv\xe9 organismy. " +
	"\xc8eskou republikou prot\xe9kaj\xed \xf8eky Vltava, Labe a Morava.\n"

func extractNamed(e *Extractor, name, src string) (*gofastcore.Document, error) {
	return e.Extract(context.Background(), bytes.NewReader([]byte(src)), int64(len(src)), &gofastcore.Options{Name: name})
}

// TestCharset: text whose set is not certain is refused without Charset,
// and read in it with; a set the text declares is not overridden; text in
// a message is read in it.
func TestCharset(t *testing.T) {
	if _, err := extractNamed(&Extractor{}, "a.txt", czech); !errors.Is(err, gofasthtml.ErrEncodingUnknown) || !errors.Is(err, gofastcore.ErrUnsupported) {
		t.Fatalf("without Charset: %v", err)
	}
	e := &Extractor{Charset: "windows-1250"}
	for _, name := range []string{"a.txt", "a.md", "a.csv"} {
		doc, err := extractNamed(e, name, czech)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(doc.Text(), "příbližně") && !strings.Contains(doc.Text(), "přibližně") {
			t.Errorf("%s: %.120q", name, doc.Text())
		}
	}
	// A page that declares its set keeps it.
	doc, err := extractNamed(e, "a.html", `<meta charset="windows-1252"><p>caf`+"\xe9")
	if err != nil || !strings.Contains(doc.Text(), "café") {
		t.Errorf("declared: %v", err)
	}
	// A message's body that declares no set.
	doc, err = extractNamed(e, "m.eml", "Subject: x\r\n\r\n"+czech)
	if err != nil || !strings.Contains(doc.Text(), "Vltava") || !strings.Contains(doc.Text(), "řeky") {
		t.Errorf("message: %v", err)
	}
	// A JSON file in Shift_JIS is recognised; no Charset needed.
	sjis := "{\"name\": \"\x93\xfa\x96\x7b\x8c\xea\x82\xcc\x83\x65\x83\x4c\x83\x58\x83\x67\x82\xc5\x82\xb7\x81\x42\x82\xb1\x82\xea\x82\xcd\x93\xfa\x96\x7b\x8c\xea\x82\xc5\x82\xb7\x81\x42\"}\n"
	doc, err = extractNamed(&Extractor{}, "a.json", sjis)
	if err != nil || !strings.Contains(doc.Text(), "日本語のテキストです") {
		t.Errorf("json: %v %.80q", err, func() string {
			if doc == nil {
				return ""
			}
			return doc.Text()
		}())
	}
}
