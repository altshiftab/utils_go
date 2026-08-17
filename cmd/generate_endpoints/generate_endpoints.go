package main

import (
	"archive/zip"
	"bytes"
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	altshiftContext "github.com/altshiftab/utils_go/pkg/context"
	altshiftUtilsEnv "github.com/altshiftab/utils_go/pkg/env"
	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	"github.com/altshiftab/utils_go/pkg/go/code_generation"
	altshiftHttpContext "github.com/altshiftab/utils_go/pkg/http/context"
	"github.com/altshiftab/utils_go/pkg/http/mux/types/endpoint"
	"github.com/altshiftab/utils_go/pkg/http/types/fetch_config"
	"github.com/altshiftab/utils_go/pkg/http/types/http_context_extractor"
	altshiftHttpUtils "github.com/altshiftab/utils_go/pkg/http/utils"
	altshiftLog "github.com/altshiftab/utils_go/pkg/log"
	altshiftContextLogger "github.com/altshiftab/utils_go/pkg/log/context_logger"
	errorLogger "github.com/altshiftab/utils_go/pkg/log/error_logger"
)

func main() {
	logger := errorLogger.Logger{
		Logger: altshiftContextLogger.New(
			slog.NewJSONHandler(os.Stderr, nil),
			&altshiftLog.ErrorContextExtractor{},
			&http_context_extractor.Extractor{},
		),
	}
	slog.SetDefault(logger.Logger)

	var path string
	flag.StringVar(&path, "path", "", "path to generate code from")

	var packageName string
	flag.StringVar(
		&packageName,
		"package-name",
		altshiftUtilsEnv.GetEnvWithDefault("GOPACKAGE", "main"),
		"The name of the package in the output.",
	)

	var variableName string
	flag.StringVar(&variableName, "variable", "x", "The name of the variable in the output.")

	var addPathComment bool
	flag.BoolVar(&addPathComment, "add-path-comment", false, "Add a comment of the path.")

	var private bool
	flag.BoolVar(
		&private,
		"private",
		false,
		"Whether the generated static content is private. Affects Cache-Control.",
	)

	flag.Parse()

	if path == "" {
		logger.FatalWithExitingMessage("Empty path.", nil)
	}

	var endpoints []*endpoint.Endpoint
	resultingPaths := []string{path}

	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		ctxWithHttp := altshiftHttpContext.WithHttpContext(context.Background())
		httpClient := &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if request == nil {
					return altshiftErrors.NewWithTrace(nil_error.New("request"))
				}

				requestUrl := request.URL
				if requestUrl == nil {
					return altshiftErrors.NewWithTrace(nil_error.New("request url"))
				}

				resultingPaths = append(resultingPaths, requestUrl.String())

				return nil
			},
		}

		response, body, err := altshiftHttpUtils.Fetch(ctxWithHttp, path, fetch_config.WithHttpClient(httpClient))
		if err != nil {
			logger.ErrorContext(
				altshiftContext.WithError(
					ctxWithHttp,
					altshiftErrors.New(fmt.Errorf("fetch: %w", err), path),
				),
				"An error occurred when fetching. Exiting.",
			)
			os.Exit(1)
		}
		if response == nil {
			logger.ErrorContext(
				altshiftContext.WithError(
					ctxWithHttp,
					altshiftErrors.New(nil_error.New("http response"), path),
				),
				"The HTTP response is nil. Exiting.",
			)
			os.Exit(1)
		}

		bytesReader := bytes.NewReader(body)
		size := int64(len(body))
		zipReader, err := zip.NewReader(bytesReader, size)
		if err != nil {
			logger.FatalWithExitingMessage(
				"An error occurred when creating a zip reader. Does the body constitute a Zip file?",
				fmt.Errorf("zip new reader: %w", err),
				bytesReader, size,
			)
		}

		endpoints, err = endpoint.NewFromZip(zipReader, true, private)
		if err != nil {
			logger.FatalWithExitingMessage(
				"An error occurred when creating endpoint specifications from zip data.",
				fmt.Errorf("endpoint specifications from zip: %w", err),
				zipReader,
			)
		}
	} else {
		var err error
		endpoints, err = endpoint.NewFromDirectory(path, true, private)
		if err != nil {
			logger.FatalWithExitingMessage(
				"An error occurred when creating endpoint specifications from a directory.",
				fmt.Errorf("endpoint specifications from directory: %w", err),
				path,
			)
		}
	}

	output, err := code_generation.GetGeneratedFileContents(
		endpoints,
		packageName,
		"github.com/altshiftab/utils_go/cmd/generate_endpoints",
		variableName,
		nil,
	)
	if err != nil {
		logger.FatalWithExitingMessage(
			"An error occurred when obtaining the generated file contents.",
			fmt.Errorf("generated file contents: %w", err),
			endpoints, packageName, variableName,
		)
	}

	if addPathComment {
		var pathOutput []byte
		for _, resultingPath := range resultingPaths {
			pathOutput = fmt.Appendf(pathOutput, "// Path: %s\n", resultingPath)
		}

		output = append(pathOutput, output...)
	}

	if fileName := code_generation.GetGeneratedFilename(); fileName != "" {
		if err := os.WriteFile(fileName, output, 0600); err != nil {
			logger.FatalWithExitingMessage(
				"An error occurred when writing the file.",
				altshiftErrors.New(fmt.Errorf("os write file: %w", err), fileName, output),
			)
		}
	} else {
		fmt.Println(string(output))
	}
}
