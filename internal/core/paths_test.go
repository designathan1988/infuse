package core

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWithinByComponent(t *testing.T) {
	root := filepath.Join("C:", "proj")
	if Within(root, filepath.Join("C:", "proj-2", "x")) {
		t.Fatal("sibling with the same prefix accepted")
	}
	if !Within(root, filepath.Join(root, "a", "b")) || !Within(root, root) {
		t.Fatal("inside rejected")
	}
	if Within(root, filepath.Join(root, "..", "other")) {
		t.Fatal("escape accepted")
	}
}

func TestCapWriterKeepsHeadAndTail(t *testing.T) {
	w := NewCapWriter(10)
	for i := 0; i < 1000; i++ {
		_, _ = w.Write([]byte("0123456789"))
	}
	s := w.String()
	if len(s) > 200 || !strings.HasPrefix(s, "0123456789") || !strings.HasSuffix(s, "0123456789") || !strings.Contains(s, "omitted") {
		t.Fatalf("%q", s)
	}
}
