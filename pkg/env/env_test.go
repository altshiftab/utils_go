package env

import (
	"errors"
	"os"
	"testing"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"

	motmedelEnvErrors "github.com/altshiftab/utils_go/pkg/env/errors"
)

func TestGetEnvWithDefault_Set(t *testing.T) {
	t.Setenv("TEST_VAR_SET", "value")
	if got := GetEnvWithDefault("TEST_VAR_SET", "default"); got != "value" {
		t.Fatalf("expected %q, got %q", "value", got)
	}
}

func TestGetEnvWithDefault_Unset(t *testing.T) {
	t.Parallel()

	os.Unsetenv("TEST_VAR_UNSET")
	if got := GetEnvWithDefault("TEST_VAR_UNSET", "default"); got != "default" {
		t.Fatalf("expected %q, got %q", "default", got)
	}
}

func TestGetEnvWithDefault_Empty(t *testing.T) {
	t.Setenv("TEST_VAR_EMPTY", "")
	if got := GetEnvWithDefault("TEST_VAR_EMPTY", "default"); got != "default" {
		t.Fatalf("expected default for empty, got %q", got)
	}
}

func TestReadEnv_Set(t *testing.T) {
	t.Setenv("TEST_READ_SET", "hello")
	got, err := ReadEnv("TEST_READ_SET")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("expected %q, got %q", "hello", got)
	}
}

func TestReadEnv_NotPresent(t *testing.T) {
	t.Parallel()

	os.Unsetenv("TEST_READ_NOT_PRESENT")
	_, err := ReadEnv("TEST_READ_NOT_PRESENT")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, motmedelEnvErrors.ErrNotPresent) {
		t.Fatalf("expected ErrNotPresent, got %v", err)
	}
}

func TestReadEnv_Empty(t *testing.T) {
	t.Setenv("TEST_READ_EMPTY", "")
	_, err := ReadEnv("TEST_READ_EMPTY")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := errors.AsType[*empty_error.Error](err); !ok {
		t.Fatalf("expected an empty error, got %v", err)
	}
}

func TestPopEnv_Set(t *testing.T) {
	t.Setenv("TEST_POP_SET", "hello")
	got, err := PopEnv("TEST_POP_SET")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("expected %q, got %q", "hello", got)
	}
	if _, found := os.LookupEnv("TEST_POP_SET"); found {
		t.Fatal("expected variable to be unset after PopEnv")
	}
}

func TestPopEnv_NotPresent(t *testing.T) {
	t.Parallel()

	os.Unsetenv("TEST_POP_NOT_PRESENT")
	_, err := PopEnv("TEST_POP_NOT_PRESENT")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, motmedelEnvErrors.ErrNotPresent) {
		t.Fatalf("expected ErrNotPresent, got %v", err)
	}
}
