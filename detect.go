package gofast

import (
	"archive/zip"
	"bytes"
	"io"
	"mime"
	"path"
	"strings"

	gofasthtml "github.com/AOShei/go-fast-html"
)

// The libraries an input can be routed to, and the archive formats this
// package opens itself.
const (
	LibPDF   = "pdf"   // go-fast-pdf
	LibOOXML = "ooxml" // go-fast-ooxml: Word, Excel and PowerPoint 2007 and later
	LibODF   = "odf"   // go-fast-odf: OpenDocument
	LibRTF   = "rtf"   // go-fast-rtf
	LibEmail = "email" // go-fast-email: eml, mbox, msg, TNEF
	LibHTML  = "html"  // go-fast-html: HTML, Markdown, text, EPUB, CSV, TSV, notebooks
	// LibArchive is an archive this package opens: Format is "zip",
	// "tar" or "gzip".
	LibArchive = "archive"
)

// Kind is where an input goes: the library that reads it and, where the
// detection decides it, the format within that library (as the
// library's Extractor.Format names it), or the archive format. The zero
// Kind is an input no library reads.
type Kind struct {
	Library string
	Format  string
}

// IsArchive reports whether k is an archive.
func (k Kind) IsArchive() bool { return k.Library == LibArchive }

// byExtension routes by the extension of a name, lowercase with its dot.
var byExtension = map[string]Kind{
	".pdf": {LibPDF, ""},

	".docx": {LibOOXML, ""}, ".docm": {LibOOXML, ""}, ".dotx": {LibOOXML, ""}, ".dotm": {LibOOXML, ""},
	".xlsx": {LibOOXML, ""}, ".xlsm": {LibOOXML, ""}, ".xltx": {LibOOXML, ""}, ".xltm": {LibOOXML, ""}, ".xlsb": {LibOOXML, ""},
	".pptx": {LibOOXML, ""}, ".pptm": {LibOOXML, ""}, ".ppsx": {LibOOXML, ""}, ".ppsm": {LibOOXML, ""},
	".potx": {LibOOXML, ""}, ".potm": {LibOOXML, ""},

	".odt": {LibODF, ""}, ".ott": {LibODF, ""}, ".odm": {LibODF, ""}, ".ods": {LibODF, ""}, ".ots": {LibODF, ""},
	".odp": {LibODF, ""}, ".otp": {LibODF, ""}, ".fodt": {LibODF, ""}, ".fods": {LibODF, ""}, ".fodp": {LibODF, ""},

	".rtf": {LibRTF, ""},

	".eml": {LibEmail, ""}, ".mbox": {LibEmail, ""}, ".mbx": {LibEmail, ""}, ".msg": {LibEmail, ""},

	".html": {LibHTML, "html"}, ".htm": {LibHTML, "html"}, ".xhtml": {LibHTML, "html"}, ".shtml": {LibHTML, "html"},
	".md": {LibHTML, "md"}, ".markdown": {LibHTML, "md"}, ".mdown": {LibHTML, "md"}, ".mkd": {LibHTML, "md"},
	".mkdn": {LibHTML, "md"}, ".mdwn": {LibHTML, "md"},
	".txt": {LibHTML, "txt"}, ".text": {LibHTML, "txt"},
	".epub": {LibHTML, "epub"}, ".csv": {LibHTML, "csv"}, ".tsv": {LibHTML, "tsv"}, ".tab": {LibHTML, "tsv"},
	".ipynb": {LibHTML, "ipynb"},

	".zip": {LibArchive, "zip"}, ".tar": {LibArchive, "tar"}, ".tgz": {LibArchive, "gzip"}, ".gz": {LibArchive, "gzip"},
}

// byMIME routes by a declared media type, lowercase without parameters.
var byMIME = map[string]Kind{
	"application/pdf": {LibPDF, ""},

	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   {LibOOXML, ""},
	"application/vnd.openxmlformats-officedocument.wordprocessingml.template":   {LibOOXML, ""},
	"application/vnd.ms-word.document.macroenabled.12":                          {LibOOXML, ""},
	"application/vnd.ms-word.template.macroenabled.12":                          {LibOOXML, ""},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         {LibOOXML, ""},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.template":      {LibOOXML, ""},
	"application/vnd.ms-excel.sheet.macroenabled.12":                            {LibOOXML, ""},
	"application/vnd.ms-excel.template.macroenabled.12":                         {LibOOXML, ""},
	"application/vnd.ms-excel.sheet.binary.macroenabled.12":                     {LibOOXML, ""},
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": {LibOOXML, ""},
	"application/vnd.openxmlformats-officedocument.presentationml.slideshow":    {LibOOXML, ""},
	"application/vnd.openxmlformats-officedocument.presentationml.template":     {LibOOXML, ""},
	"application/vnd.ms-powerpoint.presentation.macroenabled.12":                {LibOOXML, ""},
	"application/vnd.ms-powerpoint.slideshow.macroenabled.12":                   {LibOOXML, ""},
	"application/vnd.ms-powerpoint.template.macroenabled.12":                    {LibOOXML, ""},

	"application/vnd.oasis.opendocument.text":                  {LibODF, ""},
	"application/vnd.oasis.opendocument.text-template":         {LibODF, ""},
	"application/vnd.oasis.opendocument.text-master":           {LibODF, ""},
	"application/vnd.oasis.opendocument.spreadsheet":           {LibODF, ""},
	"application/vnd.oasis.opendocument.spreadsheet-template":  {LibODF, ""},
	"application/vnd.oasis.opendocument.presentation":          {LibODF, ""},
	"application/vnd.oasis.opendocument.presentation-template": {LibODF, ""},

	"application/rtf": {LibRTF, ""}, "text/rtf": {LibRTF, ""},

	"message/rfc822": {LibEmail, ""}, "message/global": {LibEmail, ""}, "application/vnd.ms-outlook": {LibEmail, ""},
	"application/mbox": {LibEmail, ""}, "application/vnd.ms-tnef": {LibEmail, ""},

	"text/html": {LibHTML, "html"}, "application/xhtml+xml": {LibHTML, "html"},
	"text/markdown": {LibHTML, "md"}, "text/plain": {LibHTML, "txt"},
	"text/csv": {LibHTML, "csv"}, "text/tab-separated-values": {LibHTML, "tsv"},
	"application/x-ipynb+json": {LibHTML, "ipynb"}, "application/epub+zip": {LibHTML, "epub"},

	"application/zip": {LibArchive, "zip"}, "application/x-zip-compressed": {LibArchive, "zip"},
	"application/x-tar": {LibArchive, "tar"},
	"application/gzip":  {LibArchive, "gzip"}, "application/x-gzip": {LibArchive, "gzip"},
}

// sniffBytes is how much of the start of an input Detect looks at.
const sniffBytes = 4096

// Detect decides where an input goes. A declared media type (mimeType,
// as a Content-Type gives it; empty or application/octet-stream when
// none) decides first, then the extension of name, then the bytes:
//
//   - "%PDF-" in the first 1 KiB is PDF;
//   - a zip archive is Office if it has [Content_Types].xml,
//     OpenDocument or EPUB if its mimetype member says so, and otherwise
//     an archive;
//   - gzip and tar (ustar) are archives;
//   - "{\rtf" is RTF; an OLE compound file, a TNEF stream, a mailbox's
//     "From " line or a message's header lines are email;
//   - an OpenDocument flat XML file is OpenDocument;
//   - binary input (a NUL byte, or many control characters, in the first
//     4 KiB: images, audio, executables) gives the zero Kind;
//   - anything else is what go-fast-html's Detect says of it: HTML when
//     it is markup, else plain text.
//
// A name whose extension is an Office 97-2003 format (.doc, .xls, .ppt and
// their templates) gives the zero Kind: no library reads those yet.
func Detect(name, mimeType string, r io.ReaderAt, size int64) Kind {
	if t := mediaType(mimeType); t != "" && t != "application/octet-stream" {
		if k, ok := byMIME[t]; ok {
			return k
		}
	}
	ext := strings.ToLower(path.Ext(name))
	if strings.HasSuffix(strings.ToLower(name), ".tar.gz") {
		return Kind{LibArchive, "gzip"}
	}
	if k, ok := byExtension[ext]; ok {
		return k
	}
	if legacyOffice[ext] {
		return Kind{}
	}
	if strings.EqualFold(path.Base(name), "winmail.dat") {
		return Kind{LibEmail, ""}
	}
	return sniff(name, r, size)
}

// legacyOffice are the extensions of the Office 97-2003 formats.
var legacyOffice = map[string]bool{
	".doc": true, ".dot": true, ".xls": true, ".xlt": true, ".xla": true,
	".ppt": true, ".pps": true, ".pot": true,
}

// KnownExtension reports whether a name's extension is one Detect routes
// to a library or an archive: the files a directory stands for.
func KnownExtension(name string) bool {
	if strings.HasSuffix(strings.ToLower(name), ".tar.gz") {
		return true
	}
	_, ok := byExtension[strings.ToLower(path.Ext(name))]
	return ok
}

// sniff decides from the bytes.
func sniff(name string, r io.ReaderAt, size int64) Kind {
	if r == nil || size <= 0 {
		return Kind{LibHTML, "txt"}
	}
	head := make([]byte, min(size, sniffBytes))
	n, _ := r.ReadAt(head, 0)
	head = head[:n]
	switch {
	case bytes.Contains(head[:min(len(head), 1024)], []byte("%PDF-")):
		return Kind{LibPDF, ""}
	case bytes.HasPrefix(head, []byte("PK\x03\x04")) || bytes.HasPrefix(head, []byte("PK\x05\x06")):
		return sniffZip(r, size)
	case bytes.HasPrefix(head, []byte{0x1f, 0x8b}):
		return Kind{LibArchive, "gzip"}
	case len(head) >= 262 && string(head[257:262]) == "ustar":
		return Kind{LibArchive, "tar"}
	case bytes.HasPrefix(head, []byte(`{\rtf`)):
		return Kind{LibRTF, ""}
	case bytes.HasPrefix(head, []byte("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1")):
		// An OLE compound file: an Outlook message is the one this
		// series reads; go-fast-email refuses another.
		return Kind{LibEmail, ""}
	case bytes.HasPrefix(head, []byte{0x78, 0x9f, 0x3e, 0x22}): // TNEF
		return Kind{LibEmail, ""}
	case bytes.HasPrefix(head, []byte("From ")):
		return Kind{LibEmail, ""}
	case messageHeaders(head):
		return Kind{LibEmail, ""}
	case bytes.Contains(head, []byte("urn:oasis:names:tc:opendocument:xmlns:office:1.0")) &&
		bytes.Contains(head, []byte("office:document")):
		return Kind{LibODF, ""}
	}
	if isBinary(head) {
		return Kind{}
	}
	return Kind{LibHTML, gofasthtml.Detect(name, head)}
}

// isBinary reports whether the start of an input is not text: it has a NUL
// byte, or more than one byte in ten is a control character other than
// white space, unless it begins with a UTF-16 byte order mark (text
// go-fast-html decodes). Images, audio, executables and the like are
// binary; no library reads them.
func isBinary(head []byte) bool {
	if bytes.HasPrefix(head, []byte{0xFF, 0xFE}) || bytes.HasPrefix(head, []byte{0xFE, 0xFF}) {
		return false
	}
	control := 0
	for _, c := range head {
		switch {
		case c == 0:
			return true
		case c < 0x20 && c != '\t' && c != '\n' && c != '\r' && c != '\f' && c != 0x1b:
			control++
		}
	}
	return control*10 > len(head)
}

// sniffZip decides what a zip archive is from its members.
func sniffZip(r io.ReaderAt, size int64) Kind {
	z, err := zip.NewReader(r, size)
	if err != nil {
		return Kind{LibArchive, "zip"}
	}
	for i, f := range z.File {
		switch {
		case f.Name == "[Content_Types].xml":
			return Kind{LibOOXML, ""}
		case i == 0 && f.Name == "mimetype" && f.UncompressedSize64 < 256:
			rc, err := f.Open()
			if err != nil {
				break
			}
			b, _ := io.ReadAll(io.LimitReader(rc, 256))
			rc.Close()
			t := strings.TrimSpace(string(b))
			switch {
			case t == "application/epub+zip":
				return Kind{LibHTML, "epub"}
			case strings.HasPrefix(t, "application/vnd.oasis.opendocument."):
				return Kind{LibODF, ""}
			}
		}
	}
	return Kind{LibArchive, "zip"}
}

// messageHeaders reports whether an input begins with the header lines of
// a message: at least two lines "Name: value", one of them a header every
// message or mail program writes.
func messageHeaders(head []byte) bool {
	known := 0
	lines := 0
	for len(head) > 0 && lines < 8 {
		line, rest, ok := bytes.Cut(head, []byte("\n"))
		if !ok {
			break
		}
		head = rest
		line = bytes.TrimRight(line, "\r")
		if len(line) == 0 {
			break
		}
		if line[0] == ' ' || line[0] == '\t' {
			continue // a folded line
		}
		name, _, ok := bytes.Cut(line, []byte(":"))
		if !ok || len(name) == 0 || bytes.ContainsAny(name, " \t") {
			return false
		}
		lines++
		switch strings.ToLower(string(name)) {
		case "received", "return-path", "from", "to", "subject", "date", "message-id", "mime-version", "delivered-to":
			known++
		}
	}
	return lines >= 2 && known >= 1
}

// mediaType is the media type of a Content-Type value, lowercase and
// without parameters ("" if none).
func mediaType(v string) string {
	if v == "" {
		return ""
	}
	if t, _, err := mime.ParseMediaType(v); err == nil {
		return t
	}
	t, _, _ := strings.Cut(v, ";")
	return strings.ToLower(strings.TrimSpace(t))
}
