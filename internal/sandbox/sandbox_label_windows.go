//go:build windows

package sandbox

// The labels of a project tree: Low where the contained process may write, explicit Medium on what it
// may not, and the temp and cache folders its toolchains need.

import (
	"fmt"
	"infuse/internal/config"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

const (
	labelLow    = "S:(ML;OICI;NW;;;LW)"
	labelMedium = "S:(ML;OICI;NW;;;ME)"
	// noDeleteChild refuses the parent's delete-child right, which would otherwise let a Low process
	// rename a Medium child away through its Low parent: deleting then depends on the child alone.
	// The right is FILE_DELETE_CHILD, 0x40, which SDDL spells DT (DC is 0x2, adding a file).
	noDeleteChild = "(D;;DT;;;WD)"
)

// labelTree makes root writable for the contained process and takes that back on every protected
// path that exists. Labels persist: each object is written once, and checking one is cheap.
func labelTree(root string, protected []string) error {
	if err := label(root, labelLow); err != nil {
		return err
	}
	if err := label(sandboxTemp(root), labelLow); err != nil {
		return err
	}
	infuse := filepath.Join(root, ".infuse")
	_ = os.MkdirAll(infuse, 0o755) // the label needs the folder; infuse owns it anyway
	keep := []string{infuse, filepath.Join(root, ".git", "hooks"), filepath.Join(root, ".git", "config")}
	for _, rel := range protectedRels(root, protected) {
		keep = append(keep, filepath.Join(root, filepath.FromSlash(rel)))
	}
	for _, p := range append(protectedRoots(root), sensitiveStores()...) {
		if isWithin(root, p) { // outside the root the default Medium label already keeps the agent out
			keep = append(keep, p)
		}
	}
	for _, p := range keep {
		if _, err := os.Stat(p); err == nil {
			if err := label(p, labelMedium); err != nil {
				return err
			}
			if err := denyDeleteChild(filepath.Dir(p)); err != nil {
				return err
			}
		}
	}
	return labelLinkedGitDir(root)
}

// labelLinkedGitDir opens what a linked worktree's git needs in the main repository, which lives
// outside the root: the worktree's own git folder and the shared stores, never config or hooks.
func labelLinkedGitDir(root string) error {
	b, err := os.ReadFile(filepath.Join(root, ".git"))
	if err != nil {
		return nil // an ordinary repository, or none: .git is a folder inside the tree
	}
	dir := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(b)), "gitdir:"))
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	common := filepath.Dir(filepath.Dir(dir))
	for _, p := range []string{dir, filepath.Join(common, "objects"), filepath.Join(common, "refs"), filepath.Join(common, "logs")} {
		if _, err := os.Stat(p); err == nil {
			if err := label(p, labelLow); err != nil {
				return err
			}
		}
	}
	return nil
}

// label writes an explicit, inheritable mandatory label unless the object already carries it.
func label(path, sddl string) error {
	cur, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.LABEL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if strings.Contains(cur.String(), strings.TrimPrefix(sddl, "S:")) {
		return nil
	}
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	sacl, _, err := sd.SACL()
	if err != nil {
		return err
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.LABEL_SECURITY_INFORMATION, nil, nil, nil, sacl); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// denyDeleteChild adds the non-inherited entry once, in front of the object's existing DACL.
func denyDeleteChild(dir string) error {
	cur, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}
	text := cur.String()
	if strings.Contains(text, noDeleteChild) {
		return nil
	}
	body := text[strings.Index(text, "D:")+2:]
	flags := ""
	if i := strings.IndexByte(body, '('); i > 0 {
		flags, body = body[:i], body[i:]
	}
	sd, err := windows.SecurityDescriptorFromString("D:" + flags + noDeleteChild + body)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}
	return nil
}

// PrepareRoot makes a new, empty folder a contained process's working tree before anything is written
// in it: the files inherit the label on creation, so a large tree never needs relabeling afterwards.
func PrepareRoot(path string) error { return label(path, labelLow) }

// sensitiveStores are the user's own stores: infuse's anchors and signing keys, shell credentials and
// agent configurations. They only need a label when a project contains them (a home folder opened as
// a project); everywhere else the default label already refuses writes.
func sensitiveStores() []string {
	var out []string
	if h, err := os.UserHomeDir(); err == nil {
		for _, name := range []string{".infuse", ".ssh", ".aws", ".gnupg", ".claude", ".codex", ".gemini"} {
			out = append(out, filepath.Join(h, name))
		}
	}
	return out
}

// protectedRoots are the other projects in the registry: a project opened inside another one must not
// become writable from the outer project's shell.
func protectedRoots(root string) []string {
	var out []string
	for _, p := range config.LoadProjects() {
		if p.Root != "" && !strings.EqualFold(filepath.Clean(p.Root), filepath.Clean(root)) {
			out = append(out, p.Root)
		}
	}
	return out
}

func isWithin(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
