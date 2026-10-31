package oracle

// The executor: "done" is decided by running the oracle, never by the agent's word (SPEC §4.6).
//
// Criteria live in .infuse/oracle/checks.json, written by the user, readable by the agent, unreachable
// for it (the guard protects the oracle and the evidence). Only the user's "Pronto?" runs the executor;
// no agent tool can. A run:
//   1. takes an immutable snapshot: a git commit of the working tree (tracked + untracked, through a
//      temporary index, the user's index untouched) checked out in a detached worktree; projects
//      without git get a copy of their files and a sha256 manifest;
//   2. runs every check there with the adapters (command, http, file, maxLines), an optional setup
//      command first (e.g. installing dependencies);
//   3. stores evidence under .infuse/evidence/<run>/ (outputs, bodies, the snapshot id) and returns a
//      verdict: aprovado only when every check passes. "Not achieved, here is the evidence" is a
//      first-class result.

import (
	"context"
	"crypto/sha256"
	"infuse/internal/core"
	"infuse/internal/journal"
	"infuse/internal/platform/process"
	"infuse/internal/security"
	"infuse/internal/lang"
	"infuse/internal/projectmap"
	"infuse/internal/workspace"
	"encoding/hex"
	"errors"
	"fmt"
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

type OracleCheck struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"` // command | http | file | maxLines
	Run         string `json:"run,omitempty"`
	URL         string `json:"url,omitempty"`
	Path        string `json:"path,omitempty"`
	Glob        string `json:"glob,omitempty"`
	Max         int    `json:"max,omitempty"`
	TimeoutSec  int    `json:"timeoutSec,omitempty"`
	Description string `json:"description,omitempty"`
	Expect      struct {
		Exit        *int   `json:"exit,omitempty"`
		Status      int    `json:"status,omitempty"`
		Contains    string `json:"contains,omitempty"`
		NotContains string `json:"notContains,omitempty"`
		Matches     string `json:"matches,omitempty"`
		Exists      *bool  `json:"exists,omitempty"`
		MaxMillis   int    `json:"maxMillis,omitempty"`
	} `json:"expect"`
}

type OracleSpec struct {
	Setup  string        `json:"setup,omitempty"` // runs in the snapshot before the checks
	Checks []OracleCheck `json:"checks"`
}

type CheckResult struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Passed   bool   `json:"passed"`
	Detail   string `json:"detail"`
	Evidence string `json:"evidence,omitempty"` // file under the run's evidence folder
	Millis   int64  `json:"millis"`
}

type Verdict struct {
	Run      string        `json:"run"`
	Started  int64         `json:"started"`
	Finished int64         `json:"finished"`
	Snapshot string        `json:"snapshot"` // git commit or sha256 of the file manifest
	Verdict  string        `json:"verdict"`  // aprovado | reprovado | sem-criterios | erro
	Summary  string        `json:"summary"`
	Checks   []CheckResult `json:"checks"`
	Session  string        `json:"session,omitempty"`
}

func SpecFile(root string) string {
	return filepath.Join(root, core.InfuseDirName, "oracle", "checks.json")
}
func EvidenceDir(root string) string { return filepath.Join(root, core.InfuseDirName, "evidence") }

func LoadOracleSpec(root string) (OracleSpec, error) {
	var s OracleSpec
	err := core.ReadJSON(SpecFile(root), &s)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	return s, err
}

var executorMu sync.Map // root → *sync.Mutex: one run per project at a time

// RunOracle runs the executor for a project folder (the session's worktree when it has one) and
// stores the evidence under evidenceRoot (the project).
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
func takeSnapshot(workDir, evDir string) (string, string, func(), error) {
	snap := filepath.Join(os.TempDir(), "infuse-oracle-"+filepath.Base(evDir))
	if core.Exists(filepath.Join(workDir, ".git")) {
		idx := filepath.Join(os.TempDir(), "infuse-index-"+filepath.Base(evDir))
		defer os.Remove(idx)
		git := func(args ...string) (string, error) {
			cmd := exec.Command("git", args...)
			process.NoConsoleWindow(cmd)
			cmd.Dir = workDir
			cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+idx)
			out, err := cmd.CombinedOutput()
			return strings.TrimSpace(string(out)), err
		}
		if _, err := git("add", "-A", "--", ".", ":(exclude).infuse"); err != nil {
			return "", "", nil, errors.New("git add (índice temporário) falhou")
		}
		tree, err := git("write-tree")
		if err != nil {
			return "", "", nil, errors.New("git write-tree falhou: " + tree)
		}
		parent := []string{}
		if head, err := git("rev-parse", "--verify", "HEAD"); err == nil && head != "" {
			parent = []string{"-p", head}
		}
		commit, err := git(append(append([]string{"commit-tree", tree}, parent...), "-m", "infuse oracle snapshot "+filepath.Base(evDir))...)
		if err != nil {
			return "", "", nil, errors.New("git commit-tree falhou: " + commit)
		}
		if out, err := workspace.GitRun(workDir, "worktree", "add", "--detach", snap, commit); err != nil {
			return "", "", nil, errors.New("git worktree add falhou: " + strings.TrimSpace(out))
		}
		linkDependencies(workDir, snap)
		_ = os.WriteFile(filepath.Join(evDir, "snapshot.txt"), []byte("git commit "+commit+"\ntree "+tree+"\n"), 0o644)
		return snap, "git:" + commit[:12], func() { _, _ = workspace.GitRun(workDir, "worktree", "remove", "--force", snap) }, nil
	}
	h := sha256.New()
	var manifest strings.Builder
	err := filepath.WalkDir(workDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel := core.RelTo(workDir, p)
		if d.IsDir() {
			if rel != "" && (projectmap.IgnoredDirs[d.Name()] || d.Name() == core.InfuseDirName) {
				return filepath.SkipDir
			}
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(b)
		line := hex.EncodeToString(sum[:]) + "  " + filepath.ToSlash(rel) + "\n"
		manifest.WriteString(line)
		h.Write([]byte(line))
		dst := filepath.Join(snap, rel)
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		return "", "", nil, err
	}
	linkDependencies(workDir, snap)
	_ = os.WriteFile(filepath.Join(evDir, "snapshot.txt"), []byte(manifest.String()), 0o644)
	id := hex.EncodeToString(h.Sum(nil))
	return snap, "sha256:" + id[:12], func() { _ = os.RemoveAll(snap) }, nil
}

// linkDependencies points installed dependency folders (node_modules, .venv, vendor) of the project
// into the snapshot, read through a link: checks run against the code snapshot with the dependencies
// already installed, without copying gigabytes.
func linkDependencies(from, snap string) {
	for _, d := range []string{"node_modules", ".venv", "venv"} {
		src := filepath.Join(from, d)
		if st, err := os.Stat(src); err == nil && st.IsDir() && !core.Exists(filepath.Join(snap, d)) {
			if err := os.Symlink(src, filepath.Join(snap, d)); err != nil && isWindows() {
				cmd := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(snap, d), src) // junction: no privilege needed
				process.NoConsoleWindow(cmd)
				_ = cmd.Run()
			}
		}
	}
}

func isWindows() bool { return filepath.Separator == '\\' }

// ListVerdicts returns the most recent runs of a project, newest first.
func ListVerdicts(root string, n int) []Verdict {
	entries, _ := os.ReadDir(EvidenceDir(root))
	var out []Verdict
	for i := len(entries) - 1; i >= 0 && len(out) < n; i-- {
		var v Verdict
		if entries[i].IsDir() && core.ReadJSON(filepath.Join(EvidenceDir(root), entries[i].Name(), "verdict.json"), &v) == nil {
			out = append(out, v)
		}
	}
	if out == nil {
		out = []Verdict{}
	}
	return out
}
