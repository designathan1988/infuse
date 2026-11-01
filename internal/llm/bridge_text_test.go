package llm

import "testing"

// A model that keeps answering in prose instead of the bridge's JSON is still answering: the text is
// the reply, and no tool call is invented from it.
func TestBridgeAcceptsAPlainTextAnswer(t *testing.T) {
	r, err := bridgeReplyOrText("ok")
	if err != nil || r.Text != "ok" || len(r.ToolCalls) != 0 {
		t.Fatalf("plain text must become the reply: %+v %v", r, err)
	}
	if _, err := bridgeReplyOrText("   "); err == nil {
		t.Fatal("an empty reply is an error")
	}
	json, err := bridgeReplyOrText(`{"text":"hello","tool_calls":[{"name":"Read","arguments":{"path":"a"}}]}`)
	if err != nil || json.Text != "hello" || len(json.ToolCalls) != 1 {
		t.Fatalf("the protocol answer must still be parsed: %+v %v", json, err)
	}
}
