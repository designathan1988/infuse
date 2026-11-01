package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSealedFilesRejectTampering(t *testing.T) {
	testHome(t)
	p := filepath.Join(t.TempDir(), "approvals.json")
	m := map[string]bool{"hook": true}
	if err := SealJSON(p, m); err != nil {
		t.Fatal(err)
	}
	var out map[string]bool
	if err := OpenJSON(p, &out); err != nil || !out["hook"] {
		t.Fatalf("%v %+v", err, out)
	}
	// rewriting the data without the key invalidates the file
	if err := os.WriteFile(p, []byte(`{"data":{"hook":true,"other":true},"sig":"00"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out = nil
	if err := OpenJSON(p, &out); err == nil {
		t.Fatal("a tampered file was accepted")
	}
	// a missing file is simply nothing approved
	if err := OpenJSON(filepath.Join(t.TempDir(), "missing.json"), &out); err != nil {
		t.Fatal(err)
	}
}
