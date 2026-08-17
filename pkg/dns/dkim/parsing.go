package dkim

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"iter"
	"regexp"
	"strings"

	"github.com/altshiftab/utils_go/pkg/abnf"
	abnfUtils "github.com/altshiftab/utils_go/pkg/abnf/utils"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
)

var (
	ErrVNotFirstTag           = errors.New("v not first tag")
	ErrMalformedTag           = errors.New("malformed tag")
	ErrMultipleTagPaths       = errors.New("multiple tag paths")
	ErrDuplicateTags          = errors.New("duplicate tags")
	ErrMissingPublicKeyData   = errors.New("missing public key data")
	ErrMalformedPublicKeyData = errors.New("malformed public key data")
	ErrUnexpectedTag          = errors.New("unexpected tag")
	ErrUnexpectedTagType      = errors.New("unexpected tag type")

	ErrMissingRequiredTag = errors.New("missing required tag")
)

var (
	reUnfold = regexp.MustCompile(`\r?\n[ \t]+`)
	reTabs   = regexp.MustCompile(`\t+`)
	reSpaces = regexp.MustCompile(` +`)
)

func extractTagPath(tagName string, tagValue []byte, tagType string) (*abnf.Path, error) {
	if tagName == "" {
		return nil, nil
	}

	if tagType == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("tag type"))
	}

	var ruleName string

	switch tagType {
	case "key":
		switch tagName {
		case "v", "h", "k", "n", "p", "s", "t":
			ruleName = fmt.Sprintf("key-%s-tag-root", tagName)
		default:
			return nil, nil
		}
	case "signature":
		switch tagName {
		case "v", "a", "b", "bh", "c", "d", "h", "i", "l", "q", "s", "t", "x", "z":
			ruleName = fmt.Sprintf("sig-%s-tag-root", tagName)
		default:
			return nil, nil
		}
	default:
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s", ErrUnexpectedTagType, tagType), tagType)
	}

	tagPaths, err := abnf.Parse(tagValue, DkimGrammar, ruleName)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("go abnf parse: %w", err), tagValue, DkimGrammar)
	}
	if len(tagPaths) == 0 {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w: %q", altshiftErrors.ErrSyntaxError, ErrMalformedTag, tagName),
			tagValue, DkimGrammar,
		)
	}
	if len(tagPaths) > 1 {
		return nil, altshiftErrors.NewWithTrace(ErrMultipleTagPaths, tagValue, DkimGrammar)
	}

	return tagPaths[0], nil
}

func extractBase64String(path *abnf.Path, value []byte) (string, error) {
	if path == nil {
		return "", nil
	}

	if len(value) == 0 {
		return "", altshiftErrors.NewWithTrace(empty_error.New("path input"))
	}

	var segments []string
	for _, p := range abnfUtils.SearchPath(path, []string{"ALPHADIGITPS"}, 2, false) {
		segments = append(segments, string(abnfUtils.ExtractPathValue(value, p)))
	}

	return strings.Join(segments, ""), nil
}

func normalizeEmailHeader(headerValue []byte) []byte {
	if len(headerValue) == 0 {
		return nil
	}

	unfolded := reUnfold.ReplaceAll(headerValue, []byte(" "))
	withoutTabs := reTabs.ReplaceAll(unfolded, []byte(""))
	collapsed := reSpaces.ReplaceAll(withoutTabs, []byte(" "))

	return bytes.TrimSpace(collapsed)
}

type tagSpecItem struct {
	Name  string
	Value []byte
}

func getTagSpecItems(path *abnf.Path, tagMap map[string]struct{}, data []byte) iter.Seq2[*tagSpecItem, error] {
	return func(yield func(*tagSpecItem, error) bool) {
		if tagMap == nil {
			yield(nil, altshiftErrors.NewWithTrace(nil_error.New("tag map")))
			return
		}

		if len(data) == 0 {
			yield(nil, altshiftErrors.NewWithTrace(empty_error.New("path input")))
			return
		}

		for _, tagSpecPath := range abnfUtils.SearchPath(path, []string{"tag-spec"}, 2, false) {
			tagNamePath := abnfUtils.SearchPathSingleName(tagSpecPath, "tag-name", 1, false)
			if tagNamePath == nil {
				yield(nil, altshiftErrors.NewWithTrace(nil_error.New("tag name path")))
				return
			}
			tagName := string(abnfUtils.ExtractPathValue(data, tagNamePath))
			if _, ok := tagMap[tagName]; ok {
				yield(
					nil,
					altshiftErrors.NewWithTrace(
						fmt.Errorf("%w: %w: %s", altshiftErrors.ErrSemanticError, ErrDuplicateTags, tagName),
					),
				)
				return
			}
			tagMap[tagName] = struct{}{}

			var tagValue []byte
			tagValuePath := abnfUtils.SearchPathSingleName(tagSpecPath, "tag-value", 1, false)
			if tagValuePath != nil {
				tagValue = bytes.TrimSpace(abnfUtils.ExtractPathValue(data, tagValuePath))
			}

			if !yield(&tagSpecItem{Name: tagName, Value: tagValue}, nil) {
				return
			}
		}
	}
}

func ParseRecord(data []byte) (*Record, error) {
	paths, err := abnfUtils.GetParsedDataPaths(DkimGrammar, data, "tag-list")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get parsed data paths: %w", err), data)
	}
	if len(paths) == 0 {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.ErrSyntaxError, data)
	}

	var record Record
	record.Raw = string(data)

	tagMap := make(map[string]struct{})

	i := 0
	for item, err := range getTagSpecItems(paths[0], tagMap, data) {
		if err != nil {
			return nil, fmt.Errorf("get tag spec item: %w", err)
		}
		if item == nil {
			return nil, altshiftErrors.NewWithTrace(nil_error.New("item"))
		}

		i += 1
		tagName := item.Name
		tagValue := item.Value

		tagPath, err := extractTagPath(tagName, tagValue, "key")
		if err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("extract tag path: %w", err), tagName, tagValue)
		}
		if tagPath == nil {
			record.Extensions = append(record.Extensions, [2]string{tagName, string(tagValue)})
			continue
		}
		if len(tagValue) == 0 {
			continue
		}

		switch tagName {
		case "v":
			if i != 1 {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("%w: %w", altshiftErrors.ErrSemanticError, ErrVNotFirstTag),
				)
			}
			record.Version = 1
		case "h":
			var algorithms []string
			for _, path := range abnfUtils.SearchPath(tagPath, []string{"key-h-tag-alg"}, 1, false) {
				algorithms = append(algorithms, string(abnfUtils.ExtractPathValue(tagValue, path)))
			}
			record.AcceptableHashAlgorithms = algorithms
		case "k":
			record.KeyType = string(
				abnfUtils.ExtractPathValue(
					tagValue,
					abnfUtils.SearchPathSingleName(tagPath, "key-k-tag-type", 1, false),
				),
			)
		case "n":
			record.Notes = string(
				abnfUtils.ExtractPathValue(
					tagValue,
					abnfUtils.SearchPathSingleName(tagPath, "qp-section", 1, false),
				),
			)
		case "p":
			base64String, err := extractBase64String(tagPath, tagValue)
			if err != nil {
				return nil, altshiftErrors.New(fmt.Errorf("extract base64 string: %w", err), tagName, tagValue)
			}
			record.PublicKeyData = base64String
		case "s":
			record.ServiceType = string(
				abnfUtils.ExtractPathValue(
					tagValue,
					abnfUtils.SearchPathSingleName(tagPath, "key-s-tag-type", 1, false),
				),
			)
		case "t":
			var flags []string
			for _, path := range abnfUtils.SearchPath(tagPath, []string{"key-t-tag-flag"}, 1, false) {
				flags = append(flags, string(abnfUtils.ExtractPathValue(tagValue, path)))
			}
			record.Flags = flags
		}
	}

	if _, ok := tagMap["p"]; !ok {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("%w: %w", altshiftErrors.ErrSemanticError, ErrMissingPublicKeyData),
		)
	}

	if _, err := record.GetPublicKey(); err != nil {
		return nil, altshiftErrors.New(
			fmt.Errorf("%w: %w: get public key: %w", altshiftErrors.ErrSemanticError, ErrMalformedPublicKeyData, err),
			record,
		)
	}

	return &record, nil
}

func ParseHeader(data []byte) (*Header, error) {
	normalizedData := normalizeEmailHeader(data)
	paths, err := abnfUtils.GetParsedDataPaths(DkimGrammar, normalizedData, "tag-list")
	if err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("get parsed data paths: %w", err), normalizedData)
	}
	if len(paths) == 0 {
		return nil, altshiftErrors.NewWithTrace(altshiftErrors.ErrSyntaxError, normalizedData)
	}

	var header Header
	header.Raw = string(data)

	tagMap := make(map[string]struct{})

	for item, err := range getTagSpecItems(paths[0], tagMap, normalizedData) {
		if err != nil {
			return nil, fmt.Errorf("get tag spec item: %w", err)
		}
		if item == nil {
			return nil, altshiftErrors.NewWithTrace(nil_error.New("item"))
		}

		tagName := item.Name
		tagValue := item.Value

		tagPath, err := extractTagPath(tagName, tagValue, "signature")
		if err != nil {
			return nil, altshiftErrors.New(fmt.Errorf("extract tag path: %w", err), tagName, tagValue)
		}
		if tagPath == nil {
			header.Extensions = append(header.Extensions, [2]string{tagName, string(tagValue)})
			continue
		}
		if len(tagValue) == 0 {
			continue
		}

		switch tagName {
		case "v":
			header.Version = 1
		case "a":
			header.Algorithm = string(tagValue)
		case "b":
			base64String, err := extractBase64String(tagPath, tagValue)
			if err != nil {
				return nil, altshiftErrors.New(fmt.Errorf("extract base64 string: %w", err), tagName, tagValue)
			}
			header.Signature = base64String
		case "bh":
			base64String, err := extractBase64String(tagPath, tagValue)
			if err != nil {
				return nil, altshiftErrors.New(fmt.Errorf("extract base64 string: %w", err), tagName, tagValue)
			}
			header.Hash = base64String
		case "c":
			header.MessageCanonicalization = string(tagValue)
		case "d":
			header.SigningDomainIdentifier = string(tagValue)
		case "h":
			var fields []string
			for _, path := range abnfUtils.SearchPath(tagPath, []string{"hdr-name"}, 1, false) {
				fields = append(fields, string(abnfUtils.ExtractPathValue(tagValue, path)))
			}
			header.SignedHeaderFields = fields
		case "i":
			header.AgentOrUserIdentifier = string(tagValue)
		case "l":
			header.BodyLengthCount = string(tagValue)
		case "q":
			var methods []string
			for _, path := range abnfUtils.SearchPath(tagPath, []string{"sig-q-tag-method"}, 1, false) {
				methods = append(methods, string(abnfUtils.ExtractPathValue(tagValue, path)))
			}
			header.QueryMethods = methods
		case "s":
			header.Selector = string(tagValue)
		case "t":
			header.SignatureTimestamp = string(tagValue)
		case "x":
			header.SignatureExpiration = string(tagValue)
		case "z":
			var fields [][2]string
			for _, path := range abnfUtils.SearchPath(tagPath, []string{"sig-z-tag-copy"}, 2, false) {
				namePath := abnfUtils.SearchPathSingleName(path, "hdr-name", 1, false)
				if namePath == nil {
					return nil, altshiftErrors.NewWithTrace(nil_error.New("header name"))
				}

				valuePath := abnfUtils.SearchPathSingleName(path, "qp-hdr-value", 1, false)
				if valuePath == nil {
					return nil, altshiftErrors.NewWithTrace(nil_error.New("header value"))
				}

				name := string(abnfUtils.ExtractPathValue(tagValue, namePath))
				value := string(abnfUtils.ExtractPathValue(tagValue, valuePath))

				fields = append(fields, [2]string{name, value})
			}

			header.CopiedHeaderFields = fields
		}
	}

	for _, tag := range []string{"v", "a", "b", "bh", "d", "h", "s"} {
		if _, ok := tagMap[tag]; !ok {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("%w: %w: %s", altshiftErrors.ErrSemanticError, ErrMissingRequiredTag, tag),
			)
		}
	}

	for _, tag := range []string{"b", "bh"} {
		var data string
		switch tag {
		case "b":
			data = header.Signature
		case "bh":
			data = header.Hash
		default:
			return nil, altshiftErrors.NewWithTrace(fmt.Errorf("%w: %s", ErrUnexpectedTag, tag))
		}

		if _, err = base64.StdEncoding.DecodeString(data); err != nil {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf(
					"%w: %s: base64 std encoding decode string: %w",
					altshiftErrors.ErrSemanticError, tag, err,
				),
				data,
			)
		}
	}

	return &header, nil
}
