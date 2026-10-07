package filoio

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/filostrings"
)

// run runs src on an engine with h's builtins and h's globals.
func run(t *testing.T, h *Host, src string) filo.Value {
	t.Helper()
	e := filo.NewEngine()
	filostrings.RegisterBuiltins(e)
	h.RegisterBuiltins(e)
	v, _, err := e.RunScript(context.Background(), src, h.Globals(), filo.EvalConfig{})
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return v
}

func newHost(stdin string) (*Host, *bytes.Buffer, *bytes.Buffer) {
	var out, errs bytes.Buffer
	return &Host{Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: &errs}, &out, &errs
}

// Not part of the language: an engine has none of it until registered.
func TestNotByDefault(t *testing.T) {
	_, _, err := filo.NewEngine().RunScript(context.Background(), `(read-file "x")`, nil, filo.EvalConfig{})
	if err == nil {
		t.Fatal("read-file ran on an engine that never registered filoio")
	}
}

func TestStreamsArgsAndStatus(t *testing.T) {
	h, out, errs := newHost("one\ntwo\nrest")
	h.Args = []string{"a", "b"}
	run(t, h, `(do
	  (out-write (str-join "," ARGS) " " (in-line))
	  (out-write (in-read 2) "|" (in-read) "|" (string (is-nil (in-line))) "\n")
	  (out-write 1000000 " " 2.5 "\n")
	  (err-write "oops\n")
	  (exit-status 3))`)
	if got := out.String(); got != "a,b one\ntw|o\nrest|#t\n1000000 2.5\n" {
		t.Fatalf("out %q", got)
	}
	if errs.String() != "oops\n" || h.Status() != 3 {
		t.Fatalf("stderr %q, status %d", errs.String(), h.Status())
	}
	if v := run(t, h, `(in-read 5)`); v.Kind != filo.KList || len(v.List) != 0 {
		t.Fatalf("in-read at the end: %v, want nil", v)
	}
}

// tempDir is a test's directory spelled with "/", which Windows takes too:
// a Filo string reads "\U" as an escape.
func tempDir(t *testing.T) string {
	return filepath.ToSlash(t.TempDir())
}

func TestFiles(t *testing.T) {
	dir := tempDir(t)
	p := dir + "/notes.txt"
	h, out, _ := newHost("")
	run(t, h, `(let ((h (file-open "`+p+`" "w")))
	  (do (file-write h "one\n" "two\n") (out-write (string (is-file "`+p+`")) "\n") (file-close h)))`)
	if got := out.String(); got != "#f\n" {
		t.Fatalf("a file being written is not there until closed: %q", got)
	}
	out.Reset()
	run(t, h, `(let ((h (file-open "`+p+`")))
	  (do (out-write (file-line h) (file-read h 2)) (file-seek h 0) (out-write (file-read h) (string (is-nil (file-read h))))
	      (file-close h)))`)
	if got := out.String(); got != "one\ntwone\ntwo\n#t" {
		t.Fatalf("read back %q", got)
	}
	if v := run(t, h, `(file-open "`+dir+"/none"+`")`); v.Kind != filo.KString {
		t.Fatalf("opening what is not there: %v, want why not", v)
	}
	run(t, h, `(let ((h (file-open "`+p+`" "a"))) (do (file-write h "three\n") (file-close h)))`)
	if v := run(t, h, `(read-file "`+p+`")`); v.Str != "one\ntwo\nthree\n" {
		t.Fatalf("append: %q", v.Str)
	}
	if v := run(t, h, `(write-file "`+dir+"/b"+`" "x")`); !v.Bool {
		t.Fatal("write-file said no")
	}
	st := run(t, h, `(file-stat "`+p+`")`)
	if st.Kind != filo.KTuple || st.Tup[0].Str != "file" || st.Tup[1].Num != 14 || st.Tup[3].Str != "disk" {
		t.Fatalf("file-stat %v", st)
	}
	if v := run(t, h, `(file-stat "`+dir+"/none"+`")`); v.Kind != filo.KList || len(v.List) != 0 {
		t.Fatalf("file-stat of nothing: %v", v)
	}
}

// A file being written that is never closed is dropped, the old one kept.
func TestUnclosedWriteIsDropped(t *testing.T) {
	p := tempDir(t) + "/keep"
	if err := os.WriteFile(p, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	h, _, _ := newHost("")
	run(t, h, `(file-write (file-open "`+p+`" "w") "new")`)
	h.Close()
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "old" {
		t.Fatalf("%q, %v", b, err)
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(p), ".keep.*"))
	if len(left) != 0 {
		t.Fatalf("temporary files left: %v", left)
	}
}

func TestDirRead(t *testing.T) {
	dir := tempDir(t)
	for _, n := range []string{"b", "a", ".c"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o700); err != nil {
		t.Fatal(err)
	}
	h, out, _ := newHost("")
	run(t, h, `(letv (es next) (dir-read "`+dir+`" 1 2)
	  (out-write (str-join " " (map (fn (e) (letv (n k) e (str-concat n ":" k))) es)) " " next))`)
	if got := out.String(); got != "a:file b:file 3" {
		t.Fatalf("page %q", got)
	}
	if v := run(t, h, `(letv (es next) (dir-read "`+dir+`" 3) next)`); v.Kind != filo.KList || len(v.List) != 0 {
		t.Fatalf("next after the last: %v", v)
	}
}

func TestEnvAndClock(t *testing.T) {
	t.Setenv("FILOIO_TEST", "yes")
	h, _, _ := newHost("")
	if v := run(t, h, `(env-get "FILOIO_TEST")`); v.Str != "yes" {
		t.Fatalf("env-get %v", v)
	}
	if v := run(t, h, `(is-nil (env-get "FILOIO_TEST_NOT_SET"))`); !v.Bool {
		t.Fatal("an unset variable is not nil")
	}
	if v := run(t, h, `(> (now) 1700000000)`); !v.Bool {
		t.Fatal("now is before 2023")
	}
}

func TestHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seen", r.Method+" "+r.Header.Get("X-Ask"))
		body := make([]byte, 64)
		n, _ := r.Body.Read(body)
		_, _ = w.Write(append([]byte("got "), body[:n]...))
	}))
	defer srv.Close()
	h, out, _ := newHost("")
	run(t, h, `(letv (status body headers) (http-request "POST" "`+srv.URL+`" "hi" (list (values "X-Ask" "yes")))
	  (out-write status " " body " " (str-join "," (map (fn (kv) (letv (k v) kv (str-concat k "=" v))) (filter (fn (kv) (letv (k v) kv (= k "X-Seen"))) headers)))))`)
	if got := out.String(); got != "200 got hi X-Seen=POST yes" {
		t.Fatalf("%q", got)
	}
	v := run(t, h, `(http-get "http://127.0.0.1:1/")`)
	if v.Kind != filo.KTuple || v.Tup[0].Num != 0 || v.Tup[1].Str == "" {
		t.Fatalf("no answer: %v, want (0 why nil)", v)
	}
}
