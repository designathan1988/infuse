//go:build !windows

package main

// macOS/Linux: the shell runs under `script`, which allocates a pty, so no cgo or extra library is needed.
// Resizing is not forwarded (script keeps its initial size).

import (
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
)

type scriptPTY struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out io.ReadCloser
}

func startPTY(cwd string, cols, rows int) (ptyProcess, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("script", "-q", "/dev/null", shell, "-l")
	} else {
		cmd = exec.Command("script", "-qfc", shell+" -l", "/dev/null")
	}
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLUMNS="+strconv.Itoa(cols), "LINES="+strconv.Itoa(rows))
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &scriptPTY{cmd: cmd, in: in, out: out}, nil
}

func (p *scriptPTY) Read(b []byte) (int, error)  { return p.out.Read(b) }
func (p *scriptPTY) Write(b []byte) (int, error) { return p.in.Write(b) }
func (p *scriptPTY) Resize(cols, rows int) error { return nil }
func (p *scriptPTY) Close() error {
	_ = p.in.Close()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return nil
}
