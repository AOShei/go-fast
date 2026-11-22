// Package gofast is the front door of the go-fast libraries: it reads a
// file of any of their formats by sending it to the library that reads
// it, and opens zip, tar and gzip archives.
//
// Detect decides where an input goes, from a declared media type, the
// extension of its name, and its bytes. Extractor reads it, and its Child
// method is a gofastcore.ChildFunc that reads embedded documents of any
// format, so a PDF attached to a message or a workbook embedded in a
// presentation is read too. The libraries themselves never import each
// other; this package imports them all.
//
// The command cmd/gofast reads files, directories and archives in
// parallel, with each archive member a document of its own.
package gofast
