// Package client_code_generation writes the TypeScript a browser calls a mux's endpoints with:
// one function per endpoint, taking and returning the types the endpoint declares, so that a
// change to what an endpoint accepts is a compile error in the code calling it rather than a
// failure at the request.
//
// # The runtime the generated code imports
//
// What is generated is not standalone. It imports the functions that perform the request, encode a
// body and raise an error from a status from "@altshiftab/utils":
//
//	import {fetchEx, fetchWithRequest} from "@altshiftab/utils/http/utils";
//	import {BadStatusCodeError} from "@altshiftab/utils/http/errors";
//	import {encode as cborEncode} from "@altshiftab/utils/cbor";
//
// That package belongs to one organisation, and this one does not. Generated code that only
// compiles for whoever can install "@altshiftab/utils" is a poor thing for a general-purpose
// library to produce: a caller elsewhere gets TypeScript that does not build, and nothing in the
// options says why. The Go here needs nothing of it -- the coupling is in script.ts.tmpl alone.
//
// What it would take to be rid of: the module the runtime is imported from becomes an option, with
// the imports written against whatever it names. The names imported (fetchEx, BadStatusCodeError
// and the rest) would have to be part of that contract, or be named individually.
package client_code_generation

import (
	"bytes"
	_ "embed"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"text/template"
	"unicode"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	clientCodeGenerationErrors "github.com/altshiftab/utils_go/pkg/http/client_code_generation/errors"
	clientCodeGenerationTypes "github.com/altshiftab/utils_go/pkg/http/client_code_generation/types"
	"github.com/altshiftab/utils_go/pkg/http/client_code_generation/types/template_options"
	endpointPkg "github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	typeGenerationTypesContext "github.com/altshiftab/utils_go/pkg/type_export/types/context"
	typeGenerationTypescriptTypes "github.com/altshiftab/utils_go/pkg/type_export/typescript/types"
)

//go:embed script.ts.tmpl
var scriptTemplateData string

func endpointContentType(endpoint *endpointPkg.Endpoint) string {
	if endpoint == nil {
		return ""
	}
	if bodyLoader := endpoint.BodyLoader; bodyLoader != nil {
		return bodyLoader.ContentType
	}
	return ""
}

const (
	contentTypeCose = "application/cose"
	contentTypeJose = "application/jose"
)

// formInputType returns the TypeScript input type for form content types, for which the Go input
// type has no TypeScript counterpart, or an empty string for other content types.
func formInputType(contentType string) string {
	switch {
	case strings.HasPrefix(contentType, "multipart/form-data"):
		return "FormData"
	case strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
		return "URLSearchParams"
	}
	return ""
}

var scriptTemplate = template.Must(
	template.New("script").Funcs(template.FuncMap{
		"dict": func(values ...any) (map[string]any, error) {
			if len(values)%2 != 0 {
				return nil, clientCodeGenerationErrors.ErrOddDictArguments
			}
			m := make(map[string]any, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("%w: %T", clientCodeGenerationErrors.ErrNonStringDictKey, values[i])
				}
				m[key] = values[i+1]
			}
			return m, nil
		},
		// hasPrefix exposes strings.HasPrefix to templates
		"hasPrefix": func(s, prefix string) bool { return strings.HasPrefix(s, prefix) },
		// hasSuffix exposes strings.HasSuffix to templates
		"hasSuffix": func(s, suffix string) bool { return strings.HasSuffix(s, suffix) },
	}).Parse(scriptTemplateData),
)

func makeTypescriptContext(endpoints []*endpointPkg.Endpoint) (*typeGenerationTypescriptTypes.Context, error) {
	typesSet := make(map[reflect.Type]struct{})

	for _, endpoint := range endpoints {
		if endpoint == nil {
			continue
		}

		hint := endpoint.Hint
		if hint == nil {
			continue
		}

		if formInputType(endpointContentType(endpoint)) == "" {
			typesSet[hint.InputType] = struct{}{}
		}
		typesSet[hint.UrlInputType] = struct{}{}
		typesSet[hint.OutputType] = struct{}{}
	}

	var typeElements []any
	for t := range typesSet {
		if t == nil {
			continue
		}
		typeElements = append(typeElements, t)
	}

	tsContext := typeGenerationTypescriptTypes.Context{Context: typeGenerationTypesContext.New()}
	if err := tsContext.Add(typeElements...); err != nil {
		return nil, altshiftErrors.New(fmt.Errorf("typescript context add: %w", err), typeElements)
	}

	return &tsContext, nil
}

var emptyInterfaceType = reflect.TypeFor[any]()

// titleCase uppercases the first rune of s, leaving the rest unchanged.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func makePathPart(path string) string {
	segments := strings.Split(
		strings.ReplaceAll(
			strings.TrimPrefix(
				path,
				"/api/",
			),
			"/",
			"-",
		),
		"-",
	)

	var casedSegments []string
	for _, segment := range segments {
		casedSegments = append(casedSegments, titleCase(segment))
	}

	return strings.ReplaceAll(strings.Join(casedSegments, ""), ".", "")
}

func isEmptyInterfaceType(t reflect.Type) bool {
	if t == nil {
		return true
	}
	return t == emptyInterfaceType || (t.Kind() == reflect.Interface && t.NumMethod() == 0)
}

func makeTemplateInput(
	endpoints []*endpointPkg.Endpoint,
	tsContext *typeGenerationTypescriptTypes.Context,
	baseUrl *url.URL,
) ([]*clientCodeGenerationTypes.TemplateInput, error) {
	if tsContext == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("typescript context"))
	}

	if len(endpoints) == 0 {
		return nil, nil
	}

	var templateInputs []*clientCodeGenerationTypes.TemplateInput

	for _, endpoint := range endpoints {
		if endpoint == nil {
			continue
		}

		method := endpoint.Method
		if method == "" {
			return nil, altshiftErrors.NewWithTrace(empty_error.New("method"), endpoint)
		}

		path := endpoint.Path
		if path == "" {
			return nil, altshiftErrors.NewWithTrace(empty_error.New("url"), endpoint)
		}

		contentType := endpointContentType(endpoint)
		typescriptFormInputType := formInputType(contentType)
		if typescriptFormInputType != "" || contentType == contentTypeCose {
			switch method {
			case "GET", "HEAD", "DELETE":
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf(
						"%w: %q, %q",
						clientCodeGenerationErrors.ErrBodylessMethodContentType, contentType, method,
					),
					endpoint,
				)
			}
		}

		var outputContentType string
		var optionalOutput bool
		var binaryOutput bool
		typescriptInputType := "void"
		typescriptUrlInputType := "void"
		typescriptOutputType := "void"

		if typescriptFormInputType != "" {
			typescriptInputType = typescriptFormInputType
		}

		if hint := endpoint.Hint; hint != nil {
			outputContentType = hint.OutputContentType
			optionalOutput = hint.OutputOptional

			// TODO: Support COSE-encrypted responses.
			if outputContentType == contentTypeCose {
				return nil, altshiftErrors.NewWithTrace(
					fmt.Errorf("%w: %q", clientCodeGenerationErrors.ErrUnsupportedOutputContentType, outputContentType),
					endpoint,
				)
			}

			// An output content type that is neither JSON-ish, textual, nor JOSE
			// denotes a binary response (e.g. application/pdf); the generated
			// function returns it as a Blob.
			binaryOutput = outputContentType != "" &&
				outputContentType != "application/json" &&
				!strings.HasSuffix(outputContentType, "+json") &&
				!strings.HasPrefix(outputContentType, "text/") &&
				outputContentType != contentTypeJose
			if binaryOutput {
				if !isEmptyInterfaceType(hint.OutputType) {
					return nil, altshiftErrors.NewWithTrace(
						fmt.Errorf("%w: %q", clientCodeGenerationErrors.ErrBinaryOutputWithOutputType, outputContentType),
						endpoint,
					)
				}
				if optionalOutput {
					return nil, altshiftErrors.NewWithTrace(
						fmt.Errorf("%w: %q", clientCodeGenerationErrors.ErrOptionalBinaryOutput, outputContentType),
						endpoint,
					)
				}
			}

			inputType := hint.InputType
			if typescriptFormInputType == "" && !isEmptyInterfaceType(inputType) {
				typeScriptType, err := tsContext.GetTypeScriptType(inputType)
				if err != nil {
					return nil, altshiftErrors.New(
						fmt.Errorf("typescript context get typescript type (input): %w", err),
						inputType,
					)
				}
				typescriptInputType, err = typeScriptType.String()
				if err != nil {
					return nil, altshiftErrors.New(fmt.Errorf("typescript type string (input): %w", err), typeScriptType)
				}
			}

			urlInputType := hint.UrlInputType
			if isEmptyInterfaceType(urlInputType) {
				typescriptUrlInputType = "void"
			} else {
				typeScriptType, err := tsContext.GetTypeScriptType(urlInputType)
				if err != nil {
					return nil, altshiftErrors.New(
						fmt.Errorf("typescript context get typescript type (url input): %w", err),
						urlInputType,
					)
				}
				typescriptUrlInputType, err = typeScriptType.String()
				if err != nil {
					return nil, altshiftErrors.New(fmt.Errorf("typescript type string (url input): %w", err), typeScriptType)
				}
			}

			outputTpe := hint.OutputType
			if binaryOutput {
				typescriptOutputType = "Blob"
			} else if isEmptyInterfaceType(outputTpe) {
				typescriptOutputType = "void"
			} else {
				typeScriptType, err := tsContext.GetTypeScriptType(outputTpe)
				if err != nil {
					return nil, altshiftErrors.New(
						fmt.Errorf("typescript context get typescript type (output): %w", err),
						outputTpe,
					)
				}
				typescriptOutputType, err = typeScriptType.String()
				if err != nil {
					return nil, altshiftErrors.New(
						fmt.Errorf("typescript type string (output): %w", err),
						outputTpe,
					)
				}
			}
		}

		useAuthentication := !endpoint.Public

		var urlString string
		if baseUrl != nil {
			urlString = baseUrl.String() + path
		} else {
			urlString = path
		}

		templateInputs = append(
			templateInputs,
			&clientCodeGenerationTypes.TemplateInput{
				Name: fmt.Sprintf(
					"%s%s",
					strings.ToLower(method),
					makePathPart(path),
				),
				InputType:                 typescriptInputType,
				UrlInputType:              typescriptUrlInputType,
				ReturnType:                typescriptOutputType,
				URL:                       urlString,
				Method:                    endpoint.Method,
				ContentType:               contentType,
				ExpectedOutputContentType: outputContentType,
				UseAuthentication:         useAuthentication,
				OptionalOutput:            optionalOutput,
				BinaryOutput:              binaryOutput,
			},
		)
	}

	return templateInputs, nil
}

func Render(
	endpoints []*endpointPkg.Endpoint,
	baseUrl *url.URL,
	options ...template_options.Option,
) (string, error) {
	if len(endpoints) == 0 {
		return "", nil
	}

	tsContext, err := makeTypescriptContext(endpoints)
	if err != nil {
		return "", fmt.Errorf("make typescript context: %w", err)
	}

	templateInputs, err := makeTemplateInput(endpoints, tsContext, baseUrl)
	if err != nil {
		return "", altshiftErrors.New(fmt.Errorf("make template input: %w", err), tsContext)
	}

	var useEncryption bool
	var useCose bool
	var hasBinaryOutput bool
	for _, templateInput := range templateInputs {
		// Determine if any endpoint requires encryption (either request or response)
		if templateInput.ContentType == contentTypeJose || templateInput.ExpectedOutputContentType == contentTypeJose {
			useEncryption = true
		}
		if templateInput.ContentType == contentTypeCose {
			useCose = true
		}
		if templateInput.BinaryOutput {
			hasBinaryOutput = true
		}
	}

	templateOptions := template_options.New(options...)

	tsContextOutput, err := tsContext.Render()
	if err != nil {
		return "", altshiftErrors.New(fmt.Errorf("typescript context render: %w", err), tsContext)
	}

	var buffer bytes.Buffer
	data := map[string]any{
		"Endpoints": templateInputs,
		"Globals": &clientCodeGenerationTypes.GlobalTemplateInput{
			CseClientPublicJwkHeader: templateOptions.CseClientPublicJwkHeader,
			CseContentEncryption:     templateOptions.CseContentEncryption,
			CseKeyAlgorithm:          templateOptions.CseKeyAlgorithm,
			CseKeyAlgorithmCurve:     templateOptions.CseKeyAlgorithmCurve,
			UseEncryption:            useEncryption,
			UseCose:                  useCose,
			HasBinaryOutput:          hasBinaryOutput,
			AuthenticationMode:       templateOptions.AuthenticationMode,
			AcceptBaseUrlArgument:    templateOptions.AcceptBaseUrlArgument,
		},
	}
	if err := scriptTemplate.Execute(&buffer, data); err != nil {
		return "", altshiftErrors.NewWithTrace(fmt.Errorf("template execute: %w", err), data)
	}

	return fmt.Sprintf("%s\n%s\n", tsContextOutput, buffer.String()), nil
}
