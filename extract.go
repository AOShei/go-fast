package gofast

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"strings"

	gofastcore "github.com/AOShei/go-fast-core"
	gofastemail "github.com/AOShei/go-fast-email"
	gofasthtml "github.com/AOShei/go-fast-html"
	gofastodf "github.com/AOShei/go-fast-odf"
	gofastooxml "github.com/AOShei/go-fast-ooxml"
	gofastpdf "github.com/AOShei/go-fast-pdf"
	gofastrtf "github.com/AOShei/go-fast-rtf"
)

// Extractor reads any format of the go-fast libraries, and archives. The
// zero value is ready to use and safe for concurrent use.
type Extractor struct {
	// MaxMemberBytes bounds one archive member decompressed. Zero means
	// DefaultMaxMemberBytes; negative, no bound.
	MaxMemberBytes int64
	// MaxArchiveBytes bounds the bytes decompressed from one archive and
	// the archives inside it. Zero means DefaultMaxArchiveBytes;
	// negative, no bound.
	MaxArchiveBytes int64
	// MaxArchiveMembers bounds the members read of an archive that is
	// read as one document (Extract, or an archive embedded in a
	// document). Zero means DefaultMaxArchiveMembers; negative, no bound.
	MaxArchiveMembers int
}

// Formats lists the formats Extract reads: those of every library, and
// the archives.
func (e *Extractor) Formats() []string {
	var out []string
	out = append(out, (&gofastpdf.Extractor{}).Formats()...)
	out = append(out, (&gofastooxml.Extractor{}).Formats()...)
	out = append(out, (&gofastodf.Extractor{}).Formats()...)
	out = append(out, (&gofastrtf.Extractor{}).Formats()...)
	out = append(out, (&gofastemail.Extractor{}).Formats()...)
	out = append(out, (&gofasthtml.Extractor{}).Formats()...)
	return append(out, "zip", "tar", "gzip")
}

var _ gofastcore.Extractor = (*Extractor)(nil)

// Extract reads an input of any format, decided by Detect from
// Options.Name and the bytes. When Options.Child is nil, embedded
// documents are read by e.Child, so a PDF attached to a message or a
// workbook inside a presentation is read too. An archive is a document of
// format zip, tar or gzip with one empty body part (with part.no_text, at
// info), whose children are its members.
// An input no library reads gives an error wrapping
// gofastcore.ErrUnsupported.
func (e *Extractor) Extract(ctx context.Context, r io.ReaderAt, size int64, opts *gofastcore.Options) (*gofastcore.Document, error) {
	o := gofastcore.Options{}
	if opts != nil {
		o = *opts
	}
	if o.Child == nil {
		o.Child = e.Child
	}
	return e.ExtractKind(ctx, Detect(o.Name, "", r, size), "", r, size, &o)
}

// ExtractKind reads an input whose Kind is decided, as Detect gives it.
// mimeType is the media type the input is declared with, if any: a
// charset parameter is the encoding of HTML, Markdown, text, CSV and TSV,
// and header=present makes the first row of CSV and TSV a header row.
func (e *Extractor) ExtractKind(ctx context.Context, k Kind, mimeType string, r io.ReaderAt, size int64, opts *gofastcore.Options) (*gofastcore.Document, error) {
	switch k.Library {
	case LibPDF:
		return (&gofastpdf.Extractor{}).Extract(ctx, r, size, opts)
	case LibOOXML:
		return (&gofastooxml.Extractor{}).Extract(ctx, r, size, opts)
	case LibODF:
		return (&gofastodf.Extractor{}).Extract(ctx, r, size, opts)
	case LibRTF:
		return (&gofastrtf.Extractor{}).Extract(ctx, r, size, opts)
	case LibEmail:
		doc, err := (&gofastemail.Extractor{}).Extract(ctx, r, size, opts)
		if err != nil && opts != nil && isCompoundFile(r) && !strings.EqualFold(pathExt(opts.Name), ".msg") &&
			(errors.Is(err, gofastcore.ErrMalformed) || errors.Is(err, gofastcore.ErrUnsupported)) {
			return nil, fmt.Errorf("gofast: an OLE compound file that is not an Outlook message (Office 97-2003 files are not read yet): %w", gofastcore.ErrUnsupported)
		}
		return doc, err
	case LibHTML:
		x := &gofasthtml.Extractor{Format: k.Format, Charset: param(mimeType, "charset")}
		x.Header = strings.EqualFold(param(mimeType, "header"), "present")
		return x.Extract(ctx, r, size, opts)
	case LibArchive:
		return e.archiveDocument(ctx, k.Format, r, size, opts)
	}
	name := ""
	if opts != nil {
		name = opts.Name
	}
	if legacyOffice[strings.ToLower(pathExt(name))] {
		return nil, fmt.Errorf("gofast: %s: Office 97-2003 files are not read yet: %w", name, gofastcore.ErrUnsupported)
	}
	return nil, fmt.Errorf("gofast: no library reads %q: %w", name, gofastcore.ErrUnsupported)
}

// Child is a gofastcore.ChildFunc that reads an embedded document of any
// format: the extractor every library's Options.Child can be set to.
func (e *Extractor) Child(ctx context.Context, name, mimeType string, r io.ReaderAt, size int64, opts *gofastcore.Options) (*gofastcore.Document, error) {
	k := Detect(name, mimeType, r, size)
	if k.Library == "" {
		return nil, fmt.Errorf("gofast: no library reads %q (%s): %w", name, mimeType, gofastcore.ErrUnsupported)
	}
	return e.ExtractKind(ctx, k, mimeType, r, size, opts)
}

// archiveDocument reads an archive as one document: no parts, and a child
// for each member, read through opts.ExtractChild (so core's depth limit
// bounds nested archives). Members left out are listed as children with
// a warning saying why, or counted in a warning on the document.
func (e *Extractor) archiveDocument(ctx context.Context, format string, r io.ReaderAt, size int64, opts *gofastcore.Options) (*gofastcore.Document, error) {
	if opts == nil {
		opts = &gofastcore.Options{}
	}
	ctx, decode := gofastcore.WithDecodeBudget(ctx, opts.Limits, gofastcore.ResolveLimit(e.MaxArchiveBytes, DefaultMaxArchiveBytes))
	defer decode.Close()
	doc := gofastcore.New(format)
	doc.Source = gofastcore.Source{Format: format, Name: opts.Name, MIME: archiveMIME[format], Size: size}
	maxMembers := int(gofastcore.ResolveLimit(int64(e.MaxArchiveMembers), DefaultMaxArchiveMembers))
	w := &archiveWalker{ctx: ctx, maxMember: gofastcore.ResolveLimit(e.MaxMemberBytes, DefaultMaxMemberBytes),
		left: max(decode.Left(), 0)}
	members, over := 0, 0
	start := w.left
	err := w.walk(format, r, size, opts.Name, func(m Member) error {
		if members == maxMembers {
			over++
			return nil
		}
		members++
		doc.Children = append(doc.Children, opts.ExtractChild(ctx, m.Path, "", m.R, m.Size))
		if m.Release != nil {
			m.Release()
		}
		return nil
	}, func(s Skipped) {
		child := gofastcore.New("")
		child.Source = gofastcore.Source{Name: s.Path}
		sev, code := gofastcore.SeverityWarning, gofastcore.WarnChildNotExtracted
		if s.Limit {
			sev, code = gofastcore.SeverityError, gofastcore.WarnLimitExceeded
		}
		child.Warn(sev, code, s.Reason)
		doc.Children = append(doc.Children, child)
	})
	decode.Take(start - max(w.left, 0))
	switch {
	case errors.Is(err, errBudget):
		doc.Warn(gofastcore.SeverityError, gofastcore.WarnLimitExceeded,
			"the limit on decoded bytes was reached; the rest of the archive was not read")
	case errors.Is(err, ErrArchive) && len(doc.Children) == 0:
		return nil, fmt.Errorf("%v: %w", err, gofastcore.ErrMalformed)
	case errors.Is(err, ErrArchive):
		doc.Warn(gofastcore.SeverityError, gofastcore.WarnInputRepaired, err.Error()+"; the members before the damage were read")
	case err != nil:
		return nil, err
	}
	if over > 0 {
		doc.Warn(gofastcore.SeverityError, gofastcore.WarnLimitExceeded,
			fmt.Sprintf("members past %d were not read (%d members)", maxMembers, over))
	}
	// An archive has no text of its own: one empty body part says so,
	// and its members are its children.
	part := gofastcore.Part{Kind: gofastcore.PartBody, Number: 1}
	part.Warn(gofastcore.SeverityInfo, gofastcore.WarnPartNoText,
		fmt.Sprintf("an archive has no text of its own; its %d members are its children", len(doc.Children)))
	part.Finish()
	doc.Parts = append(doc.Parts, part)
	return doc, nil
}

var archiveMIME = map[string]string{"zip": "application/zip", "tar": "application/x-tar", "gzip": "application/gzip"}

// isCompoundFile reports whether r begins with an OLE compound file's
// signature.
func isCompoundFile(r io.ReaderAt) bool {
	b := make([]byte, 8)
	n, _ := r.ReadAt(b, 0)
	return n == 8 && string(b) == "\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1"
}

// param is a parameter of a media type ("" if none).
func param(v, name string) string {
	if v == "" {
		return ""
	}
	if _, params, err := mime.ParseMediaType(v); err == nil {
		return params[name]
	}
	return ""
}

// pathExt is the extension of a name with / or \ separators.
func pathExt(name string) string {
	i := strings.LastIndexAny(name, `/\.`)
	if i < 0 || name[i] != '.' {
		return ""
	}
	return name[i:]
}
