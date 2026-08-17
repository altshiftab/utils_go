package content_disposition

import (
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"

	"github.com/altshiftab/utils_go/pkg/abnf"
	abnfUtils "github.com/altshiftab/utils_go/pkg/abnf/utils"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	altshiftHttpTypes "github.com/altshiftab/utils_go/pkg/http/types"
)

var Grammar *abnf.Grammar

//go:embed grammar.abnf
var grammar []byte

var (
	ErrSemanticError                      = errors.New("semantic error")
	ErrNoFilenameLabel                    = errors.New("no filename label")
	ErrNotEnoughExtensionSubpaths         = errors.New("not enough extension subpaths")
	ErrDuplicateLabel                     = errors.New("duplicate label")
	ErrUnexpectedInterestingPathMatchRule = errors.New("unexpected interesting path match rule")
	ErrUnexpectedFilenameLabel            = errors.New("unexpected filename label")
)

func getValue(data []byte, path *abnf.Path) (string, error) {
	if path == nil {
		return "", nil
	}

	var value string

	quotedStringPath := abnfUtils.SearchPathSingleName(
		path,
		"quoted-string",
		1,
		false,
	)
	if quotedStringPath != nil {
		var err error
		quotedString := string(abnfUtils.ExtractPathValue(data, quotedStringPath))
		value, err = strconv.Unquote(quotedString)
		if err != nil {
			return "", &altshiftErrors.Error{
				Message: "An error occurred when unquoting a quoted-string.",
				Cause:   err,
				Input:   quotedString,
			}
		}
	} else {
		value = string(abnfUtils.ExtractPathValue(data, path))
	}

	return value, nil
}

// TODO: Handle all errors properly.

func Parse(data []byte) (*altshiftHttpTypes.ContentDisposition, error) {
	paths, err := abnfUtils.GetParsedDataPaths(Grammar, data, "content-disposition")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.ErrSyntaxError, data)
	}

	contentDisposition := altshiftHttpTypes.ContentDisposition{
		ExtensionParameters: make(map[string]string),
	}

	interestingPaths := abnfUtils.SearchPath(
		paths[0],
		[]string{"disposition-type", "filename-parm", "disp-ext-parm"},
		4,
		false,
	)

	for _, interestingPath := range interestingPaths {
		interestingPathMatchRule := interestingPath.MatchRule
		switch interestingPathMatchRule {
		case "disposition-type":
			contentDisposition.DispositionType = strings.ToLower(string(abnfUtils.ExtractPathValue(data, interestingPath)))
		case "filename-parm":
			subpaths := interestingPath.Subpaths
			if len(subpaths) < 1 {
				return nil, errors.Join(
					ErrSemanticError,
					&altshiftErrors.Error{
						Message: "No label was found for a filename parameter.",
						Cause:   ErrNoFilenameLabel,
						Input:   subpaths,
					},
				)
			}

			labelPath := subpaths[0]
			if labelPath == nil {
				return nil, errors.Join(
					ErrSemanticError,
					&altshiftErrors.Error{
						Message: "A filename label path is nil.",
						Cause:   nil_error.New("filename label path"),
					},
				)
			}

			filenameLabel := strings.ToLower(string(abnfUtils.ExtractPathValue(data, subpaths[0])))
			switch filenameLabel {
			case "filename":
				if contentDisposition.Filename != "" {
					return nil, errors.Join(
						ErrSemanticError,
						&altshiftErrors.Error{
							Message: fmt.Sprintf("A duplicate %s label was observed.", filenameLabel),
							Cause:   ErrDuplicateLabel,
							Input:   filenameLabel,
						},
					)
				}

				filenameValuePath := abnfUtils.SearchPathSingle(
					interestingPath,
					[]string{"value"},
					1,
					false,
				)
				if filenameValuePath == nil {
					return nil, errors.Join(
						ErrSemanticError,
						&altshiftErrors.Error{
							Message: "No value path was found for the filename parameter.",
							Cause:   nil_error.New("filename value path"),
						},
					)
				}

				value, err := getValue(data, filenameValuePath)
				if err != nil {
					return nil, &altshiftErrors.Error{
						Message: "An error occurred when obtaining a parameter value.",
						Cause:   err,
						Input:   filenameValuePath,
					}
				}

				contentDisposition.Filename = value
			case "filename*":
				if contentDisposition.FilenameAsterisk != "" {
					return nil, errors.Join(
						ErrSemanticError,
						&altshiftErrors.Error{
							Message: fmt.Sprintf("A duplicate %s label was observed.", filenameLabel),
							Cause:   ErrDuplicateLabel,
							Input:   filenameLabel,
						},
					)
				}

				filenameAsteriskExtValuePath := abnfUtils.SearchPathSingle(
					interestingPath,
					[]string{"ext-value"},
					1,
					false,
				)
				if filenameAsteriskExtValuePath == nil {
					return nil, errors.Join(
						ErrSemanticError,
						&altshiftErrors.Error{
							Message: "No value path was found for the filename* parameter.",
							Cause:   nil_error.New("filename ext value path"),
						},
					)
				}

				contentDisposition.FilenameAsterisk = string(abnfUtils.ExtractPathValue(data, filenameAsteriskExtValuePath))
			default:
				return nil, &altshiftErrors.Error{
					Message: "An unexpected filename label was observed.",
					Cause:   ErrUnexpectedFilenameLabel,
					Input:   filenameLabel,
				}
			}
		case "disp-ext-parm":
			subpaths := interestingPath.Subpaths
			if len(subpaths) != 3 {
				return nil, errors.Join(
					ErrSemanticError,
					&altshiftErrors.Error{
						Message: "Not enough extension subpaths are present.",
						Cause:   ErrNotEnoughExtensionSubpaths,
						Input:   subpaths,
					},
				)
			}

			labelPath := subpaths[0]
			if labelPath == nil {
				return nil, errors.Join(
					ErrSemanticError,
					&altshiftErrors.Error{
						Message: "An extension label path is nil.",
						Cause:   nil_error.New("extension label path"),
					},
				)
			}

			label := strings.ToLower(string(abnfUtils.ExtractPathValue(data, labelPath)))
			if label == "" {
				return nil, errors.Join(
					ErrSemanticError,
					&altshiftErrors.Error{
						Message: "An extension label is empty.",
						Cause:   empty_error.New("extension label"),
					},
				)
			}

			if _, ok := contentDisposition.ExtensionParameters[label]; ok {
				return nil, errors.Join(
					ErrSemanticError,
					&altshiftErrors.Error{
						Message: fmt.Sprintf("A duplicate %s label was observed.", label),
						Cause:   ErrDuplicateLabel,
						Input:   label,
					},
				)
			}

			valuePath := subpaths[2]
			if valuePath == nil {
				return nil, errors.Join(
					ErrSemanticError,
					&altshiftErrors.Error{
						Message: "An extension value path is nil.",
						Cause:   nil_error.New("extension value path"),
					},
				)
			}

			value, err := getValue(data, valuePath)
			if err != nil {
				return nil, &altshiftErrors.Error{
					Message: "An error occurred when obtaining a parameter value.",
					Cause:   err,
					Input:   valuePath,
				}
			}

			if value == "" {
				return nil, errors.Join(
					ErrSemanticError,
					&altshiftErrors.Error{
						Message: "An extension value is empty.",
						Cause:   empty_error.New("extension value"),
					},
				)
			}

			contentDisposition.ExtensionParameters[label] = value
		default:
			return nil, &altshiftErrors.Error{
				Message: "An unexpected interesting path match rule was observed.",
				Cause:   ErrUnexpectedInterestingPathMatchRule,
				Input:   interestingPathMatchRule,
			}
		}
	}

	if len(contentDisposition.ExtensionParameters) == 0 {
		contentDisposition.ExtensionParameters = nil
	}

	return &contentDisposition, nil
}

func init() {
	var err error
	Grammar, err = abnf.ParseABNF(grammar)
	if err != nil {
		panic(fmt.Sprintf("An error occurred when parsing the grammar: %v", err))
	}
}
