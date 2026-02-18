package gitactivity

import "testing"

func TestParseNumstatValue(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"10", 10},
		{"0", 0},
		{" - ", 0},
		{"abc", 0},
		{"", 0},
	}

	for _, tt := range tests {
		got := parseNumstatValue(tt.input)
		if got != tt.want {
			t.Fatalf("parseNumstatValue(%q)=%d want=%d", tt.input, got, tt.want)
		}
	}
}
