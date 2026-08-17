package mail

import (
	"fmt"
	mailPkg "net/mail"
	"strings"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/mismatch_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/net/types/domain_parts"
)

func ValidateAddress(addressString string) error {
	trimmedAddressString := strings.TrimSpace(addressString)
	if trimmedAddressString == "" {
		return fmt.Errorf(
			"%w: %w",
			altshiftErrors.ErrValidationError,
			empty_error.New("address"),
		)
	}

	address, err := mailPkg.ParseAddress(trimmedAddressString)
	if err != nil {
		return fmt.Errorf("%w: parse address: %w", altshiftErrors.ErrValidationError, err)
	}

	exactAddress := address.Name == "" && address.Address == trimmedAddressString
	if !exactAddress {
		return fmt.Errorf(
			"%w: %w",
			altshiftErrors.ErrValidationError,
			mismatch_error.New("address", address.Address, trimmedAddressString),
		)
	}

	_, domain, found := strings.Cut(trimmedAddressString, "@")
	if !found {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, altshiftErrors.ErrBadSplit)
	}

	domainParts := domain_parts.New(domain)
	if domainParts == nil {
		return fmt.Errorf("%w: %w", altshiftErrors.ErrValidationError, nil_error.New("domain parts"))
	}

	return nil
}
