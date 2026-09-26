package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/fbc"
)

// cmdBuild compiles FILE... into one unit, as the C runtime's filo build:
// each file an entry named by it ("lib/hello.filo" is the entry "hello"), the
// same bytes the C compiler writes; --strip leaves the debug section out.
func cmdBuild(args []string, stderr io.Writer) int {
	strip := len(args) > 0 && args[0] == "--strip"
	if strip {
		args = args[1:]
	}
	if len(args) < 3 || args[0] != "-o" {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	unit, err := compileFiles(args[2:])
	if err != nil {
		return complain(stderr, err)
	}
	if strip {
		unit, err = filo.StripDebug(unit)
		if err != nil {
			return complain(stderr, err)
		}
	}
	return write(args[1], unit, stderr)
}

// cmdBundle puts units into one bundle, as the C runtime's filo bundle:
// each a member named by its file ("lib/hello.fbc" is the member "hello").
func cmdBundle(args []string, stderr io.Writer) int {
	if len(args) < 3 || args[0] != "-o" {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	var members []filo.BundleMember
	for _, path := range args[2:] {
		data, err := os.ReadFile(path) // #nosec G304 G703 -- a unit the command was given
		if err != nil {
			return complain(stderr, err)
		}
		if fbc.Kind(data) != fbc.KindUnit {
			return complain(stderr, fmt.Errorf("%s: not a unit", path))
		}
		members = append(members, filo.BundleMember{Name: entryName(path, ".fbc"), Unit: data})
	}
	bundle, err := filo.BuildBundle(members)
	if err != nil {
		return complain(stderr, err)
	}
	return write(args[1], bundle, stderr)
}

// compileFiles compiles each file as the entry named by it, in one unit.
func compileFiles(paths []string) ([]byte, error) {
	e := engine()
	var entries []filo.BuildEntry
	for _, path := range paths {
		src, err := os.ReadFile(path) // #nosec G304 G703 -- a source the command was given
		if err != nil {
			return nil, err
		}
		if len(src) == 0 {
			return nil, fmt.Errorf("%s is empty", path)
		}
		p, err := e.Compile(string(src))
		if err != nil {
			return nil, placed(path, src, err)
		}
		entries = append(entries, filo.BuildEntry{Name: entryName(path, ".filo"), Program: p})
	}
	return e.Build(entries)
}

// placed is err with where it happened in front, "path:line:col: ...", as
// the C runtime's filo writes it.
func placed(path string, src []byte, err error) error {
	pe, ok := errors.AsType[*filo.PositionError](err)
	if ok {
		return fmt.Errorf("%s:%d:%d: %w", path, pe.Line, pe.Col, err)
	}
	parse, ok := errors.AsType[*filo.ParseError](err)
	if ok {
		text := strings.TrimPrefix(string(src), "\ufeff") // the offset is into the text after it
		line, col := 1, 1
		for i := 0; i < parse.Pos && i < len(text); i++ {
			col++
			if text[i] == '\n' {
				line, col = line+1, 1
			}
		}
		return fmt.Errorf("%s: parse error at line %d, col %d: %s", path, line, col, parse.Message)
	}
	return fmt.Errorf("%s: %w", path, err)
}

// entryName is a file's name without its directory and ext: "lib/hello.filo"
// is "hello".
func entryName(path, ext string) string {
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, ext)
	if name == "" {
		return base
	}
	return name
}

func write(path string, data []byte, stderr io.Writer) int {
	err := os.WriteFile(path, data, 0o644) // #nosec G306 G703 -- the file the command was told to write, readable as a compiler writes one
	if err != nil {
		return complain(stderr, err)
	}
	return 0
}
