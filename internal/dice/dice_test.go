package dice

import "testing"

func TestNormalizeNotation(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"2d6+3", "2d6+3"},
		{"2d6 3", "2d6+3"},
		{"D20", "d20"},
		{"4d8-2", "4d8-2"},
		{"  2D10 - 2 ", "2d10-2"},
	}
	for _, tc := range cases {
		got, err := NormalizeNotation(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}

	for _, bad := range []string{"", "2d1", "101d6", "d1", "drop table", "2d6+1001", "../etc"} {
		if _, err := NormalizeNotation(bad); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
}

func TestNormalizeCount(t *testing.T) {
	n, err := NormalizeCount(0)
	if err != nil || n != 1 {
		t.Fatalf("default count = %d, %v", n, err)
	}
	if _, err := NormalizeCount(101); err == nil {
		t.Fatal("expected count limit")
	}
}
