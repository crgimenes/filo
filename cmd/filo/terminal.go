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

// helpKey scrolls the h page, or leaves it; Ctrl-C still quits.
func (s *session) helpKey(k string) bool {
	page := max(s.helpPage-1, 1)
	switch k {
	case "\x03":
		return false
	case "h", "q", "Q", "\x1b":
		s.help = false
	case "\x1b[A", "\x1bOA":
		s.helpTop--
	case "\x1b[B", "\x1bOB":
		s.helpTop++
	case "\x1b[5~", "b":
		s.helpTop -= page
	case "\x1b[6~", " ":
		s.helpTop += page
	}
	s.helpTop = max(s.helpTop, 0)
	return true
}

// key acts on what a key sent; false to quit.
func (s *session) key(k []byte) bool {
	if len(k) == 0 {
		return true
	}
	if s.help {
		return s.helpKey(string(k))
	}
	s.msg = ""
	if s.run.Done() {
		s.msg = s.ending()
	}
	switch string(k) {
	case "h":
		s.help, s.helpTop = true, 0
	case "x":
		s.bytes = !s.bytes
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
