//go:build !windows || cgo

package mountwin

import (
	"errors"
	"time"
)

// The mount uses the cgofuse no-cgo Windows host. Release binaries are built with CGO_ENABLED=0;
// other platforms use their own FUSE stack in a later slice.
func Run(point, root string, protected []string, timeout time.Duration) error {
	return errors.New("the project mount is available in the no-cgo Windows build with WinFsp installed")
}
