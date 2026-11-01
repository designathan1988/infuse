//go:build !windows

package config

// macOS: the login Keychain through `security`. Linux: libsecret through `secret-tool`.

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

type osSecrets struct{}

func (osSecrets) Set(name, value string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("security", "add-generic-password", "-U", "-a", "infuse", "-s", name, "-w", value).Run()
	}
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return errors.New("secret-tool (libsecret) not found")
	}
	cmd := exec.Command("secret-tool", "store", "--label="+name, "service", name, "account", "infuse")
	cmd.Stdin = strings.NewReader(value)
	return cmd.Run()
}

func (osSecrets) Get(name string) (string, error) {
	var out []byte
	var err error
	if runtime.GOOS == "darwin" {
		out, err = exec.Command("security", "find-generic-password", "-a", "infuse", "-s", name, "-w").Output()
	} else {
		out, err = exec.Command("secret-tool", "lookup", "service", name, "account", "infuse").Output()
	}
	if err != nil {
		return "", nil // not found
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

func (osSecrets) Delete(name string) error {
	if runtime.GOOS == "darwin" {
		_ = exec.Command("security", "delete-generic-password", "-a", "infuse", "-s", name).Run()
		return nil
	}
	_ = exec.Command("secret-tool", "clear", "service", name, "account", "infuse").Run()
	return nil
}
