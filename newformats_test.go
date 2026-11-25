package gofast

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/AOShei/go-fast-core"
)

// Saved web pages, subtitles, calendars and contacts: by name, media type
// and bytes.
func TestDetectNewFormats(t *testing.T) {
	mht := "From: <Saved by Blink>\r\nSnapshot-Content-Location: https://example.org/\r\nSubject: Page\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: multipart/related; type=\"text/html\"; boundary=B\r\n\r\n" +
		"--B\r\nContent-Type: text/html\r\nContent-Location: https://example.org/\r\n\r\n<p>the page</p>\r\n--B--\r\n"
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:1\r\nSUMMARY:Meeting\r\nDTSTART:20261125T100000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	vcf := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Jane Doe\r\nEMAIL:jane@example.org\r\nEND:VCARD\r\n"
	vtt := "WEBVTT\n\n00:01.000 --> 00:02.000\nHello\n"
	srt := "1\n00:00:01,000 --> 00:00:02,000\nHello\n"
	for _, c := range []struct {
		name, mime, body string
		want             Kind
	}{
		{"page.mht", "", mht, Kind{LibHTML, "mht"}},
		{"page.MHTML", "", mht, Kind{LibHTML, "mht"}},
		{"", "", mht, Kind{LibHTML, "mht"}},
		{"", "application/x-mimearchive", mht, Kind{LibHTML, "mht"}},
		{"a.srt", "", srt, Kind{LibHTML, "srt"}},
		{"a.vtt", "", vtt, Kind{LibHTML, "vtt"}},
		{"", "text/vtt", vtt, Kind{LibHTML, "vtt"}},
		{"", "", vtt, Kind{LibHTML, "vtt"}},
		{"a.ics", "", ics, Kind{LibEmail, ""}},
		{"", "text/calendar", ics, Kind{LibEmail, ""}},
		{"", "", ics, Kind{LibEmail, ""}},
		{"", "", "BEGIN:VEVENT\r\nSUMMARY:x\r\nEND:VEVENT\r\n", Kind{LibEmail, ""}},
		{"a.vcf", "", vcf, Kind{LibEmail, ""}},
		{"", "text/vcard", vcf, Kind{LibEmail, ""}},
		{"", "", "home.begin:vcard\nfn:x\nhome.end:vcard\n", Kind{LibEmail, ""}},
		// A message with a multipart/related body is a message.
		{"", "", "From: a@example.org\r\nTo: b@example.org\r\nSubject: s\r\nMIME-Version: 1.0\r\n" +
			"Content-Type: multipart/related; boundary=B\r\n\r\n--B\r\n\r\nx\r\n--B--\r\n", Kind{LibEmail, ""}},
	} {
		if got := Detect(c.name, c.mime, strings.NewReader(c.body), int64(len(c.body))); got != c.want {
			t.Errorf("Detect(%q, %q, %.30q) = %+v, want %+v", c.name, c.mime, c.body, got, c.want)
		}
	}
}

func TestExtractNewFormats(t *testing.T) {
	for _, c := range []struct{ name, body, format, text string }{
		{"a.mht", "From: <Saved by Blink>\r\nContent-Type: multipart/related; boundary=B\r\n\r\n--B\r\nContent-Type: text/html\r\n\r\n<p>the page</p>\r\n--B--\r\n", "mht", "the page"},
		{"a.srt", "1\n00:00:01,000 --> 00:00:02,000\nHello\n", "srt", "Hello"},
		{"a.vtt", "WEBVTT\n\n00:01.000 --> 00:02.000\nHello\n", "vtt", "Hello"},
		{"a.ics", "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:1\r\nSUMMARY:Meeting\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n", "ics", "Meeting"},
		{"a.vcf", "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Jane Doe\r\nEND:VCARD\r\n", "vcf", "Jane Doe"},
	} {
		doc, err := (&Extractor{}).Extract(context.Background(), bytes.NewReader([]byte(c.body)), int64(len(c.body)), &gofastcore.Options{Name: c.name})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if err := doc.Validate(); err != nil {
			t.Fatalf("%s: Validate: %v", c.name, err)
		}
		if doc.Source.Format != c.format || !strings.Contains(doc.Text(), c.text) {
			t.Errorf("%s: format %q, text %q", c.name, doc.Source.Format, doc.Text())
		}
	}
}
