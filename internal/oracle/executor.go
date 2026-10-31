package oracle

import (
	"context"
	"fmt"
	"infuse/internal/core"
	"infuse/internal/journal"
	"infuse/internal/lang"
	"infuse/internal/platform/process"
	"infuse/internal/projectmap"
	"infuse/internal/security"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// The executor: run the checks against an immutable snapshot.

func RunOracle(ctx context.Context, evidenceRoot, workDir, session string) (Verdict, error) {
	mu, _ := executorMu.LoadOrStore(evidenceRoot, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	v := Verdict{Run: time.Now().Format("20060102-150405.000"), Started: time.Now().UnixMilli(), Session: session, Checks: []CheckResult{}}
	spec, err := LoadOracleSpec(evidenceRoot)
	if err != nil {
		return v, fmt.Errorf("checks.json inválido: %w", err)
	}
	dir := filepath.Join(EvidenceDir(evidenceRoot), v.Run)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return v, err
	}
	finish := func() (Verdict, error) {
		v.Finished = time.Now().UnixMilli()
		_ = core.WriteJSON(filepath.Join(dir, "verdict.json"), v)
		journal.Journal(evidenceRoot, journal.JournalEntry{Kind: "oracle", Detail: fmt.Sprintf("veredito %s (%s) — evidência em .infuse/evidence/%s", v.Verdict, v.Summary, v.Run)})
		return v, nil
	}
	if len(spec.Checks) == 0 {
		v.Verdict, v.Summary = "sem-criterios", "nenhuma verificação em .infuse/oracle/checks.json"
		return finish()
	}

	snapDir, snapID, cleanup, err := takeSnapshot(workDir, dir)
	if err != nil {
		v.Verdict, v.Summary = "erro", "snapshot falhou: "+err.Error()
		return finish()
	}
	defer cleanup()
	v.Snapshot = snapID

	if strings.TrimSpace(spec.Setup) != "" {
		res := runCheckCommand(ctx, snapDir, spec.Setup, 600)
		_ = os.WriteFile(filepath.Join(dir, "setup.log"), []byte(res.out), 0o644)
		if res.exit != 0 {
			v.Verdict, v.Summary = "erro", fmt.Sprintf("preparação falhou (exit %d) — veja setup.log", res.exit)
			return finish()
		}
	}
	passed := 0
	for i, c := range spec.Checks {
		if c.ID == "" {
			c.ID = fmt.Sprintf("check-%d", i+1)
		}
		start := time.Now()
		r := runCheck(ctx, snapDir, dir, c)
		r.Millis = time.Since(start).Milliseconds()
		if c.Expect.MaxMillis > 0 && r.Passed && r.Millis > int64(c.Expect.MaxMillis) {
			r.Passed = false
			r.Detail += fmt.Sprintf("; levou %d ms (máximo %d)", r.Millis, c.Expect.MaxMillis)
		}
		if r.Passed {
			passed++
		}
		v.Checks = append(v.Checks, r)
	}
	if passed == len(spec.Checks) {
		v.Verdict = "aprovado"
	} else {
		v.Verdict = "reprovado"
	}
	v.Summary = fmt.Sprintf("%d de %d verificações passaram", passed, len(spec.Checks))
	return finish()
}

type cmdResult struct {
	out  string
	exit int
}

func runCheckCommand(ctx context.Context, dir, command string, timeoutSec int) cmdResult {
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, core.FindRealBash(), "-c", command)
	process.NoConsoleWindow(cmd)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CI=1", "INFUSE_ORACLE=1")
	buf := core.NewCapWriter(512 << 10)
	cmd.Stdout, cmd.Stderr = buf, buf
	err := cmd.Run()
	code := core.ExitCode(err)
	if cctx.Err() == context.DeadlineExceeded {
		code = -1
		_, _ = buf.Write([]byte(fmt.Sprintf("\n[tempo esgotado após %ds]", timeoutSec)))
	}
	return cmdResult{security.RedactSecrets(buf.String()), code}
}

func runCheck(ctx context.Context, snap, evDir string, c OracleCheck) CheckResult {
	r := CheckResult{ID: c.ID, Kind: c.Kind}
	evidence := func(name, content string) {
		r.Evidence = name
		_ = os.WriteFile(filepath.Join(evDir, name), []byte(content), 0o644)
	}
	textOK := func(text string) (bool, string) {
		if c.Expect.Contains != "" && !strings.Contains(text, c.Expect.Contains) {
			return false, fmt.Sprintf("não contém %q", c.Expect.Contains)
		}
		if c.Expect.NotContains != "" && strings.Contains(text, c.Expect.NotContains) {
			return false, fmt.Sprintf("contém %q", c.Expect.NotContains)
		}
		if c.Expect.Matches != "" {
			re, err := regexp.Compile(c.Expect.Matches)
			if err != nil {
				return false, "regex inválida: " + err.Error()
			}
			if !re.MatchString(text) {
				return false, fmt.Sprintf("não casa com /%s/", c.Expect.Matches)
			}
		}
		return true, ""
	}
	safe := regexp.MustCompile(`[^\w.-]+`).ReplaceAllString(c.ID, "_")
	switch c.Kind {
	case "command":
		res := runCheckCommand(ctx, snap, c.Run, c.TimeoutSec)
		evidence(safe+".log", fmt.Sprintf("$ %s\n%s\n[exit %d]\n", c.Run, res.out, res.exit))
		want := 0
		if c.Expect.Exit != nil {
			want = *c.Expect.Exit
		}
		if res.exit != want {
			r.Detail = fmt.Sprintf("exit %d (esperado %d)", res.exit, want)
			return r
		}
		ok, why := textOK(res.out)
		r.Passed, r.Detail = ok, core.FirstNonEmpty(why, fmt.Sprintf("exit %d", res.exit))
	case "http":
		cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(cctx, "GET", c.URL, nil)
		if err != nil {
			r.Detail = err.Error()
			return r
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			r.Detail = "sem resposta: " + err.Error()
			return r
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		evidence(safe+".http", fmt.Sprintf("GET %s\nHTTP %d\n\n%s", c.URL, resp.StatusCode, b))
		want := c.Expect.Status
		if want == 0 {
			want = 200
		}
		if resp.StatusCode != want {
			r.Detail = fmt.Sprintf("HTTP %d (esperado %d)", resp.StatusCode, want)
			return r
		}
		ok, why := textOK(string(b))
		r.Passed, r.Detail = ok, core.FirstNonEmpty(why, fmt.Sprintf("HTTP %d", resp.StatusCode))
	case "file":
		p := filepath.Join(snap, filepath.FromSlash(c.Path))
		b, err := os.ReadFile(p)
		exists := err == nil
		wantExists := true
		if c.Expect.Exists != nil {
			wantExists = *c.Expect.Exists
		}
		if exists != wantExists {
			r.Detail = map[bool]string{true: "o arquivo existe", false: "o arquivo não existe"}[exists]
			return r
		}
		if !exists {
			r.Passed, r.Detail = true, "não existe, como esperado"
			return r
		}
		ok, why := textOK(string(b))
		r.Passed, r.Detail = ok, core.FirstNonEmpty(why, fmt.Sprintf("%d bytes", len(b)))
	case "maxLines":
		re := lang.GlobToRegex(c.Glob)
		var over []string
		_ = filepath.WalkDir(snap, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if projectmap.IgnoredDirs[d.Name()] || d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			rel := filepath.ToSlash(core.RelTo(snap, p))
			if !re.MatchString(rel) {
				return nil
			}
			if b, err := os.ReadFile(p); err == nil {
				if n := strings.Count(string(b), "\n") + 1; n > c.Max {
					over = append(over, fmt.Sprintf("%s: %d linhas", rel, n))
				}
			}
			return nil
		})
		sort.Strings(over)
		evidence(safe+".txt", strings.Join(over, "\n"))
		r.Passed = len(over) == 0
		r.Detail = map[bool]string{true: fmt.Sprintf("nenhum arquivo acima de %d linhas", c.Max), false: fmt.Sprintf("%d arquivo(s) acima de %d linhas", len(over), c.Max)}[r.Passed]
	default:
		r.Detail = "tipo de verificação desconhecido: " + c.Kind
	}
	return r
}

// takeSnapshot freezes the working tree. With git: a commit of everything (tracked + untracked, honoring
// .gitignore) built through a temporary index, checked out in a detached worktree. Without git: a copy
// of the files plus a sha256 manifest. Returns the folder to run in, its id, and a cleanup.
