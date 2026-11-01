package agent

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// User messages and starting/stopping a turn.

func userBlocks(text string, atts []Attachment, imgs []InlineImage) []Block {
	blocks := []Block{{Type: "text", Text: text}}
	for _, a := range atts {
		b, err := os.ReadFile(a.Path)
		if err != nil {
			continue
		}
		if mt, ok := imageExt[strings.ToLower(filepath.Ext(a.Path))]; ok {
			blocks = append(blocks, Block{Type: "image", MediaType: mt, Data: base64.StdEncoding.EncodeToString(b)})
			continue
		}
		if len(b) > 400_000 {
			b = b[:400_000]
		}
		blocks = append(blocks, Block{Type: "text", Text: fmt.Sprintf("<attached-file path=%q>\n%s\n</attached-file>", a.Path, string(b))})
	}
	for _, im := range imgs {
		if im.Data != "" {
			blocks = append(blocks, Block{Type: "image", MediaType: im.MediaType, Data: im.Data})
		}
	}
	return blocks
}

func (s *Session) Send(text string, atts []Attachment, imgs []InlineImage) error {
	s.Mu.Lock()
	busy := s.State != "idle" && s.State != "error"
	s.Mu.Unlock()
	if busy {
		return errors.New(M("session.busy").Render(s.uiLang()))
	}
	hook := s.runHooks(context.Background(), "UserPromptSubmit", "", map[string]any{"prompt": text})
	if hook.Block {
		return errors.New(hook.blockMsg().Render(s.uiLang()))
	}
	s.refreshPolicy() // the settings snapshot and the policy scan happen before the turn starts
	s.Mu.Lock()
	if s.Model == "" {
		s.Model = s.settings.Model
	}
	s.lastError = ""
	s.gate.reset()
	s.resetTurnLocks()
	s.Turn++
	s.TurnStarts = append(s.TurnStarts, len(s.Messages))
	blocks := userBlocks(text, atts, imgs)
	if hook.Message != "" { // context a UserPromptSubmit hook adds to the message
		blocks = append(blocks, Block{Type: "text", Text: "<hook-context>\n" + hook.Message + "\n</hook-context>"})
	}
	s.Messages = append(s.Messages, Message{Role: "user", Content: blocks, TS: time.Now().UnixMilli()})
	if s.Turn == 1 && (s.Title == "") {
		s.Title = truncate(strings.SplitN(strings.TrimSpace(text), "\n", 2)[0], 60)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.Working = true
	s.Mu.Unlock()
	// The tree at the start of the turn: the base of the "Changes" view and of the shell-write audit, which
	// judges every file changed by anything other than the write tools.
	if err := snapshotTurn(s.Root, s.workRoot(), s.ID, s.Turn); err != nil {
		s.Mu.Lock()
		s.snapshotErr = err
		s.Mu.Unlock()
	} else {
		s.Mu.Lock()
		s.snapshotErr = nil
		s.Mu.Unlock()
	}
	s.emit("turn", map[string]any{"turn": s.Turn, "text": text, "title": s.Title})
	s.save()
	go s.run(ctx)
	return nil
}

func (s *Session) Stop() {
	s.Mu.Lock()
	c := s.cancel
	s.Mu.Unlock()
	if c != nil {
		c()
	}
}
