package main

import (
	"context"
	"os"
	"testing"
)

func TestWebSearchLive(t *testing.T) {
	if os.Getenv("INFUSE_LIVE") == "" {
		t.Skip("INFUSE_LIVE not set")
	}
	for _, e := range searchEngines {
		saved := searchEngines
		searchEngines = []searchEngine{e}
		res, _, err := WebSearch(context.Background(), "golang net/http documentation", 5)
		searchEngines = saved
		t.Logf("%s: err=%v n=%d", e.name, err, len(res))
		for _, r := range res {
			t.Logf("  %s | %s", r.Title, r.URL)
		}
	}
}
