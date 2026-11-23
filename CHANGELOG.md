# Changelog

## Unreleased

The first version: one command and one package for every go-fast format,
and archives.

- **`cmd/gofast`**: reads files, directories and standard input in
  parallel (`-j`), every format of go-fast-pdf, -ooxml, -odf, -rtf, -email
  and -html, and zip, tar and gzip archives, each archive member a document
  of its own (`data.zip/reports/q3.pdf`), archives inside archives down to
  `-max-depth`. JSON Lines in the order of the inputs and members, or one
  file per document in a tree under `-dir`; Markdown and text; core's
  render flags and limits.
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
- **JSON and YAML** (`.json`, `.jsonl`, `.ndjson`, `.geojson`, `.yaml`,
  `.yml`, and their media types) go to go-fast-html, as text; never by
  content.
- **`-charset`** (`Extractor.Charset`): the character set of text that
  declares none and whose set is not certain from its bytes. Given to
  go-fast-email for text in messages, and used for a file go-fast-html
  refuses with `ErrEncodingUnknown`; never before, so it does not override
  a `<meta>`.
- Pins: go-fast-core v0.1.2, go-fast-pdf v0.3.0, go-fast-ooxml v0.2.0,
  go-fast-odf v0.2.0, go-fast-rtf v0.2.0, go-fast-email v0.2.0,
  go-fast-html v0.3.0. Over the 6,644 documents of the public corpus and
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
- Checks: files read from zip, tar and tar.gz archives give the same
  documents as the files read on their own (449 files of every format);
  2,699 corpus files read with no Validate error; tests for detection,
  archives, bounds, order, `-dir` and failures; `FuzzArchive`,
  `FuzzDetect`; CI on Linux, macOS and Windows, Go 1.25 and stable. An
  independent review's findings, in two rounds (among them a hang when a
  memory bound was smaller than nested archives, nested members not
  counted against memory, sparse tar members, a decode budget not
  settled before children) are fixed, with a test for each.
