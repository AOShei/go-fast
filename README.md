# go-fast

The front door of the go-fast libraries. It reads a file of any of their formats by sending it to the library that reads it, opens zip, tar and gzip archives, and reads many files in parallel. Everything comes out in the shared document model, `gofastcore.Document` (`github.com/AOShei/go-fast-core`).

**Status: v0.1.1 (2026-11-24).** The changes are in [`CHANGELOG.md`](CHANGELOG.md).

```
go install github.com/AOShei/go-fast/cmd/gofast@latest
gofast -dir out/ ~/documents/            # one JSON file per document, in a tree like the input's
gofast reports.zip > reports.jsonl       # each file in the archive is one JSON line
gofast -format markdown notes.docx       # one document as Markdown
```

## What is read, and by which library

| Formats | Library |
|---|---|
| PDF | [go-fast-pdf](https://github.com/AOShei/go-fast-pdf) |
| Word, Excel and PowerPoint 2007 and later (`.docx`, `.xlsx`, `.xlsb`, `.pptx` and their macro and template variants), and Office documents saved as one XML file: Flat OPC, Word 2003's XML document and Excel 2003's XML spreadsheet | [go-fast-ooxml](https://github.com/AOShei/go-fast-ooxml) |
| Word, Excel and PowerPoint 97-2003 (`.doc`, `.xls`, `.ppt` and their templates and shows) | [go-fast-msbin](https://github.com/AOShei/go-fast-msbin) |
| OpenDocument text, spreadsheets and presentations, and their flat XML forms | [go-fast-odf](https://github.com/AOShei/go-fast-odf) |
| RTF | [go-fast-rtf](https://github.com/AOShei/go-fast-rtf) |
| Email: `.eml`, `.mbox`, Outlook `.msg`, TNEF (`winmail.dat`) | [go-fast-email](https://github.com/AOShei/go-fast-email) |
| HTML, Markdown, plain text, EPUB, CSV, TSV, Jupyter notebooks, JSON and YAML (`.json`, `.jsonl`, `.ndjson`, `.geojson`, `.yaml`, `.yml`; as text, never by content) | [go-fast-html](https://github.com/AOShei/go-fast-html) |
| XML of any schema (`.xml`, `.dita`, `.ditamap`, `.docbook`, `.tei`), with built-in profiles for Akoma Ntoso, HL7 CDA, CLML, DITA, DocBook, Formex, JATS, Légifrance, patents, S1000D, Service-Public and TEI | [go-fast-xml](https://github.com/AOShei/go-fast-xml) |
| zip, tar, gzip (`.tar.gz`, `.tgz`, `.gz`) | this module |

**How the format is decided** (`Detect`):
1. A declared media type, for an embedded document (an attachment's `Content-Type`). The types mail programs give files they don't know better (`application/octet-stream`, `application/zip`, `text/plain`) yield to a known extension: a `.docx` sent as `application/zip` is still Word. A file declared `text/plain` with no such extension is text. `application/xml`, `text/xml` and any other `+xml` type are XML, but for `application/xhtml+xml` and `image/svg+xml`, which go-fast-html reads.
2. The extension of the name.
3. The bytes:
   - `%PDF-1.` or `%PDF-2.` in the first 1 KiB is PDF.
   - A zip archive is Office if it has `[Content_Types].xml`, OpenDocument or EPUB if its `mimetype` member says so, and otherwise an archive.
   - gzip and tar are archives.
   - `{\rtf` is RTF.
   - An OLE compound file is Word, Excel or PowerPoint 97-2003 (go-fast-msbin) when its directory holds those formats' streams (go-fast-msbin's `Detect`), and otherwise email.
   - A TNEF stream, a mailbox's `From ` line (an address, then a date with a time) or a message's header lines (at least two of the headers messages have, one of them `From`, `Received`, `Return-Path`, `Message-ID`, `MIME-Version` or `Delivered-To`) are email.
   - An OpenDocument flat XML file is OpenDocument.
   - XML is go-fast-xml's: input that begins with an XML declaration (`<?xml`), that has a document type declaration naming a root element other than `html`, or whose first element declares a namespace. A first element named `html` or `svg`, or in the XHTML or SVG namespace, stays with go-fast-html. XML with none of these marks (no declaration, no document type, no namespace) is read as HTML, unless its name or media type says XML.
   - Binary input (images, audio, executables) is read by none.
   - Anything else is what go-fast-html makes of it: HTML when it is markup, else plain text.

**Office documents saved as XML** go to go-fast-ooxml, whichever of the steps above sent them to go-fast-xml (a `.xml` name, an XML media type, or the bytes). They are known by the root element and its namespace, within the first 64 KiB, as go-fast-ooxml knows them: Flat OPC's `pkg:package` (read as `docx`, `xlsx` or `pptx`, with `extra.packaging` `flat_opc`), Word 2003's `w:wordDocument` (format `wordml`) and Excel 2003's `Workbook` (format `spreadsheetml`). XML with a document type declaration stays with go-fast-xml. The root is found in UTF-8, UTF-16 and the single- and double-byte sets Office writes (the Windows code pages, ISO 8859, Shift_JIS, GBK, EUC-KR, Big5). The media types `application/vnd.ms-wordml` and `application/vnd.ms-spreadsheetml` go to go-fast-ooxml too.

**Office 97-2003 files** are decided by their bytes even when their name (`.doc`, `.dot`, `.xls`, `.xlt`, `.xla`, `.ppt`, `.pps`, `.pot`) or media type (`application/msword`, `application/vnd.ms-excel`, `application/vnd.ms-powerpoint`) says what they are. A compound file goes to go-fast-msbin when its directory holds a Word, Excel or PowerPoint file. Other bytes go where they say: RTF, HTML or XML saved as `.doc` or `.xls`, a zip named `.xls`. Bytes no library reads (an Excel 2 to 4 workbook, a Word 2 document) go to go-fast-msbin, which says what the file is. An Outlook message named `.doc` is still a message. A compound file named as Office that go-fast-msbin's `Detect` does not name (its directory is damaged) and go-fast-email does not read gets go-fast-msbin's error.

**Text that declares no character set** and is not UTF-8 is read in the set its bytes are certainly in (Shift_JIS, GBK, Big5, EUC-KR, windows-1251, windows-1252, UTF-16 without a byte order mark), with the warning `text.encoding_detected`, by go-fast-html and go-fast-email; go-fast-xml also recognises an encoding from the bytes, with the same warning. Where no set is certain, a text file is refused (`gofasthtml.ErrEncodingUnknown`, which wraps `ErrUnsupported`), XML is refused (`ErrUnsupported`) and a message's text keeps U+FFFD; `-charset` (`Extractor.Charset`) gives the set to use then. It never overrides a set the input declares: a `<meta>`, an XML declaration's `encoding` or a `charset` parameter wins.

**Embedded documents** are read by whichever library reads their format. A PDF attached to a message, a workbook embedded in a presentation, and a zip attached to a message (whose members become the zip's children) all come with the document that holds them. Core's depth limit (`-max-depth`, default 4) bounds how deeply they nest.

## Archives

The command reads **each file of an archive as a document of its own**. Its `Source.Name` is the archive's name and the path inside it: `data.zip/reports/q3.pdf`. Archives inside archives are opened too, down to `-max-depth`.

A file read from an archive gives the same document as the file read on its own, except for its name. Over 449 files of every format, packed as zip (stored and deflated), tar and tar.gz, the documents were identical.

**Members that are not read** are reported on standard error with the reason, and the exit status is then 1:
- an encrypted zip member;
- a sparse tar member (its bytes in the archive are not its content);
- a zip member said to be larger than its compressed bytes allow (deflate makes at most 1,032 bytes of one);
- a compression method Go does not decode (only stored and deflated are read);
- a name that is not a path inside the archive (an element `..`);
- a member past one of the bounds below;
- an archive nested past `-max-depth`.

A tar hard link to a member before it is another name for that member, read under its own name: it is noted on standard error, not counted as a failure. A hard link to a file not in the archive is a member not read. Directories, symbolic links and devices are skipped silently.

A member's name is cleaned: no leading `/` or drive letter, and no `.` element or control character.

**Bounds**, against archives built to exhaust memory or time:

| Bound | Flag | Default |
|---|---|---|
| One member, decompressed (held in memory to be read; a member stored uncompressed in a zip or plain tar is read in place) | `-max-member-bytes` | 1 GiB |
| Everything decompressed from one archive and the archives in it | `-max-archive-bytes` | 64 GiB |
| Decompressed members held in memory at once by all the workers | `-max-memory-bytes` | 4 GiB (a larger member is still read, alone; an archive inside an archive stays held until its members are read; so does a `.gz` or `.tar.gz` file, below) |
| Members of an archive read as one document (`Extractor.Extract`, or an archive embedded in a document) | `Extractor.MaxArchiveMembers` | 10,000 |

A `.gz` or `.tar.gz` file of up to 1 GiB is read into memory and held while it is walked, counted under `-max-memory-bytes`: go-fast-core's `inflate` decompresses it about twice as fast as `compress/gzip`, which reads a larger file a piece at a time. The 1 GiB trades memory for speed: a 187 MB `.tar.gz` of 2,457 ODT files is walked in 0.46 s instead of 1.0 s, with a peak of 430 MB instead of 72 MB (the file, and the garbage collector's headroom over it). Zip members are decompressed with `inflate` too, a piece at a time, whatever their size.

A zip cut short, which has lost the directory at its end, is read by its members' own headers (go-fast-core's `ziprecover`, as every go-fast library reads one). The members found are read; the one the file ends inside is reported as not read, and the archive is then reported damaged, saying how many members were found. Finding where members end is bounded by `-max-archive-bytes`.

A zip whose members overlap is refused: no real archive shares bytes between members, and it is how the zip bombs that reach petabytes are built.

As one document (`Extractor.Extract`, or embedded in a document), an archive has format `zip`, `tar` or `gzip`. It has one empty `body` part, with `part.no_text` at info, and its members are its children.

## The command

```
gofast [flags] file-or-directory...
```

**Inputs.**
- A directory stands for the files in it and below it whose extension is one of the formats above or an archive's. Office lock files (`~$...`) are skipped.
- A file named on the command line is read whatever its name, its format decided from its bytes.
- `-` is standard input. Standard input, a pipe or a device named as a file is read into memory, up to `-max-input-bytes` (1 GiB by default).

**Output.**
- With one input that is not an archive, the JSON is indented.
- Otherwise each document is one line (JSON Lines), **in the order of the inputs and of the members of each archive**, whatever order the workers finish in.
- `-dir DIR` writes each document to a file of its own instead, in a tree that mirrors the inputs: `DIR/sub/a.pdf.json`, or `DIR/data.zip/reports/q3.pdf.json` for an archive member. Nothing is written outside `DIR`, whatever a member's name says. Names are made ones every system can create: characters Windows forbids become `_`, a trailing dot or space is replaced, and a device name such as `CON` or `NUL` gets a leading `_`. Where the file system ignores case (Windows, macOS), a document whose file would differ from another's only in case is written with `~2` (`~3`, ...) before its extension, and noted on standard error.
- Interrupting the run (Ctrl-C) stops it; a second interrupt ends it at once.
- Only documents go to standard output. Errors, and a note for each document with an error-level warning, go to standard error, with a summary line at the end. The exit status is 1 if any input or member could not be read, and 2 for a usage error.

| Flag | |
|---|---|
| `-format json\|markdown\|text` | what to print (default `json`) |
| `-o FILE` | write to a file instead of standard output |
| `-dir DIR` | one file per document, in a tree under `DIR` |
| `-j N` | files read at once (default: the number of CPUs). With more than one, a library that would also read one file's pages in parallel reads them one at a time |
| `-images none\|meta\|data` | images: none, a block for each (default), or with their bytes |
| `-spans` | inline styling, links, and declared display of ranges of text |
| `-cells` | the cells of spreadsheets with their types and formulas |
| `-password` | for encrypted files |
| `-charset LABEL` | the character set (`shift_jis`, `gbk`, `windows-1250`, ...) of text that declares none and whose set is not certain from its bytes; without it such a text file is not read, and text in a message has U+FFFD |
| `-xml-profile FILE` | a go-fast-xml profile file (see its `PROFILE_FORMAT.md`), tried before the built-in profiles and chosen, as they are, by the root element, so it is safe in a batch of any formats; may be given more than once (`Extractor.XMLProfiles`) |
| `-xml-attributes=false` | in XML no profile fits, leave out the note go-fast-xml makes of each attribute, for data-style XML (`Extractor.XMLAttributes`) |
| `-max-input-bytes`, `-max-decoded-bytes`, `-max-text-bytes`, `-max-depth` | core's limits (`gofastcore.Limits`), for every document; `-max-input-bytes` also bounds standard input and pipes |
| `-max-member-bytes`, `-max-archive-bytes`, `-max-memory-bytes` | the archive bounds above |
| `-quiet` | say nothing about files that were read |
| `-version` | print the version |
| `-furniture`, `-stored[=kinds]`, `-part-markers`, `-exclude-display[=reasons]`, `-children` | the render flags every go-fast command shares; they affect Markdown and text only |

## The package

```go
e := &gofast.Extractor{}
doc, err := e.Extract(ctx, r, size, &gofastcore.Options{Name: "report.pdf"})
```

- `Detect(name, mimeType, r, size)` says where an input goes (a `Kind`: a library and a format).
- `Extractor.Extract` reads any input; when `Options.Child` is unset, embedded documents are read by `Extractor.Child`.
- `Extractor.Child` is a `gofastcore.ChildFunc` that any library's `Options.Child` can be set to.
- `Extractor.Walk` calls a function for each file of an archive, with the bounds above, for a program that reads members as documents of their own, as the command does.

The go-fast libraries never import each other; this module imports them all. It requires each at a tagged release.

## Not read yet

- Objects embedded through OLE: an Office 2007 file wrapped in an OLE object (as RTF and older Office files embed one), and a PDF wrapped in one. They are listed as unsupported.
- Images, audio and video. The series reads no text from pixels or sound.
- zip members compressed with methods other than stored and deflated, and encrypted zip members.

## Licence

MIT; see [`LICENSE`](LICENSE).
