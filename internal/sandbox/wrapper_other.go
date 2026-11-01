//go:build !linux

package sandbox

import (
	"fmt"
	"os"
)

func supportedArch() bool { return false }

// RunWrapper is never reached off Linux: Wrap leaves the command untouched there.
func RunWrapper([]string) int {
	fmt.Fprintln(os.Stderr, "[infuse] sandbox: no wrapper on this platform")
	return 126
}
