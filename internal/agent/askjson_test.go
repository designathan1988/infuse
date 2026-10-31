package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// A malformed answer goes back to the model with the parser's error, as the result of its own call; the
// second, valid answer is the one kept. The request always forces the one tool.
func TestAskJSONReturnsTheErrorToTheModelAndKeepsTheValidAnswer(t *testing.T) {
	var seen []ModelRequest
	p := &scriptProvider{steps: []func(ModelRequest) (ModelResult, error){
		func(r ModelRequest) (ModelResult, error) {
			seen = append(seen, r)
			return ModelResult{Content: []Block{{Type: "tool_use", ID: "a", Name: "Facts", Input: json.RawMessage(`{"facts":[{"line":1,_kind:"rule"}]}`)}}, StopReason: "tool_use"}, nil
		},
		func(r ModelRequest) (ModelResult, error) {
			seen = append(seen, r)
			return ModelResult{Content: []Block{{Type: "tool_use", ID: "b", Name: "Facts", Input: json.RawMessage(`{"facts":[{"line":3,"kind":"rule","fact":"x","quote":"y"}]}`)}}, StopReason: "tool_use"}, nil
		},
	}}
	var out struct{ Facts []knowledgeFact }
	if _, err := askJSON(context.Background(), p, "m", "sys", "doc", factsSpec, 100, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Facts) != 1 || out.Facts[0].Line != 3 {
		t.Fatalf("%+v", out)
	}
	if len(seen) != 2 || seen[0].ForceTool != "Facts" || len(seen[0].Tools) != 1 {
		t.Fatalf("requests: %+v", seen)
	}
	retry := seen[1].Messages
	last := retry[len(retry)-1].Content[0]
	if last.Type != "tool_result" || last.ToolUseID != "a" || !last.IsError || !strings.Contains(last.Content, "invalid character") {
		t.Fatalf("the error did not go back as the call's result: %+v", last)
	}
}

// An answer that never parses is an error for the caller, never an empty result read as "nothing found".
func TestAskJSONNeverTurnsAFailureIntoAnEmptyResult(t *testing.T) {
	p := &scriptProvider{steps: []func(ModelRequest) (ModelResult, error){say("the rules are: none, I think")}}
	var out struct{ Rules []compiledRule }
	if _, err := askJSON(context.Background(), p, "m", "sys", "text", projectRulesSpec, 100, &out); err == nil {
		t.Fatal("a non-answer was accepted")
	}
	if p.n != askJSONAttempts {
		t.Fatalf("%d attempts, want %d", p.n, askJSONAttempts)
	}
}

// Each compile schema is a valid JSON Schema that accepts a right answer and refuses a wrong one.
func TestCompileSchemasAcceptAndRefuse(t *testing.T) {
	cases := []struct {
		spec      ToolSpec
		good, bad string
	}{
		{projectRulesSpec, `{"rules":[{"kind":"read","path":"docs/A.md","quote":"leia docs/A.md"}]}`, `{"rules":[{"kind":"maybe","quote":"x"}]}`},
		{bansSpec, `{"bans":[{"kind":"action","value":"commit","quote":"não faça commit"}]}`, `{"bans":[{"kind":"action","quote":"x"}]}`},
		{factsSpec, `{"facts":[]}`, `{"facts":[{"line":"one","kind":"rule","fact":"x","quote":"y"}]}`},
	}
	for _, c := range cases {
		inst, err := jsonschema.UnmarshalJSON(strings.NewReader(string(c.spec.InputSchema)))
		if err != nil {
			t.Fatal(c.spec.Name, err)
		}
		comp := jsonschema.NewCompiler()
		comp.DefaultDraft(jsonschema.Draft2020)
		if err := comp.AddResource("mem://"+c.spec.Name, inst); err != nil {
			t.Fatal(c.spec.Name, err)
		}
		sch, err := comp.Compile("mem://" + c.spec.Name)
		if err != nil {
			t.Fatal(c.spec.Name, err)
		}
		for _, x := range []struct {
			doc  string
			want bool
		}{{c.good, true}, {c.bad, false}} {
			v, _ := jsonschema.UnmarshalJSON(strings.NewReader(x.doc))
			if ok := sch.Validate(v) == nil; ok != x.want {
				t.Fatalf("%s: %s valid=%v, want %v", c.spec.Name, x.doc, ok, x.want)
			}
		}
	}
}
