//go:build linux && !amd64 && !arm64

package sandbox

import (
	"fmt"
	"os"
	"runtime"
)

func supportedArch() bool { return false }

// RunWrapper is never reached: Wrap leaves the command untouched on an architecture whose Landlock
// syscall numbers are not known here.
func RunWrapper([]string) int {
	fmt.Fprintf(os.Stderr, "[infuse] sandbox: unsupported architecture %s\n", runtime.GOARCH)
	return 126
}
