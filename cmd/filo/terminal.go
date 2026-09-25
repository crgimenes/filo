package main

import (
	"errors"
	"io"
	"os"
	"time"

	"golang.org/x/term"
)

// debugTerminal runs the session on the terminal: the alternate screen in
// raw mode, a paint after every key and every change of size, until quit.
func debugTerminal(s *session, in *os.File, out *os.File) error {
	inFd, outFd := int(in.Fd()), int(out.Fd()) // #nosec G115 -- a file descriptor fits an int
	if !term.IsTerminal(inFd) || !term.IsTerminal(outFd) {
		return errors.New("debug needs a terminal")
	}
	old, err := term.MakeRaw(inFd)
	if err != nil {
		return err
	}
	defer func() { _ = term.Restore(inFd, old) }()
	_, _ = io.WriteString(out, "\x1b[?1049h\x1b[?25l")
	defer func() { _, _ = io.WriteString(out, "\x1b[0m\x1b[?25h\x1b[?1049l") }()

	keys := make(chan []byte)
	go func() {
		buf := make([]byte, 64)
		for {
			n, err := in.Read(buf)
			if err != nil {
				close(keys)
				return
			}
			keys <- append([]byte(nil), buf[:n]...)
		}
	}()
	tick := time.NewTicker(250 * time.Millisecond) // the size, without a signal Windows lacks
	defer tick.Stop()
	scr := &screen{}
	for {
		w, h, err := term.GetSize(outFd)
		if err != nil {
			return err
		}
		scr.resize(w, h)
		draw(s, scr)
		err = scr.flush(out)
		if err != nil {
			return err
		}
		select {
		case k, ok := <-keys:
			if !ok || !s.key(k) {
				return nil
			}
		case <-tick.C:
		}
	}
}

// key acts on what a key sent; false to quit.
func (s *session) key(k []byte) bool {
	if len(k) == 0 {
		return true
	}
	s.msg = ""
	if s.run.Done() {
		s.msg = s.ending()
	}
	switch string(k) {
	case "q", "Q", "\x1b", "\x03": // Esc alone, or Ctrl-C
		return false
	case "s":
		s.line(false)
	case "n":
		s.line(true)
	case "i":
		s.instruction()
	case "c":
		s.finish()
	case "b":
		s.back()
	case "r":
		s.startOver()
	case " ":
		s.toggleBreak()
	case "\x1b[A", "\x1bOA":
		s.moveCursor(-1)
	case "\x1b[B", "\x1bOB":
		s.moveCursor(1)
	case "\x1b[5~":
		s.moveCursor(-10)
	case "\x1b[6~":
		s.moveCursor(10)
	}
	return true
}
