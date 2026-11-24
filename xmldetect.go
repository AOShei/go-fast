package gofast

import (
	"bytes"
	"strings"
)

// isXML reports whether the start of an input is XML for go-fast-xml: it
// begins with an XML declaration, has a document type declaration that
// names a root element other than html, or its first element declares a
// namespace. A first element named html or svg, or in the XHTML or SVG
// namespace, is go-fast-html's: XHTML and SVG are read there. head is
// the start of the input; a UTF-16 byte order mark is followed.
func isXML(head []byte) bool {
	switch {
	case bytes.HasPrefix(head, []byte{0xFF, 0xFE}):
		head = narrow(head[2:], 0)
	case bytes.HasPrefix(head, []byte{0xFE, 0xFF}):
		head = narrow(head[2:], 1)
	default:
		head = bytes.TrimPrefix(head, []byte("\xEF\xBB\xBF"))
	}
	head = bytes.TrimLeft(head, " \t\r\n")
	declared := len(head) > 5 && bytes.HasPrefix(head, []byte("<?xml")) && strings.IndexByte(" \t\r\n?", head[5]) >= 0
	root, namespace, doctype, ok := firstElement(head)
	if d := strings.ToLower(doctype); d == "html" || d == "svg" {
		return false
	}
	if !ok {
		return declared || doctype != ""
	}
	if local := strings.ToLower(root[strings.IndexByte(root, ':')+1:]); local == "html" || local == "svg" {
		return false
	}
	if namespace == "http://www.w3.org/1999/xhtml" || namespace == "http://www.w3.org/2000/svg" {
		return false
	}
	return declared || doctype != "" || namespace != ""
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

// firstElement finds the first element of the start of an XML or HTML
// input, past processing instructions, comments and a document type
// declaration, and returns its name, the namespace it declares (the
// value of xmlns, or "-" when it declares only prefixed ones) and the
// root element the document type declaration names, if any. ok is false
// when there is text first, or no element begins in head.
func firstElement(head []byte) (name, namespace, doctype string, ok bool) {
	for {
		head = bytes.TrimLeft(head, " \t\r\n")
		switch {
		case bytes.HasPrefix(head, []byte("<?")):
			i := bytes.Index(head, []byte("?>"))
			if i < 0 {
				return "", "", doctype, false
			}
			head = head[i+2:]
		case bytes.HasPrefix(head, []byte("<!--")):
			i := bytes.Index(head[4:], []byte("-->"))
			if i < 0 {
				return "", "", doctype, false
			}
			head = head[4+i+3:]
		case bytes.HasPrefix(head, []byte("<!")):
			// A document type declaration, with any internal subset.
			if f := bytes.Fields(head[2:min(len(head), 256)]); len(f) > 1 && strings.EqualFold(string(f[0]), "DOCTYPE") {
				doctype = string(f[1])
				if i := strings.IndexAny(doctype, ">["); i >= 0 {
					doctype = doctype[:i]
				}
			}
			depth, i := 0, 2
			for ; i < len(head); i++ {
				if c := head[i]; c == '[' {
					depth++
				} else if c == ']' {
					depth--
				} else if c == '>' && depth <= 0 {
					break
				}
			}
			if i >= len(head) {
				return "", "", doctype, false
			}
			head = head[i+1:]
		case len(head) > 1 && head[0] == '<' && isNameStart(head[1]):
			end := bytes.IndexByte(head, '>')
			if end < 0 {
				end = len(head) // the tag runs past what is looked at
			}
			tag := string(head[1:end])
			n := strings.IndexAny(tag, " \t\r\n/")
			if n < 0 {
				n = len(tag)
			}
			name, attrs := tag[:n], tag[n:]
			return name, xmlns(attrs), doctype, true
		default:
			return "", "", doctype, false
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
