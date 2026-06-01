package web

import (
	"io/fs"
	"testing"
)

func TestDistHasIndex(t *testing.T) {
	f, err := Dist()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(f, "index.html"); err != nil {
		t.Errorf("index.html missing from embedded dist: %v", err)
	}
}
