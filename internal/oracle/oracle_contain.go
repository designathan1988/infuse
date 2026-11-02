package oracle

// Everything the executor runs is agent-authored: the project's tests, its build, its server. It runs
// contained, with the snapshot as the only tree it may write, so a malicious test can reach neither the
// user's store nor the project's records. Its temp and toolchain caches live apart from the agent's
// shell, keyed by a secret the shell cannot read, or a planted build cache could answer for the tests.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/platform/process"
	"infuse/internal/sandbox"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// containment is how one run starts its children.
type containment struct {
	snap    string
	key     string
	setting *bool
}

func newContainment(evidenceRoot, snap string) containment {
	return containment{snap: snap, key: isolationKey(evidenceRoot), setting: config.EffectiveSettings(evidenceRoot).ContainProcesses}
}

// command builds a shell for script in the snapshot.
func (x containment) command(ctx context.Context, script string) *exec.Cmd {
	argv := sandbox.WrapIsolated([]string{core.FindRealBash(), "-c", script}, x.snap, nil, x.key, x.setting)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	process.NoConsoleWindow(cmd)
	cmd.Dir = x.snap
	cmd.Env = x.env()
	return cmd
}

// env is the environment of a check. Windows redirects temp and caches in the wrapper; elsewhere the
// contained process can only write the snapshot, so they are pointed there.
func (x containment) env() []string {
	env := append(os.Environ(), "CI=1")
	if runtime.GOOS == "windows" || !sandbox.Contained(x.setting) {
		return env
	}
	t := filepath.Join(x.snap, ".tmp")
	_ = os.MkdirAll(t, 0o755)
	return append(env, "TMPDIR="+t, "XDG_CACHE_HOME="+t, "GOCACHE="+filepath.Join(t, "go"), "npm_config_cache="+filepath.Join(t, "npm"))
}

// isolationKey is the secret that names this project's executor caches. It lives in the user's store,
// labeled no-read-up: a key the contained shell could read would give it the folder's name.
func isolationKey(root string) string {
	p := anchorFile(root) + ".key"
	if b, err := os.ReadFile(p); err == nil && len(b) >= 32 {
		return string(b)
	}
	protectStore()
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	key := hex.EncodeToString(b)
	_ = os.WriteFile(p, []byte(key), 0o600)
	_ = sandbox.LabelNoReadUp(p)
	return key
}
