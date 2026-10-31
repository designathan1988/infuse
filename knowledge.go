package main

// What the project already knows. Before the agent acts on a request, infuse makes sure the project's
// own documents — instructions, docs/, memory, state, decisions, audits, plans, reports — were read to
// the last line and that what they say about problems, causes, decisions, plans, prohibitions and
// measurements is in the agent's context. infuse does not ask a model to "read" a document: it hands
// the model every part of it, in order, so the whole document is covered by construction; from each
// part the model extracts facts with a verbatim quote and a line number, and code keeps a fact only when
// the quote is on those lines. Digests are stored by content hash in .infuse/knowledge/ and rebuilt only
// for documents that changed.

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// knowledgeDirs are folders whose documents are the project's knowledge.
var knowledgeDirs = map[string]bool{"docs": true, "doc": true, "memory": true, "MEMORY": true, "notes": true, "adr": true, "adrs": true, "decisions": true, "reports": true, "wiki": true, "design": true, "specs": true, "spec": true, "rfcs": true, "plans": true}

var knowledgeExts = map[string]bool{".md": true, ".markdown": true, ".txt": true, ".rst": true, ".adoc": true}

// knowledgeInventory lists the project's documents (relative paths): the instruction files, what they
// point to, the documents at the root and in knowledge folders, the pinned docs.
func knowledgeInventory(root string) []string {
	r := loadProjectRules(root)
	seen := map[string]bool{}
	var out []string
	add := func(rel string) {
		rel = filepath.ToSlash(rel)
		if !seen[rel] && len(out) < 80 {
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
		_ = filepath.WalkDir(filepath.Join(root, dir), func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if p != filepath.Join(root, dir) && (!deep || snapSkipDirs[d.Name()] || ignoredDirs[d.Name()]) {
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
	walkKnowledge(filepath.Join(infuseDirName, "docs"), true)
	return out
}

type knowledgeFact struct {
	Line  int    `json:"line"`
	Kind  string `json:"kind"` // problem | cause | decision | plan | rule | measurement | state
	Fact  string `json:"fact"`
	Quote string `json:"quote"`
}

type knowledgeDigest struct {
	File    string          `json:"file"`
	Hash    string          `json:"hash"`
	Lines   int             `json:"lines"`
	Parts   int             `json:"parts"` // parts handed to the model: all of them, by construction
	Facts   []knowledgeFact `json:"facts"`
	Dropped int             `json:"dropped"` // facts whose quote was not on the lines given
	Built   int64           `json:"built"`
}

func knowledgeDir(root string) string { return filepath.Join(root, infuseDirName, "knowledge") }

func fileHash(b []byte) string { h := sha1.Sum(b); return hex.EncodeToString(h[:]) }

func loadDigest(root, hash string) *knowledgeDigest {
	var d knowledgeDigest
	if readJSON(filepath.Join(knowledgeDir(root), hash+".json"), &d) != nil || d.Hash != hash {
		return nil
	}
	return &d
}

const digestPrompt = `You are given one part of a document of a software project, with line numbers. Extract every statement an engineer working on this project must know before acting: problems found and their causes, decisions and their reasons, plans and their steps and order, rules and prohibitions, measurements and limits, and the current state of work. Do not judge or summarize away details; keep numbers, names and paths.
Answer only with JSON: {"facts":[{"line":<line number where the quote starts>,"kind":"problem|cause|decision|plan|rule|measurement|state","fact":"<the statement, in English, self-contained>","quote":"<words copied exactly from the document, in its own language, 8 to 40 words>"}]}
If the part has nothing of that kind, answer {"facts":[]}.`

const digestPartLines = 400

// digestDocument hands the whole document to the model, part by part, and keeps the verified facts.
func (s *Session) digestDocument(ctx context.Context, p Provider, rel string, b []byte) (*knowledgeDigest, error) {
	lines := strings.Split(string(b), "\n")
	d := &knowledgeDigest{File: rel, Hash: fileHash(b), Lines: len(lines), Built: time.Now().UnixMilli()}
	for start := 0; start < len(lines); start += digestPartLines {
		end := min(start+digestPartLines, len(lines))
		var part strings.Builder
		for i := start; i < end; i++ {
			fmt.Fprintf(&part, "%d\t%s\n", i+1, lines[i])
		}
		res, err := p.Stream(ctx, ModelRequest{Model: s.fastModel(), System: digestPrompt, Messages: []Message{{Role: "user", Content: []Block{{Type: "text", Text: fmt.Sprintf("Document %s, lines %d–%d of %d:\n\n%s", rel, start+1, end, len(lines), part.String())}}}}, MaxTokens: 8000}, func(StreamEvent) {})
		if err != nil {
			return nil, err
		}
		d.Parts++
		s.mu.Lock()
		s.Cost += s.settings.costOf(s.fastModel(), res.Usage)
		s.mu.Unlock()
		raw := msgText(Message{Content: res.Content})
		if i, j := strings.Index(raw, "{"), strings.LastIndex(raw, "}"); i >= 0 && j > i {
			raw = raw[i : j+1]
		}
		var out struct {
			Facts []knowledgeFact `json:"facts"`
		}
		if json.Unmarshal([]byte(raw), &out) != nil {
			continue
		}
		window := normSpace(strings.Join(lines[start:end], "\n"))
		for _, f := range out.Facts {
			q := normSpace(f.Quote)
			if len(q) < 12 || !strings.Contains(window, q) || f.Line < start+1 || f.Line > end {
				d.Dropped++
				continue
			}
			d.Facts = append(d.Facts, f)
		}
	}
	return d, nil
}

// ensureKnowledge builds the digests of the documents that are new or changed (three at a time).
func (s *Session) ensureKnowledge(ctx context.Context, p Provider) {
	if s.settings.ReadProjectKnowledge != nil && !*s.settings.ReadProjectKnowledge {
		return
	}
	var todo []string
	for _, rel := range knowledgeInventory(s.Root) {
		b, err := os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(rel)))
		if err != nil || loadDigest(s.Root, fileHash(b)) != nil {
			continue
		}
		todo = append(todo, rel)
	}
	if len(todo) == 0 {
		return
	}
	_ = os.MkdirAll(knowledgeDir(s.Root), 0o755)
	s.emit("gate", map[string]any{"message": fmt.Sprintf("[infuse] Reading the project's documents to the end before the agent acts: %d document(s) new or changed.", len(todo))})
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for _, rel := range todo {
		wg.Add(1)
		sem <- struct{}{}
		go func(rel string) {
			defer wg.Done()
			defer func() { <-sem }()
			b, err := os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(rel)))
			if err != nil {
				return
			}
			d, err := s.digestDocument(ctx, p, rel, b)
			if err != nil {
				Journal(s.Root, JournalEntry{Kind: "gate", Detail: "leitura de " + rel + " falhou: " + firstLine(err.Error())})
				return
			}
			_ = writeJSON(filepath.Join(knowledgeDir(s.Root), d.Hash+".json"), d)
			s.emit("knowledge", map[string]any{"file": rel, "facts": len(d.Facts), "lines": d.Lines})
		}(rel)
	}
	wg.Wait()
}

// knowledgePrompt is what the project already knows, for the system prompt: every current digest.
func knowledgePrompt(root string) string {
	type entry struct {
		rel string
		d   *knowledgeDigest
	}
	var list []entry
	for _, rel := range knowledgeInventory(root) {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		if d := loadDigest(root, fileHash(b)); d != nil && len(d.Facts) > 0 {
			list = append(list, entry{rel, d})
		}
	}
	if len(list) == 0 {
		return ""
	}
	sort.SliceStable(list, func(i, j int) bool { return isInstructionFile(list[i].rel) && !isInstructionFile(list[j].rel) })
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
