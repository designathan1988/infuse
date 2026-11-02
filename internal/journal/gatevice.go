package journal

// Which vice each refusal stands for. The table is data (internal/data/gate_vices.json): a refusal is
// a message id, the vice is the exact catalog entry, and an id with several causes is found through
// the messages it carries.

import (
	"encoding/json"
	"infuse/internal/core"
	"infuse/internal/data"
	"sync"
)

var gateVices = sync.OnceValue(func() map[string]string {
	out := map[string]string{}
	_ = json.Unmarshal(data.Read("gate_vices.json"), &out)
	return out
})

// GateVices lists every refusal id and its vice.
func GateVices() map[string]string { return gateVices() }

// VicesOf returns the vices a refusal stands for: the innermost messages decide, because a wrapper
// such as "the architecture check refused this" says less than what it carries.
func VicesOf(m core.Msg) []string {
	if v, ok := m.Args["vice"].(string); ok && v != "" { // the gate itself tells two causes apart
		return []string{v}
	}
	var out []string
	for _, inner := range nestedMsgs(m) {
		out = append(out, VicesOf(inner)...)
	}
	if len(out) == 0 {
		if v := gateVices()[m.ID]; v != "" {
			out = append(out, v)
		}
	}
	return dedupe(out)
}

func nestedMsgs(m core.Msg) []core.Msg {
	var out []core.Msg
	for _, a := range m.Args {
		switch v := a.(type) {
		case core.Msg:
			out = append(out, v)
		case []core.Msg:
			out = append(out, v...)
		}
	}
	return out
}

func dedupe(list []string) []string {
	seen := map[string]bool{}
	out := list[:0]
	for _, v := range list {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// Refused records a refusal: the journal entry carries its first vice, the ledger gets every one.
func Refused(root string, e JournalEntry, m core.Msg) {
	list := VicesOf(m)
	if len(list) == 0 {
		Journal(root, e)
		return
	}
	for _, v := range list {
		Observe(root, e, v)
	}
}
