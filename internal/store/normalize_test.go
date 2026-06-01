package store

import "testing"

func TestNormalizeTag(t *testing.T) {
	cases := map[string]string{
		"  React ":   "react",
		"React":      "react",
		"front  end": "front end",
		"FRONTEND":   "frontend",
		"前端":         "前端",
		"":           "",
	}
	for in, want := range cases {
		if got := NormalizeTag(in); got != want {
			t.Errorf("NormalizeTag(%q) = %q, want %q", in, got, want)
		}
	}
}
