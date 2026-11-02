package filter

// The trailer: what infuse appends to a command's output, for every agent (SPEC §3: command output +
// oracle state + untouched dependents). It runs on every command, so nothing here may walk the
// project: it reads the stored map and the end of the journal.

import (
	"fmt"
	"infuse/internal/core"
	"infuse/internal/journal"
	"infuse/internal/oracle"
	"infuse/internal/projectmap"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// buildTrailer is what infuse appends to a command's output, in English (trailerMsgs gives the
// messages); ruleLines are lines already written.
func BuildTrailer(root, cwd, original string, actions []TransformAction, started int64, rules *RuleSet, ruleLines []string) string {
	var notes []core.Msg
	for _, l := range ruleLines {
		notes = append(notes, core.M("text.verbatim", "text", l))
	}
	return joinMsgLines(TrailerMsgs(root, cwd, original, actions, started, rules, notes))
}

// joinMsgLines is the English of messages, one per line.
func joinMsgLines(ms []core.Msg) string {
	return strings.Join(MsgStrings(ms), "\n")
}

// trailerMsgs is what infuse appends to a command's output: notes (the guard's, the rules'), what was
// written and the dependents not touched yet, the map of a listed folder, what the command cost and
// the oracle's state. A caller with a session records each one with the call, so the user reads them
// in their language.
func TrailerMsgs(root, cwd, original string, actions []TransformAction, started int64, rules *RuleSet, notes []core.Msg) []core.Msg {
	lines := append([]core.Msg{}, notes...)
	lines = append(lines, actionMessages(actions)...)

	m := projectmap.Peek(root)
	if order := writtenPaths(root, actions, started); len(order) > 0 {
		lines = append(lines, writtenMessages(root, m, order, rules)...)
	}
	lines = append(lines, listedDirMessages(root, cwd, original, rules, m)...)
	lines = append(lines, costMessages(root, original, started)...)
	return append(lines, oracleMessages(root)...)
}

// actionMessages is the trailer's note per transformation.
func actionMessages(actions []TransformAction) []core.Msg {
	var lines []core.Msg
	for _, a := range actions {
		switch a.Kind {
		case ActHeredocToFile:
			lines = append(lines, core.M("shell.action", "detail", a.Detail))
		case ActLesson:
			lines = append(lines, viceMessage(a.Vice, a.Detail))
		}
	}
	return lines
}

// viceMessage states a vice with its remedy: the signature and the remedy always travel together.
func viceMessage(vice, detail string) core.Msg {
	if _, remedy := journal.Vice(vice); remedy != "" {
		return core.M("shell.vice", "vice", vice, "detail", detail, "remedy", remedy)
	}
	return core.M("shell.vice_bare", "vice", vice, "detail", detail)
}

// writtenPaths lists, in order and without repeats, the files written during the command.
func writtenPaths(root string, actions []TransformAction, started int64) []string {
	written := map[string]bool{}
	var order []string
	add := func(rel string) {
		if !written[rel] {
			written[rel] = true
			order = append(order, rel)
		}
	}
	for _, e := range journal.ReadJournalSince(root, started) {
		if e.Kind == "write" && e.Path != "" {
			add(e.Path)
		}
	}
	for _, a := range actions {
		if a.Kind == ActHeredocToFile && a.Path != "" {
			add(core.RelTo(root, a.Path))
		}
	}
	return order
}

// writtenMessages is what the trailer says about the files written: the list, the dependents the
// work has not reached yet and the write rules that matched.
func writtenMessages(root string, m *projectmap.ProjectMap, order []string, rules *RuleSet) []core.Msg {
	var lines []core.Msg
	shown := order
	more := ""
	if len(shown) > 20 {
		shown, more = shown[:20], "…"
	}
	lines = append(lines, core.M("shell.written", "files", strings.Join(shown, ", ")+more))
	if m != nil {
		touched := journal.TouchedFiles(root)
		for _, w := range order {
			touched[w] = true
		}
		for i, w := range order {
			if i >= 8 {
				break
			}
			if d := projectmap.UntouchedDependentsMsg(m, w, touched); !d.Empty() {
				lines = append(lines, d)
			}
		}
	}
	abs := make([]string, 0, len(order))
	for _, w := range order {
		abs = append(abs, filepath.Join(root, filepath.FromSlash(w)))
	}
	return append(lines, ApplyWriteRulesMsgs(root, rules, abs)...)
}

// listedDirMessages describes the directory an ls/tree command listed, when it is one. A command that
// may have changed the tree, or a project with no map yet, reads that one folder from disk.
func listedDirMessages(root, cwd, original string, rules *RuleSet, m *projectmap.ProjectMap) []core.Msg {
	listed, ok := ListedDirectory(original)
	if !ok {
		return nil
	}
	target := ResolveIn(cwd, core.FromMsysPath(listed))
	st, err := os.Stat(target)
	if err != nil || !st.IsDir() {
		return nil
	}
	rel := core.RelTo(root, target)
	if rel == "" {
		rel = "."
	}
	if (m == nil || MayChangeTree(original)) && core.Within(root, target) {
		m = projectmap.RefreshDir(root, rel)
	}
	if m == nil {
		return nil
	}
	var lines []core.Msg
	if d := projectmap.DescribeDir(m, rel); d != "" {
		lines = append(lines, core.M("text.verbatim", "text", d)) // the map's listing is data, the same in every language
	}
	return append(lines, ApplyListRulesMsgs(root, rules, rel)...)
}

// costMessages records what the command took and speaks up when it was slow: execution cost is
// evidence about the agent's own work. Only a slow command pays for reading its history.
func costMessages(root, original string, started int64) []core.Msg {
	if started <= 0 {
		return nil
	}
	took := time.Now().UnixMilli() - started
	journal.RecordCost(root, original, took)
	if took < journal.SlowCommandMillis {
		return nil
	}
	c, ok := journal.CostOf(root, original)
	if !ok {
		return nil
	}
	return []core.Msg{core.M("shell.cost", "seconds", secondsText(took), "typical", secondsText(c.Median), "runs", c.Runs)}
}

func secondsText(ms int64) string { return fmt.Sprintf("%.1f", float64(ms)/1000) }

// verdictWords says the executor's result codes in the model's language.
var verdictWords = map[string]string{"aprovado": "approved", "reprovado": "rejected", "sem-criterios": "no criteria", "erro": "error"}

// oracleMessages is the oracle's state, read from what the executor stored: never run from here.
func oracleMessages(root string) []core.Msg {
	spec, err := oracle.LoadOracleSpec(root)
	if err != nil || len(spec.Checks) == 0 {
		return nil
	}
	list := oracle.ListVerdicts(root, 1)
	if len(list) == 0 {
		return []core.Msg{core.M("shell.oracle_none", "checks", len(spec.Checks))}
	}
	v := list[0]
	word := verdictWords[v.Verdict]
	if word == "" {
		word = v.Verdict
	}
	if !v.SpecVerified {
		word += " (not verified)"
	}
	if v.Finished < journal.LastWrite(root) {
		return []core.Msg{core.M("shell.oracle_stale", "verdict", word, "run", v.Run)}
	}
	return []core.Msg{core.M("shell.oracle", "verdict", word, "checks", len(v.Checks), "run", v.Run, "summary", v.Summary)}
}
