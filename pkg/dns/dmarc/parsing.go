package dmarc

import (
	"errors"
	"fmt"
	"strings"

	abnfUtils "github.com/altshiftab/utils_go/pkg/abnf/utils"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
)

var (
	ErrUnexpectedKey   = errors.New("unexpected key")
	ErrMultipleSameKey = errors.New("multiple keys with the same name")
)

var keyValueNames = []string{
	"dmarc-request",
	"dmarc-srequest",
	"dmarc-auri",
	"dmarc-furi",
	"dmarc-adkim",
	"dmarc-aspf",
	"dmarc-ainterval",
	"dmarc-fo",
	"dmarc-rfmt",
	"dmarc-percent",
}

// caseInsensitiveTags lists tags whose values are case-insensitive per the
// DMARC ABNF. They are lowercased on parse so analysis can use exact
// comparisons.
var caseInsensitiveTags = map[string]bool{
	"p":     true,
	"sp":    true,
	"adkim": true,
	"aspf":  true,
	"fo":    true,
	"rf":    true,
}

func ParseDmarcRecord(data []byte) (*Record, error) {
	paths, err := abnfUtils.GetParsedDataPaths(DmarcGrammar, data, "dmarc-record")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get parsed data paths: %w", err))
	}
	if len(paths) == 0 {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.ErrSyntaxError)
	}

	record := &Record{Raw: string(data)}
	fields := map[string]*string{
		"p":     &record.P,
		"sp":    &record.Sp,
		"rua":   &record.Rua,
		"ruf":   &record.Ruf,
		"adkim": &record.Adkim,
		"aspf":  &record.Aspf,
		"ri":    &record.Ri,
		"fo":    &record.Fo,
		"rf":    &record.Rf,
		"pct":   &record.Pct,
	}

	for _, termPath := range abnfUtils.SearchPath(paths[0], keyValueNames, 1, false) {
		keyValuePair := strings.SplitN(string(abnfUtils.ExtractPathValue(data, termPath)), "=", 2)
		key := strings.ToLower(strings.TrimSpace(keyValuePair[0])) //nolint:nilaway // strings.SplitN never returns nil for a non-zero count.
		value := strings.TrimSpace(keyValuePair[1])

		field, ok := fields[key]
		if !ok {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %w: %s", altshiftErrors.ErrSemanticError, ErrUnexpectedKey, key),
				key,
			)
		}

		if *field != "" {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %w: %s", altshiftErrors.ErrSemanticError, ErrMultipleSameKey, key),
				key,
			)
		}

		if caseInsensitiveTags[key] {
			value = strings.ToLower(value)
		}
		*field = value
	}

	return record, nil
}
