package selfupdate

import "testing"

func TestNeedsUpgrade(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cur, lat string
		want     bool
	}{
		{"0.7.0", "0.7.1", true},
		{"0.7.1", "0.7.1", false},
		{"0.8.0", "0.7.1", false},
		{"v0.7.0", "v0.7.1", true},
		{"0.1.0-dev", "0.7.1", true},
		{"dev", "0.7.1", true},
		{"", "0.7.1", true},
		{"0.7.1", "", false},
		{"0.7", "0.7.1", true},
		{"1.0.0", "1.0.0", false},
	}
	for _, tc := range cases {
		if got := NeedsUpgrade(tc.cur, tc.lat); got != tc.want {
			t.Errorf("NeedsUpgrade(%q, %q) = %v, want %v", tc.cur, tc.lat, got, tc.want)
		}
	}
}

func TestNormalizeVersion(t *testing.T) {
	t.Parallel()
	if got := NormalizeVersion(" v1.2.3 "); got != "1.2.3" {
		t.Fatalf("got %q", got)
	}
}
