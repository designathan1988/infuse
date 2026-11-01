//go:build windows && !cgo

package mountwin

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/winfsp/cgofuse/fuse"
)

// Run mounts root at point and blocks until Ctrl+C, the optional timeout, or an external unmount. A
// missing WinFsp is an error, never a silent fallback to the real tree.
func Run(point, root string, protected []string, timeout time.Duration) (err error) {
	fs, err := New(root, protected)
	if err != nil {
		return err
	}
	host := fuse.NewFileSystemHost(fs)
	host.SetCapCaseInsensitive(true)
	host.SetCapReaddirPlus(true)
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("WinFsp: %v (is WinFsp installed?)", r)
		}
	}()
	if timeout > 0 {
		time.AfterFunc(timeout, func() { host.Unmount() })
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	defer signal.Stop(stop)
	go func() {
		<-stop
		host.Unmount()
	}()
	if !host.Mount(point, nil) {
		return fmt.Errorf("WinFsp could not mount %s", point)
	}
	return nil
}
