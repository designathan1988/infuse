package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waitMission(t *testing.T, s *Session, states ...string) *Mission {
	t.Helper()
	for i := 0; i < 1500; i++ {
		if m := s.missionSnapshot(); m != nil {
			for _, st := range states {
				if m.State == st {
					return m
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("mission did not reach %v: %+v", states, s.missionSnapshot())
	return nil
}

func TestMissionRunsUntilCriteriaPassOrBudget(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	writeSpec(t, root, `{"checks":[{"id":"arquivo","kind":"file","path":"a.txt","expect":{"contains":"ok"}}]}`)
	s := newSession(root)
	s.Provider = "mock"

	// criteria never met by the (scripted) agent: the mission stops at the turn limit, with each verdict kept
	if err := s.StartMission("crie a.txt com ok", 5, 2); err != nil {
		t.Fatal(err)
	}
	m := waitMission(t, s, "orcamento")
	if len(m.Steps) != 2 || m.Steps[0].Verdict != "reprovado" || m.Steps[0].Run == "" {
		t.Fatalf("%+v", m)
	}
	// the agent was sent back with the failing criterion
	found := false
	for _, msg := range s.Messages {
		if t := msgText(msg); len(t) > 0 && strings.Contains(t, "[Missão — turn 2 of 2") && strings.Contains(t, "arquivo:") {
			found = true
		}
	}
	if !found {
		t.Fatal("the next turn did not carry the failing criteria")
	}

	// once the criterion holds, the mission ends approved after one turn
	waitIdle(t, s)
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("ok\n"), 0o644)
	if err := s.StartMission("confirme", 5, 5); err != nil {
		t.Fatal(err)
	}
	if m := waitMission(t, s, "aprovada"); len(m.Steps) != 1 {
		t.Fatalf("%+v", m)
	}
}

func TestImpasseEndsTheMission(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Mission = &Mission{Goal: "x", State: "running", BudgetUSD: 1, MaxTurns: 3}
	out, isErr := impasseTool{}.Run(context.Background(), &ToolContext{S: s, Root: root, Cwd: root}, json.RawMessage(`{"reason":"sem acesso ao banco","tried":["conectar com as credenciais do .env"],"evidence":"ECONNREFUSED","needs":"a URL do banco de testes"}`))
	if isErr {
		t.Fatal(out)
	}
	m := s.missionSnapshot()
	if m.State != "impasse" || m.Impasse == nil || m.Impasse.Needs != "a URL do banco de testes" {
		t.Fatalf("%+v", m)
	}
}
