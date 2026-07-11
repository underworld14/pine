package server

import "testing"

func TestAllowedOriginRejectsBadInputs(t *testing.T) {
	cases := []struct {
		origin string
		want   bool
	}{
		{"http://localhost:3000", true},
		{"http://127.0.0.1:8080", true},
		{"http://evil.example.com", false},
		{"not a url with spaces", false}, // url.Parse error
		{"localhost", false},             // no scheme → Host is empty
		{"", false},
	}
	for _, tc := range cases {
		if got := allowedOrigin(tc.origin); got != tc.want {
			t.Errorf("allowedOrigin(%q) = %v, want %v", tc.origin, got, tc.want)
		}
	}
}
