package image

import (
	"context"
	"os/exec"
	"testing"
)

func execCommand(t *testing.T, name string, args ...string) error {
	t.Helper()
	return exec.CommandContext(context.Background(), name, args...).Run()
}
