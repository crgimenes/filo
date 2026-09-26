package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/fbc"
)

// cmdDecompile writes a unit's entry points back as Filo (fbc.Decompile),
// formatted as filofmt formats: on standard output, each under a "; NAME.filo"
// line when there are more than one, or with -o DIR as DIR/NAME.filo, the
// paths written on standard output in the unit's order — the order filo build
// takes them in to make the same unit again.
func cmdDecompile(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("filo decompile", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	dir := fs.String("o", "", "the directory to write NAME.filo files into")
	err := fs.Parse(args)
	if err != nil || fs.NArg() < 1 || fs.NArg() > 2 {
		_, _ = io.WriteString(stderr, help("decompile"))
		return 2
	}
	data, err := read(fs.Arg(0), stdin)
	if err != nil {
		return complain(stderr, err)
	}
	switch fbc.Kind(data) {
	case fbc.KindBundle:
		data, err = bundleMember(data, fs.Arg(1))
		if err != nil {
			return complain(stderr, err)
		}
	case fbc.KindUnit:
	default:
		return complain(stderr, errors.New("not a unit or a bundle: a source is Filo already"))
	}
	u, err := fbc.Read(data)
	if err != nil {
		return complain(stderr, err)
	}
	srcs, err := fbc.Decompile(u)
	if err != nil {
		return complain(stderr, err)
	}
	for i, s := range srcs {
		text, err := filo.FormatWithConfig(s.Text, filo.DefaultFormatConfig())
		if err != nil {
			return complain(stderr, fmt.Errorf("%s: %w", s.Name, err))
		}
		if *dir != "" {
			path := filepath.Join(*dir, s.Name+".filo")
			code := write(path, []byte(text), stderr)
			if code != 0 {
				return code
			}
			_, _ = fmt.Fprintln(stdout, path)
			continue
		}
		if len(srcs) > 1 {
			if i > 0 {
				_, _ = io.WriteString(stdout, "\n")
			}
			_, _ = fmt.Fprintf(stdout, "; %s.filo\n", s.Name)
		}
		_, _ = io.WriteString(stdout, text)
	}
	return 0
}
