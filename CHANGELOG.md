# Changelog

## v0.1.13 (2026-11-28)

Pins go-fast-core v0.1.17 (after v0.1.15), go-fast-ooxml v0.7.3 (after
v0.7.1), go-fast-xml v0.5.3 (after v0.5.1), go-fast-pdf v0.6.2 (after
v0.6.0), go-fast-msbin v0.2.2 (after v0.2.1) and go-fast-html v0.7.3
(after v0.7.2). No routing changes.

- **Bytes that are not UTF-8 are checked once, by core's XML scanner,**
  and counted alike. go-fast-ooxml, go-fast-xml and go-fast-pdf report
  them as `text.unmapped_glyphs` ("N byte sequences are not valid UTF-8
  and are read as U+FFFD").
- **Encrypted packages are told by core's `envelope`**, in go-fast-ooxml,
  go-fast-pdf and go-fast-msbin. A rights-managed file known only by an
  `EUL-` licence is now refused as rights-managed.
- **go-fast-html's warning for bytes not valid in the encoding** says
  "are read as U+FFFD" (was "appear as"), in the singular for one.

## v0.1.12 (2026-11-27)

Pins go-fast-pdf v0.6.0 (after v0.5.0), go-fast-ooxml v0.7.1 (after
v0.7.0), go-fast-msbin v0.2.1 (after v0.2.0), go-fast-xml v0.5.1 (after
v0.5.0), go-fast-html v0.7.2 (after v0.7.1) and go-fast-core v0.1.15
(after v0.1.14).

- **XPS and OpenXPS are read, by go-fast-pdf.** By name (`.xps`,
  `.oxps`), by media type (`application/vnd.ms-xpsdocument`,
  `application/oxps`) or by the bytes: a zip archive whose package
  relationships lead to a fixed document sequence (type
  `.../fixedrepresentation`, in either namespace), as go-fast-pdf tells
  them. That test comes before the one for Office, since an XPS document
  has a content-types table as an Office file does; such files went to
  go-fast-ooxml, which refused them. An encrypted XPS document, a
  compound file, goes to go-fast-pdf by its name, which says what
  protects it; it went to go-fast-email, which refused it as an unknown
  compound file.
- **The `&#0;` warning everywhere.** ooxml v0.7.1 and xml v0.5.1 report
  character references to no character as `text.unmapped_glyphs`, as
  html does; xml's `input.repaired` message is as in v0.4.1 again. html
  v0.7.2 says such references "are read as U+FFFD", as ooxml does (xml
  says "appear as" until its next release).
- **Checked against v0.1.11 on the public corpus:** output changes in 14
  of 10,313 dev files and 3 of 3,414 held-out files. In dev, the 11 XPS
  files: 8 are read (the XPS specification, 453 pages, among them), where
  ooxml refused them, and 3 encrypted ones are refused by go-fast-pdf,
  which says what encrypts them; and three XML files whose
  `input.repaired` message is v0.4.1's again. In held-out (counted only),
  the 3 XPS files.

## v0.1.11 (2026-11-27)

Pins go-fast-core v0.1.14 (after v0.1.13) and go-fast-html v0.7.1
(after v0.7.0).

- **Office 2003 XML with a document type declaration goes to
  go-fast-ooxml.** A WordprocessingML 2003 or SpreadsheetML 2003 file, or
  Flat OPC, whose root comes after one document type declaration that
  declares nothing (`<!DOCTYPE Workbook>`, or one naming an external DTD)
  is read by go-fast-ooxml, which reads such files since v0.7.0; it went
  to go-fast-xml. A declaration with an internal subset, or a second one,
  still sends the file to go-fast-xml, since go-fast-ooxml refuses it.
  The scan of a declaration also skips quoted strings now, so a `>`
  or `[` inside an identifier or an entity's value is not taken for its
  end or a subset.
- **html v0.7.1:** a character reference that names no character
  (`&#0;`, a surrogate, a number past U+10FFFF) is counted in a
  `text.unmapped_glyphs` warning of its own.
- **core v0.1.14:** the XML scanner is faster in its default reading,
  which go-fast-ooxml and go-fast-odf use; a VBA project that names UTF-16
  as its code page is read as bytes again.
- **Checked against v0.1.10 on the public corpus:** every output is the
  same, in 10,313 dev files and 3,414 held-out files.

## v0.1.10 (2026-11-27)

Pins go-fast-ooxml v0.7.0 (after v0.6.0), go-fast-msbin v0.2.0 (after
v0.1.5) and go-fast-xml v0.5.0 (after v0.4.1).

- **All the libraries now read with go-fast-core's shared packages.**
  ooxml, msbin and xml dropped their own copies of the XML scanner, the
  character-set decoders, the Office package and compound-file readers
  and the Office helpers, as odf, html, rtf and email did in v0.1.9. Their
  CHANGELOGs have the details. What changes with it:
  - **ooxml:** some warning and error messages are reworded. A compound
    file embedded in a document and damaged is read past the damage and
    listed as unsupported, where it was listed as failed. Names are kept
    to 4 KiB. A SpreadsheetML 2003 file that starts with a document type
    declaration declaring nothing is still sent to go-fast-xml (the
    umbrella sends any such file there), so nothing changes for it here.
  - **msbin:** in Word 6/95 documents, a byte the code page leaves
    undefined in 0x80 to 0x9F is the C1 control (removed as text, counted
    as `text.control_characters_removed`), where it was U+FFFD counted as
    `text.unmapped_glyphs`; a lead byte before an ASCII letter keeps the
    letter; code page 950 decodes the HKSCS pairs. Names are kept to
    4 KiB.
  - **xml:** ISO-2022-JP, `cp936` and `latin9` are read. `&#0;` is
    U+FFFD and counted as a repair, and the `input.repaired` message
    names it.
- **Checked against v0.1.9 on the public corpus:** output changes in 3 of
  10,313 dev files and none of 3,414 held-out files. The three are XML
  (two XLIFF files and a feed) with a `&#0;`, whose `input.repaired`
  message now ends "a character reference to U+0000". The changes in
  Office and legacy Office output that the libraries measured are in
  their own test files, which this corpus does not hold.

## v0.1.9 (2026-11-27)

Pins go-fast-core v0.1.13 (after v0.1.11), go-fast-odf v0.3.1 (after
v0.3.0), go-fast-html v0.7.0 (after v0.6.0), go-fast-rtf v0.3.0 (after
v0.2.1), go-fast-email v0.4.0 (after v0.3.0) and go-fast-pdf v0.5.0
(after v0.4.0).

- **One copy of the shared code.** go-fast-core now holds the readers
  more than one library needs: character sets (`charset`), XML
  (`xmlscan`), Office packages (`opc`), compound files (`cfb`), MIME
  (`rfc822`), HTML character references (`entity`) and the text
  conventions and Office helpers. odf, html, rtf and email use them in
  these releases; ooxml, msbin, xml and pdf follow in theirs. New with
  them: html decodes ISO-2022-JP, email decodes KOI8, the ISO-8859 parts
  and Big5 with HKSCS, GBK byte 0x80 is the euro sign everywhere, and a
  byte a code page leaves undefined in 0x80 to 0x9F is the C1 control
  (removed as text), as the WHATWG Encoding Standard has it.
- **go-fast-pdf v0.5.0:** tables in files without tags, footnotes as
  blocks of kind footnote, paragraphs continued past them (its CHANGELOG
  has the measurements).
- **Checked against main on the public corpus:** output changes in 6 of
  10,313 dev files and 6 of 3,414 held-out files. In dev: three saved web
  pages whose subject, in ISO-2022-JP, is now decoded; a vCard whose value
  ends in a form feed, now a line break; an OpenDocument drawing that is
  refused either way, with its message reworded; a Factur-X invoice whose
  six short lines pdf v0.5.0 reads as paragraphs, not headings. In
  held-out (counted only): three saved web pages, one ics, one msg and
  one XML invoice.

## v0.1.8 (2026-11-27)

Pins go-fast-ooxml v0.6.0 (after v0.5.3).

- **Visio 2013 and later** go to go-fast-ooxml v0.6.0, which reads
  drawings, templates and stencils, with and without macros, under the
  format name `vsdx` (`Source.MIME` tells them apart): by name
  (`.vsdx`, `.vsdm`, `.vstx`, `.vstm`, `.vssx`, `.vssm`, in a directory
  walk too), by their six media types, and by their bytes: a zip with
  `[Content_Types].xml`, as before, or one whose package relationships
  (`_rels/.rels`) name a Visio document, which some programs write
  without `[Content_Types].xml`. The binary `.vsd` and the XML `.vdx`
  of Visio 2003-2010 are not read as Visio.
- **Checked against main on the public corpus:** output changes only
  in the Visio sets, 26 of 10,302 dev files and 12 of 3,411 held-out
  files. Before, 19 dev and 9 held-out Visio files were refused ("no
  Word, Excel or PowerPoint document"), and 7 dev and 3 held-out files
  were opened as zip archives, each member a document of its own (in
  dev: POI's 6 fuzzer cases, whose zip directory is damaged, so that
  detection by bytes cannot look for `[Content_Types].xml`, and a
  libvisio file without one). All 38 are now read as Visio, by their
  name.

## v0.1.7 (2026-11-26)

Pins go-fast-xml v0.4.1 (after v0.2.1) and go-fast-core v0.1.11 (after v0.1.9).

- **go-fast-core v0.1.11** (was v0.1.9). `gofast -stored=` also accepts
  the stored kinds `shape_data` and `linked_data` (core v0.1.10), which
  Visio drawings in go-fast-ooxml will use. Markdown and text join a
  continued paragraph to the one it continues when only footnotes stand
  between them, and write the footnotes after it (core v0.1.11): a
  paragraph cut by a page break, with the notes of the page's foot
  between, is one paragraph again.
  Against core v0.1.10 on the public corpus (13,713 files), the JSON,
  text and Markdown are unchanged: the released go-fast-pdf does not yet
  give footnotes between the pieces.
- **go-fast-xml v0.4.1** (was v0.2.1): 26 built-in profiles (were 13).
  New: electronic invoices (Cross Industry Invoice: Factur-X, ZUGFeRD 2,
  XRechnung; UBL: Peppol), DILA case law with KALI and CNIL, Canadian
  federal laws, translation files (TMX, XLIFF 1.x and 2.x), PubMed
  citations, news agency formats (NewsML-G2, NITF) and procurement
  notices (EU eForms, TED, BOAMP). Children: invoices and XLIFF files
  hand their base64 attachments to the library for their media type,
  NewsML-G2 an item's XHTML body and BOAMP a notice's HTML page to
  go-fast-html.
- **Translation files are routed to go-fast-xml by name**, in a
  directory walk too: `.xlf`, `.xliff`, `.tmx`, and the XLIFF 1.2 of
  Trados, Phrase and memoQ, `.sdlxliff`, `.mxliff`, `.mqxliff`. Not
  `.nitf` or `.ntf`, which binary imagery files also use: NITF stories
  are named `.xml`.
- **Factur-X and ZUGFeRD PDFs:** the invoice a PDF embeds reaches
  go-fast-xml and is read with the `cii` profile. Checked on 4 PDFs of
  the ZUGFeRD corpus (new dev set `pdf-facturx`); before, it was read as
  generic XML.
- **Checked against main (go-fast-xml v0.2.1) on the public corpus:**
  output changes in 4,640 of 10,302 dev files and, by hash, 1,574 of
  3,411 held-out files, all in the XML sets and the four Factur-X PDFs.
  The sets with a new profile change but for files still read
  generically or refused (dev: 18 of 223 invoices, 2 of 524 eForms
  notices, as go-fast-xml reports) and 63 of 1,436 DILA files already
  read with `legifrance`. Akoma Ntoso, CLML and JATS change in 14, 32
  and 1 dev files, where a section number written as its own paragraph
  now takes its own section's attributes (go-fast-xml v0.3.0). No other
  set changes.

## v0.1.6 (2026-11-26)

Pins go-fast-odf v0.3.0 (after v0.2.2).

- **OpenDocument drawings are read** (go-fast-odf v0.3.0): `.odg`, the
  template `.otg` and the flat `.fodg`, and the media types
  `application/vnd.oasis.opendocument.graphics` and `graphics-template`,
  are routed to go-fast-odf, by name in a directory walk too. A drawing
  is one part of kind `page` per page, its shapes in reading order, the
  text of its master page as header or footer. A drawing without a name
  already went to go-fast-odf, which refused it.
- **Old files are read without a false warning** (go-fast-odf v0.3.0): a
  document type declaration that declares nothing, as files of around
  2005 open their manifest and formulas with, is skipped, not refused.
- **Checked against v0.1.5 on the public corpus:** output changes in 66
  of 8,609 dev files: 50 of the 51 `.odg` (the other is a presentation
  saved as `.odg`, read as one before), all 15 `.fodg`, and one `.odt`
  that loses a false `input.stream_damaged`. By hash, 21 of 2,852
  held-out files change: all 16 `.odg` and 4 `.fodg`, and one `.odt`
  that gains an embedded formula. No other output changes.

## v0.1.5 (2026-11-26)

Pins go-fast-html v0.6.0 (after v0.5.0).

- **JSON and YAML are cut along their structure** (go-fast-html v0.6.0):
  blocks of whole values, 1 KiB by default, with their paths and the
  comments around them; a list of records cut across blocks (or at the
  top level) as a table, cells as written; JSON Lines a value a line;
  JSON with comments. `-data-block-bytes` (`Extractor.DataBlockBytes`)
  sets the block size.
- **Markdown front matter** (YAML or TOML) is the first block and fills
  the metadata (title, authors, description, tags, dates, every other key
  in `Extra["front_matter.<key>"]`, nulls kept as written).
- **More names are routed:** `.mdc` (Cursor rules), `.qmd` (Quarto) and
  `.rmd` (R Markdown) as Markdown; `.jsonc` and `.jsonlines` as JSON.
- **Checked against v0.1.4 on the public corpus:** output changes only in
  JSON, JSON Lines, YAML and Markdown with front matter: 178 of their 189
  dev files and 47 of their 51 held-out files (the dev files unchanged
  are one block either way, 385 bytes at most). No other output changes,
  in 8,354 dev files or, by hash, 2,781 held-out files.

## v0.1.4 (2026-11-26)

Pins go-fast-html v0.5.0 (after v0.4.4), go-fast-email v0.3.0 (after
v0.2.1), go-fast-core v0.1.9 (after v0.1.8), go-fast-pdf v0.4.0 (after
v0.3.1), go-fast-ooxml v0.5.3 (after v0.5.2) and go-fast-msbin v0.1.5
(after v0.1.4).

- **Five more formats are routed:**
  - web pages saved as one file (`.mht`, `.mhtml`, `multipart/related`
    saved by a browser), read by go-fast-html as the page with its
    images;
  - subtitles: SubRip (`.srt`) and WebVTT (`.vtt`, `.webvtt`), read by
    go-fast-html, a paragraph per cue with its times;
  - calendars (`.ics`, `.ifb`, `.vcs`, `text/calendar`) and contacts
    (`.vcf`, `.vcard`, `text/vcard`), read by go-fast-email, a heading
    and details for each event or person. A meeting invitation inside an
    email is read as a calendar.
  Files are recognised by name, media type and content (an MHTML header,
  a `BEGIN:VCALENDAR` or `BEGIN:VCARD` first line, the WebVTT signature).
- **PDF (go-fast-pdf v0.4.0):** paragraphs and reading order. A sentence
  is no longer cut where it passes between two copies of one typeface; an
  entry of a table of contents is a paragraph of its own; text in Hebrew
  and Arabic reads in the direction of its paragraph. See go-fast-pdf's
  CHANGELOG.
- Workbooks (go-fast-ooxml, go-fast-msbin through go-fast-core v0.1.9)
  are read about 1 to 2 percent faster; no cell changes.
- **Checked against v0.1.3 on the public corpus:** output changes only in
  the five formats routed now (ics, vcf, mht, srt, vtt): 302 of their 307
  dev files and 108 of their 109 held-out files, read as text before (the
  others are SubRip files of uncertain encoding, refused before and now).
  No other output changes, in 5,227 dev files or, by hash, 1,684 held-out
  files. (The PDF and Office corpora are compared in their libraries.)

## v0.1.3 (2026-11-25)

Pins go-fast-core v0.1.8 (after v0.1.6), go-fast-ooxml v0.5.2 (after
v0.5.0), go-fast-msbin v0.1.4 (after v0.1.3), go-fast-pdf v0.3.1 (after
v0.3.0) and go-fast-xml v0.2.1 (after v0.2.0).

- **Workbooks are read faster.** go-fast-ooxml and go-fast-msbin show a
  sheet's numbers through one engine, go-fast-core's `numfmt`, and
  remember what each format made of a value, so a number repeated on a
  sheet is copied, not worked out again (core's `numfmt.Cache`: 256
  entries a sheet in ooxml, 4,096 in msbin). No cell changes.
- go-fast-pdf v0.3.1 decompresses with go-fast-core's `inflate`; the
  error of a damaged stream in a warning begins `inflate:` where it said
  `zlib:`.
- go-fast-xml v0.2.1 changes only its own command.
- **Checked against v0.1.2 on the public corpus:** no output changes, in
  5,038 dev files or, by hash, 1,633 held-out files.

## v0.1.2 (2026-11-25)

Pins go-fast-xml v0.2.0 (after v0.1.0).

- **Web feeds** (go-fast-xml v0.2.0, after v0.1.0): RSS 0.91 to 2.0,
  RSS 1.0 and Atom 1.0 are read with go-fast-xml's new `feed` profile,
  each item a section with its title as a heading. The HTML an item
  holds (an RSS `description` or `content:encoded`, Atom `type="html"`
  or `"xhtml"`) is handed to the child reader and read by go-fast-html,
  so a feed is one document with an HTML document for each piece of HTML;
  before, that HTML was literal markup in the feed's text. `.rss` and
  `.atom` names now go to go-fast-xml, and a directory stands for them.
- go-fast-xml v0.2.0 also no longer counts, in
  `xml.unmapped_attributes`, the attributes that are in the output as
  notes.
- **Checked against v0.1.1 on the public corpus:** every change is in
  XML. 76 of the 76 dev feeds are read with the `feed` profile and give
  3,280 HTML children; all 3,356 documents pass `Validate`. In 87 other
  dev XML files only the `xml.unmapped_attributes` warning changes (16
  lose it). Held-out files, compared by hash: 64 change, all XML (24
  feeds).

## v0.1.1 (2026-11-24)

Pins go-fast-ooxml v0.5.0 (after v0.4.0) and adds go-fast-msbin v0.1.3;
the other libraries and go-fast-core v0.1.6 are as in v0.1.0.

- **Office documents saved as XML are read by go-fast-ooxml** (v0.5.0,
  after v0.4.0): Flat OPC (`pkg:package`, read as `docx`, `xlsx` or
  `pptx` with `extra.packaging` `flat_opc`), Word 2003's XML document
  (`w:wordDocument`, format `wordml`) and Excel 2003's XML spreadsheet
  (`Workbook`, format `spreadsheetml`). They are known by the root
  element and its namespace within the first 64 KiB, whatever their name
  or media type: before, they went to go-fast-xml, which read them as
  generic XML (deleted revisions as text, pictures as base64, no
  headings or cell formats). XML with a document type declaration stays
  with go-fast-xml. `application/vnd.ms-wordml` and
  `application/vnd.ms-spreadsheetml` go to go-fast-ooxml.
- **Word, Excel and PowerPoint 97-2003 files are read by go-fast-msbin**
  (v0.1.3): `.doc`, `.dot`, `.xls`, `.xlt`, `.xla`, `.ppt`, `.pps`,
  `.pot`, `application/msword`, `application/vnd.ms-excel` and
  `application/vnd.ms-powerpoint`, and any OLE compound file whose
  directory go-fast-msbin's `Detect` names, whatever its name; other
  compound files go to go-fast-email as before. A file named or declared
  as one of these is decided by its bytes: RTF, HTML or XML saved as
  `.doc` or `.xls`, and a zip named `.xls`, are read by the library for
  those bytes. Bytes no library reads go to go-fast-msbin, which names
  what they are. Attachments and embedded objects follow: Word, Excel
  and PowerPoint files attached to messages or embedded in RTF are now
  read. A directory now stands for these files too. Before, they were
  refused ("Office 97-2003 files are not read yet").
- **Checked against go-fast-msbin's corpus** (2,528 files): every file
  `gofastmsbin` reads gives the same document through `gofast`, apart
  from embedded documents, which `gofast` reads more of. Through
  `gofast`, 12 files that `gofastmsbin` refuses are read by their
  bytes: Excel and Word 2003 XML saved as `.xls` and `.doc` (3),
  Graphviz `.dot` files (3), JSON and text named `.doc` (2), `.xlsx` and
  `.docx` files named `.xls` and `.doc` (3), and a zip of XML files
  named `.xls` (1). Held-out files were
  compared by hash and format only. In the public corpus, 3 documents
  change: two messages with an attached `.doc`, and an RTF file with
  embedded Word, Excel and PowerPoint objects. A fourth changes only in
  the wording of the error for an embedded OLE object that is no
  Office file.
- **Checked:** the 42 such files of go-fast-ooxml's soak set give, through
  `gofast`, what `gofastooxml` gives (in one, only the wording of errors
  for embedded Office 97-2003 files, which the umbrella reads itself).
  The 5,036 documents of the public corpus's dev set, and its held-out
  set compared by hash, are unchanged from v0.1.0.

## v0.1.0 (2026-11-24)

The first version: one command and one package for every go-fast format,
and archives.

- **`cmd/gofast`**: reads files, directories and standard input in
  parallel (`-j`), every format of go-fast-pdf, -ooxml, -odf, -rtf, -email,
  -html and -xml, and zip, tar and gzip archives, each archive member a
  document of its own (`data.zip/reports/q3.pdf`), archives inside archives
  down to `-max-depth`. JSON Lines in the order of the inputs and members,
  or one file per document in a tree under `-dir`; Markdown and text;
  core's render flags and limits.
- **XML is read by go-fast-xml** (v0.1.0): `.xml`, `.dita`, `.ditamap`,
  `.docbook` and `.tei` files, `application/xml`, `text/xml` and other
  `+xml` types (not XHTML or SVG), and input that begins with an XML
  declaration, has a document type declaration naming a root other than
  `html`, or whose first element declares a namespace (not XHTML or SVG).
  Its twelve built-in profiles are chosen by the root element. A
  directory now stands for its XML files too. On main before, such files
  were read by go-fast-html as HTML, or left out of a directory; and an
  EPUB's manifest items of an XML type outside its reading order (a page
  map, media overlays) were read as HTML and are now read by go-fast-xml.
  `-charset` is tried for XML whose encoding go-fast-xml does not read.
  `-xml-profile FILE` (`Extractor.XMLProfiles`) adds the caller's
  profiles, tried before the built-in ones and chosen by the root element
  as they are; `-xml-attributes=false` (`Extractor.XMLAttributes`) leaves
  out the attribute notes of generic reading. Forcing one profile on
  every XML file, as `gofastxml -profile` does, is left out: in a batch
  of mixed files it would misread the others.
  Through the umbrella, the 4,587 XML files of the public corpus's dev
  set give the same JSON as `gofastxml`, with and without `-spans`; named
  without an extension, 4,584 are detected as XML by their bytes, and 3
  that have no declaration, document type or namespace are read as HTML.
- **Package `gofast`**: `Detect` (declared media type, extension, bytes),
  `Extractor.Extract` for any input, `Extractor.Child` to read embedded
  documents of any format, `Extractor.Walk` over an archive's members.
- **Bounds on archives**: one member decompressed (1 GiB), all that is
  decompressed from an archive (64 GiB), members held in memory at once
  by all workers (4 GiB), members of an archive read as one document
  (10,000). Overlapping zip members (zip bombs), sparse tar members and
  zip members larger than their compressed size allows are not read;
  member names are cleaned, and `-dir` writes nothing outside its
  directory, with names every system can create.
- **Decompression with go-fast-core's `inflate`**, about twice as fast
  as the standard library: zip members, and `.gz` and `.tar.gz` files of
  up to 1 GiB, which are read into memory and held while they are walked
  (counted under `-max-memory-bytes`); a larger one is read with
  `compress/gzip`. The same output as before; a `.tar.gz` cut short is
  reported as `inflate: the data ends early`, not `unexpected EOF`.
- **A zip cut short is read** by its members' own headers, with
  go-fast-core's `ziprecover` (go-fast-core v0.1.6): the members found are
  read, the one the file ends inside is reported as not read, and the
  archive is reported damaged, with how many members were found. Before,
  such an archive was not read at all. On a zip of 150 ODT files and 150
  messages cut before its directory, the 300 documents are byte-identical
  to the whole archive's.
- **JSON and YAML** (`.json`, `.jsonl`, `.ndjson`, `.geojson`, `.yaml`,
  `.yml`, and their media types) go to go-fast-html, as text; never by
  content.
- **`-charset`** (`Extractor.Charset`): the character set of text that
  declares none and whose set is not certain from its bytes. Given to
  go-fast-email for text in messages, and used for a file go-fast-html
  refuses with `ErrEncodingUnknown`; never before, so it does not override
  a `<meta>`.
- Pins: go-fast-core v0.1.6 (the comparisons below were made with
  v0.1.2), go-fast-pdf v0.3.0, go-fast-ooxml v0.4.0,
  go-fast-odf v0.2.2, go-fast-rtf v0.2.1, go-fast-email v0.2.1,
  go-fast-html v0.4.4, go-fast-xml v0.1.0. Against go-fast-odf v0.2.0, -rtf v0.2.0, -email
  v0.2.0 and -html v0.3.0, over the 22,421 ODF, RTF, email and HTML
  documents of the benchmark set and the 506 the command reads of the
  public corpus, with default flags and with `-cells -spans -images data`,
  36 documents differ, all by go-fast-html v0.4's encodings: 20 pages and
  a KOI8-R message that were read as windows-1252 are decoded (ISO-8859-6,
  EUC-JP, ISO-8859-2 and others); 15 pages declare their encoding past
  the first 1024 bytes, which is now used: 14 keep the same text and lose
  the warning that it was recognised from their bytes, and 1 that declares
  UTF-8 but is written in windows-1252 is read as UTF-8, as a browser
  reads it: an HTML comment has U+FFFD where it had `ï` and `ë`, with
  `text.unmapped_glyphs`. 3 pages refused for their encoding are read.
  go-fast-odf, -rtf and -email give the same output.
  go-fast-ooxml v0.4.0 (after v0.3.0) decompresses with go-fast-core's
  `inflate`, reads a cut file with its `ziprecover`, and fixes three
  readers that kept a view of its scanner: over 8,543 documents (its
  corpus, fixtures and encrypted files, and the 1,980 of its batch
  benchmark), 10 differ with default flags and 14 with `-cells -spans
  -images data`: 4 workbooks keep shared formulas they lost, 2 Word files
  their heading and list styles, and 8 damaged files made by fuzzers or
  for tests give other messages or other recovered blocks.
  Earlier, over the 6,644 documents of the public corpus and
  go-fast-ooxml's, the same files are read as with the earlier pins
  (go-fast-core v0.1.1, -pdf v0.2.1, -odf v0.1.3, -rtf v0.1.2, -email
  v0.1.3, -html v0.2.0); 12 documents differ, by the shared warning codes
  and the warnings now raised where control characters are removed.
  go-fast-pdf v0.3.0 (after v0.2.2) adds `input.decrypted` and
  `text.control_characters_removed`: over the 1,650 PDF files of the
  public corpus, the same files are read, 1,575 documents are the same,
  35 differ only by those warnings, 3 lose U+FFFD from their strings, and
  1 loses the language escapes of its document information.
  go-fast-ooxml v0.2.0 (after v0.1.1) raises the shared codes of core
  v0.1.2 in place of seven of its own and sets `Metadata.Encrypted`: over
  its 6,615 corpus and soak files, the same files are read, 6,513
  documents are the same, and 102 differ only by the code strings, the
  message on damaged document properties and `encrypted: true`.
  go-fast-ooxml v0.3.0 (after v0.2.0) is faster and uses less memory,
  rounds the fifteenth digit of a number as LibreOffice does, reads the
  fonts of a cell's runs, and corrects two warning messages: over the same
  6,615 files, the same files are read, 6,582 documents are the same, 31
  differ by 5,515 numbers in their fifteenth digit, and 2 by the message
  on Excel 4.0 macro sheets.
- Checks: files read from zip, tar and tar.gz archives give the same
  documents as the files read on their own (449 files of every format);
  2,699 corpus files read with no Validate error; tests for detection,
  archives, bounds, order, `-dir` and failures; `FuzzArchive`,
  `FuzzDetect`; CI on Linux, macOS and Windows, Go 1.25 and stable. An
  independent review's findings, in two rounds (among them a hang when a
  memory bound was smaller than nested archives, nested members not
  counted against memory, sparse tar members, a decode budget not
  settled before children) are fixed, with a test for each.
