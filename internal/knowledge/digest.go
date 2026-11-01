package knowledge

// What the project already knows. infuse hands each project document to the model in full, part by
// part; a fact is kept only when its quote appears on the lines given. Digests are stored by content
// hash in .infuse/knowledge/ and rebuilt only for documents that changed.

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"infuse/internal/core"
	"infuse/internal/fsx"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// knowledgeDirs are folders whose documents are the project's knowledge.
var knowledgeDirs = map[string]bool{"docs": true, "doc": true, "memory": true, "MEMORY": true, "notes": true, "adr": true, "adrs": true, "decisions": true, "reports": true, "wiki": true, "design": true, "specs": true, "spec": true, "rfcs": true, "plans": true}

var knowledgeExts = map[string]bool{".md": true, ".markdown": true, ".txt": true, ".rst": true, ".adoc": true}

// InventoryLimit bounds how many documents infuse digests in one pass. When the inventory reaches
// it, the caller tells the user instead of silently reading less.
const InventoryLimit = 200

// Inventory lists the project's documents (relative paths): the instruction files, what they point to,
// the documents at the root and in knowledge folders.
func Inventory(root string) []string {
	r := LoadProjectRules(root)
	seen := map[string]bool{}
	var out []string
	add := func(rel string) {
		rel = filepath.ToSlash(rel)
		if !seen[rel] && len(out) < InventoryLimit {
			if fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil && !fi.IsDir() && fi.Size() < 3<<20 {
				seen[rel] = true
				out = append(out, rel)
			}
		}
	}
	for _, f := range r.Files {
		add(f)
	}
	for _, f := range r.Required {
		add(f)
	}
	walkKnowledge := func(dir string, deep bool) {
		base := filepath.Join(root, dir)
		_ = fsx.Walk(base, fsx.Snapshot, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if p != base && !deep {
					return filepath.SkipDir
				}
				return nil
			}
			if knowledgeExts[strings.ToLower(filepath.Ext(p))] {
				rel, _ := filepath.Rel(root, p)
				add(rel)
			}
			return nil
		})
	}
	walkKnowledge(".", false)
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.IsDir() && knowledgeDirs[e.Name()] {
			walkKnowledge(e.Name(), true)
		}
	}
	walkKnowledge(filepath.Join(core.InfuseDirName, "docs"), true)
	return out
}

type Fact struct {
	Line  int    `json:"line"`
	Kind  string `json:"kind"` // problem | cause | decision | plan | rule | measurement | state
	Fact  string `json:"fact"`
	Quote string `json:"quote"`
}

type Digest struct {
	File    string `json:"file"`
	Hash    string `json:"hash"`
	Lines   int    `json:"lines"`
	Parts   int    `json:"parts"` // parts handed to the model: all of them, by construction
	Facts   []Fact `json:"facts"`
	Dropped int    `json:"dropped"` // facts whose quote was not on the lines given
	Built   int64  `json:"built"`
}

func Dir(root string) string { return filepath.Join(root, core.InfuseDirName, "knowledge") }

func FileHash(b []byte) string { h := sha1.Sum(b); return hex.EncodeToString(h[:]) }

func LoadDigest(root, hash string) *Digest {
	var d Digest
	if core.ReadJSON(filepath.Join(Dir(root), hash+".json"), &d) != nil || d.Hash != hash {
		return nil
	}
	return &d
}

const DigestPrompt = `You are given one part of a document of a software project, with line numbers. Extract every statement an engineer working on this project must know before acting: problems found and their causes, decisions and their reasons, plans and their steps and order, rules and prohibitions, measurements and limits, and the current state of work. Do not judge or summarize away details; keep numbers, names and paths.
Answer by calling Facts. For each fact: line is the line number where the quote starts; fact is the statement, in English, self-contained; quote is words copied exactly from the document, in its own language, 8 to 40 words.
If the part has nothing of that kind, call Facts with an empty list.`

// FactsSpec is the shape of a digested document part.
var FactsSpec = core.ToolSpec{Name: "Facts", Description: "What an engineer must know from this part of the document, each with its verbatim quote.",
	InputSchema: core.RawJSON(`{"type":"object","properties":{"facts":{"type":"array","items":{"type":"object","properties":{"line":{"type":"integer"},"kind":{"type":"string","enum":["problem","cause","decision","plan","rule","measurement","state"]},"fact":{"type":"string"},"quote":{"type":"string"}},"required":["line","kind","fact","quote"]}}},"required":["facts"]}`)}

const DigestPartLines = 400

// KnowledgePrompt is what the project already knows, for the system prompt: every current digest.
func KnowledgePrompt(root string) string {
	type entry struct {
		rel string
		d   *Digest
	}
	var list []entry
	for _, rel := range Inventory(root) {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		if d := LoadDigest(root, FileHash(b)); d != nil && len(d.Facts) > 0 {
			list = append(list, entry{rel, d})
		}
	}
	if len(list) == 0 {
		return ""
	}
	sort.SliceStable(list, func(i, j int) bool { return IsInstructionFile(list[i].rel) && !IsInstructionFile(list[j].rel) })
	var b strings.Builder
	b.WriteString("## What this project already knows\nFacts from the project's own documents, read to the last line by infuse; each quote was checked against the file. Diagnose against them before acting: a problem may already be described here with its cause and its plan. Cite the document (and read it to the end yourself) in your Diagnosis.\n")
	for _, e := range list {
		fmt.Fprintf(&b, "\n### %s (%d lines)\n", e.rel, e.d.Lines)
		for _, f := range e.d.Facts {
			fmt.Fprintf(&b, "- L%d [%s] %s — “%s”\n", f.Line, f.Kind, f.Fact, f.Quote)
			if b.Len() > 160000 {
				b.WriteString("\n(more facts in the documents themselves)\n")
				return b.String()
			}
		}
	}
	return b.String()
}
