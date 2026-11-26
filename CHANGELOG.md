# Changelog

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
