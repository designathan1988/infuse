//go:build !windows && !cgo

package desktop

// Without cgo there is no WKWebView/WebKitGTK binding; this build serves the UI only (`infuse serve`).

import (
	"fmt"
	"os"
)

func RunWindow(url string) {
	fmt.Fprintln(os.Stderr, "infuse: esta compilação não inclui a janela nativa (compile com CGO_ENABLED=1). Interface em "+url)
	select {}
}
