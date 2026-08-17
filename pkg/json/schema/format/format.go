// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package format defines format checkers for the format keyword.
// By default the format keyword is always accepted.
// If this package is imported, the format keyword will be verified
// as described by the JSON schema docs.
package format

import (
	"github.com/altshiftab/utils_go/pkg/json/schema/internal/validator"
	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// init registers the defined formats.
func init() {
	validator.RegisterFormatValidator("date", dateFormat)
	validator.RegisterFormatValidator("date-time", dateTimeFormat)
	validator.RegisterFormatValidator("duration", durationFormat)
	validator.RegisterFormatValidator("email", emailFormat)
	validator.RegisterFormatValidator("hostname", hostnameFormat)
	validator.RegisterFormatValidator("idn-email", idnEmailFormat)
	validator.RegisterFormatValidator("idn-hostname", idnHostnameFormat)
	validator.RegisterFormatValidator("ipv4", ipv4Format)
	validator.RegisterFormatValidator("ipv6", ipv6Format)
	validator.RegisterFormatValidator("iri", iriFormat)
	validator.RegisterFormatValidator("iri-reference", iriReferenceFormat)
	validator.RegisterFormatValidator("json-pointer", jsonPointerFormat)
	validator.RegisterFormatValidator("regex", regexFormat)
	validator.RegisterFormatValidator("relative-json-pointer", relativeJSONPointerFormat)
	validator.RegisterFormatValidator("time", timeFormat)
	validator.RegisterFormatValidator("uri", uriFormat)
	validator.RegisterFormatValidator("uri-reference", uriReferenceFormat)
	validator.RegisterFormatValidator("uuid", uuidFormat)
}

// RegisterFormatValidator registers a custom format validator.
// If a schema uses format with the given keyword, this function
// will be called to validate the schema. The function will be
// called with an instance value. If the format does not match
// the instance, the function should return an error.
func RegisterFormatValidator(format string, fv func(any, *schema.ValidationState) error) {
	validator.RegisterFormatValidator(format, fv)
}
