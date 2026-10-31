package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A fake gh on PATH: the failed run's log is found for the current branch.
func TestFailedCILogsFromGh(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("fake gh written as a .bat")
	}
	testHome(t)
	root := gitRepo(t)
	bin := t.TempDir()
	bat := "@echo off\r\n" +
		"if \"%1 %2\"==\"run list\" (echo [{\"databaseId\":1,\"conclusion\":\"success\",\"name\":\"lint\",\"headSha\":\"aaaaaaaa1\"},{\"databaseId\":7,\"conclusion\":\"failure\",\"name\":\"testes\",\"headSha\":\"bbbbbbbb2\"}]& exit /b 0)\r\n" +
		"if \"%1 %2 %3\"==\"run view 7\" (echo FAIL soma.test.js: esperado 4, recebido 5& exit /b 0)\r\n" +
		"exit /b 1\r\n"
	_ = os.WriteFile(filepath.Join(bin, "gh.bat"), []byte(bat), 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := FailedCILogs(root)
	if err != nil || !strings.Contains(out, "Workflow: testes (run 7") || !strings.Contains(out, "esperado 4, recebido 5") {
		t.Fatalf("%v\n%s", err, out)
	}
}
