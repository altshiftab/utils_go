// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate go run github.com/altshiftab/utils_go/pkg/json/schema/internal/cmd/validatorgen

// Package validator contains functions to handle different schema arguments.
package validator

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"unicode/utf8"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/json/schema/notes"
	schema "github.com/altshiftab/utils_go/pkg/json/schema/types"
)

// ToInt converts arg into a types.PartInt.
func ToInt(arg schema.PartValue) (schema.PartInt, error) {
	switch v := arg.(type) {
	case schema.PartInt:
		return v, nil
	case schema.PartFloat:
		iv := math.Trunc(float64(v))
		if iv != float64(v) {
			return 0, fmt.Errorf("%w: got float %v, expect int", schema.ErrInvalidSchema, arg)
		}
		return schema.PartInt(int(iv)), nil
	default:
		return 0, fmt.Errorf("%w: got %T, expect int", schema.ErrInvalidSchema, arg)
	}
}

// ToFloat converts arg into a types.PartFloat.
func ToFloat(arg schema.PartValue) (schema.PartFloat, error) {
	switch v := arg.(type) {
	case schema.PartInt:
		return schema.PartFloat(v), nil
	case schema.PartFloat:
		return v, nil
	default:
		return 0, fmt.Errorf("%w: got %T, expect float", schema.ErrInvalidSchema, arg)
	}
}

// ValidateTrue is used for keywords that always match.
// These keywords have meaning for the schema, but don't affect
// whether the schema validates an instance.
func ValidateTrue(schema.PartValue, any, *schema.ValidationState) error {
	return nil
}

// ValidateAllOf implements the allOf keyword.
func ValidateAllOf(arg schema.PartSchemas, instance any, state *schema.ValidationState) error {
	subState, err := state.Child()
	if err != nil {
		return err
	}

	var keepNotes []notes.Notes
	var topErr error
	for i, s := range arg {
		if err := s.ValidateInPlaceSchema(instance, subState); err != nil {
			schema.AddError(&topErr, err, fmt.Sprintf("allOf/%d", i))
		} else {
			if !subState.Notes.IsEmpty() {
				keepNotes = append(keepNotes, subState.Notes)
			}
		}
		subState.Notes.Clear()
	}

	if topErr == nil {
		state.Notes.AddNotes(keepNotes...)
	}

	return topErr
}

// ValidateAnyOf implements the anyOf keyword.
func ValidateAnyOf(arg schema.PartSchemas, instance any, state *schema.ValidationState) error {
	subState, err := state.Child()
	if err != nil {
		return err
	}

	var keepNotes []notes.Notes
	ok := false
	var topErr error
	var branchErrs []error
	for i, s := range arg {
		if err := s.ValidateInPlaceSchema(instance, subState); err != nil {
			if !schema.IsValidationError(err) {
				schema.AddError(&topErr, err, "")
			} else {
				var be error
				schema.AddError(&be, err, fmt.Sprintf("anyOf/%d", i))
				branchErrs = append(branchErrs, be)
			}
		} else {
			ok = true
			if !subState.Notes.IsEmpty() {
				keepNotes = append(keepNotes, subState.Notes)
			}

			// Continue to check all subschemas to
			// check for errors and collect notes.
		}
		subState.Notes.Clear()
	}
	if !ok {
		schema.AddValidationErrorStruct(&topErr, &schema.ValidationError{Message: `no "anyof" schema matches`})
		for _, be := range branchErrs {
			schema.AddError(&topErr, be, "")
		}
	} else {
		state.Notes.AddNotes(keepNotes...)
	}

	return topErr
}

// ValidateOneOf implements the oneOf keyword.
func ValidateOneOf(arg schema.PartSchemas, instance any, state *schema.ValidationState) error {
	subState, err := state.Child()
	if err != nil {
		return err
	}

	var keepNotes notes.Notes
	c := 0
	var topErr error
	for _, s := range arg {
		if err := s.ValidateInPlaceSchema(instance, subState); err != nil {
			if !schema.IsValidationError(err) {
				schema.AddError(&topErr, err, "")
			}
		} else {
			c++
			keepNotes = subState.Notes
		}
		subState.Notes.Clear()
	}
	if c != 1 {
		if c == 0 {
			schema.AddValidationErrorStruct(&topErr, &schema.ValidationError{Message: `no match for "oneof" schema`})
		} else {
			schema.AddValidationErrorStruct(&topErr, &schema.ValidationError{Message: fmt.Sprintf(`%d matches for "oneof" schema`, c)})
		}
	} else {
		state.Notes.AddNotes(keepNotes)
	}
	return topErr
}

// ValidateNot implements the not keyword.
func ValidateNot(arg schema.PartSchema, instance any, state *schema.ValidationState) error {
	subState, err := state.Child()
	if err != nil {
		return err
	}

	if err := arg.S.ValidateInPlaceSchema(instance, subState); err != nil {
		if !schema.IsValidationError(err) {
			return err
		}
		state.Notes.AddNotes(subState.Notes)
		return nil
	} else {
		return &schema.ValidationError{
			Message: `"not" schema matched`,
		}
	}
}

// ValidateIf implements the if keyword.
// This is always valid, but records a note for the "then" and "else" keywords.
func ValidateIf(arg schema.PartSchema, instance any, state *schema.ValidationState) error {
	subState, err := state.Child()
	if err != nil {
		return err
	}

	ok := false
	if err := arg.S.ValidateInPlaceSchema(instance, subState); err != nil {
		if !schema.IsValidationError(err) {
			return err
		}
	} else {
		ok = true
		state.Notes.AddNotes(subState.Notes)
	}
	state.Notes.Set("if", ok)
	return nil
}

// ValidateThen implements the then keyword.
func ValidateThen(arg schema.PartSchema, instance any, state *schema.ValidationState) error {
	v, ok := state.Notes.Get("if")
	if !ok || !v.(bool) {
		return nil
	}

	subState, err := state.Child()
	if err != nil {
		return err
	}

	err = arg.S.ValidateInPlaceSchema(instance, subState)
	if err == nil {
		state.Notes.AddNotes(subState.Notes)
	}
	return err
}

// ValidateElse implements the else keyword.
func ValidateElse(arg schema.PartSchema, instance any, state *schema.ValidationState) error {
	v, ok := state.Notes.Get("if")
	if !ok || v.(bool) {
		return nil
	}

	subState, err := state.Child()
	if err != nil {
		return err
	}

	err = arg.S.ValidateInPlaceSchema(instance, subState)
	if err == nil {
		state.Notes.AddNotes(subState.Notes)
	}
	return err
}

// ValidateDependentSchemas implements the dependentSchemas keyword.
func ValidateDependentSchemas(arg schema.PartMapSchema, instance any, state *schema.ValidationState) error {
	subState, err := state.Child()
	if err != nil {
		return err
	}

	var keepNotes []notes.Notes
	var topErr error
	for name, s := range arg {
		_, _, ok := instanceField(name, instance)
		if !ok {
			continue
		}
		if err := s.ValidateInPlaceSchema(instance, subState); err != nil {
			schema.AddError(&topErr, err, "dependentSchemas/"+name)
		} else {
			if !subState.Notes.IsEmpty() {
				keepNotes = append(keepNotes, subState.Notes)
			}
		}
	}

	if topErr == nil {
		state.Notes.AddNotes(keepNotes...)
	}

	return topErr
}

// prefixItemsNote is the type of the note recorded for prefixItems.
// We need to track both the length of the array and the schema,
// as prefixItems only affects items in the same types.
type prefixItemsNote struct {
	idx    int
	schema *schema.Schema
}

// ValidatePrefixItems implements the prefixItems keyword.
func ValidatePrefixItems(arg schema.PartSchemas, instance any, state *schema.ValidationState) error {
	note := prefixItemsNote{
		idx:    len(arg),
		schema: state.Schema,
	}
	notes.AppendNote(&state.Notes, "prefixItems", note)

	applyDefaults := state.Opts != nil && state.Opts.ApplyDefaults

	if a, ok := instance.([]any); ok {
		// Skip reflection in the common case of a JSON array.
		for i, s := range arg {
			if i >= len(a) {
				break
			}

			val := a[i]
			if applyDefaults && reflect.ValueOf(val).IsZero() {
				pv, hasDefault := s.LookupKeyword("default")
				if hasDefault {
					val = pv.(schema.PartAny).V
					a[i] = val
				}
			}

			if err := validateItemSchema(s, val, i, state); err != nil {
				return err
			}
		}
	} else {
		v := reflect.ValueOf(instance)
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
			return nil
		}

		ln := v.Len()

		for i, s := range arg {
			if i >= ln {
				break
			}

			indexVal := v.Index(i)
			val := indexVal.Interface()
			if applyDefaults && indexVal.IsZero() {
				pv, hasDefault := s.LookupKeyword("default")
				if hasDefault {
					defVal := pv.(schema.PartAny).V
					if err := setDefault(indexVal, defVal); err != nil {
						return err
					}
					val = defVal
				}
			}

			if err := validateItemSchema(s, val, i, state); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateItemSchema validates one array element against a subschema,
// tracking the element index in the instance location.
func validateItemSchema(s *schema.Schema, val any, idx int, state *schema.ValidationState) error {
	state.PushInstanceToken(strconv.Itoa(idx))
	err := s.ValidateSubSchema(val, state)
	if err != nil {
		err = schema.EnsureInstanceLocation(err, state.InstancePointer())
	}
	state.PopInstanceToken()
	return err
}

// ValidateItems implements the items keyword.
func ValidateItems(arg schema.PartSchema, instance any, state *schema.ValidationState) error {
	idx := 0
	if pins, ok := state.Notes.Get("prefixItems"); ok {
		for _, pin := range pins.([]prefixItemsNote) {
			if pin.schema == state.Schema {
				idx = pin.idx
				break
			}
		}
	}

	if a, ok := instance.([]any); ok {
		// Skip reflection in the common case of a JSON array.

		if idx < len(a) {
			state.Notes.Set("items", true)
		}

		for ; idx < len(a); idx++ {
			if err := validateItemSchema(arg.S, a[idx], idx, state); err != nil {
				return err
			}
		}
	} else {
		v := reflect.ValueOf(instance)
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
			return nil
		}

		ln := v.Len()

		if idx < ln {
			state.Notes.Set("items", true)
		}

		for ; idx < ln; idx++ {
			e := v.Index(idx).Interface()
			if err := validateItemSchema(arg.S, e, idx, state); err != nil {
				return err
			}
		}
	}

	return nil
}

// ValidateContains implements the contains keyword.
func ValidateContains(arg schema.PartSchema, instance any, state *schema.ValidationState) error {
	// If there is a minContains keyword in the schema with value 0,
	// then "contains" is always valid.
	hasMinContainsZero := false
	for i := state.Index + 1; i < len(state.Schema.Parts); i++ {
		part := &state.Schema.Parts[i]
		if part.Keyword.Name == "minContains" {
			if i, ok := part.Value.(schema.PartInt); ok {
				if i == 0 {
					hasMinContainsZero = true
					break
				}
			}
		}
	}

	topOK := hasMinContainsZero
	var matched []int
	if s, ok := instance.([]any); ok {
		// Skip reflection in the common case of a JSON array.

		for i, e := range s {
			if err := arg.S.ValidateSubSchema(e, state); err == nil {
				topOK = true
				matched = append(matched, i)
			}
		}
	} else {
		v := reflect.ValueOf(instance)
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
			return nil
		}

		ln := v.Len()

		for i := range ln {
			e := v.Index(i).Interface()
			if err := arg.S.ValidateSubSchema(e, state); err == nil {
				topOK = true
				matched = append(matched, i)
			}
		}
	}

	if !topOK {
		return &schema.ValidationError{
			Message: `no array element matches "contains" schema`,
		}
	}

	notes.AppendNote(&state.Notes, "contains", matched...)

	return nil
}

// propertiesNote is the type of the node recorded for properties.
// We need to track the field and the schema,
// as additionalProperties looks for properties in the same types.
type propertiesNote struct {
	field  string
	schema *schema.Schema
}

// ValidateProperties implements the properties keyword.
func ValidateProperties(arg schema.PartMapSchema, instance any, state *schema.ValidationState) error {
	applyDefaults := state.Opts != nil && state.Opts.ApplyDefaults

	requiredPV, hasRequired := state.Schema.LookupKeyword("required")
	var required schema.PartStrings
	if hasRequired {
		required = requiredPV.(schema.PartStrings)
	}

	m, isMap := instance.(map[string]any)
	pm, isPtrToMap := instance.(*map[string]any)

	var topErr error
	for name, s := range arg {
		var (
			defaultVal any
			hasDefault bool
		)
		if applyDefaults && !slices.Contains(required, name) {
			var pv schema.PartValue
			pv, hasDefault = s.LookupKeyword("default")
			if hasDefault {
				defaultVal = pv.(schema.PartAny).V
			}
		}

		f, jsonName, ok := instanceField(name, instance)
		if !ok {
			// This field does not appear in the instance.

			// If we are applying defaults, and this is a map,
			// add an entry to the map.
			if hasDefault {
				if isMap {
					m[jsonName] = defaultVal
				} else if isPtrToMap {
					//nolint:nilaway // A nil default value is a valid map entry.
					(*pm)[jsonName] = defaultVal
				}

				// Add a note for additionalProperties to read.
				note := propertiesNote{
					field:  jsonName,
					schema: state.Schema,
				}
				notes.AppendNote(&state.Notes, "properties", note)
			}

			continue
		}

		if hasDefault {
			var set bool
			if isMap {
				_, have := m[jsonName]
				set = !have
			} else if isPtrToMap {
				_, have := (*pm)[jsonName]
				set = !have
			} else {
				set = reflect.ValueOf(f).IsZero()
			}
			if set {
				if err := setField(instance, jsonName, defaultVal); err != nil {
					return err
				}
				f = defaultVal
			}
		}

		// Track instance location for nested validation errors.
		state.PushInstanceToken(jsonName)
		if err := s.ValidateSubSchema(f, state); err != nil {
			// Ensure nested errors carry instance location pointer.
			err = schema.EnsureInstanceLocation(err, state.InstancePointer())
			schema.AddError(&topErr, err, "properties/"+name)
		}
		state.PopInstanceToken()

		// Add a note for additionalProperties to read.
		note := propertiesNote{
			field:  jsonName,
			schema: state.Schema,
		}
		notes.AppendNote(&state.Notes, "properties", note)
	}
	return topErr
}

// ValidatePatternProperties implements the patternProperties keyword.
func ValidatePatternProperties(arg schema.PartMapSchema, instance any, state *schema.ValidationState) error {
	// The argument is a mapping from regexp strings to schemas.
	// Compile the regexp strings.
	// TODO: Cache the regexp compilation somewhere?
	type regexpSchema struct {
		re *regexp.Regexp
		s  *schema.Schema
	}
	var res []regexpSchema
	for reString, s := range arg {
		re, err := regexp.Compile(reString)
		if err != nil {
			return motmedelErrors.NewWithTrace(
				fmt.Errorf(`"patternProperties" regexp %q failed: %w`, reString, err),
				reString,
			)
		}
		res = append(res, regexpSchema{re, s})
	}

	// Fetch all the field names found in the instance.
	names, ok := instanceFieldNames(instance)
	if !ok {
		return nil
	}

	// For each field name in the instance, look in the regexps.
	// If there is a match, validate against the corresponding types.
	var topErr error
	for name := range names.byExactName {
		for _, r := range res {
			if !r.re.MatchString(name) {
				continue
			}

			if vf, jsonName, ok := instanceField(name, instance); ok {
				state.PushInstanceToken(jsonName)
				if err := r.s.ValidateSubSchema(vf, state); err != nil {
					err = schema.EnsureInstanceLocation(err, state.InstancePointer())
					schema.AddError(&topErr, err, "patternProperties/"+name)
				}
				state.PopInstanceToken()

				// Add a note for additionalProperties to read.
				note := propertiesNote{
					field:  jsonName,
					schema: state.Schema,
				}
				notes.AppendNote(&state.Notes, "patternProperties", note)
			}
		}
	}
	return topErr
}

// ValidateAdditionalProperties implements the additionalProperties keyword.
func ValidateAdditionalProperties(arg schema.PartSchema, instance any, state *schema.ValidationState) error {
	names, ok := instanceFieldNames(instance)
	if !ok {
		return nil
	}

	found := make(map[string]bool)
	for _, key := range []string{"properties", "patternProperties"} {
		if notes, ok := state.Notes.Get(key); ok {
			for _, note := range notes.([]propertiesNote) {
				if note.schema == state.Schema {
					found[note.field] = true
				}
			}
		}
	}

	// When the argument is the "false" schema, any additional property is
	// simply unknown, and deserves a clearer message than
	// "false schema never matches".
	isFalseSchema := false
	if pv, ok := arg.S.LookupKeyword("$bool"); ok {
		isFalseSchema = !bool(pv.(schema.PartBool))
	}

	var topErr error
	for name := range names.byExactName {
		if found[name] {
			continue
		}
		if vf, _, ok := instanceField(name, instance); ok {
			state.PushInstanceToken(name)
			if err := arg.S.ValidateSubSchema(vf, state); err != nil {
				if isFalseSchema {
					if validationError, ok := errors.AsType[*schema.ValidationError](err); ok {
						validationError.Message = fmt.Sprintf("unknown property %q", name)
						validationError.KeywordLocation = "#"
					}
				}
				err = schema.EnsureInstanceLocation(err, state.InstancePointer())
				schema.AddError(&topErr, err, "additionalProperties")
			}
			state.PopInstanceToken()
		}
		note := propertiesNote{
			field:  name,
			schema: state.Schema,
		}
		notes.AppendNote(&state.Notes, "additionalProperties", note)
	}
	return topErr
}

// ValidatePropertyNames implements the propertyNames keyword.
func ValidatePropertyNames(arg schema.PartSchema, instance any, state *schema.ValidationState) error {
	names, ok := instanceFieldNames(instance)
	if !ok {
		return nil
	}
	var topErr error
	for name := range names.byExactName {
		if err := arg.S.ValidateSubSchema(name, state); err != nil {
			schema.AddError(&topErr, err, "propertyNames/"+name)
		}
	}
	return topErr
}

// ValidateUnevaluatedItems implements the unevaluatedItems keyword.
func ValidateUnevaluatedItems(arg schema.PartSchema, instance any, state *schema.ValidationState) error {
	if b, ok := state.Notes.Get("items"); ok {
		if b.(bool) {
			return nil
		}
	}

	if b, ok := state.Notes.Get("unevaluatedItems"); ok {
		if b.(bool) {
			return nil
		}
	}

	idx := 0
	if pins, ok := state.Notes.Get("prefixItems"); ok {
		for _, pin := range pins.([]prefixItemsNote) {
			idx = max(idx, pin.idx)
		}
	}
	var contains []int
	if c, ok := state.Notes.Get("contains"); ok {
		contains = c.([]int)
	}

	if a, ok := instance.([]any); ok {
		// Skip reflection in the common case of a JSON array.

		if idx < len(a) {
			state.Notes.Set("unevaluatedItems", true)
		}

		for ; idx < len(a); idx++ {
			if slices.Contains(contains, idx) {
				continue
			}
			if err := validateItemSchema(arg.S, a[idx], idx, state); err != nil {
				return err
			}
		}
	} else {
		v := reflect.ValueOf(instance)
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
			return nil
		}

		ln := v.Len()

		if idx < ln {
			state.Notes.Set("unevaluatedItems", true)
		}

		for ; idx < ln; idx++ {
			if slices.Contains(contains, idx) {
				continue
			}
			e := v.Index(idx).Interface()
			if err := validateItemSchema(arg.S, e, idx, state); err != nil {
				return err
			}
		}
	}

	return nil
}

// ValidateUnevaluatedProperties implements the unevaluatedProperties keyword.
func ValidateUnevaluatedProperties(arg schema.PartSchema, instance any, state *schema.ValidationState) error {
	// Collect all the names seen by the properties or
	// patternProperties or additionalProperties keywords.
	// The keyword sorting order must ensure that unevaluatedProperties
	// follows those keywords.
	found := make(map[string]bool)
	for _, key := range []string{"properties", "patternProperties", "additionalProperties", "unevaluatedProperties"} {
		if notes, ok := state.Notes.Get(key); ok {
			for _, note := range notes.([]propertiesNote) {
				found[note.field] = true
			}
		}
	}

	names, ok := instanceFieldNames(instance)
	if !ok {
		return nil
	}

	var topErr error
	for name := range names.byExactName {
		if found[name] {
			continue
		}
		if vf, _, ok := instanceField(name, instance); ok {
			state.PushInstanceToken(name)
			if err := arg.S.ValidateSubSchema(vf, state); err != nil {
				err = schema.EnsureInstanceLocation(err, state.InstancePointer())
				schema.AddError(&topErr, err, "unevaluatedProperties/"+name)
			}
			state.PopInstanceToken()
		}
		note := propertiesNote{
			field:  name,
			schema: state.Schema,
		}
		notes.AppendNote(&state.Notes, "unevaluatedProperties", note)
	}

	return topErr
}

// typeNameInteger is the JSON type name for integers.
const typeNameInteger = "integer"

// ValidateType implements the type keyword.
func ValidateType(arg schema.PartStringOrStrings, instance any, state *schema.ValidationState) error {
	match := func(typ string) (bool, error) {
		switch typ {
		case "null":
			return instance == nil, nil
		case "boolean":
			if _, ok := instance.(bool); ok {
				return true, nil
			}
			if instance == nil {
				return false, nil
			}
			return reflect.TypeOf(instance).Kind() == reflect.Bool, nil
		case "object":
			if _, ok := instance.(map[string]any); ok {
				// JSON object
				return true, nil
			}
			if _, ok := instance.(*map[string]any); ok {
				// JSON object
				return true, nil
			}
			if instance == nil {
				return false, nil
			}
			typ := reflect.TypeOf(instance)
			if typ.Kind() == reflect.Pointer {
				typ = typ.Elem()
			}
			return typ.Kind() == reflect.Struct, nil
		case "array":
			typ := reflect.TypeOf(instance)
			return typ != nil && (typ.Kind() == reflect.Array || typ.Kind() == reflect.Slice), nil
		case "number":
			if instance == nil {
				return false, nil
			}
			//nolint:exhaustive // Only numeric kinds are numbers; the default case rejects the rest.
			switch reflect.TypeOf(instance).Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
				reflect.Float32, reflect.Float64:

				return true, nil
			default:
				return false, nil
			}
		case "string":
			if _, ok := instance.(string); ok {
				return true, nil
			}
			if instance == nil {
				return false, nil
			}
			return reflect.TypeOf(instance).Kind() == reflect.String, nil
		case typeNameInteger:
			if instance == nil {
				return false, nil
			}
			v := reflect.ValueOf(instance)
			if v.CanInt() || v.CanUint() {
				return true, nil
			}
			if v.CanFloat() {
				f := v.Float()
				return math.Trunc(f) == f && !math.IsInf(f, 0), nil
			}
			return false, nil
		default:
			return false, fmt.Errorf(`%w: "type" argument is unsupported string %q`, schema.ErrInvalidSchema, typ)
		}
	}

	typeForError := func(instance any) string {
		if instance == nil {
			return "null"
		}

		//nolint:exhaustive // The default case covers the remaining kinds with %T.
		switch reflect.TypeOf(instance).Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return typeNameInteger
		case reflect.Float32, reflect.Float64:
			f := reflect.ValueOf(instance).Float()
			if math.Trunc(f) == f && !math.IsInf(f, 0) {
				return typeNameInteger
			}
			return "number"
		case reflect.Bool:
			return "boolean"
		case reflect.Struct, reflect.Map:
			return "object"
		case reflect.Slice, reflect.Array:
			return "array"
		case reflect.String:
			return "string"
		default:
			return fmt.Sprintf("%T", instance)
		}
	}

	if arg.Strings == nil {
		ok, err := match(arg.String)
		if err != nil {
			return err
		}
		if !ok {
			return &schema.ValidationError{
				Message: fmt.Sprintf("instance has type %q, want %q", typeForError(instance), arg.String),
			}
		}
		return nil
	} else {
		for _, s := range arg.Strings {
			ok, err := match(s)
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
		}
		return &schema.ValidationError{
			Message: fmt.Sprintf("instance has type %q, want one of %v", typeForError(instance), arg),
		}
	}
}

// ValidateEnum implements the enum keyword.
func ValidateEnum(arg schema.PartAny, instance any, state *schema.ValidationState) error {
	// TODO: we have to be able to compare a map[string]any to a struct,
	// and a []any to a slice of some other type.
	s, ok := arg.V.([]any)
	if !ok {
		return fmt.Errorf(`%w: "enum" argument is %T, must be []any`, schema.ErrInvalidSchema, arg.V)
	}
	for _, e := range s {
		if reflect.DeepEqual(instance, e) {
			return nil
		}
	}
	return &schema.ValidationError{
		Message: `no "enum" value matched`,
	}
}

// ValidateConst implements the const keyword.
func ValidateConst(arg schema.PartAny, instance any, state *schema.ValidationState) error {
	// TODO: we have to be able to compare a map[string]any to a struct,
	// and a []any to a slice of some other type.
	if !reflect.DeepEqual(instance, arg.V) {
		return &schema.ValidationError{
			Message: fmt.Sprintf(`"const" failed: got %v, want %v`, instance, arg.V),
		}
	}
	return nil
}

// ValidateMultipleOf implements the multipleOf keyword.
func ValidateMultipleOf(arg schema.PartFloat, instance any, state *schema.ValidationState) error {
	if arg <= 0 {
		return fmt.Errorf(`%w: "multipleOf" argument is %v, must be greater than zero`, schema.ErrInvalidSchema, arg)
	}
	f, ok := instanceFloat(instance)
	if !ok {
		return nil
	}
	quo := f / float64(arg)
	if quo == math.Trunc(quo) && !math.IsInf(quo, 0) {
		return nil
	}
	if isDecimalMultipleOf(f, float64(arg)) {
		return nil
	}
	return &schema.ValidationError{
		Message: fmt.Sprintf(`"multipleof" failed: value %v is not a multiple of %v`, instance, arg),
	}
}

// isDecimalMultipleOf reports whether f is a multiple of arg using
// exact decimal arithmetic. JSON numbers are decimal literals, but they
// are decoded into binary floats, so a quotient like 19.99/0.01 is not
// exactly 1999 in float64. The shortest decimal representation of a
// float64 recovers the decimal literal that produced it, and dividing
// those as rationals gives the answer the schema author intended.
func isDecimalMultipleOf(f, arg float64) bool {
	rf, ok := new(big.Rat).SetString(strconv.FormatFloat(f, 'g', -1, 64))
	if !ok {
		return false
	}
	ra, ok := new(big.Rat).SetString(strconv.FormatFloat(arg, 'g', -1, 64))
	if !ok || ra.Sign() == 0 {
		return false
	}
	return new(big.Rat).Quo(rf, ra).IsInt()
}

// ValidateMaximum implements the maximum keyword.
func ValidateMaximum(arg schema.PartFloat, instance any, state *schema.ValidationState) error {
	f, ok := instanceFloat(instance)
	if !ok {
		return nil
	}
	if schema.PartFloat(f) > arg {
		return &schema.ValidationError{
			Message: fmt.Sprintf(`value %v is larger than "maximum" limit %v`, instance, arg),
		}
	}
	return nil
}

// ValidateExclusiveMaximum implements the exclusiveMaximum keyword.
func ValidateExclusiveMaximum(arg schema.PartFloat, instance any, state *schema.ValidationState) error {
	f, ok := instanceFloat(instance)
	if !ok {
		return nil
	}
	if schema.PartFloat(f) >= arg {
		return &schema.ValidationError{
			Message: fmt.Sprintf(`value %v is not less than "exclusiveMaximum" limit %v`, instance, arg),
		}
	}
	return nil
}

// ValidateMinimum implements the minimum keyword.
func ValidateMinimum(arg schema.PartFloat, instance any, state *schema.ValidationState) error {
	f, ok := instanceFloat(instance)
	if !ok {
		return nil
	}
	if schema.PartFloat(f) < arg {
		return &schema.ValidationError{
			Message: fmt.Sprintf(`value %v is smaller than "minimum" limit %v`, instance, arg),
		}
	}
	return nil
}

// ValidateExclusiveMinimum implements the exclusiveMinimum keyword.
func ValidateExclusiveMinimum(arg schema.PartFloat, instance any, state *schema.ValidationState) error {
	f, ok := instanceFloat(instance)
	if !ok {
		return nil
	}
	if schema.PartFloat(f) <= arg {
		return &schema.ValidationError{
			Message: fmt.Sprintf(`value %v is not greater than "exclusiveMinimum" limit %v`, instance, arg),
		}
	}
	return nil
}

// ValidateMaxLength implements the maxLength keyword.
func ValidateMaxLength(arg schema.PartInt, instance any, state *schema.ValidationState) error {
	if arg < 0 {
		return fmt.Errorf(`%w: "maxLength" argument is %d, must be non-negative`, schema.ErrInvalidSchema, arg)
	}
	if s, ok := instanceString(instance); ok {
		if schema.PartInt(utf8.RuneCountInString(s)) > arg {
			return &schema.ValidationError{
				Message: fmt.Sprintf(`value %q too long for "maxLength" argument %d`, s, arg),
			}
		}
	}
	return nil
}

// ValidateMinLength implements the minLength keyword.
func ValidateMinLength(arg schema.PartInt, instance any, state *schema.ValidationState) error {
	if arg < 0 {
		return fmt.Errorf(`%w: "minLength" argument is %d, must be non-negative`, schema.ErrInvalidSchema, arg)
	}
	if s, ok := instanceString(instance); ok {
		if schema.PartInt(utf8.RuneCountInString(s)) < arg {
			return &schema.ValidationError{
				Message: fmt.Sprintf(`value %q too short for "minLength" argument %d`, s, arg),
			}
		}
	}
	return nil
}

// ValidatePattern implements the pattern keyword.
func ValidatePattern(arg schema.PartString, instance any, state *schema.ValidationState) error {
	s, ok := instanceString(instance)
	if !ok {
		return nil
	}

	re, err := regexp.Compile(string(arg))
	if err != nil {
		return motmedelErrors.NewWithTrace(
			fmt.Errorf(`"pattern" regexp %q failed: %w`, arg, err),
			string(arg),
		)
	}

	if !re.MatchString(s) {
		return &schema.ValidationError{
			Message: fmt.Sprintf(`"pattern" regexp %q did not match %q`, arg, s),
		}
	}

	return nil
}

// ValidateMaxItems implements the maxItems keyword.
func ValidateMaxItems(arg schema.PartInt, instance any, state *schema.ValidationState) error {
	var ln int
	if a, ok := instance.([]any); ok {
		ln = len(a)
	} else {
		v := reflect.ValueOf(instance)
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
			return nil
		}
		ln = v.Len()
	}

	if schema.PartInt(ln) > arg {
		return &schema.ValidationError{
			Message: fmt.Sprintf(`length %d too long for "maxItems" argument %d`, ln, arg),
		}
	}

	return nil
}

// ValidateMinItems implements the minItems keyword.
func ValidateMinItems(arg schema.PartInt, instance any, state *schema.ValidationState) error {
	var ln int
	if a, ok := instance.([]any); ok {
		ln = len(a)
	} else {
		v := reflect.ValueOf(instance)
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
			return nil
		}
		ln = v.Len()
	}

	if schema.PartInt(ln) < arg {
		return &schema.ValidationError{
			Message: fmt.Sprintf(`length %d too short for "minItems" argument %d`, ln, arg),
		}
	}

	return nil
}

// ValidateUniqueItems implements the uniqueItems keyword.
func ValidateUniqueItems(arg schema.PartBool, instance any, state *schema.ValidationState) error {
	if !arg {
		return nil
	}

	v := reflect.ValueOf(instance)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return nil
	}
	ln := v.Len()

	allComparable := true
	for i := range ln {
		if !v.Index(i).Comparable() {
			allComparable = false
			break
		}
	}

	if allComparable {
		m := make(map[any]bool)

		for i := range ln {
			evi := v.Index(i).Interface()
			if m[evi] {
				return &schema.ValidationError{
					Message: fmt.Sprintf(`"uniqueItems" failure: %v appears more than once`, evi),
				}
			}
			m[evi] = true
		}
	} else {
		for i := range ln {
			for j := i + 1; j < ln; j++ {
				if reflect.DeepEqual(v.Index(i).Interface(), v.Index(j).Interface()) {
					return &schema.ValidationError{
						Message: fmt.Sprintf(`"uniqueItems" failure: %v appears more than once`, v.Index(i).Interface()),
					}
				}
			}
		}
	}

	return nil
}

// ValidateMaxContains implements the maxContains keyword.
func ValidateMaxContains(arg schema.PartInt, instance any, state *schema.ValidationState) error {
	if matched, ok := state.Notes.Get("contains"); ok {
		ln := len(matched.([]int))
		if schema.PartInt(ln) > arg {
			return &schema.ValidationError{
				Message: fmt.Sprintf(`%d array elements match "contains" schema, more than "maxContains" requirement %d`, ln, arg),
			}
		}
	}
	return nil
}

// ValidateMinContains implements the minContains keyword.
func ValidateMinContains(arg schema.PartInt, instance any, state *schema.ValidationState) error {
	if matched, ok := state.Notes.Get("contains"); ok {
		ln := len(matched.([]int))
		if schema.PartInt(ln) < arg {
			return &schema.ValidationError{
				Message: fmt.Sprintf(`%d array elements match "contains" schema, less than "minContains" requirement %d`, ln, arg),
			}
		}
	}
	return nil
}

// ValidateMaxProperties implements the maxProperties keyword.
func ValidateMaxProperties(arg schema.PartInt, instance any, state *schema.ValidationState) error {
	names, ok := instanceFieldNames(instance)
	if !ok {
		return nil
	}
	ln := len(names.byExactName)
	if schema.PartInt(ln) > arg {
		return &schema.ValidationError{
			Message: fmt.Sprintf(`number of properties %d is more than "maxProperties" required %d`, ln, arg),
		}
	}
	return nil
}

// ValidateMinProperties implements the minProperties keyword.
func ValidateMinProperties(arg schema.PartInt, instance any, state *schema.ValidationState) error {
	names, ok := instanceFieldNames(instance)
	if !ok {
		return nil
	}
	ln := len(names.byExactName)
	if schema.PartInt(ln) < arg {
		return &schema.ValidationError{
			Message: fmt.Sprintf(`number of properties %d is less than "minProperties" required %d`, ln, arg),
		}
	}
	return nil
}

// ValidateRequired implements the required keyword.
func ValidateRequired(arg schema.PartStrings, instance any, state *schema.ValidationState) error {
	names, ok := instanceFieldNames(instance)
	if !ok {
		return nil
	}

	var topErr error
	for _, s := range arg {
		if _, found := names.byExactName[s]; !found {
			err := &schema.ValidationError{
				Message: fmt.Sprintf("missing required property %q", s),
			}
			schema.AddError(&topErr, err, "required")
		}
	}
	return topErr
}

// ValidateDependentRequired implements the dependentRequired keyword.
func ValidateDependentRequired(arg schema.PartAny, instance any, state *schema.ValidationState) error {
	m, ok := arg.V.(map[string]any)
	if !ok {
		return fmt.Errorf(`%w: "dependentRequired" argument type %T, want map[string]any`, schema.ErrInvalidSchema, arg)
	}

	names, ok := instanceFieldNames(instance)
	if !ok {
		return nil
	}

	for k, v := range m {
		if _, found := names.byExactName[k]; !found {
			continue
		}

		ns, ok := v.([]any)
		if !ok {
			return fmt.Errorf(`%w: "dependentRequired" element %q type %T, want []any`, schema.ErrInvalidSchema, k, v)
		}

		for _, e := range ns {
			n, ok := e.(string)
			if !ok {
				return fmt.Errorf(`%w: "dependentRequired" element %q element type %T, want string`, schema.ErrInvalidSchema, k, e)
			}
			if _, found := names.byExactName[n]; !found {
				return &schema.ValidationError{
					Message: fmt.Sprintf(`"dependentRequired" failure: have field %q but not field %q`, k, n),
				}
			}
		}
	}

	return nil
}

// formatValidator is the type of a function that validates a format.
type formatValidator func(any, *schema.ValidationState) error

// formatValidators maps format keywords to functions that validate them.
var formatValidators map[string]formatValidator

// formatValidatorsLock is a lock for formatValidators.
var formatValidatorsLock sync.Mutex

// RegisterFormatValidator records a validator to use for
// a format keyword.
func RegisterFormatValidator(format string, fv formatValidator) {
	formatValidatorsLock.Lock()
	defer formatValidatorsLock.Unlock()
	if formatValidators == nil {
		formatValidators = make(map[string]formatValidator)
	}
	formatValidators[format] = fv
}

// ValidateFormat implements the format keyword.
func ValidateFormat(arg schema.PartString, instance any, state *schema.ValidationState) error {
	if state.Opts == nil || !state.Opts.ValidateFormat {
		return nil
	}

	formatValidatorsLock.Lock()
	defer formatValidatorsLock.Unlock()
	fv := formatValidators[string(arg)]
	if fv == nil {
		return nil
	}
	err := fv(instance, state)
	if err != nil && !schema.IsValidationError(err) {
		err = &schema.ValidationError{
			Message: err.Error(),
		}
	}
	return err
}

// ValidateDefault implements the default keyword.
func ValidateDefault(arg schema.PartAny, instance any, state *schema.ValidationState) error {
	// This supplies a default value, but it always validates.
	return nil
}

// instanceFloat returns instance as a floating-point number,
// and reports whether the instance is a number.
// Strings are not numbers; numeric keywords ignore non-numeric instances.
func instanceFloat(instance any) (float64, bool) {
	v := reflect.ValueOf(instance)
	switch {
	case v.CanInt():
		return float64(v.Int()), true
	case v.CanUint():
		return float64(v.Uint()), true
	case v.CanFloat():
		return v.Float(), true
	default:
		return 0, false
	}
}

// instanceString returns instance as a string,
// and reports whether the instance is a string.
// This accepts defined types whose underlying type is string.
func instanceString(instance any) (string, bool) {
	if s, ok := instance.(string); ok {
		return s, true
	}
	if instance == nil {
		return "", false
	}
	if v := reflect.ValueOf(instance); v.Kind() == reflect.String {
		return v.String(), true
	}
	return "", false
}

// ValidateDependencies validates the draft7 dependencies keyword.
// This is also used for later drafts, as an optional feature.
func ValidateDependencies(arg schema.PartMapArrayOrSchema, instance any, state *schema.ValidationState) error {
	names, ok := instanceFieldNames(instance)
	if !ok {
		return nil
	}

	subState, err := state.Child()
	if err != nil {
		return err
	}

	var keepNotes []notes.Notes
	var topErr error
	for name, as := range arg {
		if _, found := names.byExactName[name]; !found {
			continue
		}

		if as.Schema != nil {
			if err := as.Schema.ValidateInPlaceSchema(instance, subState); err != nil {
				schema.AddError(&topErr, err, "dependencies/"+name)
			} else {
				if !subState.Notes.IsEmpty() {
					keepNotes = append(keepNotes, subState.Notes)
				}
			}
		} else {
			for _, n := range as.Array {
				if _, found := names.byExactName[n]; !found {
					return &schema.ValidationError{
						Message: fmt.Sprintf(`"dependencies" failure: have field %q but not field %q`, name, n),
					}
				}
			}
		}
	}

	if topErr == nil && len(keepNotes) > 0 {
		state.Notes.AddNotes(keepNotes...)
	}

	return topErr
}
