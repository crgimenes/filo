package main

import (
	"strings"
	"testing"
)

func TestFix(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		changed bool
	}{
		{
			name:    "root let unwrapped",
			in:      "(let ()\n  (set host \"localhost\")\n  (set port 3210))\n",
			want:    "(set host \"localhost\")\n(set port 3210)\n",
			changed: true,
		},
		{
			name:    "root let with comments preserved",
			in:      "; config file\n(let ()\n  ; the host\n  (set host \"h\")\n  (set port 1))\n",
			want:    "; config file\n; the host\n(set host \"h\")\n(set port 1)\n",
			changed: true,
		},
		{
			name:    "let with bindings stays",
			in:      "(let ((x 1))\n  (set y x))\n",
			want:    "(let ((x 1))\n  (set y x))\n",
			changed: false,
		},
		{
			name:    "multiple top-level forms stay",
			in:      "(set a 1)\n(let () (set b 2))\n",
			want:    "(set a 1)\n(let () (set b 2))\n",
			changed: false,
		},
		{
			name:    "const fold",
			in:      "(set port (* 8 1000))\n(set label (string 42))\n",
			want:    "(set port 8000)\n(set label \"42\")\n",
			changed: true,
		},
		{
			name:    "fold skips spans with comments",
			in:      "(set x (+ 1 ; one\n  2))\n",
			want:    "(set x (+ 1 ; one\n  2))\n",
			changed: false,
		},
		{
			name:    "parens inside strings untouched",
			in:      "(set s \"(+ 1 2)\")\n",
			want:    "(set s \"(+ 1 2)\")\n",
			changed: false,
		},
		{
			name:    "nested fold reduces innermost constants",
			in:      "(set x (+ n (* 2 60)))\n",
			want:    "(set x (+ n 120))\n",
			changed: true,
		},
		{
			name:    "list is not folded",
			in:      "(set l (list 1 2))\n",
			want:    "(set l (list 1 2))\n",
			changed: false,
		},
		{
			name:    "unwrap then fold in one run",
			in:      "(let ()\n  (set t (+ 30 30)))\n",
			want:    "(set t 60)\n",
			changed: true,
		},
		{
			name:    "unbalanced input left alone",
			in:      "(set x (+ 1 2\n",
			want:    "(set x (+ 1 2\n",
			changed: false,
		},
		{
			name:    "already modern file untouched",
			in:      "(set host \"h\")\n(def f (fn (n) (+ n 1)))\n(set v (f 2))\n",
			want:    "(set host \"h\")\n(def f (fn (n) (+ n 1)))\n(set v (f 2))\n",
			changed: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := Fix(tc.in)
			if got != tc.want {
				t.Errorf("Fix output:\n%q\nwant:\n%q", got, tc.want)
			}
			if changed != tc.changed {
				t.Errorf("changed = %v, want %v", changed, tc.changed)
			}

			// Idempotence: a second run must be a no-op.
			again, c := Fix(got)
			if c || again != got {
				t.Errorf("Fix is not idempotent: second run changed the output")
			}
		})
	}
}

func TestRunStdin(t *testing.T) {
	var out, errOut strings.Builder
	in := strings.NewReader("(let ()\n  (set port (* 8 1000)))\n")
	code := run(nil, in, &out, &errOut)
	if code != 0 {
		t.Fatalf("run exit = %d, stderr: %s", code, errOut.String())
	}
	want := "(set port 8000)\n"
	if out.String() != want {
		t.Fatalf("stdout = %q, want %q", out.String(), want)
	}
}
