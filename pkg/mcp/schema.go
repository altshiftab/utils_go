package mcp

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
)

// ReadArguments reads a call's arguments into input, refusing a member the tool does not take, as a
// *ToolError the model is shown.
func ReadArguments(arguments jsontext.Value, input any) error {
	if err := json.Unmarshal(arguments, input, json.RejectUnknownMembers(true)); err != nil {
		return &ToolError{Message: fmt.Sprintf("The arguments are not of the expected shape: %v", err)}
	}
	return nil
}

// ObjectSchema is the schema of a tool's arguments: an object of the properties, the required among
// them, and no others.
func ObjectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) != 0 {
		schema["required"] = required
	}
	return schema
}

// PropertySchema is the schema of one argument, of the JSON type -- a name, or a list of names for
// one of several -- with what else it says.
func PropertySchema(jsonType any, description string, more map[string]any) map[string]any {
	schema := map[string]any{"type": jsonType, "description": description}
	for key, value := range more {
		schema[key] = value
	}
	return schema
}

// ReadOnly is the annotations of a tool that changes nothing, which a client may run without asking.
func ReadOnly(title string) *Annotations {
	return &Annotations{Title: title, ReadOnlyHint: true, IdempotentHint: true}
}
