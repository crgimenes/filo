package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/crgimenes/filo/fbc"
)

// part is a unit to look at: the file itself, or a member of a bundle.
type part struct {
	name string
	data []byte
}

// parts are the units of a file: itself when it is one, its members when it
// is a bundle.
func parts(path string, data []byte) ([]part, int, error) {
	switch fbc.Kind(data) {
	case fbc.KindUnit:
		return []part{{filepath.Base(path), data}}, 0, nil
	case fbc.KindBundle:
		b, err := fbc.ReadBundle(data)
		if err != nil {
			return nil, 0, err
		}
		var ps []part
		table := len(data)
		for _, m := range b.Members {
			ps = append(ps, part{m.Name, data[m.Unit.Off : m.Unit.Off+m.Unit.Len]})
			table -= m.Unit.Len
		}
		return ps, table, nil
	}
	return nil, 0, errors.New("not a unit or a bundle (filo build makes one from source)")
}

// cmdCheck says of each unit whether a VM has what it asks for: the
// functions it imports and the extern globals it reads. The VM is this
// command's (the core, math and strings), or the one a profile describes.
func cmdCheck(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("filo check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	vm := fs.String("vm", "", "a profile of the VM: the names it gives, one a line")
	err := fs.Parse(args)
	if err != nil || fs.NArg() > 1 {
		_, _ = io.WriteString(stderr, help("check"))
		return 2
	}
	var offers map[string]bool
	if *vm != "" {
		text, err := os.ReadFile(*vm) // #nosec G304 G703 -- the profile the command was given
		if err != nil {
			return complain(stderr, err)
		}
		offers = profile(text)
	}
	path := fs.Arg(0)
	if path == "" {
		path = "-"
	}
	data, err := read(path, stdin)
	if err != nil {
		return complain(stderr, err)
	}
	ps, _, err := parts(path, data)
	if err != nil {
		return complain(stderr, err)
	}
	code := 0
	for _, p := range ps {
		lack, imports, externs, err := lacking(p.data, offers)
		if err != nil {
			return complain(stderr, fmt.Errorf("%s: %w", p.name, err))
		}
		if len(lack) > 0 {
			code = 1
			_, _ = fmt.Fprintf(stdout, "%s  lacks %d: %s\n", p.name, len(lack), strings.Join(lack, ", "))
			continue
		}
		_, _ = fmt.Fprintf(stdout, "%s  runs: %d imports, %d externs\n", p.name, imports, externs)
	}
	return code
}

// profile is the names a VM's profile lists: one a line, "#" comments.
func profile(text []byte) map[string]bool {
	names := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			names[line] = true
		}
	}
	return names
}

// lacking is what a unit asks for that the VM does not give, the functions
// first, and how many of each it asks for.
func lacking(data []byte, offers map[string]bool) ([]string, int, int, error) {
	u, err := fbc.Read(data)
	if err != nil {
		return nil, 0, 0, err
	}
	if offers == nil {
		lu, err := engine().LoadUnit(data)
		if err != nil {
			return nil, 0, 0, err
		}
		return lu.Missing(nil), len(u.Imports), len(u.Externs), nil
	}
	var lack []string
	for _, name := range u.Imports {
		if !offers[name] {
			lack = append(lack, name)
		}
	}
	for i, name := range u.Globals {
		if u.Externs[i] && !offers[name] {
			lack = append(lack, name)
		}
	}
	return lack, len(u.Imports), len(u.Externs), nil
}

// cmdSize says where a file's bytes go: each unit's header and sections.
func cmdSize(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 1 {
		_, _ = io.WriteString(stderr, help("size"))
		return 2
	}
	path := "-"
	if len(args) == 1 {
		path = args[0]
	}
	data, err := read(path, stdin)
	if err != nil {
		return complain(stderr, err)
	}
	ps, table, err := parts(path, data)
	if err != nil {
		return complain(stderr, err)
	}
	w := bufio.NewWriter(stdout)
	if fbc.Kind(data) == fbc.KindBundle {
		_, _ = fmt.Fprintf(w, "%s  %d bytes: %d members, %d of header and table\n",
			filepath.Base(path), len(data), len(ps), table)
	}
	for _, p := range ps {
		u, err := fbc.Read(p.data)
		if err != nil {
			return complain(stderr, fmt.Errorf("%s: %w", p.name, err))
		}
		_, _ = fmt.Fprintf(w, "%s  %d bytes\n", p.name, len(p.data))
		_, _ = fmt.Fprintf(w, "  %-10s %7d\n", "header", u.HeaderSize)
		rest := len(p.data) - u.HeaderSize
		for _, s := range u.Sections {
			name := fmt.Sprintf("kind %d", s.Kind)
			if s.Kind < len(fbc.SectionNames) && fbc.SectionNames[s.Kind] != "" {
				name = fbc.SectionNames[s.Kind]
			}
			_, _ = fmt.Fprintf(w, "  %-10s %7d\n", name, s.Len)
			rest -= s.Len
		}
		if rest != 0 {
			_, _ = fmt.Fprintf(w, "  %-10s %7d\n", "between", rest)
		}
	}
	err = w.Flush()
	if err != nil {
		return complain(stderr, err)
	}
	return 0
}
