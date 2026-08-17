// Package tests runs the official JSON-Schema-Test-Suite
// (https://github.com/json-schema-org/JSON-Schema-Test-Suite)
// against this module's JSON schema implementation.
//
// The tests and remotes directories are vendored copies of the
// suite, synced by the testgen command via go generate.
package tests

//go:generate go run github.com/altshiftab/utils_go/pkg/json/schema/internal/cmd/testgen
