# Changelog

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
