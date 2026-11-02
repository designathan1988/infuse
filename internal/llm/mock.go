package llm

// The mock provider plays a scripted agent: it reacts to the last user text with deterministic tool
// calls, so the whole loop (streaming, permissions, the filter, checkpoints) runs without a key.
// Its "demo" flow follows the protocol infuse enforces — list, read, diagnose, run, write — so the
// demonstration shows a complete turn instead of stopping at the diagnosis gate.

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"infuse/internal/core"
)

type mockProvider struct{}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Stream(ctx context.Context, req ModelRequest, emit func(StreamEvent)) (ModelResult, error) {
	if req.ForceTool != "" { // a forced structured answer (askJSON): the empty answer is valid and honest
		in := json.RawMessage("{}")
		emit(StreamEvent{Kind: "tool_start", ID: "tu_f", Name: req.ForceTool})
		emit(StreamEvent{Kind: "tool_input", ID: "tu_f", Text: "{}"})
		emit(StreamEvent{Kind: "tool_end", ID: "tu_f", Name: req.ForceTool})
		return ModelResult{Content: []Block{{Type: "tool_use", ID: "tu_f", Name: req.ForceTool, Input: in}}, StopReason: "tool_use", Usage: Usage{Input: 300, Output: 5}}, nil
	}
	d := &mockDialogue{req: req, emit: emit}
	if d.isReview() { // the independent reviewer is played by the same scripted model
		return d.say("VERDICT: APPROVE")
	}
	if d.lastIsToolResult() {
		r := d.last()
		if d.isMission() {
			if res, ok := d.missionStepToolResult(r); ok {
				return res, nil
			}
		} else {
			if name, input, ok := d.demoStep(); ok {
				return d.call(name, input)
			}
			if strings.Contains(r.Content, "ls-step") {
				return d.call("Write", map[string]any{"path": "src/demo.ts", "content": "export const demo = () => `ok ${1}`;\n"})
			}
		}
		return d.say("Done. The tool returned: " + core.Truncate(strings.TrimSpace(r.Content), 160))
	}
	text := ""
	for _, b := range d.lastBlocks() {
		if b.Type == "text" {
			text += b.Text
		}
	}
	if d.isMission() {
		if strings.Contains(text, "[Mission") {
			return d.call("Diagnosis", d.missionDiagnosis())
		}
		return d.say("The scripted provider cannot make this criterion true.")
	}
	if strings.HasPrefix(strings.TrimSpace(text), "[infuse]") { // infuse's own note is not a user request
		return d.say("The scripted provider cannot make this criterion true.")
	}
	switch {
	case mockWants(text, "demo"):
		return d.call("Bash", map[string]any{"command": "echo ls-step && ls"})
	case mockWants(text, "heredoc"):
		return d.call("Bash", map[string]any{"command": "cat > src/h.ts <<EOF\nexport const h = `x${y}`;\nEOF\necho feito"})
	case mockWants(text, "oracle") || mockWants(text, "oráculo"):
		return d.call("Write", map[string]any{"path": ".infuse/oracle/done.md", "content": "hack"})
	case mockWants(text, "pergunta"):
		return d.call("AskUserQuestion", map[string]any{"question": "Which approach?", "options": []string{"A", "B"}})
	case mockWants(text, "browser"):
		return d.call("Browser", map[string]any{"action": "navigate", "url": "http://localhost:7471/"})
	}
	return d.say("Hello. I am the demonstration provider: ask for “demo”, “heredoc”, “oracle” or “question” to see the filter act.")
}

func mockWants(text, word string) bool { return strings.Contains(strings.ToLower(text), word) }

// mockDialogue reads the conversation and answers the next scripted step.
type mockDialogue struct {
	req  ModelRequest
	emit func(StreamEvent)
}

func (d *mockDialogue) lastBlocks() []Block {
	msgs := d.req.Messages
	if len(msgs) == 0 {
		return nil
	}
	return msgs[len(msgs)-1].Content
}

func (d *mockDialogue) last() Block {
	bs := d.lastBlocks()
	if len(bs) == 0 {
		return Block{}
	}
	return bs[0]
}

func (d *mockDialogue) lastIsToolResult() bool {
	l := d.last()
	return l.Type == "tool_result"
}

// missionDiagnosis is evidence-free on purpose: a mission starts on a project with no citable text,
// and the bootstrap case allows the diagnosis without quotes.
// missionStepToolResult answers the tools a mission's first steps run: the diagnosis is followed by
// a plan, and the plan's open tasks are then marked blocked when the goal cannot be met.
func (d *mockDialogue) missionStepToolResult(r Block) (ModelResult, bool) {
	if strings.Contains(r.Content, "diagnosis registered") {
		res, err := d.call("TodoWrite", map[string]any{"tasks": []map[string]any{
			{"id": "1", "text": "make the mission goal true", "status": "in_progress", "proof": "the Done? criteria pass"},
		}})
		return res, err == nil
	}
	if strings.Contains(r.Content, "plan recorded") {
		if blocked, ok := d.blockTodo(); ok {
			res, err := d.call("TodoWrite", blocked)
			return res, err == nil
		}
		res, err := d.say("Nothing further can be done without the missing information; the plan is blocked.")
		return res, err == nil
	}
	return ModelResult{}, false
}

func (d *mockDialogue) missionDiagnosis() map[string]any {
	return map[string]any{
		"problem":    "the mission asks for a file that does not exist yet",
		"root_cause": "the project is empty, so there is no existing file to quote as evidence",
		"approach":   "bootstrap the project and then create the requested file",
		"why_cause":  "the missing file is the whole scope of this mission",
		"evidence":   []map[string]string{},
	}
}

// blockTodo marks the latest plan's open tasks as blocked, once. Without reading the plan back, a
// reply to the "plan recorded" result would keep re-submitting it and never let the turn end.
func (d *mockDialogue) blockTodo() (map[string]any, bool) {
	for i := len(d.req.Messages) - 1; i >= 0; i-- {
		for _, b := range d.req.Messages[i].Content {
			if b.Type != "tool_use" || b.Name != "TodoWrite" {
				continue
			}
			var in struct {
				Tasks []struct{ ID, Text, Status, Proof string }
			}
			if json.Unmarshal(b.Input, &in) != nil {
				return nil, false
			}
			open := false
			tasks := make([]map[string]any, 0, len(in.Tasks))
			for _, t := range in.Tasks {
				if t.Status != "done" && t.Status != "blocked" {
					open, t.Status = true, "blocked"
				}
				tasks = append(tasks, map[string]any{"id": t.ID, "text": t.Text, "status": t.Status, "proof": t.Proof})
			}
			if !open {
				return nil, false
			}
			return map[string]any{"tasks": tasks}, true
		}
	}
	return nil, false
}

func (d *mockDialogue) say(s string) (ModelResult, error) {
	for _, ch := range strings.SplitAfter(s, " ") {
		d.emit(StreamEvent{Kind: "text", Text: ch})
		time.Sleep(8 * time.Millisecond)
	}
	return ModelResult{Content: []Block{{Type: "text", Text: s}}, StopReason: "end_turn", Usage: Usage{Input: 1200, Output: len(s) / 4}}, nil
}

func (d *mockDialogue) call(name string, input any) (ModelResult, error) {
	in, _ := json.Marshal(input)
	id := "tu_" + name
	d.emit(StreamEvent{Kind: "tool_start", ID: id, Name: name})
	d.emit(StreamEvent{Kind: "tool_input", ID: id, Text: string(in)})
	d.emit(StreamEvent{Kind: "tool_end", ID: id, Name: name})
	return ModelResult{Content: []Block{{Type: "text", Text: "Working on it."}, {Type: "tool_use", ID: id, Name: name, Input: in}}, StopReason: "tool_use", Usage: Usage{Input: 1500, Output: 60}}, nil
}

// demoStep is the protocol the demo follows, one tool per model turn: LS, Read, Diagnosis, Bash,
// Write. It stops when every step has run, so a second "demo" message just answers.
func (d *mockDialogue) demoStep() (string, map[string]any, bool) {
	if !d.isDemo() {
		return "", nil, false
	}
	counts := map[string]int{}
	for _, msg := range d.req.Messages {
		for _, b := range msg.Content {
			if b.Type == "tool_use" {
				counts[b.Name]++
			}
		}
	}
	switch {
	case counts["LS"] == 0:
		return "LS", map[string]any{"path": "."}, true
	case counts["Read"] == 0:
		return "Read", map[string]any{"path": d.demoFile()}, true
	case counts["Diagnosis"] == 0:
		return "Diagnosis", d.demoDiagnosis(), true
	case counts["Bash"] == 0:
		return "Bash", map[string]any{"command": "echo ls-step && ls"}, true
	case counts["TodoWrite"] == 0:
		return "TodoWrite", map[string]any{"tasks": []map[string]any{
			{"id": "1", "text": "Create the demo file", "status": "in_progress", "proof": "src/demo.ts exists and exports demo"},
			{"id": "2", "text": "Confirm the turn closes with every task done", "status": "pending", "proof": "the plan shows both tasks done"},
		}}, true
	case counts["Contract"] == 0: // multi-step work: the user confirms the restatement first
		return "Contract", map[string]any{
			"goal":     "Create the demo file inside the project and close the turn with a verified report.",
			"criteria": []string{"src/demo.ts exists and exports demo", "every claim in the report matches what ran"},
		}, true
	case counts["Write"] == 0:
		return "Write", map[string]any{"path": "src/demo.ts", "content": "export const demo = () => `ok ${1}`;\n"}, true
	case counts["TodoWrite"] == 1:
		return "TodoWrite", map[string]any{"tasks": []map[string]any{
			{"id": "1", "text": "Create the demo file", "status": "done", "proof": "src/demo.ts exists and exports demo"},
			{"id": "2", "text": "Confirm the turn closes with every task done", "status": "done", "proof": "the plan shows both tasks done"},
		}}, true
	case counts["Report"] == 0:
		return "Report", map[string]any{
			"summary": "Created src/demo.ts from the project's own documentation.",
			"claims": []map[string]any{
				{"text": "src/demo.ts was created", "kind": "file", "ref": "src/demo.ts"},
				{"text": "Every task in the plan is done", "kind": "other"},
			},
		}, true
	}
	return "", nil, false
}

func (d *mockDialogue) isReview() bool {
	for _, msg := range d.req.Messages {
		for _, b := range msg.Content {
			if b.Type == "text" && strings.Contains(b.Text, "VERDICT:") {
				return true
			}
		}
	}
	return false
}

func (d *mockDialogue) isDemo() bool {
	for _, msg := range d.req.Messages {
		if msg.Role != "user" {
			continue
		}
		for _, b := range msg.Content {
			if b.Type == "text" && mockWants(b.Text, "demo") {
				return true
			}
		}
	}
	return false
}

// isMission keeps the mission script out of the demo: both run on the same provider, and the demo
// has its own multi-step protocol.
func (d *mockDialogue) isMission() bool {
	for _, msg := range d.req.Messages {
		if msg.Role != "user" {
			continue
		}
		for _, b := range msg.Content {
			if b.Type == "text" && strings.Contains(b.Text, "[Mission") {
				return true
			}
		}
	}
	return false
}

// demoFile picks the documentation file first: a diagnosis must cite what the project already knows.
func (d *mockDialogue) demoFile() string {
	for _, msg := range d.req.Messages {
		for _, b := range msg.Content {
			if b.Type != "tool_result" {
				continue
			}
			for _, line := range strings.Split(b.Content, "\n") {
				name := strings.TrimSpace(line)
				if name == "" || strings.HasSuffix(name, "/") || strings.ContainsAny(name, " \t") {
					continue
				}
				if strings.HasSuffix(strings.ToLower(name), ".md") {
					return name
				}
			}
		}
	}
	return "README.md"
}

func (d *mockDialogue) demoDiagnosis() map[string]any {
	file := d.demoFile()
	q1, q2 := mockQuotes(d.last().Content)
	return map[string]any{
		"problem":    "The demo asks for a small file to be created in the project.",
		"root_cause": "The project has no demo file yet, so there is nothing to show.",
		"approach":   "Create src/demo.ts exporting a small function built from the project's own conventions.",
		"why_cause":  "Adding the missing file removes the cause directly instead of working around one symptom.",
		"evidence": []map[string]any{
			{"file": file, "quote": q1, "supports": "what the project documents about its own purpose"},
			{"file": file, "quote": q2, "supports": "the conventions the demo file should follow"},
		},
	}
}

// mockQuotes extracts two verbatim lines from a Read result (numbered "N\tline").
func mockQuotes(read string) (string, string) {
	var qs []string
	for _, line := range strings.Split(read, "\n") {
		i := strings.IndexByte(line, '\t')
		if i < 0 {
			continue
		}
		text := strings.TrimSpace(line[i+1:])
		if len(strings.Fields(text)) < 3 {
			continue
		}
		qs = append(qs, text)
		if len(qs) == 2 {
			break
		}
	}
	switch len(qs) {
	case 0:
		return "the project has no documentation to quote", "the project has no documentation to quote"
	case 1:
		return qs[0], qs[0]
	}
	return qs[0], qs[1]
}
