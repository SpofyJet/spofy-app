package pack

import "testing"

func TestVersionsCompareByNumbersAndUnknownOnesDoNot(t *testing.T) {
	for _, c := range []struct {
		a, b  string
		order int
		ok    bool
	}{
		{"0.3.0.120", "0.3.0.120", 0, true},
		{"0.3.0", "0.3.0.0", 0, true},
		{"0.10.0.1", "0.9.0.400", 1, true},
		{"0.2.0.99", "0.3.0.1", -1, true},
		{"0.0.0.2", "0.0.0.1", 1, true},
		{"1.0", "0.99.99.99", 1, true},
		{"dev", "0.3.0.1", 0, false},
		{"0.3.0.1", "", 0, false},
		{"0.3.-1", "0.3.0", 0, false},
		{"0.3.01", "0.3.1", 0, false},
		{"0.3..1", "0.3.0.1", 0, false},
	} {
		order, ok := Compare(c.a, c.b)
		if order != c.order || ok != c.ok {
			t.Errorf("Compare(%q, %q) = %d, %v; want %d, %v", c.a, c.b, order, ok, c.order, c.ok)
		}
	}
}
