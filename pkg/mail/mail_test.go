package mail

import (
	"errors"
	"testing"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
)

func TestValidateAddress_Valid(t *testing.T) {
	t.Parallel()

	valid := []string{
		"foo@example.com",
		"foo.bar@example.co.uk",
		"a+b@example.com",
	}

	for _, addr := range valid {
		if err := ValidateAddress(addr); err != nil {
			t.Errorf("ValidateAddress(%q) unexpected error: %v", addr, err)
		}
	}
}

func TestValidateAddress_Empty(t *testing.T) {
	t.Parallel()
	err := ValidateAddress("")
	if err == nil {
		t.Fatal("expected error for empty address")
	}
	if !errors.Is(err, motmedelErrors.ErrValidationError) {
		t.Fatalf("expected ErrValidationError, got %v", err)
	}
}

func TestValidateAddress_Whitespace(t *testing.T) {
	t.Parallel()
	if err := ValidateAddress("   "); err == nil {
		t.Fatal("expected error for whitespace-only")
	}
}

func TestValidateAddress_Malformed(t *testing.T) {
	t.Parallel()

	invalid := []string{
		"not-an-email",
		"missing@",
		"@missing-local.com",
		"foo@bar@example.com",
	}

	for _, addr := range invalid {
		if err := ValidateAddress(addr); err == nil {
			t.Errorf("expected error for %q", addr)
		}
	}
}

func TestValidateAddress_RejectsNamedAddress(t *testing.T) {
	t.Parallel()
	// Addresses with names like "Name <foo@bar.com>" should be rejected
	// because ValidateAddress requires the parsed Address.Name to be empty.
	if err := ValidateAddress("Name <foo@example.com>"); err == nil {
		t.Fatal("expected error for named address")
	}
}
