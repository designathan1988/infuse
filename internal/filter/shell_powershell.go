package filter

// The interposed PowerShell. On Windows neither Codex nor Gemini CLI runs bash: both look for pwsh on
// PATH, then powershell. The launcher puts this binary first on that PATH, under the name pwsh, for
// that one process. It applies what a command line allows without POSIX parsing being exact: the
// guard, the command rules, the journal, the sandbox and the trailer. Heredocs do not exist here, so
// there is no transform.

import (
	"fmt"
	"infuse/internal/config"
	"infuse/internal/core"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// PowerShellShimPath is where the launcher's PowerShell shim lives: a folder of its own, because the
// name must never reach a PATH outside a launched agent.
func PowerShellShimPath() string {
	name := "pwsh"
	if runtime.GOOS == "windows" {
		name = "pwsh.exe"
	}
	return filepath.Join(filepath.Dir(shimPath()), "ps", name)
}

// EnsurePowerShellShim copies this binary under the PowerShell name, on Windows only.
func EnsurePowerShellShim() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	return copySelf(PowerShellShimPath())
}

// powerShellCommandIndex finds -Command (any abbreviation PowerShell takes) and returns its position.
func powerShellCommandIndex(args []string) int {
	for i, a := range args {
		low := strings.ToLower(a)
		if strings.HasPrefix(low, "-c") && strings.HasPrefix("-command", low) {
			return i
		}
	}
	return -1
}

// realPowerShell finds the PowerShell the shim stands for, on PATH but never the shim's own folder.
func realPowerShell() string {
	skip := filepath.Dir(PowerShellShimPath())
	for _, name := range []string{"pwsh", "powershell"} {
		for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
			if strings.EqualFold(filepath.Clean(dir), skip) {
				continue
			}
			if p, err := exec.LookPath(filepath.Join(dir, name)); err == nil {
				return p
			}
		}
	}
	return ""
}

// RunPowerShell is the role of this binary when it is named pwsh or powershell.
func RunPowerShell(args []string, policyFor func(root string, protected []string) Policy) int {
	real := realPowerShell()
	if real == "" {
		fmt.Fprintln(os.Stderr, "infuse: no PowerShell found on PATH")
		return 127
	}
	cwd, _ := os.Getwd()
	root := core.FindRoot(cwd)
	i := powerShellCommandIndex(args)
	if i == -1 || i+1 >= len(args) {
		return passthrough(root, real, args)
	}
	started := time.Now().UnixMilli()
	startShellWatcher(root)
	rules := LoadRules(root)
	guard := NewGuard(root, policyFor(root, config.EffectiveSettings(root).ProtectedPaths))
	guarded, guardNotes, gerr := guard.GuardCommandMsgs(args[i+1], cwd)
	if gerr != nil {
		return reportRefusal(gerr)
	}
	rewritten, ruleLines := ApplyCommandRulesMsgs(root, rules, guarded)
	command, reguardNotes, gerr := guard.Reguard(guarded, rewritten, cwd)
	if gerr != nil {
		return reportRefusal(gerr)
	}
	recordCommand(root, guarded, TransformResult{Command: guarded})
	newArgs := append([]string{}, args...)
	newArgs[i+1] = command
	code := passthrough(root, real, newArgs)
	finishShellRun(root, cwd, guarded, nil, started, rules, append(append(guardNotes, ruleLines...), reguardNotes...))
	return code
}
