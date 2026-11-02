package workspace

import (
	"infuse/internal/core"
	"path/filepath"
	"strconv"
)

// CarryTurn makes one turn's manifest the base of the next: what a turn changed before it could be
// audited (it failed, was cancelled, or infuse stopped) is still judged, by the turn that follows.
func CarryTurn(root, session string, from, to int) error {
	var m treeManifest
	if err := core.ReadJSON(filepath.Join(treeDir(root), session, strconv.Itoa(from)+".json"), &m); err != nil {
		return err
	}
	m.Turn = to
	return core.WriteJSON(filepath.Join(treeDir(root), session, strconv.Itoa(to)+".json"), m)
}
