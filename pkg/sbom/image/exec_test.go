package image

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func execCommand(t *testing.T, name string, args ...string) error {
	t.Helper()
	return exec.CommandContext(context.Background(), name, args...).Run()
}

// writeScript writes an executable script with forks held off: a child forked meanwhile would keep it open for
// writing, and running it fail with ETXTBSY (go.dev/issue/22315).
func writeScript(t *testing.T, path string, content string) {
	t.Helper()
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil { //nolint:gosec // executable test script
		t.Fatalf("write script: %v", err)
	}
}
