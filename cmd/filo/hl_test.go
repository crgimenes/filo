package main

import "testing"

// The classes filo-term's hl.c gives the same lines (checked against it on
// every .filo of the projects): 0 plain, 1 comment, 2 string, 3 number,
// 4 keyword.
func TestHighlightAsTheEdt(t *testing.T) {
	cases := []struct {
		line   string
		open   bool // a string open at the start
		want   string
		isOpen bool // and at the end
	}{
		{`(def fib (fn (n) "a;b" -5 x2 10))`, false, "044400000044000002222200000003300", false},
		{`(if-x deff def? 1.5e3) ; c "q"`, false, "000000000000000033333001111111", false},
		{`(set s "multi`, false, "0444000222222", true},
		{`line \" end" 12`, true, "222222222222033", false},
	}
	for _, c := range cases {
		cls, open := hlClasses(c.line, c.open)
		got := make([]byte, len(cls))
		for i, k := range cls {
			got[i] = '0' + k
		}
		if string(got) != c.want || open != c.isOpen {
			t.Errorf("%s\n got %s %v\nwant %s %v", c.line, got, open, c.want, c.isOpen)
		}
	}
}
