//go:build !windows && !cgo

package desktop

// Without cgo there is no WKWebView/WebKitGTK binding; this build serves the UI only (`infuse serve`).

import (
	"fmt"
	"os"
)

func RunWindow(url string) {
	fmt.Fprintln(os.Stderr, "infuse: this build does not include the native window (build with CGO_ENABLED=1). UI at "+url)
	select {}
}
