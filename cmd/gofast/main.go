// Command gofast extracts documents of every format the go-fast libraries
// read (PDF; Word, Excel and PowerPoint; OpenDocument; RTF; email; HTML,
// Markdown, text, EPUB, CSV, TSV and Jupyter notebooks) into their shared
// document model and prints them as JSON, Markdown or plain text. It opens
// zip, tar and gzip archives, and reads many files in parallel.
//
//	gofast [flags] file-or-directory...
//
// A directory stands for the files in it and below it whose extension is
// one of those formats or an archive's (.zip, .tar, .tgz, .gz). A file
// named on the command line is read whatever its name, its format decided
// from its bytes. "-" is standard input.
//
// Each file of an archive is a document of its own, named by the archive
// and its path inside it ("data.zip/reports/q3.pdf"); archives inside
// archives are opened too, down to -max-depth. Embedded documents (a PDF
// attached to a message, a workbook in a presentation, a zip attached to a
// message) are read by whichever library reads their format, and come
// with the document that holds them.
//
// With one input that is not an archive the JSON is indented; otherwise
// each document is one line (JSON Lines), in the order of the inputs and
// of the members in each archive, whatever order the workers finish in.
// -dir writes each document to a file of its own instead, in a tree
// that mirrors the inputs. Only documents go to standard output:
// everything said about them goes to standard error, and the exit status
// is 1 if any input or member could not be read.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AOShei/go-fast"
	gofastcore "github.com/AOShei/go-fast-core"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	// After the first interrupt, the next one ends the process at once.
	context.AfterFunc(ctx, stop)
	os.Exit(run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// config is what the flags set.
type config struct {
	format    string
	output    string
	dir       string
	jobs      int
	quiet     bool
	opts      gofastcore.Options
	render    gofastcore.RenderOptions
	extractor *gofast.Extractor
	maxDepth  int
	maxMemory int64
	maxInput  int64 // bytes of standard input or a pipe read into memory
}

// run is the command: it reads args and writes documents to stdout and
// messages to stderr, and returns the exit status.
func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("gofast", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var (
		format      = flags.String("format", "json", "what to print: json, markdown or text")
		output      = flags.String("o", "", "write to this file instead of standard output")
		dir         = flags.String("dir", "", "write each document to a file of its own in this directory, in a tree that mirrors the inputs (name.ext.json, .md or .txt)")
		jobs        = flags.Int("j", runtime.NumCPU(), "files read at once")
		images      = flags.String("images", "meta", "images: none, meta (a block for each, with its size and ID) or data (and their bytes)")
		spans       = flags.Bool("spans", false, "record inline styling, links and declared display of ranges of text")
		cells       = flags.Bool("cells", false, "record the cells of spreadsheets with their types and formulas")
		password    = flags.String("password", "", "password passed to the extractor of encrypted files")
		charset     = flags.String("charset", "", "the character set (shift_jis, gbk, windows-1250, ...) of text that declares none and whose set is not recognised from its bytes: such text files are refused without it, and text in messages has U+FFFD")
		maxInput    = flags.Int64("max-input-bytes", 0, "bytes of one file or member (0: each library's default; -1: no limit)")
		maxDecoded  = flags.Int64("max-decoded-bytes", 0, "bytes decompressed in one document and the files embedded in it (0: each library's default; -1: no limit)")
		maxText     = flags.Int64("max-text-bytes", 0, "bytes of text a document and the files embedded in it may hold (0: the default, 256 MiB; -1: no limit)")
		maxDepth    = flags.Int("max-depth", 0, "how deeply archives inside archives, and embedded documents, are followed (0: the default, 4)")
		maxMember   = flags.Int64("max-member-bytes", 0, "bytes of one archive member decompressed (0: the default, 1 GiB; -1: no limit)")
		maxArchive  = flags.Int64("max-archive-bytes", 0, "bytes decompressed from one archive and the archives in it (0: the default, 64 GiB; -1: no limit)")
		maxMemory   = flags.Int64("max-memory-bytes", 0, "bytes of decompressed archive members held at once by all the workers together (0: the default, 4 GiB); one member larger than it is still read, alone")
		quiet       = flags.Bool("quiet", false, "say nothing about files that were read; failures are still reported")
		showVersion = flags.Bool("version", false, "print the version and exit")
	)
	renderFlags := gofastcore.AddRenderFlags(flags)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: gofast [flags] file-or-directory...")
		fmt.Fprintln(stderr, "Extracts files of every go-fast format, and the files in zip, tar and gzip archives. A directory stands for the files in it and below it.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, "gofast", moduleVersion())
		return 0
	}
	fail := func(format string, args ...any) int {
		fmt.Fprintf(stderr, "gofast: "+format+"\n", args...)
		return 2
	}
	if flags.NArg() == 0 {
		flags.Usage()
		return 2
	}
	if *format != "json" && *format != "markdown" && *format != "text" {
		return fail("unknown format %q: json, markdown or text", *format)
	}
	if *output != "" && *dir != "" {
		return fail("-o and -dir cannot be used together")
	}
	if *jobs < 1 {
		return fail("-j wants at least 1")
	}
	c := &config{format: *format, output: *output, dir: *dir, jobs: *jobs, quiet: *quiet, maxDepth: *maxDepth,
		maxMemory: *maxMemory, maxInput: *maxInput,
		extractor: &gofast.Extractor{MaxMemberBytes: *maxMember, MaxArchiveBytes: *maxArchive, Charset: *charset}}
	if c.maxMemory == 0 {
		c.maxMemory = 4 << 30
	}
	switch *images {
	case "none":
		c.opts.Images = gofastcore.ImagesNone
	case "meta":
		c.opts.Images = gofastcore.ImagesMeta
	case "data":
		c.opts.Images = gofastcore.ImagesData
	default:
		return fail("unknown value %q for -images: none, meta or data", *images)
	}
	c.opts.Spans, c.opts.Cells, c.opts.Password = *spans, *cells, *password
	c.opts.Limits = gofastcore.Limits{MaxInputBytes: *maxInput, MaxDecodedBytes: *maxDecoded, MaxTextBytes: *maxText,
		MaxDepth: *maxDepth}
	c.opts.Child = c.extractor.Child
	if c.jobs > 1 {
		// The files are read in parallel: a library that would also
		// read the pages of one in parallel reads them one at a time.
		c.opts.Workers = 1
	}
	renderFlags.Apply(&c.render)

	inputs, status := collect(flags.Args(), stderr)
	if len(inputs) == 0 {
		return max(status, 1)
	}
	if s := c.process(ctx, inputs, stdin, stdout, stderr); s > status {
		status = s
	}
	return status
}

// input is a file to read: named on the command line or found in a
// directory named there.
type input struct {
	path string // as given, or found
	rel  string // its path under -dir: slash-separated
	std  bool   // standard input
}

// collect makes the list of inputs.
func collect(args []string, stderr io.Writer) ([]input, int) {
	var inputs []input
	status := 0
	for _, arg := range args {
		if arg == "-" {
			inputs = append(inputs, input{path: "-", rel: "stdin", std: true})
			continue
		}
		info, err := os.Stat(arg)
		if err != nil {
			fmt.Fprintf(stderr, "gofast: %v\n", err)
			status = 1
			continue
		}
		if !info.IsDir() {
			inputs = append(inputs, input{path: arg, rel: filepath.Base(arg)})
			continue
		}
		var found []input
		err = filepath.WalkDir(arg, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				fmt.Fprintf(stderr, "gofast: %v\n", err)
				status = 1
				return nil
			}
			// Office keeps a lock file named "~$..." beside an open file.
			if d.Type().IsRegular() && gofast.KnownExtension(d.Name()) && !strings.HasPrefix(d.Name(), "~$") {
				rel, err := filepath.Rel(arg, p)
				if err != nil {
					rel = filepath.Base(p)
				}
				found = append(found, input{path: p, rel: filepath.ToSlash(rel)})
			}
			return nil
		})
		if err != nil {
			fmt.Fprintf(stderr, "gofast: %v\n", err)
			status = 1
		}
		sort.Slice(found, func(i, j int) bool { return found[i].path < found[j].path })
		inputs = append(inputs, found...)
	}
	return inputs, status
}

// task is one document to read, or one thing to report in its place.
type task struct {
	seq     int
	display string // the name messages give it
	name    string // Source.Name
	rel     string // its path under -dir
	kind    gofast.Kind
	r       io.ReaderAt
	size    int64
	done    func() // when the task is finished with r
	err     error  // the task is this error, reported in its place
	note    string // the task is this note, reported in its place
}

// result is a task read.
type result struct {
	task
	doc *gofastcore.Document
	err error
}

// process reads the inputs with c.jobs workers and writes the documents
// in order, and returns the exit status.
func (c *config) process(ctx context.Context, inputs []input, stdin io.Reader, stdout, stderr io.Writer) int {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	start := time.Now()

	// One input that is not an archive: its JSON is indented.
	single := len(inputs) == 1 && !inputs[0].std && c.dir == ""
	if single {
		if k, ok := kindOfFile(inputs[0].path); !ok || k.IsArchive() {
			single = false
		}
	}

	window := make(chan struct{}, 4*c.jobs) // tasks between the producer and the writer
	tasks := make(chan task)
	results := make(chan result, c.jobs)
	mem := newMemory(ctx, c.maxMemory)

	go c.produce(ctx, inputs, stdin, tasks, window, mem)

	var wg sync.WaitGroup
	for range c.jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range tasks {
				results <- c.extract(ctx, t)
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	w := newWriter(c, stdout, stderr, single)
	defer w.close()
	pending := map[int]result{}
	next := 0
	for r := range results {
		pending[r.seq] = r
		for {
			r, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			next++
			w.write(r)
			<-window
		}
	}
	if ctx.Err() != nil {
		fmt.Fprintln(stderr, "gofast: interrupted")
		w.status = 1
	}
	if err := w.close(); err != nil {
		fmt.Fprintf(stderr, "gofast: writing: %v\n", err)
		return 1
	}
	if !c.quiet {
		fmt.Fprintf(stderr, "gofast: %d documents, %d not read, in %.1fs\n", w.documents, w.failures, time.Since(start).Seconds())
	}
	return w.status
}

// kindOfFile detects the kind of a file named on the command line.
func kindOfFile(p string) (gofast.Kind, bool) {
	f, err := os.Open(p)
	if err != nil {
		return gofast.Kind{}, false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return gofast.Kind{}, false
	}
	return gofast.Detect(p, "", f, st.Size()), true
}

// produce turns the inputs into tasks, in order: a file is one task, an
// archive one task for each of its members.
func (c *config) produce(ctx context.Context, inputs []input, stdin io.Reader, tasks chan<- task, window chan struct{}, mem *memory) {
	defer close(tasks)
	seq := 0
	send := func(t task) bool {
		select {
		case window <- struct{}{}:
		case <-ctx.Done():
			if t.done != nil {
				t.done()
			}
			return false
		}
		t.seq = seq
		seq++
		select {
		case tasks <- t:
			return true
		case <-ctx.Done():
			// The writer waits for every sequence number it was given.
			if t.done != nil {
				t.done()
			}
			t.err, t.r, t.done = ctx.Err(), nil, nil
			tasks <- t
			return false
		}
	}
	for _, in := range inputs {
		if ctx.Err() != nil {
			return
		}
		r, size, closeFn, err := open(in, stdin, c.maxInput)
		if err != nil {
			if !send(task{display: in.path, err: err}) {
				return
			}
			continue
		}
		k := gofast.Detect(in.path, "", r, size)
		if in.std {
			k = gofast.Detect("", "", r, size)
		}
		name := filepath.Base(in.path)
		if !k.IsArchive() {
			if !send(task{display: in.path, name: name, rel: in.rel, kind: k, r: r, size: size, done: closeFn}) {
				return
			}
			continue
		}
		// The archive's file stays open until its last member is read.
		refs := &refCount{close: closeFn}
		refs.add()
		err = c.extractor.Walk(ctx, k, in.path, r, size, gofast.WalkOptions{MaxDepth: c.maxDepth, Hold: mem.hold},
			func(m gofast.Member) error {
				refs.add()
				release := m.Release
				ok := send(task{display: in.path + "/" + m.Path, name: name + "/" + m.Path, rel: in.rel + "/" + m.Path,
					kind: gofast.Detect(m.Path, "", m.R, m.Size), r: m.R, size: m.Size, done: func() {
						if release != nil {
							release()
						}
						refs.done()
					}})
				if !ok {
					return ctx.Err()
				}
				return nil
			},
			func(s gofast.Skipped) {
				t := task{display: in.path + "/" + s.Path, err: errors.New("not read: " + s.Reason)}
				if s.Note {
					t.err, t.note = nil, s.Reason
				}
				send(t)
			})
		refs.done()
		if err != nil && ctx.Err() == nil {
			if !send(task{display: in.path, err: err}) {
				return
			}
		}
	}
}

// open opens an input for reading at any offset: a regular file in
// place; standard input, a pipe or a device read into memory, up to
// maxInput bytes (0: 1 GiB; negative: no bound).
func open(in input, stdin io.Reader, maxInput int64) (io.ReaderAt, int64, func(), error) {
	limit := maxInput
	switch {
	case limit == 0:
		limit = 1 << 30
	case limit < 0:
		limit = math.MaxInt64 - 1
	}
	readAll := func(r io.Reader) (io.ReaderAt, int64, func(), error) {
		b, err := io.ReadAll(io.LimitReader(r, limit+1))
		if err != nil {
			return nil, 0, nil, err
		}
		if int64(len(b)) > limit {
			return nil, 0, nil, fmt.Errorf("larger than %d bytes, the bound on input read into memory (-max-input-bytes)", limit)
		}
		return bytesAt(b), int64(len(b)), func() {}, nil
	}
	if in.std {
		return readAll(stdin)
	}
	f, err := os.Open(in.path)
	if err != nil {
		return nil, 0, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, nil, err
	}
	if !st.Mode().IsRegular() {
		defer f.Close()
		return readAll(f)
	}
	return f, st.Size(), func() { f.Close() }, nil
}

// extract reads one task.
func (c *config) extract(ctx context.Context, t task) (res result) {
	res.task = t
	if t.done != nil {
		defer t.done()
	}
	if t.err != nil || t.note != "" {
		return res
	}
	defer func() {
		// A library that panics on an input fails that input, not the
		// run.
		if p := recover(); p != nil {
			res.doc, res.err = nil, fmt.Errorf("panic: %v", p)
		}
	}()
	o := c.opts
	o.Name = t.name
	res.doc, res.err = c.extractor.ExtractKind(ctx, t.kind, "", t.r, t.size, &o)
	return res
}

// refCount closes an archive's file when its members are all read.
type refCount struct {
	n     atomic.Int64
	close func()
}

func (r *refCount) add() { r.n.Add(1) }
func (r *refCount) done() {
	if r.n.Add(-1) == 0 && r.close != nil {
		r.close()
	}
}

// memory bounds the bytes of decompressed members held at once.
type memory struct {
	ctx  context.Context
	mu   sync.Mutex
	cond *sync.Cond
	used int64
	cap  int64
}

func newMemory(ctx context.Context, limit int64) *memory {
	m := &memory{ctx: ctx, cap: limit}
	m.cond = sync.NewCond(&m.mu)
	// A cancelled run wakes whoever waits.
	context.AfterFunc(ctx, func() {
		m.mu.Lock()
		m.cond.Broadcast()
		m.mu.Unlock()
	})
	return m
}

// hold waits until n bytes fit beside what others hold (own is what the
// caller itself holds, which it cannot give back while it waits), takes
// them, and returns the function that gives them back. More than the
// bound is taken when nothing else is held, so a large member is read
// alone. It gives up when the run is cancelled.
func (m *memory) hold(n, own int64) (func(), error) {
	if m.cap < 0 {
		return func() {}, nil
	}
	m.mu.Lock()
	for m.used-own > 0 && m.used > m.cap-min(n, m.cap) && m.ctx.Err() == nil {
		m.cond.Wait()
	}
	if err := m.ctx.Err(); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.used += n
	m.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			m.used -= n
			m.cond.Broadcast()
			m.mu.Unlock()
		})
	}, nil
}

// writer writes documents in order.
type writer struct {
	c         *config
	stderr    io.Writer
	out       *bufio.Writer
	file      *os.File
	single    bool
	written   int
	documents int
	failures  int
	status    int
	paths     map[string]bool // files written under -dir
	closed    bool
	err       error
}

func newWriter(c *config, stdout, stderr io.Writer, single bool) *writer {
	w := &writer{c: c, stderr: stderr, single: single, paths: map[string]bool{}}
	switch {
	case c.dir != "":
	case c.output != "":
		f, err := os.Create(c.output)
		if err != nil {
			w.err = err
			return w
		}
		w.file, w.out = f, bufio.NewWriter(f)
	default:
		w.out = bufio.NewWriter(stdout)
	}
	return w
}

// write writes one result: a document, or what went wrong with it.
func (w *writer) write(r result) {
	if r.err == nil && r.task.err != nil {
		r.err = r.task.err
	}
	if r.note != "" {
		if !w.c.quiet {
			fmt.Fprintf(w.stderr, "%s: %s\n", r.display, r.note)
		}
		return
	}
	if r.err != nil {
		if !errors.Is(r.err, context.Canceled) {
			fmt.Fprintf(w.stderr, "%s: %v\n", r.display, r.err)
		}
		w.failures++
		w.status = 1
		return
	}
	if w.err != nil {
		return
	}
	w.documents++
	if !w.c.quiet {
		if note := remarks(r.doc); note != "" {
			fmt.Fprintf(w.stderr, "%s: %s\n", r.display, note)
		}
	}
	if w.c.dir != "" {
		if err := w.writeFile(r); err != nil {
			fmt.Fprintf(w.stderr, "%s: %v\n", r.display, err)
			w.status = 1
		}
		return
	}
	if err := w.render(w.out, r, true); err != nil {
		w.err = err
	}
	w.written++
}

// render writes a document in the chosen format; between says whether to
// separate it from the documents before it in one stream.
func (w *writer) render(out io.Writer, r result, between bool) error {
	switch w.c.format {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetEscapeHTML(false)
		if w.single {
			enc.SetIndent("", "  ")
		}
		return enc.Encode(r.doc)
	case "markdown":
		if between && !w.single {
			if w.written > 0 {
				fmt.Fprintln(out)
			}
			fmt.Fprintf(out, "<!-- %s -->\n\n", strings.ReplaceAll(r.display, "--", "- -"))
		}
		if err := r.doc.WriteMarkdown(out, &w.c.render); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out)
		return err
	default:
		if between && !w.single {
			if w.written > 0 {
				fmt.Fprintln(out)
			}
			fmt.Fprintf(out, "==> %s <==\n", r.display)
		}
		if err := r.doc.WriteText(out, &w.c.render); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out)
		return err
	}
}

var extensions = map[string]string{"json": ".json", "markdown": ".md", "text": ".txt"}

// writeFile writes a document to its file under -dir.
func (w *writer) writeFile(r result) error {
	p, err := outputPath(w.c.dir, r.rel, extensions[w.c.format])
	if err != nil {
		return err
	}
	// Names that differ only in case are one file on Windows and macOS:
	// there, the second gets a number.
	for i := 2; w.paths[pathKey(p)]; i++ {
		q := strings.TrimSuffix(p, extensions[w.c.format]) + fmt.Sprintf("~%d", i) + extensions[w.c.format]
		if !w.paths[pathKey(q)] {
			fmt.Fprintf(w.stderr, "%s: written to %s, as another document's file has the same name but for case\n", r.display, q)
			p = q
			break
		}
	}
	w.paths[pathKey(p)] = true
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(f)
	err = w.render(bw, r, false)
	if e := bw.Flush(); err == nil {
		err = e
	}
	if e := f.Close(); err == nil {
		err = e
	}
	return err
}

// outputPath is the file a document whose path under -dir is rel is
// written to: inside dir whatever rel says, with names every system can
// create (no characters Windows forbids, no trailing dot or space, no
// device name such as CON or NUL).
func outputPath(dir, rel, ext string) (string, error) {
	var parts []string
	for _, el := range strings.Split(filepath.ToSlash(rel), "/") {
		switch el {
		case "", ".":
			continue
		case "..":
			el = "_"
		}
		el = strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f || strings.ContainsRune(`<>:"\|?*`, r) {
				return '_'
			}
			return r
		}, el)
		if t := strings.TrimRight(el, ". "); t != el {
			el = t + "_"
		}
		if reserved(el) {
			el = "_" + el
		}
		parts = append(parts, el)
	}
	if len(parts) == 0 {
		parts = []string{"document"}
	}
	p := filepath.Join(append([]string{dir}, parts...)...) + ext
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	absP, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if r, err := filepath.Rel(absDir, absP); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside %s", path.Clean(rel), dir)
	}
	return p, nil
}

// pathKey is what makes two output files one: their names, without
// case where the file system ignores it.
func pathKey(p string) string {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.ToLower(p)
	}
	return p
}

// reserved reports whether a name is a Windows device name, with or
// without an extension.
func reserved(name string) bool {
	base, _, _ := strings.Cut(strings.ToUpper(name), ".")
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9'
}

// close flushes and closes the output; it may be called more than once.
func (w *writer) close() error {
	if w.closed {
		return w.err
	}
	w.closed = true
	if w.out != nil {
		if err := w.out.Flush(); err != nil && w.err == nil {
			w.err = err
		}
	}
	if w.file != nil {
		if err := w.file.Close(); err != nil && w.err == nil {
			w.err = err
		}
	}
	return w.err
}

// remarks sums up what a document's warnings say that a person running
// the command would want to know without reading the JSON: anything of
// severity error.
func remarks(doc *gofastcore.Document) string {
	counts := map[string]int{}
	for _, w := range doc.AllWarnings() {
		if w.Severity == gofastcore.SeverityError {
			counts[w.Code]++
		}
	}
	if len(counts) == 0 {
		return ""
	}
	codes := make([]string, 0, len(counts))
	for code := range counts {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for i, code := range codes {
		if n := counts[code]; n > 1 {
			codes[i] = fmt.Sprintf("%s (%d)", code, n)
		}
	}
	return "warnings: " + strings.Join(codes, ", ")
}

// bytesAt reads from a byte slice.
type bytesAt []byte

func (b bytesAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	if off >= int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// moduleVersion is the version this command was built from.
func moduleVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}
