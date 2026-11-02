//go:build windows

package config

import (
	"bytes"
	"os"
	"testing"
)

func TestSecretFileRoundTripKeepsNoPlainBytes(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	store := osSecrets{}
	name := "infuse-test-" + t.Name()
	if err := store.Set(name, "sk-plain-value"); err != nil {
		t.Fatal(err)
	}
	if v, err := store.Get(name); err != nil || v != "sk-plain-value" {
		t.Fatalf("%q %v", v, err)
	}
	b, err := os.ReadFile(secretFile(name))
	if err != nil || bytes.Contains(b, []byte("sk-plain-value")) {
		t.Fatalf("the key is on disk in the clear: %v", err)
	}
	if err := store.Delete(name); err != nil {
		t.Fatal(err)
	}
	if v, _ := store.Get(name); v != "" {
		t.Fatalf("deleted secret still reads %q", v)
	}
}
