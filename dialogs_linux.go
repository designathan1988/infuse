//go:build linux

package main

// Linux: the GTK file chooser (through zenity, which is GTK) or KDE's dialog under Plasma.

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

type linuxDialogs struct{}

func (linuxDialogs) run(gtk, kde []string) (string, error) {
	try := [][]string{gtk, kde}
	if strings.Contains(strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP")), "kde") {
		try = [][]string{kde, gtk}
	}
	for _, c := range try {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		out, err := exec.Command(c[0], c[1:]...).Output()
		if err != nil {
			return "", nil // cancelled
		}
		return strings.TrimSpace(string(out)), nil
	}
	return "", errors.New("nenhum diálogo de arquivos disponível (instale zenity)")
}

func (d linuxDialogs) PickFolder(title string) (string, error) {
	return d.run([]string{"zenity", "--file-selection", "--directory", "--title=" + title},
		[]string{"kdialog", "--getexistingdirectory", os.Getenv("HOME"), "--title", title})
}

func (d linuxDialogs) PickFiles(title string) ([]string, error) {
	out, err := d.run([]string{"zenity", "--file-selection", "--multiple", "--separator=\n", "--title=" + title},
		[]string{"kdialog", "--getopenfilename", os.Getenv("HOME"), "--multiple", "--separate-output", "--title", title})
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

func osDialogs(owner func() uintptr) NativeDialogs { return linuxDialogs{} }
