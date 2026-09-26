package fbc

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// The listings in ../testdata/bytecode are the C runtime's filo dump of the
// units beside them: the Go listing is held to them byte for byte.
func TestListingIsTheCOne(t *testing.T) {
	for _, name := range []string{"prog", "stripped", "demo"} {
		ext := ".fbc"
		if name == "demo" {
			ext = ".fbb"
		}
		data := readFile(t, "../testdata/bytecode/"+name+ext)
		want := readFile(t, "../testdata/bytecode/"+name+".dump")
		var got bytes.Buffer
		var err error
		switch Kind(data) {
		case KindUnit:
			var u *Unit
			u, err = Read(data)
			if err == nil {
				err = u.Dump(&got)
			}
		case KindBundle:
			var b *Bundle
			b, err = ReadBundle(data)
			if err == nil {
				err = b.Dump(&got)
			}
		}
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got.Bytes(), want) {
			t.Errorf("%s: the listing differs from the C one:\n%s", name, got.String())
		}
	}
}

func TestInstructionsAndPlaces(t *testing.T) {
	u, err := Read(readFile(t, "../testdata/bytecode/prog.fbc"))
	if err != nil {
		t.Fatal(err)
	}
	// fn 4 is half's body: (/ n 2) on line 1
	f := u.Fns[4]
	in := u.Insn(f.Off + 2)
	if in.Op != OpCallB || in.Len != 2 || in.Operands != "2 5" || in.Note != "/" {
		t.Fatalf("got %+v", in)
	}
	line, col, ok := u.Position(in.PC)
	if !ok || line != 1 || col != 19 {
		t.Fatalf("position %d:%d %v, want 1:19", line, col, ok)
	}
	if u.Insn(u.Code.Len).Len != 0 {
		t.Fatal("an instruction past the code read")
	}
	if !u.Externs[2] || u.Globals[2] != "base" {
		t.Fatalf("globals %v, externs %v", u.Globals, u.Externs)
	}
	s, err := Read(readFile(t, "../testdata/bytecode/stripped.fbc"))
	if err != nil {
		t.Fatal(err)
	}
	_, _, ok = s.Position(0)
	if ok {
		t.Fatal("a stripped unit says where")
	}
}

func TestDamageIsReported(t *testing.T) {
	data := readFile(t, "../testdata/bytecode/prog.fbc")
	bad := bytes.Clone(data)
	bad[len(bad)-1] ^= 0x40
	u, err := Read(bad)
	if err != nil || u.ChecksumOK {
		t.Fatalf("a wrong checksum: %v, ok %v; want it read and reported", err, u != nil && u.ChecksumOK)
	}
	for _, c := range []struct {
		data []byte
		why  string
	}{
		{[]byte("(+ 1 2)"), "no \\x7fFBC"},
		{readFile(t, "../testdata/bytecode/demo.fbb"), "kind is not 1"},
		{data[:19], "no \\x7fFBC"},
	} {
		_, err = Read(c.data)
		if err == nil || !strings.Contains(err.Error(), c.why) {
			t.Errorf("got %v, want %q", err, c.why)
		}
	}
}

// FuzzReadAndList feeds units and bundles, the checksum put right so a
// mutation reaches the sections: a damaged one is refused or listed, and
// never takes the host down.
func FuzzReadAndList(f *testing.F) {
	for _, name := range []string{"prog.fbc", "stripped.fbc", "upper.fbc", "demo.fbb"} {
		f.Add(readFile(f, "../testdata/bytecode/"+name))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) >= header {
			sum := checksum(data)
			data[8], data[9], data[10], data[11] = byte(sum), byte(sum>>8), byte(sum>>16), byte(sum>>24)
		}
		var out bytes.Buffer
		u, err := Read(data)
		if err == nil {
			_ = u.Dump(&out)
			for pc := 0; pc < u.Code.Len; pc++ {
				_ = u.Insn(pc).String()
				_, _, _ = u.Position(pc)
			}
		}
		b, err := ReadBundle(data)
		if err == nil {
			_ = b.Dump(&out)
		}
	})
}

func readFile(t testing.TB, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
