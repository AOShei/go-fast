package gofast

import (
	"bytes"
	"io"
	"strings"
)

// isXML reports whether the start of an input is XML for go-fast-xml: it
// begins with an XML declaration, has a document type declaration that
// names a root element other than html, or its first element declares a
// namespace. A first element named html or svg, or in the XHTML or SVG
// namespace, is go-fast-html's: XHTML and SVG are read there. head is
// the start of the input; a UTF-16 byte order mark is followed.
func isXML(head []byte) bool {
	head = xmlText(head, false)
	declared := len(head) > 5 && bytes.HasPrefix(head, []byte("<?xml")) && strings.IndexByte(" \t\r\n?", head[5]) >= 0
	e := firstElement(head)
	if d := strings.ToLower(e.doctype); d == "html" || d == "svg" {
		return false
	}
	if !e.found {
		return declared || e.doctype != ""
	}
	if local := strings.ToLower(e.name[strings.IndexByte(e.name, ':')+1:]); local == "html" || local == "svg" {
		return false
	}
	namespace := xmlns(e.attrs)
	if namespace == "http://www.w3.org/1999/xhtml" || namespace == "http://www.w3.org/2000/svg" {
		return false
	}
	return declared || e.doctype != "" || namespace != ""
}

// The roots of the Office documents saved as one XML file, which
// go-fast-ooxml reads: a local name in a namespace.
var officeRoots = map[[2]string]bool{
	{"package", "http://schemas.microsoft.com/office/2006/xmlPackage"}:       true, // Flat OPC
	{"wordDocument", "http://schemas.microsoft.com/office/word/2003/wordml"}: true, // WordprocessingML 2003
	{"Workbook", "urn:schemas-microsoft-com:office:spreadsheet"}:             true, // SpreadsheetML 2003
}

// officeHead is how far into an input officeXML looks for the root, as
// far as go-fast-ooxml does.
const officeHead = 64 << 10

// officeXML reports whether an input is an Office document saved as one
// XML file: its root element is Flat OPC's pkg:package, Word 2003's
// w:wordDocument or Excel 2003's Workbook, in their namespaces, with at
// most one document type declaration before it, which declares nothing
// (no internal subset), as go-fast-ooxml reads them. The first sniffBytes are looked
// at, and, when the root is not reached within them, up to officeHead.
// The markup is read in any encoding that writes ASCII as single bytes
// (UTF-8, ISO 8859, the Windows code pages, Shift JIS, GBK, EUC-KR,
// Big5) and in UTF-16, with or without a byte order mark.
func officeXML(r io.ReaderAt, size int64) bool {
	if r == nil || size <= 0 {
		return false
	}
	head := make([]byte, min(size, sniffBytes))
	n, _ := r.ReadAt(head, 0)
	ok, more := officeRoot(head[:n])
	if more && n == len(head) && int64(n) < min(size, officeHead) {
		head = make([]byte, min(size, officeHead))
		n, _ = r.ReadAt(head, 0)
		ok, _ = officeRoot(head[:n])
	}
	return ok
}

// officeRoot says whether head is the start of an Office document saved
// as XML; more is true when head ends before the root element's tag
// does, so that more of the input could decide.
func officeRoot(head []byte) (ok, more bool) {
	e := firstElement(xmlText(head, true))
	if e.subset || e.dtds > 1 {
		// go-fast-ooxml reads a document type declaration only as
		// go-fast-core's xmlscan accepts one: before the root, the only
		// one, declaring nothing.
		return false, false
	}
	if !e.found || !e.closed {
		return false, e.cut
	}
	prefix, local, found := strings.Cut(e.name, ":")
	if !found {
		prefix, local = "", e.name
	}
	return officeRoots[[2]string{local, namespaceOf(e.attrs, prefix)}], false
}

// xmlText turns the start of an XML input into bytes, a character a
// byte, so that its markup can be looked at: past a UTF-8 byte order
// mark, or narrowed from UTF-16 when a byte order mark says so (or,
// with bomless, when it begins "<" as UTF-16), and past white space.
func xmlText(head []byte, bomless bool) []byte {
	switch {
	case bytes.HasPrefix(head, []byte{0xFF, 0xFE}):
		head = narrow(head[2:], 0)
	case bytes.HasPrefix(head, []byte{0xFE, 0xFF}):
		head = narrow(head[2:], 1)
	case bomless && len(head) > 1 && head[0] == '<' && head[1] == 0:
		head = narrow(head, 0)
	case bomless && len(head) > 1 && head[0] == 0 && head[1] == '<':
		head = narrow(head, 1)
	default:
		head = bytes.TrimPrefix(head, []byte("\xEF\xBB\xBF"))
	}
	return bytes.TrimLeft(head, " \t\r\n")
}

// narrow turns UTF-16 text into bytes, a character a byte, so that its
// markup can be looked at: a character beyond ASCII becomes 0x80. lo is
// the place of the low byte in each pair (0 little-endian, 1 big-endian).
func narrow(b []byte, lo int) []byte {
	out := make([]byte, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		c, other := b[i+lo], b[i+1-lo]
		if other != 0 || c >= 0x80 {
			c = 0x80
		}
		out = append(out, c)
	}
	return out
}

// element is what firstElement finds.
type element struct {
	name    string // the first element's name, as written
	attrs   string // what follows the name in its start tag
	doctype string // the root element a document type declaration names
	found   bool   // an element begins in head, before any text
	closed  bool   // its start tag ends in head
	dtd     bool   // a document type declaration comes first
	dtds    int    // the document type declarations before the element
	subset  bool   // one of them has an internal subset
	cut     bool   // head ends before an element, or its start tag, ends
}

// firstElement finds the first element of the start of an XML or HTML
// input, past processing instructions, comments and a document type
// declaration. found is false when there is text first, or no element
// begins in head.
func firstElement(head []byte) (e element) {
	for {
		head = bytes.TrimLeft(head, " \t\r\n")
		switch {
		case len(head) == 0:
			e.cut = true
			return e
		case bytes.HasPrefix(head, []byte("<?")):
			i := bytes.Index(head, []byte("?>"))
			if i < 0 {
				e.cut = true
				return e
			}
			head = head[i+2:]
		case bytes.HasPrefix(head, []byte("<!--")):
			i := bytes.Index(head[4:], []byte("-->"))
			if i < 0 {
				e.cut = true
				return e
			}
			head = head[4+i+3:]
		case bytes.HasPrefix(head, []byte("<!")):
			// A document type declaration, with any internal subset.
			e.dtd = true
			e.dtds++
			if f := bytes.Fields(head[2:min(len(head), 256)]); len(f) > 1 && strings.EqualFold(string(f[0]), "DOCTYPE") {
				e.doctype = string(f[1])
				if i := strings.IndexAny(e.doctype, ">["); i >= 0 {
					e.doctype = e.doctype[:i]
				}
			}
			// Quoted strings (a public or system identifier, an entity's
			// value) can hold brackets and '>'.
			depth, i, quote := 0, 2, byte(0)
		scan:
			for ; i < len(head); i++ {
				switch c := head[i]; {
				case quote != 0:
					if c == quote {
						quote = 0
					}
				case c == '"' || c == '\'':
					quote = c
				case c == '[':
					depth++
					e.subset = true
				case c == ']':
					depth--
				case c == '>' && depth <= 0:
					break scan
				}
			}
			if i >= len(head) {
				e.cut = true
				return e
			}
			head = head[i+1:]
		case len(head) > 1 && head[0] == '<' && isNameStart(head[1]):
			end := bytes.IndexByte(head, '>')
			e.found, e.closed = true, end >= 0
			if end < 0 {
				end = len(head) // the tag runs past what is looked at
				e.cut = true
			}
			tag := string(head[1:end])
			n := strings.IndexAny(tag, " \t\r\n/")
			if n < 0 {
				n = len(tag)
			}
			e.name, e.attrs = tag[:n], tag[n:]
			return e
		case len(head) == 1 && head[0] == '<':
			e.cut = true
			return e
		default:
			return e
		}
	}
}

func isNameStart(c byte) bool {
	return c == '_' || c == ':' || c >= 0x80 || (c|0x20 >= 'a' && c|0x20 <= 'z')
}

// xmlns returns the value of an xmlns attribute among a tag's attributes,
// "-" when there is only an xmlns:prefix one, and "" when there is none.
func xmlns(attrs string) string {
	found := ""
	for {
		i := strings.Index(attrs, "xmlns")
		if i < 0 {
			return found
		}
		if i > 0 && strings.IndexByte(" \t\r\n", attrs[i-1]) < 0 {
			attrs = attrs[i+5:]
			continue
		}
		rest := attrs[i+5:]
		if strings.HasPrefix(rest, ":") {
			found = "-"
			attrs = rest
			continue
		}
		rest = strings.TrimLeft(rest, " \t\r\n")
		if !strings.HasPrefix(rest, "=") {
			attrs = rest
			continue
		}
		rest = strings.TrimLeft(rest[1:], " \t\r\n")
		if rest == "" {
			return "-"
		}
		q := rest[0]
		if q != '"' && q != '\'' {
			return "-"
		}
		v, _, _ := strings.Cut(rest[1:], string(q))
		return strings.TrimSpace(v) // xmlns="" declares none
	}
}

// namespaceOf returns the namespace a start tag's attributes declare for
// a prefix: the value of xmlns:prefix, or of xmlns when prefix is empty;
// "" when they declare none.
func namespaceOf(attrs, prefix string) string {
	want := "xmlns"
	if prefix != "" {
		want += ":" + prefix
	}
	for {
		attrs = strings.TrimLeft(attrs, " \t\r\n")
		eq := strings.IndexByte(attrs, '=')
		if eq < 0 {
			return ""
		}
		name := strings.TrimSpace(attrs[:eq])
		rest := strings.TrimLeft(attrs[eq+1:], " \t\r\n")
		if rest == "" || rest[0] != '"' && rest[0] != '\'' {
			return ""
		}
		value, after, ok := strings.Cut(rest[1:], rest[:1])
		if !ok {
			return ""
		}
		if name == want {
			return value
		}
		attrs = after
	}
}
