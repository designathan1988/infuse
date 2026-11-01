//go:build linux

package sandbox

// The Linux sandbox runs the command through this binary's hidden wrapper mode, which applies a
// Landlock ruleset to itself and then execs the target. Landlock is inherited across exec.

import (
	"encoding/json"
	"os"
)

func Wrap(argv []string, root string, protected []string) []string {
	if len(argv) == 0 || root == "" || !supportedArch() {
		return argv
	}
	self, err := os.Executable()
	if err != nil {
		return argv
	}
	b, _ := json.Marshal(protected)
	return append([]string{self, "__sandbox", root, string(b), "--"}, argv...)
}
