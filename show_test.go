package filo

import (
	"os"
	"strings"
	"testing"
)

// Show writes what the C runtime's filo show writes: these are its outputs
// for two of the examples, kept in the C repository's testdata/cli.
func TestShowIsTheCRuntimes(t *testing.T) {
	cases := []struct {
		file, stage, want string
	}{
		{"examples/double.filo", "ir", `2:1     let
2:1       def double
2:13        fn  names x
2:21          callb *
2:24            local x  (frame 0 out, slot 0)
2:26            const 2
3:1       call
3:2         global double
3:9         const 21`},
		{"examples/constants.filo", "tree", `2:1     list of 3
2:2       symbol +
2:4       list of 3
2:5         symbol *
2:7         number 2
2:9         number 3
2:12      list of 4
2:13        symbol if
2:16        list of 3
2:17          symbol <
2:19          number 1
2:21          number 2
2:24        number 4
2:26        number 5`},
		{"examples/constants.filo", "folded", `2:1     number 10`},
	}
	for _, c := range cases {
		src, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		lines, err := NewEngine().Show(string(src), c.stage)
		if err != nil || strings.Join(lines, "\n") != c.want {
			t.Errorf("%s %s (%v):\n%s", c.stage, c.file, err, strings.Join(lines, "\n"))
		}
	}
	_, err := NewEngine().Show("1", "tokens")
	if err == nil || err.Error() != "no stage named tokens" {
		t.Fatalf("an unknown stage: %v", err)
	}
}
