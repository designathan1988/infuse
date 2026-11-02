//go:build darwin

package sandbox

// macOS containment: sandbox-exec with a Seatbelt profile (seatbelt.go). The tool ships with the
// system; where it is missing the platform reports no containment.

import (
	"fmt"
	"os"
	"os/exec"
)

const sandboxExec = "/usr/bin/sandbox-exec"

func platformAvailable() bool {
	_, err := os.Stat(sandboxExec)
	return err == nil
}

// RunWrapper applies the profile and runs argv. It fails closed, as on every platform.
func RunWrapper(root string, protected []string, argv []string) int {
	profile := seatbeltProfile(root, os.TempDir(), protectedRels(root, protected))
	cmd := exec.Command(sandboxExec, append([]string{"-p", profile}, argv...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "[infuse] the command was not run: OS containment is unavailable here (%v). The user can turn containProcesses off in the settings.\n", err)
		return 126
	}
	return 0
}
