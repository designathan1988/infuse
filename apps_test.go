package main

import "testing"

func TestDetectAppsFromStartList(t *testing.T) {
	list := []startApp{
		{Name: "Claude", AppID: "Chrome._crx_abc"}, // PWA shortcut: skipped
		{Name: "Claude", AppID: "Claude_xyz!Claude"},
		{Name: "ChatGPT", AppID: "OpenAI.ChatGPT_abc!App"},
		{Name: "ChatGPT Helper", AppID: "Other"},
	}
	apps := detectApps(list)
	byID := map[string]App{}
	for _, a := range apps {
		byID[a.ID] = a
	}
	if c := byID["claude-desktop"]; !c.Found || c.AppID != "Claude_xyz!Claude" || c.Path != `shell:AppsFolder\Claude_xyz!Claude` {
		t.Fatalf("claude: %+v", c)
	}
	if g := byID["chatgpt-desktop"]; !g.Found || g.AppID != "OpenAI.ChatGPT_abc!App" {
		t.Fatalf("chatgpt: %+v", g)
	}
	if _, err := LaunchApp("nope", ""); err == nil {
		t.Fatal("unknown app accepted")
	}
}
