package retry_after

import (
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"

	"github.com/altshiftab/utils_go/pkg/abnf"
	abnfUtils "github.com/altshiftab/utils_go/pkg/abnf/utils"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

//go:embed grammar.abnf
var grammar []byte

var RetryAfterGrammar *abnf.Grammar

var (
	ErrInvalidHttpDate     = errors.New("invalid http date")
	ErrInvalidDelaySeconds = errors.New("invalid delay seconds")
	ErrNoPathMatch         = errors.New("neither HTTP-date or delay-seconds matched")
)

func Parse(data []byte) (*altshiftHttpTypes.RetryAfter, error) {
	paths, err := abnfUtils.GetParsedDataPaths(RetryAfterGrammar, data, "Retry-After")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.ErrSyntaxError, data)
	}

	retryAfter := &altshiftHttpTypes.RetryAfter{Raw: string(data)}

	path := paths[0]

	httpDatePath := abnfUtils.SearchPathSingleName(path, "HTTP-date", 2, false)
	if httpDatePath != nil {
		httpDateString := string(abnfUtils.ExtractPathValue(data, httpDatePath))
		if httpDateString == "" {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %w", altshiftErrors.ErrSemanticError, empty_error.New("http date")),
			)
		}

		httpDate, err := parseHttpDate(httpDateString)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf(
					"%w: %w: parse http date: %w",
					altshiftErrors.ErrSemanticError,
					ErrInvalidHttpDate,
					err,
				),
				httpDateString,
			)
		}

		retryAfter.WaitTime = httpDate

		return retryAfter, nil
	}

	delaySecondsPath := abnfUtils.SearchPathSingleName(path, "delay-seconds", 2, false)
	if delaySecondsPath != nil {
		delaySecondsString := string(abnfUtils.ExtractPathValue(data, delaySecondsPath))
		if delaySecondsString == "" {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %w", altshiftErrors.ErrSemanticError, empty_error.New("delay seconds")),
			)
		}

		delaySeconds, err := strconv.Atoi(delaySecondsString)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf(
					"%w: %w: strconv atoi: %w",
					altshiftErrors.ErrSemanticError,
					ErrInvalidDelaySeconds,
					err,
				),
				delaySecondsString,
			)
		}

		retryAfter.WaitTime = time.Duration(delaySeconds) * time.Second

		return retryAfter, nil
	}

	return nil, altshiftErrors.NewWithTrace(ErrNoPathMatch)
}

func init() {
	var err error
	RetryAfterGrammar, err = abnf.ParseABNF(grammar)
	if err != nil {
		panic(fmt.Sprintf("goabnf parse abnf (retry after grammar): %v", err))
	}
}

// httpDateLayouts are the three formats RFC 9110 Section 5.6.7 requires a
// recipient to accept: the IMF-fixdate a sender has to generate, and the two
// obs-date formats that a recipient still has to read. They correspond to
// the IMF-fixdate, rfc850-date and asctime-date of the grammar.
var httpDateLayouts = []string{time.RFC1123, time.RFC850, time.ANSIC}

// parseHttpDate reads an HTTP-date in any of the formats RFC 9110
// Section 5.6.7 defines. An asctime-date carries no zone, and RFC 9110 reads
// every HTTP-date as GMT, which is what time.Parse defaults to.
func parseHttpDate(value string) (time.Time, error) {
	for _, layout := range httpDateLayouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, altshiftErrors.NewWithTrace(ErrInvalidHttpDate, value)
}
