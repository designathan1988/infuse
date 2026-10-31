package main

// Digestion of the project's documents: the model calls and the session orchestration. Inventory,
// storage and the prompt live in internal/knowledge.

import (
	"context"
	"fmt"
	"infuse/internal/knowledge"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type knowledgeFact = knowledge.Fact
type knowledgeDigest = knowledge.Digest

const digestPrompt = knowledge.DigestPrompt
const digestPartLines = knowledge.DigestPartLines

var factsSpec = knowledge.FactsSpec

func knowledgeDir(root string) string             { return knowledge.Dir(root) }
func fileHash(b []byte) string                    { return knowledge.FileHash(b) }
func loadDigest(root, hash string) *knowledgeDigest { return knowledge.LoadDigest(root, hash) }
func knowledgeInventory(root string) []string     { return knowledge.Inventory(root) }
func knowledgePrompt(root string) string          { return knowledge.KnowledgePrompt(root) }

// digestDocument hands the whole document to the model, part by part, and keeps the verified facts.
func (s *Session) digestDocument(ctx context.Context, p Provider, rel string, b []byte) (*knowledgeDigest, error) {
	lines := strings.Split(string(b), "\n")
	d := &knowledgeDigest{File: rel, Hash: knowledge.FileHash(b), Lines: len(lines), Built: time.Now().UnixMilli()}
	for start := 0; start < len(lines); start += knowledge.DigestPartLines {
		end := min(start+knowledge.DigestPartLines, len(lines))
		var part strings.Builder
		for i := start; i < end; i++ {
			fmt.Fprintf(&part, "%d\t%s\n", i+1, lines[i])
		}
		var out struct {
			Facts []knowledgeFact `json:"facts"`
		}
		usage, err := askJSON(ctx, p, s.fastModel(), knowledge.DigestPrompt, fmt.Sprintf("Document %s, lines %d–%d of %d:\n\n%s", rel, start+1, end, len(lines), part.String()), knowledge.FactsSpec, 16000, &out)
		s.mu.Lock()
		s.Cost += s.settings.CostOf(s.fastModel(), usage)
		s.mu.Unlock()
		if err != nil { // a part not read is a document not known: no digest is kept, the next run tries again
			return nil, fmt.Errorf("lines %d–%d: %w", start+1, end, err)
		}
		d.Parts++
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
	for _, rel := range knowledge.Inventory(s.Root) {
		b, err := os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(rel)))
		if err != nil || knowledge.LoadDigest(s.Root, knowledge.FileHash(b)) != nil {
			continue
		}
		todo = append(todo, rel)
	}
	if len(todo) == 0 {
		return
	}
	_ = os.MkdirAll(knowledge.Dir(s.Root), 0o755)
	s.emitMsg("gate", M("knowledge.reading", "count", len(todo)))
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
			_ = writeJSON(filepath.Join(knowledge.Dir(s.Root), d.Hash+".json"), d)
			s.emit("knowledge", map[string]any{"file": rel, "facts": len(d.Facts), "lines": d.Lines})
		}(rel)
	}
	wg.Wait()
}
