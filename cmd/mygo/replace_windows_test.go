//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The app a replacement replaced can still run from its old name while
// the next replacement comes, as mygo dev rebuilds it: Windows renames a
// running executable but deletes it only once it has stopped.
func TestReplacePathWhileOldOnesRun(t *testing.T) {
	ping := filepath.Join(os.Getenv("SystemRoot"), "System32", "PING.EXE")
	exe, err := os.ReadFile(ping)
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "App Dev.exe")
	run := func() {
		cmd := exec.Command(dst, "-n", "60", "127.0.0.1")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	}
	if err := os.WriteFile(dst, exe, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		run()
		src := filepath.Join(dir, "new.exe")
		if err := os.WriteFile(src, exe, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := replacePath(src, dst); err != nil {
			t.Fatalf("replacement %d: %v", i+1, err)
		}
	}
}
