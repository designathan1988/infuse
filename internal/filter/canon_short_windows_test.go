//go:build windows

package filter

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func short8dot3(p string) string {
	in, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return ""
	}
	buf := make([]uint16, 1024)
	n, err := windows.GetShortPathName(in, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buf[:n]))
}
