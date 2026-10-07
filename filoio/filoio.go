// Package filoio gives Filo scripts the machine: the process's arguments and
// streams, files, the environment, the clock and HTTP. It is not part of the
// language and no engine has it by default: a program that embeds Filo
// registers it only when it wants its scripts to reach those, and the filo
// command does. The names and what they answer are rocchetto's, so a script
// runs the same in that shell and under filo.
package filoio

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/crgimenes/filo"
)

// Host is what the builtins act on. Args, Stdin, Stdout and Stderr are the
// process's (or a test's); Client is the HTTP client, nil for one with a
// 30 s deadline. Status is what exit-status left, for the caller to exit
// with.
type Host struct {
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Client *http.Client

	mu     sync.Mutex
	in     *bufio.Reader
	status int
	files  map[int]*handle
	next   int
}

// handle is an open file: read through a buffer, or written to a temporary
// file that takes the name only at file-close ("w"), or appended to ("a").
type handle struct {
	f    *os.File
	r    *bufio.Reader
	path string // "w": the name the temporary file takes at close
}

// Status is what exit-status set, 0 when nothing did.
func (h *Host) Status() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status
}

// Globals are the values a run starts with: ARGS, the words for the script.
func (h *Host) Globals() map[string]filo.Value {
	args := make([]filo.Value, len(h.Args))
	for i, a := range h.Args {
		args[i] = filo.VString(a)
	}
	return map[string]filo.Value{"ARGS": filo.VList(args)}
}

// Close closes what a run left open; a file being written is dropped.
func (h *Host) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, f := range h.files {
		_ = f.f.Close()
		if f.path != "" {
			_ = os.Remove(f.f.Name())
		}
		delete(h.files, id)
	}
}

// RegisterBuiltins adds the machine's builtins to eng.
func (h *Host) RegisterBuiltins(eng *filo.Engine) {
	for name, fn := range map[string]filo.Builtin{
		"in-read":      h.inRead,
		"in-line":      h.inLine,
		"out-write":    func(_ context.Context, a []filo.Value) (filo.Value, error) { return put(h.Stdout, "out-write", a) },
		"err-write":    func(_ context.Context, a []filo.Value) (filo.Value, error) { return put(h.Stderr, "err-write", a) },
		"exit-status":  h.exitStatus,
		"read-file":    readFile,
		"write-file":   writeFile,
		"is-file":      isFile,
		"file-open":    h.fileOpen,
		"file-read":    h.fileRead,
		"file-line":    h.fileLine,
		"file-write":   h.fileWrite,
		"file-seek":    h.fileSeek,
		"file-close":   h.fileClose,
		"file-stat":    fileStat,
		"dir-read":     dirRead,
		"path-resolve": pathResolve,
		"env-get":      envGet,
		"env-list":     envList,
		"now":          now,
		"time-zone":    timeZone,
		"http-get":     h.httpGet,
		"http-request": h.httpRequest,
	} {
		eng.MustRegisterBuiltin(name, fn)
	}
}

var nilValue = filo.VList(nil)

func str(name string, a []filo.Value, i int) (string, error) {
	s, err := a[i].AsString()
	if err != nil {
		return "", fmt.Errorf("%s: argument %d: %w", name, i+1, err)
	}
	return s, nil
}

func whole(name string, a []filo.Value, i int, lo, hi float64) (int, error) {
	n, err := a[i].AsNumber()
	if err != nil {
		return 0, fmt.Errorf("%s: argument %d: %w", name, i+1, err)
	}
	if n != float64(int64(n)) || n < lo || n > hi {
		return 0, fmt.Errorf("%s: argument %d: a whole number from %v to %v", name, i+1, lo, hi)
	}
	return int(n), nil
}

func arity(name string, a []filo.Value, lo, hi int) error {
	if len(a) < lo || len(a) > hi {
		return fmt.Errorf("%s expects %d to %d arguments", name, lo, hi)
	}
	return nil
}

// path is a script's path as the shell reads it: ~ is the home.
func path(name string, a []filo.Value, i int) (string, error) {
	p, err := str(name, a, i)
	if err != nil {
		return "", err
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
		p = home + p[1:]
	}
	return p, nil
}

// why is a failure as a script reads it back: a string, not an error.
func why(err error) filo.Value {
	if pe, ok := errors.AsType[*os.PathError](err); ok {
		return filo.VString(pe.Err.Error())
	}
	return filo.VString(err.Error())
}

// put writes strings and numbers, byte for byte, as out-write does.
func put(w io.Writer, name string, a []filo.Value) (filo.Value, error) {
	var b strings.Builder
	for i, v := range a {
		switch v.Kind {
		case filo.KString:
			b.WriteString(v.Str)
		case filo.KNumber:
			b.WriteString(numberText(v))
		default:
			return filo.Value{}, fmt.Errorf("%s: argument %d: expects strings or numbers", name, i+1)
		}
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return filo.Value{}, fmt.Errorf("%s: %w", name, err)
	}
	return filo.VBool(true), nil
}

// numberText is a number as rocchetto's out-write spells it: a whole one
// in digits (int-text's 1000000, not string's 1e+06), any other as string.
func numberText(v filo.Value) string {
	if v.Num == math.Trunc(v.Num) && math.Abs(v.Num) < 1<<53 {
		return strconv.FormatInt(int64(v.Num), 10)
	}
	return strings.TrimSuffix(filo.FormatValue(v), "\n")
}

func (h *Host) input() *bufio.Reader {
	if h.in == nil {
		h.in = bufio.NewReader(h.Stdin)
	}
	return h.in
}

// (in-read) the rest of the input, "" at its end; (in-read n) at most n
// bytes, nil at the end.
func (h *Host) inRead(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("in-read", a, 0, 1); err != nil {
		return filo.Value{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(a) == 0 {
		b, err := io.ReadAll(h.input())
		if err != nil {
			return filo.Value{}, fmt.Errorf("in-read: %w", err)
		}
		return filo.VString(string(b)), nil
	}
	n, err := whole("in-read", a, 0, 1, 1<<30)
	if err != nil {
		return filo.Value{}, err
	}
	buf := make([]byte, n)
	got, err := io.ReadAtLeast(h.input(), buf, 1)
	if got == 0 && (err == io.EOF || err == io.ErrUnexpectedEOF) {
		return nilValue, nil
	}
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return filo.Value{}, fmt.Errorf("in-read: %w", err)
	}
	return filo.VString(string(buf[:got])), nil
}

// (in-line) the next line with its "\n", nil at the end.
func (h *Host) inLine(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("in-line", a, 0, 0); err != nil {
		return filo.Value{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return line("in-line", h.input())
}

func line(name string, r *bufio.Reader) (filo.Value, error) {
	s, err := r.ReadString('\n')
	if s == "" && err == io.EOF {
		return nilValue, nil
	}
	if err != nil && err != io.EOF {
		return filo.Value{}, fmt.Errorf("%s: %w", name, err)
	}
	return filo.VString(s), nil
}

// (exit-status n) the status the process ends with, 0 to 255; it does not
// end the script. (exit-status) reads it.
func (h *Host) exitStatus(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("exit-status", a, 0, 1); err != nil {
		return filo.Value{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(a) == 1 {
		n, err := whole("exit-status", a, 0, 0, 255)
		if err != nil {
			return filo.Value{}, err
		}
		h.status = n
	}
	return filo.VNum(float64(h.status)), nil
}

// (read-file p) a whole file as a string; an error when it is not there.
func readFile(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("read-file", a, 1, 1); err != nil {
		return filo.Value{}, err
	}
	p, err := path("read-file", a, 0)
	if err != nil {
		return filo.Value{}, err
	}
	b, err := os.ReadFile(p) // #nosec G304 -- reading the files a script names is this package's purpose
	if err != nil {
		return filo.Value{}, fmt.Errorf("read-file: %w", err)
	}
	return filo.VString(string(b)), nil
}

// (write-file p s) whether it was written.
func writeFile(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("write-file", a, 2, 2); err != nil {
		return filo.Value{}, err
	}
	p, err := path("write-file", a, 0)
	if err != nil {
		return filo.Value{}, err
	}
	s, err := str("write-file", a, 1)
	if err != nil {
		return filo.Value{}, err
	}
	return filo.VBool(os.WriteFile(p, []byte(s), 0o644) == nil), nil // #nosec G306 -- a script's file, as a shell's redirect makes it
}

// (is-file p) whether p is a file (not a directory).
func isFile(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("is-file", a, 1, 1); err != nil {
		return filo.Value{}, err
	}
	p, err := path("is-file", a, 0)
	if err != nil {
		return filo.Value{}, err
	}
	st, err := os.Stat(p)
	return filo.VBool(err == nil && st.Mode().IsRegular()), nil
}

// (file-open p [mode]) to read, "w" to write (the old file stays until
// file-close), "a" to add at the end: a handle, or why not.
func (h *Host) fileOpen(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("file-open", a, 1, 2); err != nil {
		return filo.Value{}, err
	}
	p, err := path("file-open", a, 0)
	if err != nil {
		return filo.Value{}, err
	}
	mode := ""
	if len(a) == 2 {
		if mode, err = str("file-open", a, 1); err != nil {
			return filo.Value{}, err
		}
	}
	var hd handle
	switch mode {
	case "", "r":
		f, err := os.Open(p) // #nosec G304 -- opening the files a script names is this package's purpose
		if err != nil {
			return why(err), nil
		}
		hd = handle{f: f, r: bufio.NewReader(f)}
	case "w":
		f, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".*")
		if err != nil {
			return why(err), nil
		}
		hd = handle{f: f, path: p}
	case "a":
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644) // #nosec G302 G304 -- a script's file, as a shell's >> makes it
		if err != nil {
			return why(err), nil
		}
		hd = handle{f: f}
	default:
		return filo.Value{}, fmt.Errorf("file-open: mode %q: want \"w\" or \"a\", or none to read", mode)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.files == nil {
		h.files = map[int]*handle{}
	}
	h.next++
	h.files[h.next] = &hd
	return filo.VNum(float64(h.next)), nil
}

func (h *Host) file(name string, a []filo.Value) (int, *handle, error) {
	id, err := whole(name, a, 0, 1, 1<<31)
	if err != nil {
		return 0, nil, err
	}
	f := h.files[id]
	if f == nil {
		return 0, nil, fmt.Errorf("%s: no file open as %d", name, id)
	}
	return id, f, nil
}

// (file-read h [n]) the next n bytes (4096), nil at the end.
func (h *Host) fileRead(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("file-read", a, 1, 2); err != nil {
		return filo.Value{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, f, err := h.file("file-read", a)
	if err != nil {
		return filo.Value{}, err
	}
	if f.r == nil {
		return filo.Value{}, errors.New("file-read: the file is open to write")
	}
	n := 4096
	if len(a) == 2 {
		if n, err = whole("file-read", a, 1, 1, 1<<30); err != nil {
			return filo.Value{}, err
		}
	}
	buf := make([]byte, n)
	got, err := io.ReadAtLeast(f.r, buf, 1)
	if got == 0 && (err == io.EOF || err == io.ErrUnexpectedEOF) {
		return nilValue, nil
	}
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return filo.Value{}, fmt.Errorf("file-read: %w", err)
	}
	return filo.VString(string(buf[:got])), nil
}

// (file-line h) the next line with its "\n", nil at the end.
func (h *Host) fileLine(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("file-line", a, 1, 1); err != nil {
		return filo.Value{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, f, err := h.file("file-line", a)
	if err != nil {
		return filo.Value{}, err
	}
	if f.r == nil {
		return filo.Value{}, errors.New("file-line: the file is open to write")
	}
	return line("file-line", f.r)
}

// (file-write h s ...) #t, or why not.
func (h *Host) fileWrite(_ context.Context, a []filo.Value) (filo.Value, error) {
	if len(a) < 1 {
		return filo.Value{}, errors.New("file-write expects a handle and what to write")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, f, err := h.file("file-write", a)
	if err != nil {
		return filo.Value{}, err
	}
	if f.r != nil {
		return filo.Value{}, errors.New("file-write: the file is open to read")
	}
	v, err := put(f.f, "file-write", a[1:])
	if err != nil {
		return why(err), nil
	}
	return v, nil
}

// (file-seek h off) for one open to read: #t, or why not.
func (h *Host) fileSeek(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("file-seek", a, 2, 2); err != nil {
		return filo.Value{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, f, err := h.file("file-seek", a)
	if err != nil {
		return filo.Value{}, err
	}
	if f.r == nil {
		return filo.Value{}, errors.New("file-seek: the file is open to write")
	}
	off, err := whole("file-seek", a, 1, 0, 1<<53)
	if err != nil {
		return filo.Value{}, err
	}
	if _, err := f.f.Seek(int64(off), io.SeekStart); err != nil {
		return why(err), nil
	}
	f.r.Reset(f.f)
	return filo.VBool(true), nil
}

// (file-close h) #t, or why what was written could not be kept.
func (h *Host) fileClose(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("file-close", a, 1, 1); err != nil {
		return filo.Value{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	id, f, err := h.file("file-close", a)
	if err != nil {
		return filo.Value{}, err
	}
	delete(h.files, id)
	err = f.f.Close()
	if f.path == "" {
		if err != nil {
			return why(err), nil
		}
		return filo.VBool(true), nil
	}
	if err == nil {
		err = os.Rename(f.f.Name(), f.path)
	}
	if err != nil {
		_ = os.Remove(f.f.Name())
		return why(err), nil
	}
	return filo.VBool(true), nil
}

func kind(m os.FileMode) string {
	switch {
	case m&os.ModeSymlink != 0:
		return "link"
	case m.IsDir():
		return "dir"
	}
	return "file"
}

// (file-stat p [#t]) nil, or (kind size mtime from): kind "file", "dir" or
// "link" (#t follows the link), mtime in seconds since 1970 UTC, from
// "disk" (rocchetto's are "home", "tree" and "site").
func fileStat(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("file-stat", a, 1, 2); err != nil {
		return filo.Value{}, err
	}
	p, err := path("file-stat", a, 0)
	if err != nil {
		return filo.Value{}, err
	}
	stat := os.Lstat
	if len(a) == 2 && a[1].Kind == filo.KBool && a[1].Bool {
		stat = os.Stat
	}
	st, err := stat(p)
	if err != nil {
		return nilValue, nil
	}
	return filo.VTuple([]filo.Value{
		filo.VString(kind(st.Mode())),
		filo.VNum(float64(st.Size())),
		filo.VNum(float64(st.ModTime().Unix())),
		filo.VString("disk"),
	}), nil
}

// (dir-read d [from [count]]) (entries next): entries (name kind) in byte
// order, dot files too, at most count (256, up to 1024), and next where the
// next page starts, nil after the last.
func dirRead(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("dir-read", a, 1, 3); err != nil {
		return filo.Value{}, err
	}
	d, err := path("dir-read", a, 0)
	if err != nil {
		return filo.Value{}, err
	}
	from, count := 0, 256
	if len(a) >= 2 {
		if from, err = whole("dir-read", a, 1, 0, 9e15); err != nil {
			return filo.Value{}, err
		}
	}
	if len(a) == 3 {
		if count, err = whole("dir-read", a, 2, 1, 1024); err != nil {
			return filo.Value{}, err
		}
	}
	entries, err := os.ReadDir(d)
	if err != nil {
		return filo.Value{}, fmt.Errorf("dir-read: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	first := min(from, len(entries))
	last := min(first+count, len(entries))
	items := make([]filo.Value, 0, last-first)
	for _, e := range entries[first:last] {
		items = append(items, filo.VTuple([]filo.Value{filo.VString(e.Name()), filo.VString(kind(e.Type()))}))
	}
	next := nilValue
	if last < len(entries) {
		next = filo.VNum(float64(last))
	}
	return filo.VTuple([]filo.Value{filo.VList(items), next}), nil
}

// (path-resolve p) the absolute path.
func pathResolve(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("path-resolve", a, 1, 1); err != nil {
		return filo.Value{}, err
	}
	p, err := path("path-resolve", a, 0)
	if err != nil {
		return filo.Value{}, err
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filo.Value{}, fmt.Errorf("path-resolve: %w", err)
	}
	return filo.VString(abs), nil
}

// (env-get name) the variable, nil when it is not set.
func envGet(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("env-get", a, 1, 1); err != nil {
		return filo.Value{}, err
	}
	name, err := str("env-get", a, 0)
	if err != nil {
		return filo.Value{}, err
	}
	v, ok := os.LookupEnv(name)
	if !ok {
		return nilValue, nil
	}
	return filo.VString(v), nil
}

// (env-list) every variable as (name value).
func envList(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("env-list", a, 0, 0); err != nil {
		return filo.Value{}, err
	}
	env := os.Environ()
	sort.Strings(env)
	items := make([]filo.Value, 0, len(env))
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		items = append(items, filo.VTuple([]filo.Value{filo.VString(k), filo.VString(v)}))
	}
	return filo.VList(items), nil
}

// (now) seconds since 1970 UTC.
func now(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("now", a, 0, 0); err != nil {
		return filo.Value{}, err
	}
	return filo.VNum(float64(time.Now().Unix())), nil
}

// (time-zone) minutes east of UTC, now.
func timeZone(_ context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("time-zone", a, 0, 0); err != nil {
		return filo.Value{}, err
	}
	_, off := time.Now().Zone()
	return filo.VNum(float64(off / 60)), nil
}

// (http-get url) is (http-request "GET" url).
func (h *Host) httpGet(ctx context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("http-get", a, 1, 1); err != nil {
		return filo.Value{}, err
	}
	return h.httpRequest(ctx, []filo.Value{filo.VString("GET"), a[0]})
}

// (http-request method url [body [headers]]) (status body headers),
// headers a list of (name value); status 0 when no answer came, body then
// saying why.
func (h *Host) httpRequest(ctx context.Context, a []filo.Value) (filo.Value, error) {
	if err := arity("http-request", a, 2, 4); err != nil {
		return filo.Value{}, err
	}
	method, err := str("http-request", a, 0)
	if err != nil {
		return filo.Value{}, err
	}
	url, err := str("http-request", a, 1)
	if err != nil {
		return filo.Value{}, err
	}
	var body io.Reader
	if len(a) >= 3 {
		s, err := str("http-request", a, 2)
		if err != nil {
			return filo.Value{}, err
		}
		body = strings.NewReader(s)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return filo.Value{}, fmt.Errorf("http-request: %w", err)
	}
	if len(a) == 4 {
		if err := headers(req, a[3]); err != nil { // #nosec G602 -- len(a) is 4 here
			return filo.Value{}, err
		}
	}
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req) // #nosec G107 G704 -- reaching the URL a script names is this package's purpose
	if err != nil {
		return filo.VTuple([]filo.Value{filo.VNum(0), filo.VString(err.Error()), nilValue}), nil
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return filo.VTuple([]filo.Value{filo.VNum(0), filo.VString(err.Error()), nilValue}), nil
	}
	names := make([]string, 0, len(resp.Header))
	for k := range resp.Header {
		names = append(names, k)
	}
	sort.Strings(names)
	hs := make([]filo.Value, 0, len(names))
	for _, k := range names {
		for _, v := range resp.Header[k] {
			hs = append(hs, filo.VTuple([]filo.Value{filo.VString(k), filo.VString(v)}))
		}
	}
	return filo.VTuple([]filo.Value{filo.VNum(float64(resp.StatusCode)), filo.VString(string(b)), filo.VList(hs)}), nil
}

func headers(req *http.Request, v filo.Value) error {
	list, err := v.AsList()
	if err != nil {
		return fmt.Errorf("http-request: headers: %w", err)
	}
	for i, pair := range list {
		kv := pair.Tup
		if pair.Kind == filo.KList {
			kv = pair.List
		}
		if len(kv) != 2 || kv[0].Kind != filo.KString || kv[1].Kind != filo.KString {
			return fmt.Errorf("http-request: header %d: want (name value)", i+1)
		}
		req.Header.Add(kv[0].Str, kv[1].Str)
	}
	return nil
}
