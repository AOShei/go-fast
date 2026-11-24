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
	LibXML   = "xml"   // go-fast-xml: XML of any schema, with profiles for some
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
	".json":  {LibHTML, "json"}, ".jsonl": {LibHTML, "json"}, ".ndjson": {LibHTML, "json"}, ".geojson": {LibHTML, "json"},
	".yaml": {LibHTML, "yaml"}, ".yml": {LibHTML, "yaml"},

	".xml": {LibXML, ""}, ".dita": {LibXML, ""}, ".ditamap": {LibXML, ""}, ".docbook": {LibXML, ""},
	".tei": {LibXML, ""},

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
	"application/json": {LibHTML, "json"}, "application/x-ndjson": {LibHTML, "json"}, "application/geo+json": {LibHTML, "json"},
	"application/yaml": {LibHTML, "yaml"}, "application/x-yaml": {LibHTML, "yaml"}, "text/yaml": {LibHTML, "yaml"},
	"text/x-yaml": {LibHTML, "yaml"},

	"application/xml": {LibXML, ""}, "text/xml": {LibXML, ""},
	// Tika's names for Office 2003's XML formats.
	"application/vnd.ms-wordml": {LibOOXML, ""}, "application/vnd.ms-spreadsheetml": {LibOOXML, ""},

	"application/zip": {LibArchive, "zip"}, "application/x-zip-compressed": {LibArchive, "zip"},
	"application/x-tar": {LibArchive, "tar"},
	"application/gzip":  {LibArchive, "gzip"}, "application/x-gzip": {LibArchive, "gzip"},
}

// sniffBytes is how much of the start of an input Detect looks at.
const sniffBytes = 4096

// Detect decides where an input goes. A declared media type (mimeType,
// as a Content-Type gives it) decides first, unless it is one mail
// programs give whatever the file (application/octet-stream,
// application/zip, text/plain); then the extension of name; then, but
// for text/plain, which is text, the bytes:
//
//   - "%PDF-1." or "%PDF-2." in the first 1 KiB is PDF;
//   - a zip archive is Office if it has [Content_Types].xml,
//     OpenDocument or EPUB if its mimetype member says so, and otherwise
//     an archive;
//   - gzip and tar (ustar) are archives;
//   - "{\rtf" is RTF; an OLE compound file, a TNEF stream, a mailbox's
//     "From " line (an address, then a date with a time) or a message's
//     header lines (at least two of the headers messages have, one of
//     them From, Received, Return-Path, Message-ID, MIME-Version or
//     Delivered-To) are email;
//   - an OpenDocument flat XML file is OpenDocument;
//   - XML is go-fast-xml's: input that begins with an XML declaration, or
//     whose first element declares a namespace, unless that element is
//     html or svg, which go-fast-html reads;
//   - binary input (a NUL byte, or many control characters, in the first
//     4 KiB: images, audio, executables) gives the zero Kind;
//   - anything else is what go-fast-html's Detect says of it: HTML when
//     it is markup, else plain text.
//
// Whatever decides that an input is go-fast-xml's (its media type, its
// name or its bytes), an Office document saved as one XML file is
// go-fast-ooxml's: Flat OPC (pkg:package), Word 2003's XML document
// (w:wordDocument) and Excel 2003's XML spreadsheet (Workbook), by the
// root element and its namespace, within the first 64 KiB.
//
// A name whose extension is an Office 97-2003 format (.doc, .xls, .ppt and
// their templates) gives the zero Kind: no library reads those yet.
func Detect(name, mimeType string, r io.ReaderAt, size int64) Kind {
	k := detect(name, mimeType, r, size)
	if k.Library == LibXML && officeXML(r, size) {
		return Kind{LibOOXML, ""}
	}
	return k
}

// detect is Detect but for Office documents saved as XML.
func detect(name, mimeType string, r io.ReaderAt, size int64) Kind {
	t := mediaType(mimeType)
	if k, ok := byMIME[t]; ok && !generic[t] {
		return k
	}
	if strings.HasSuffix(t, "+xml") && !xmlToHTML[t] {
		return Kind{LibXML, ""}
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
	if t == "text/plain" {
		// Declared text with no extension saying otherwise is text.
		return Kind{LibHTML, "txt"}
	}
	return sniff(name, r, size)
}

// xmlToHTML are the XML media types go-fast-html reads: XHTML and SVG
// (an SVG's text without its drawing's attributes).
var xmlToHTML = map[string]bool{"application/xhtml+xml": true, "image/svg+xml": true}

// generic are the media types a mail program gives files it does not
// know better: a .docx or an .epub sent as application/zip, a .pdf as
// text/plain. A known extension, or the bytes, decide over them.
var generic = map[string]bool{
	"application/octet-stream": true, "application/zip": true, "application/x-zip-compressed": true, "text/plain": true,
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
	case bytes.Contains(head[:min(len(head), 1024)], []byte("%PDF-1.")) || bytes.Contains(head[:min(len(head), 1024)], []byte("%PDF-2.")):
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
	case mboxLine(head):
		return Kind{LibEmail, ""}
	case messageHeaders(head):
		return Kind{LibEmail, ""}
	case bytes.Contains(head, []byte("urn:oasis:names:tc:opendocument:xmlns:office:1.0")) &&
		bytes.Contains(head, []byte("office:document")):
		return Kind{LibODF, ""}
	case isXML(head):
		return Kind{LibXML, ""}
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
// a message: only lines "Name: value" (and folded lines) up to the blank
// line, at least two of them headers messages have, one of those a
// header only mail carries.
func messageHeaders(head []byte) bool {
	known, strong, lines := 0, 0, 0
	cut := len(head) == sniffBytes // the input goes on past what is looked at
	for len(head) > 0 && lines < 32 {
		line, rest, ok := bytes.Cut(head, []byte("\n"))
		if !ok && cut {
			break // a line cut off by the end of what is looked at
		}
		head = rest
		line = bytes.TrimRight(line, "\r")
		if len(line) == 0 {
			break
		}
		if line[0] == ' ' || line[0] == '\t' {
			if lines == 0 {
				return false
			}
			continue // a folded line
		}
		name, _, ok := bytes.Cut(line, []byte(":"))
		if !ok || len(name) == 0 || bytes.ContainsAny(name, " \t") {
			return false
		}
		lines++
		switch strings.ToLower(string(name)) {
		case "received", "return-path", "from", "message-id", "mime-version", "delivered-to":
			known++
			strong++
		case "to", "subject", "date", "cc", "reply-to", "content-type", "sender", "x-mailer", "in-reply-to", "references":
			known++
		}
	}
	return known >= 2 && strong >= 1
}

// mboxLine reports whether an input begins with a mailbox's separator
// line: "From ", an address, then a date with a time ("From a@b Mon Jan  1
// 00:00:00 2024").
func mboxLine(head []byte) bool {
	line, _, _ := bytes.Cut(head, []byte("\n"))
	rest, ok := bytes.CutPrefix(line, []byte("From "))
	if !ok {
		return false
	}
	fields := bytes.Fields(rest)
	if len(fields) < 3 {
		return false
	}
	for _, f := range fields[1:] {
		if len(f) == 8 && f[2] == ':' && f[5] == ':' {
			return true
		}
	}
	return false
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
