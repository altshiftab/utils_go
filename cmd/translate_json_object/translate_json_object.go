package main

import (
	"encoding/json/v2"
	"flag"
	"fmt"
	"log/slog"
	"os"

	motmedelEnv "github.com/altshiftab/utils_go/pkg/env"
	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/go/code_generation"
	"github.com/altshiftab/utils_go/pkg/go/code_generation/translate"
	motmedelLog "github.com/altshiftab/utils_go/pkg/log"
	motmedelContextLogger "github.com/altshiftab/utils_go/pkg/log/context_logger"
	errorLogger "github.com/altshiftab/utils_go/pkg/log/error_logger"
)

func run() error {
	var path string
	flag.StringVar(&path, "path", "", "path to generate code from")

	var packageName string
	flag.StringVar(
		&packageName,
		"package-name",
		motmedelEnv.GetEnvWithDefault("GOPACKAGE", "main"),
		"The name of the package in the output.",
	)

	flag.Parse()

	if path == "" {
		return empty_error.New("path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return motmedelErrors.NewWithTrace(fmt.Errorf("os read file: %w", err), path)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return motmedelErrors.NewWithTrace(fmt.Errorf("json unmarshal: %w", err), data)
	}

	code, err := translate.Map(m)
	if err != nil {
		return motmedelErrors.New(fmt.Errorf("translate map: %w", err), m)
	}

	output, err := code_generation.MakeFileContent(
		code,
		packageName,
		"translate_json_object",
		nil,
	)
	if err != nil {
		return motmedelErrors.New(fmt.Errorf("make file content: %w", err), code, packageName)
	}

	if fileName := code_generation.GetGeneratedFilename(); fileName != "" {
		if err := os.WriteFile(fileName, output, 0600); err != nil {
			return motmedelErrors.NewWithTrace(fmt.Errorf("os write file: %w", err), fileName, output)
		}
	} else {
		fmt.Println(string(output))
	}

	return nil
}

func main() {
	logger := errorLogger.Logger{
		Logger: motmedelContextLogger.New(
			slog.NewJSONHandler(os.Stderr, nil),
			&motmedelLog.ErrorContextExtractor{},
		),
	}
	slog.SetDefault(logger.Logger)

	if err := run(); err != nil {
		logger.FatalWithExitingMessage("An error occurred.", err)
	}
}
