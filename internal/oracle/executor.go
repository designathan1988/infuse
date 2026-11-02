package oracle

import (
	"context"
	"fmt"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/fsx"
	"infuse/internal/journal"
	"infuse/internal/lang"
	"infuse/internal/platform/process"
	"infuse/internal/sandbox"
	"infuse/internal/security"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// The executor: run the checks against an immutable snapshot.

// uiText renders a catalog message in the language the user reads infuse in.
func uiText(id string, kv ...any) string {
	return core.M(id, kv...).Render(config.LoadSettings().Language)
}

// RunOracle runs the user's criteria for a project (turn 0 when the caller has none).
func RunOracle(ctx context.Context, evidenceRoot, workDir, session string) (Verdict, error) {
	return RunOracleTurn(ctx, evidenceRoot, workDir, session, 0)
}

// RunOracleTurn is the executor. The verdict carries the hash of the criteria it used and the turn it
// belongs to, and the criteria are copied beside the evidence: a verdict can always be traced back to
// the exact criteria and snapshot that produced it.
func RunOracleTurn(ctx context.Context, evidenceRoot, workDir, session string, turn int) (Verdict, error) {
	mu, _ := executorMu.LoadOrStore(evidenceRoot, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	v := Verdict{Run: time.Now().Format("20060102-150405.000"), Started: time.Now().UnixMilli(), Session: session, Turn: turn, Project: projectID(evidenceRoot), Checks: []CheckResult{}}
	spec, err := LoadOracleSpec(evidenceRoot)
	if err != nil {
		return v, fmt.Errorf("invalid checks.json: %w", err)
	}
	v.SpecHash, v.Criteria = specHash(spec), len(spec.Checks)
	dir := filepath.Join(EvidenceDir(evidenceRoot), v.Run)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return v, err
	}
	writeSpecCopy(dir, spec)
	finish := func() (Verdict, error) {
		v.Finished = time.Now().UnixMilli()
		v.Signature = signVerdict(v)
		_ = core.WriteJSON(filepath.Join(dir, "verdict.json"), v)
		journal.Journal(evidenceRoot, journal.JournalEntry{Kind: "oracle", Detail: fmt.Sprintf("verdict %s (%s) — evidence in .infuse/evidence/%s", v.Verdict, v.Summary, v.Run)})
		return v, nil
	}
	// the criteria are verified first: an empty spec is only "no criteria" when nobody removed them
	if err := VerifyCriteria(evidenceRoot, spec); err != nil {
		v.Verdict, v.Summary = "erro", err.Error()
		return finish()
	}
	if len(spec.Checks) == 0 {
		v.Verdict, v.Summary = "sem-criterios", uiText("oracle.noChecks")
		return finish()
	}
	if needsServer(spec) && strings.TrimSpace(spec.Server) == "" {
		v.Verdict, v.Summary = "erro", uiText("oracle.httpNeedsServer")
		return finish()
	}
	v.Warnings = specWarnings(spec)
	snap, err := takeSnapshot(workDir, dir)
	if err != nil {
		v.Verdict, v.Summary = "erro", uiText("oracle.snapshotFailed", "error", err.Error())
		return finish()
	}
	defer snap.cleanup()
	v.Snapshot, v.Fingerprint = snap.id, snap.fingerprint
	measureSnapshot(ctx, &v, evidenceRoot, workDir, dir, spec, snap)
	return finish()
}

// measureSnapshot runs the criteria against the snapshot and fills the verdict in.
func measureSnapshot(ctx context.Context, v *Verdict, evidenceRoot, workDir, dir string, spec OracleSpec, snap *snapshot) {
	warnings, refused := checkDependencies(workDir, snap.trees)
	v.Warnings = append(v.Warnings, warnings...)
	if refused != "" {
		v.Verdict, v.Summary = "erro", refused
		return
	}
	x := newContainment(evidenceRoot, snap.dir)
	if !sandbox.Contained(x.setting) {
		v.Warnings = append(v.Warnings, uiText("oracle.notContained"))
	}
	if snap.outsideLinks > 0 {
		v.Warnings = append(v.Warnings, uiText("oracle.outsideLinks", "count", snap.outsideLinks))
	}
	if summary, ok := runSetup(ctx, x, dir, spec.Setup); !ok {
		v.Verdict, v.Summary = "erro", summary
		return
	}
	pid, stop, summary, ok := startSnapshotServer(ctx, spec, x, dir)
	if !ok {
		v.Verdict, v.Summary = "erro", summary
		return
	}
	v.ServerPID = pid
	defer stop()
	var passed int
	var stable bool
	v.Checks, passed, v.Dependencies, stable = runChecksOnSnapshot(ctx, x, dir, spec.Checks, workDir, snap.dir)
	if !stable { // what the checks measured is not what the tree holds now, or a check patched its own tools
		v.Verdict, v.Summary = "erro", uiText("oracle.dependencies")
		return
	}
	v.Verdict, v.Summary = verdictOf(v.Checks, passed)
}

// runChecksOnSnapshot runs the checks and reports whether the installed dependencies stayed the same,
// in the live tree (what the verdict speaks for) and in the snapshot (what the checks ran).
func runChecksOnSnapshot(ctx context.Context, x containment, dir string, checks []OracleCheck, workDir, snapDir string) (results []CheckResult, passed int, deps string, stable bool) {
	deps = fsx.DependencyFingerprint(workDir)
	snapDeps := fsx.DependencyFingerprint(snapDir)
	results, passed = runChecks(ctx, x, dir, checks)
	stable = fsx.DependencyFingerprint(workDir) == deps && fsx.DependencyFingerprint(snapDir) == snapDeps
	return results, passed, deps, stable
}

// verdictOf reads the results: a check that could not be measured makes the run an error, never a pass
// and never a plain failure of the project.
func verdictOf(checks []CheckResult, passed int) (verdict, summary string) {
	for _, c := range checks {
		if c.Errored {
			return "erro", c.Detail
		}
	}
	summary = uiText("oracle.summary", "passed", passed, "total", len(checks))
	if passed == len(checks) {
		return "aprovado", summary
	}
	return "reprovado", summary
}

// startSnapshotServer starts the criteria's server from the snapshot when there is one to start.
func startSnapshotServer(ctx context.Context, spec OracleSpec, x containment, dir string) (pid int, stop func(), summary string, ok bool) {
	if !needsServer(spec) || strings.TrimSpace(spec.Server) == "" {
		return 0, func() {}, "", true
	}
	stop, pid, err := startServer(ctx, x, dir, spec)
	if err != nil {
		return 0, nil, err.Error(), false
	}
	return pid, stop, "", true
}

// runSetup runs the spec's preparation command against the snapshot; ok is false when it failed.
func runSetup(ctx context.Context, x containment, dir, setup string) (summary string, ok bool) {
	if strings.TrimSpace(setup) == "" {
		return "", true
	}
	res := runCheckCommand(ctx, x, setup, 600)
	_ = os.WriteFile(filepath.Join(dir, "setup.log"), []byte(res.out), 0o644)
	if res.exit != 0 {
		return uiText("oracle.setupFailed", "exit", res.exit), false
	}
	return "", true
}

// runChecks runs every check against the snapshot in order and returns the results and how many passed.
func runChecks(ctx context.Context, x containment, dir string, checks []OracleCheck) ([]CheckResult, int) {
	results := make([]CheckResult, 0, len(checks))
	passed := 0
	for i, c := range checks {
		if c.ID == "" {
			c.ID = fmt.Sprintf("check-%d", i+1)
		}
		start := time.Now()
		r := runCheck(ctx, x, dir, c)
		r.Target = checkTarget(c)
		r.Millis = time.Since(start).Milliseconds()
		if c.Expect.MaxMillis > 0 && r.Passed && r.Millis > int64(c.Expect.MaxMillis) {
			r.Passed = false
			r.Detail += fmt.Sprintf("; took %d ms (maximum %d)", r.Millis, c.Expect.MaxMillis)
		}
		if r.Passed {
			passed++
		}
		results = append(results, r)
	}
	return results, passed
}

type cmdResult struct {
	out  string
	exit int
}

func runCheckCommand(ctx context.Context, x containment, command string, timeoutSec int) cmdResult {
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()
	cmd := x.command(cctx, command)
	buf := core.NewCapWriter(512 << 10)
	cmd.Stdout, cmd.Stderr = buf, buf
	err := cmd.Start()
	if err == nil {
		process.AttachJob(cmd)
		err = cmd.Wait()
		process.ReleaseJob(cmd)
	}
	code := core.ExitCode(err)
	if cctx.Err() == context.DeadlineExceeded {
		code = -1
		_, _ = buf.Write([]byte(fmt.Sprintf("\n[timed out after %ds]", timeoutSec)))
	}
	return cmdResult{security.RedactSecrets(buf.String()), code}
}

func runCheck(ctx context.Context, x containment, evDir string, c OracleCheck) CheckResult {
	safe := regexp.MustCompile(`[^\w.-]+`).ReplaceAllString(c.ID, "_")
	switch c.Kind {
	case "command":
		return checkCommand(ctx, x, evDir, safe, c)
	case "http":
		return checkHTTP(ctx, evDir, safe, c)
	case "browser":
		return checkBrowser(ctx, evDir, safe, c)
	case "file":
		return checkFile(x.snap, c)
	case "maxLines":
		return checkMaxLines(x.snap, evDir, safe, c)
	default:
		return CheckResult{ID: c.ID, Kind: c.Kind, Detail: "unknown check type: " + c.Kind}
	}
}

// writeEvidence records a check's evidence file under the run directory.
func writeEvidence(r *CheckResult, evDir, name, content string) {
	r.Evidence = name
	_ = os.WriteFile(filepath.Join(evDir, name), []byte(content), 0o644)
}

// textOK applies the expectation's text conditions to a check's output.
func textOK(c OracleCheck, text string) (bool, string) {
	if c.Expect.Contains != "" && !strings.Contains(text, c.Expect.Contains) {
		return false, fmt.Sprintf("does not contain %q", c.Expect.Contains)
	}
	if c.Expect.NotContains != "" && strings.Contains(text, c.Expect.NotContains) {
		return false, fmt.Sprintf("contains %q", c.Expect.NotContains)
	}
	if c.Expect.Matches != "" {
		re, err := regexp.Compile(c.Expect.Matches)
		if err != nil {
			return false, "invalid regex: " + err.Error()
		}
		if !re.MatchString(text) {
			return false, fmt.Sprintf("does not match /%s/", c.Expect.Matches)
		}
	}
	return true, ""
}

// checkCommand runs a command in the snapshot and checks its exit code and output.
func checkCommand(ctx context.Context, x containment, evDir, safe string, c OracleCheck) CheckResult {
	r := CheckResult{ID: c.ID, Kind: c.Kind}
	res := runCheckCommand(ctx, x, c.Run, c.TimeoutSec)
	writeEvidence(&r, evDir, safe+".log", fmt.Sprintf("$ %s\n%s\n[exit %d]\n", c.Run, res.out, res.exit))
	want := 0
	if c.Expect.Exit != nil {
		want = *c.Expect.Exit
	}
	if res.exit != want {
		r.Detail = fmt.Sprintf("exit %d (esperado %d)", res.exit, want)
		return r
	}
	ok, why := textOK(c, res.out)
	r.Passed, r.Detail = ok, core.FirstNonEmpty(why, fmt.Sprintf("exit %d", res.exit))
	return r
}

// checkHTTP requests a URL and checks the response status and body.
func checkHTTP(ctx context.Context, evDir, safe string, c OracleCheck) CheckResult {
	r := CheckResult{ID: c.ID, Kind: c.Kind}
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
	writeEvidence(&r, evDir, safe+".http", fmt.Sprintf("GET %s\nHTTP %d\n\n%s", c.URL, resp.StatusCode, b))
	want := c.Expect.Status
	if want == 0 {
		want = 200
	}
	if resp.StatusCode != want {
		r.Detail = fmt.Sprintf("HTTP %d (esperado %d)", resp.StatusCode, want)
		return r
	}
	ok, why := textOK(c, string(b))
	r.Passed, r.Detail = ok, core.FirstNonEmpty(why, fmt.Sprintf("HTTP %d", resp.StatusCode))
	return r
}

// checkFile checks whether a file in the snapshot exists as expected and its content.
func checkFile(snap string, c OracleCheck) CheckResult {
	r := CheckResult{ID: c.ID, Kind: c.Kind}
	p := filepath.Join(snap, filepath.FromSlash(c.Path))
	b, err := os.ReadFile(p)
	exists := err == nil
	wantExists := true
	if c.Expect.Exists != nil {
		wantExists = *c.Expect.Exists
	}
	if exists != wantExists {
		r.Detail = map[bool]string{true: "the file exists", false: "the file does not exist"}[exists]
		return r
	}
	if !exists {
		r.Passed, r.Detail = true, "does not exist, as expected"
		return r
	}
	ok, why := textOK(c, string(b))
	r.Passed, r.Detail = ok, core.FirstNonEmpty(why, fmt.Sprintf("%d bytes", len(b)))
	return r
}

// checkMaxLines checks that no file matching the glob in the snapshot exceeds the line limit.
func checkMaxLines(snap, evDir, safe string, c OracleCheck) CheckResult {
	r := CheckResult{ID: c.ID, Kind: c.Kind}
	re := lang.GlobToRegex(c.Glob)
	var over []string
	_ = fsx.Walk(snap, fsx.Snapshot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
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
	writeEvidence(&r, evDir, safe+".txt", strings.Join(over, "\n"))
	r.Passed = len(over) == 0
	r.Detail = map[bool]string{true: fmt.Sprintf("no file above %d lines", c.Max), false: fmt.Sprintf("%d file(s) above %d lines", len(over), c.Max)}[r.Passed]
	return r
}
