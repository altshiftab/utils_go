// Package js_error_report holds what a page's own JavaScript reports about the errors it ran into:
// the bodies of the reports it posts, as opposed to the ones a browser posts on its own through the
// Reporting API.
package js_error_report

type ErrorDetails struct {
	// Message is empty whenever the error was made without one -- `new Error()`, thrown or
	// rejected with -- which says nothing but is a thing that happens, and is reported as it
	// was found, an empty message being a value like any other to the page that reads it.
	Message string `json:"message,omitzero" jsonschema:"message,minlength:0"`
	Cause   any    `json:"cause,omitzero"`
	// Stack is empty whenever no JavaScript frame made the error. A DOMException raised inside
	// the browser -- the one a media element rejects play() with, say -- has the property and
	// has nothing to put in it, and a page reports the property as it found it, since only an
	// absent value is dropped on the way. Every string is otherwise held to a minimum length of
	// one, which refused exactly those reports: a 422 for the errors a page cannot explain
	// itself, and no record of them anywhere.
	Stack string `json:"stack,omitzero" jsonschema:"stack,minlength:0"`
	Name  string `json:"name,omitzero"`
	Code  int    `json:"code,omitzero"`
}

type BaseErrorBody struct {
	Type  string        `json:"type"`
	Raw   string        `json:"raw,omitzero"`
	Error *ErrorDetails `json:"error,omitzero"`
}

type ErrorBody struct {
	BaseErrorBody
	ColNo int `json:"colno"`
	// Filename is empty for a muted error -- what a page is told about an exception thrown by a
	// script from another origin, which it is deliberately told nothing about: the location is
	// reported as the empty string, the line and column as zero, and the message as "Script
	// error.". It stays required, because the browser always sends it; only the minimum length
	// goes, which would otherwise refuse every report of a cross-origin script.
	Filename string `json:"filename" jsonschema:"filename,minlength:0"`
	LineNo   int    `json:"lineno"`
	Message  string `json:"message"`
}
