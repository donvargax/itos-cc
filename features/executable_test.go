package features

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBinaryExecutableBytes(t *testing.T) { ExecutableContract(t, binary{}) }
func TestBinaryExecutableRunsAfterOrdinaryWrite(t *testing.T) {
	checkExecutableRuns(t, runtime.GOOS)
}
func checkExecutableRuns(t *testing.T, goos string) {
	t.Helper()
	d := binary{}
	file := filepath.Join(t.TempDir(), "run.sh")
	text := "#!/bin/sh\nexit 0\n"
	program := file
	args := []string{}
	if goos == "windows" {
		file = filepath.Join(t.TempDir(), "run.cmd")
		text = "@exit /b 0\r\n"
		program = "cmd"
		args = []string{"/C", file}
	}
	if err := d.Write(file, []byte("ordinary")); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteExecutable(file, []byte(text)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if goos != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Fatal("overwrite left file non-executable")
	}
	if out, err := exec.CommandContext(t.Context(), program, args...).CombinedOutput(); err != nil {
		t.Fatalf("owned fixture cannot execute: %v %s", err, out)
	}
}
