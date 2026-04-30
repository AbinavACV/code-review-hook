package tokens

import "testing"

func TestEstimate(t *testing.T) {
	cases := []struct {
		name string
		in   string
		min  int
		max  int
	}{
		{"empty", "", 0, 0},
		{"single word", "hello", 1, 2},
		{"short sentence", "the quick brown fox jumps", 5, 8},
		{"code block", "func foo(ctx context.Context, x int) (string, error) { return \"\", nil }", 15, 25},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Estimate(c.in)
			if got < c.min || got > c.max {
				t.Fatalf("Estimate(%q) = %d, want %d..%d", c.in, got, c.min, c.max)
			}
		})
	}
}
