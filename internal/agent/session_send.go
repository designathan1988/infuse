package agent

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"infuse/internal/oracle"
	"infuse/internal/workspace"
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
	// the turn is claimed under the lock that checks it: a second Send or a Revert sees it at once,
	// not only when the loop reaches its first request
	s.Mu.Lock()
	busy := s.State != "idle" && s.State != "error"
	idle := s.State
	if !busy {
		s.State = "thinking"
	}
	s.Mu.Unlock()
	if busy {
		return errors.New(M("session.busy").Render(s.uiLang()))
	}
	hook := s.runHooks(context.Background(), "UserPromptSubmit", "", map[string]any{"prompt": text})
	if hook.Block {
		s.setState(idle)
		return errors.New(hook.blockMsg().Render(s.uiLang()))
	}
	s.Mu.Lock()
	s.settings = EffectiveSettings(s.Root)
	if s.Model == "" {
		s.Model = s.settings.Model
	}
	s.lastError = ""
	s.gate.reset()
	s.resetTurnLocks()
	carry := s.AuditPending && s.Turn > 0 // what the unaudited turn changed stays in this turn's audit
	s.AuditPending, s.Outcome = true, ""
	s.Turn++
	s.TurnStarts = append(s.TurnStarts, len(s.Messages))
	s.TurnRequest = text
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
	if err := s.turnBase(carry); err != nil {
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

// turnBase records the tree the turn is judged against: the tree as it is now, or, after a turn that
// ended before its audit (failed, cancelled, killed), that turn's own base.
func (s *Session) turnBase(carry bool) error {
	if carry && workspace.CarryTurn(s.Root, s.ID, s.Turn-1, s.Turn) == nil {
		return nil
	}
	oracle.RecordDependencies(s.Root, s.workRoot()) // before the agent can touch them
	return snapshotTurn(s.Root, s.workRoot(), s.ID, s.Turn)
}

func (s *Session) Stop() {
	s.Mu.Lock()
	c := s.cancel
	s.Mu.Unlock()
	if c != nil {
		c()
	}
}
