package errors

import (
	"errors"
)

var (
	ErrMultipleOptionsWithSameName = errors.New("multiple options with same name")
	ErrNameNotFound                = errors.New("name not found")
	ErrUnsetOption                 = errors.New("unset option")
	ErrUnexpectedArgument          = errors.New("unexpected argument")
	ErrMissingRequiredOption       = errors.New("missing required option")
	ErrUnexpectedOptionValue       = errors.New("unexpected option value")
	ErrAmbiguousOption             = errors.New("ambiguous option")
	ErrInvalidChoice               = errors.New("invalid choice")
	ErrMutuallyExclusiveOptions    = errors.New("mutually exclusive options")
	ErrUndeclaredOption            = errors.New("undeclared option")
	ErrMissingPositional           = errors.New("missing positional argument")
	ErrAmbiguousPositionals        = errors.New("more than one variadic positional argument")
	ErrHelp                        = errors.New("help requested")
	ErrCompletion                  = errors.New("completion requested")
)
