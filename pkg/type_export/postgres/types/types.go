package types

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"

	motmedelErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	motmedelReflect "github.com/altshiftab/utils_go/pkg/reflect"
	typeExportErrors "github.com/altshiftab/utils_go/pkg/type_export/errors"
	postgresErrors "github.com/altshiftab/utils_go/pkg/type_export/postgres/errors"
	"github.com/altshiftab/utils_go/pkg/type_export/postgres/types/tag"
	typeExportContext "github.com/altshiftab/utils_go/pkg/type_export/types/context"
	"github.com/altshiftab/utils_go/pkg/type_export/types/type_declaration"
	"github.com/altshiftab/utils_go/pkg/utils"
)

// TODO: Move.

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func toSnakeCase(s string) string {
	s = matchFirstCap.ReplaceAllString(s, "${1}_${2}")
	s = matchAllCap.ReplaceAllString(s, "${1}_${2}")
	return strings.ToLower(s)
}

func resolveIdType(interfaceDeclaration *InterfaceDeclaration) (string, error) {
	if interfaceDeclaration == nil || interfaceDeclaration.InterfaceDeclaration == nil || interfaceDeclaration.c == nil {
		return "", nil
	}

	for _, property := range interfaceDeclaration.Properties {
		if property == nil {
			continue
		}

		propertyField := property.Field
		if propertyField == nil {
			continue
		}

		identifier := property.Identifier
		var typeString string

		postgresTag := tag.New(propertyField.Tag.Get("postgres"))
		if postgresTag != nil {
			if postgresTag.Skip {
				continue
			}

			if name := postgresTag.Name; name != "" {
				identifier = name
			}

			if tagType := postgresTag.Type; tagType != "" {
				typeString = tagType
			}
		}

		if identifier != "id" {
			continue
		}

		if typeString == "" {
			postgresType, err := interfaceDeclaration.c.GetPostgresType(property.Field.Type)
			if err != nil {
				return "", fmt.Errorf("context get postgres type: %w", err)
			}
			if utils.IsNil(postgresType) {
				return "", motmedelErrors.NewWithTrace(nil_error.New("postgres type"))
			}

			typeString, err = postgresType.String()
			if err != nil {
				return "", fmt.Errorf("postgres type string: %w", err)
			}
		}

		return typeString, nil
	}

	return "", nil
}

type AssociativeTable struct {
	Source *InterfaceDeclaration
	Target *InterfaceDeclaration
}

func (a *AssociativeTable) String() (string, error) {
	var propertyStrings []string

	source := a.Source
	if source == nil {
		return "", motmedelErrors.NewWithTrace(fmt.Errorf("%w (source)", nil_error.New("interface declaration")))
	}
	sourceName := a.Source.QualifiedName()

	sourceIdType, err := resolveIdType(source)
	if err != nil {
		return "", motmedelErrors.New(fmt.Errorf("resolve id type: %w", err), source)
	}
	if sourceIdType == "" {
		sourceIdType = "uuid"
	}

	target := a.Target
	if target == nil {
		return "", motmedelErrors.NewWithTrace(fmt.Errorf("%w (target)", nil_error.New("interface declaration")))
	}
	targetName := a.Target.QualifiedName()

	targetIdType, err := resolveIdType(target)
	if err != nil {
		return "", motmedelErrors.New(fmt.Errorf("resolve id type: %w", err), target)
	}
	if targetIdType == "" {
		targetIdType = "uuid"
	}

	for _, nameAndType := range [][2]string{{sourceName, sourceIdType}, {targetName, targetIdType}} {
		name, typ := nameAndType[0], nameAndType[1]
		propertyStrings = append(propertyStrings, fmt.Sprintf("\t%[1]s_id %s NOT NULL REFERENCES %[1]s(id) ON DELETE CASCADE", name, typ))
	}
	propertyStrings = append(propertyStrings, fmt.Sprintf("\tPRIMARY KEY (%s_id, %s_id)", sourceName, targetName))

	return fmt.Sprintf(
		"CREATE TABLE %s (\n%s\n);",
		fmt.Sprintf("%s_%s", sourceName, targetName),
		strings.Join(propertyStrings, ",\n"),
	), nil
}

func isTime(t reflect.Type) bool {
	return t.Name() == "Time" && t.PkgPath() == "time"
}

type Context struct {
	*typeExportContext.Context

	// Roots holds the top-level struct types registered through Add. Render
	// emits a table for a struct only if it is a root or is reachable from one
	// through a non-skipped field, so a type referenced solely by a
	// `postgres:"-"` field never produces an orphan table.
	Roots []reflect.Type
}

// Add registers the given values like the embedded context does and, in
// addition, records the resulting top-level struct types as roots so Render can
// distinguish persisted types from those reachable only via skipped fields.
func (c *Context) Add(values ...any) error {
	for _, value := range values {
		if rootType := rootStructType(value); rootType != nil {
			c.Roots = append(c.Roots, rootType)
		}
	}

	return c.Context.Add(values...)
}

// rootStructType mirrors the normalization the embedded context applies when
// deciding which values become top-level type declarations: indirection is
// removed and map/slice/array values are reduced to their element type. It
// returns nil for anything that does not resolve to a struct (which the
// embedded context likewise ignores).
func rootStructType(value any) reflect.Type {
	var reflectType reflect.Type
	switch v := value.(type) {
	case reflect.Type:
		reflectType = v
	case reflect.Value:
		reflectType = v.Type()
	default:
		reflectType = reflect.TypeOf(v)
	}
	if reflectType == nil {
		return nil
	}

	reflectType = motmedelReflect.RemoveIndirection(reflectType)
	if kind := reflectType.Kind(); kind == reflect.Map || kind == reflect.Slice || kind == reflect.Array {
		reflectType = motmedelReflect.RemoveIndirection(reflectType.Elem())
	}

	if reflectType.Kind() != reflect.Struct {
		return nil
	}

	return reflectType
}

func (c *Context) GetPostgresType(reflectType reflect.Type) (Type, error) {
	reflectType = motmedelReflect.RemoveIndirection(reflectType)

	var postgresType Type

	//exhaustive:ignore
	switch kind := reflectType.Kind(); kind {
	case reflect.Struct:
		if isTime(reflectType) {
			postgresType = Timestamp
		} else {
			typeDeclaration, err := utils.MapGetNonZero(c.TypeDeclarations, reflectType)
			if err != nil {
				return nil, motmedelErrors.New(fmt.Errorf("map get non zero: %w", err), c.TypeDeclarations, reflectType)
			}

			interfaceDeclaration, err := utils.Convert[*type_declaration.InterfaceDeclaration](typeDeclaration)
			if err != nil {
				return nil, motmedelErrors.New(fmt.Errorf("convert: %w", err), typeDeclaration)
			}

			if genericTypeInfo := interfaceDeclaration.GenericTypeInfo; genericTypeInfo != nil {
				return nil, motmedelErrors.NewWithTrace(postgresErrors.ErrGenericTypesUnsupported)
			}

			postgresType = (&InterfaceDeclaration{InterfaceDeclaration: interfaceDeclaration, c: c}).TypeReference()
		}
	case reflect.Int8, reflect.Uint8, reflect.Int16, reflect.Uint16:
		postgresType = SmallInt
	case reflect.Int32, reflect.Uint32, reflect.Int, reflect.Uint:
		postgresType = Integer
	case reflect.Int64, reflect.Uint64:
		postgresType = BigInt
	case reflect.Float32:
		postgresType = Real
	case reflect.Float64:
		postgresType = DoublePrecision
	case reflect.String:
		postgresType = Text
	case reflect.Bool:
		postgresType = Boolean
	case reflect.Slice, reflect.Array:
		elemType := motmedelReflect.RemoveIndirection(reflectType.Elem())
		if elemType.Kind() == reflect.Uint8 {
			postgresType = ByteA
			break
		}

		itemPostgresType, err := c.GetPostgresType(elemType)
		if err != nil {
			return nil, motmedelErrors.New(fmt.Errorf("context get postgres type: %w", err), elemType)
		}

		if typeReference, ok := itemPostgresType.(*TypeReference); ok {
			postgresType = &AssociativeTable{Target: typeReference.TypeDeclaration}
		} else {
			postgresType = &ArrayType{ItemsType: itemPostgresType}
		}
	case reflect.Interface:
		postgresType = Jsonb
	default:
		return nil, motmedelErrors.NewWithTrace(fmt.Errorf("%w: %T", typeExportErrors.ErrUnsupportedKind, kind), kind)
	}

	return postgresType, nil
}

// referencedStructTypes returns the struct types a field references for table
// purposes: the struct itself for a (pointer to) struct field, or the element
// struct for a slice/array of structs. time.Time and non-struct types yield
// nothing. This mirrors the reference shapes String emits (a reference column
// or an associative table).
func referencedStructTypes(fieldType reflect.Type) []reflect.Type {
	directType := motmedelReflect.RemoveIndirection(fieldType)
	//exhaustive:ignore
	switch directType.Kind() {
	case reflect.Struct:
		if isTime(directType) {
			return nil
		}
		return []reflect.Type{directType}
	case reflect.Slice, reflect.Array:
		elemType := motmedelReflect.RemoveIndirection(directType.Elem())
		if elemType.Kind() == reflect.Struct && !isTime(elemType) {
			return []reflect.Type{elemType}
		}
	}

	return nil
}

// persistedTableTypes computes the set of struct types that should be emitted as
// tables: the roots, plus every struct reachable from a root through a
// non-skipped field. A struct reachable only via `postgres:"-"` fields is
// excluded — it is still registered (other producers, e.g. TypeScript, need it)
// but must not be materialized as a table here.
func (c *Context) persistedTableTypes() map[reflect.Type]bool {
	persisted := map[reflect.Type]bool{}
	var worklist []reflect.Type

	enqueue := func(reflectType reflect.Type) {
		reflectType = motmedelReflect.RemoveIndirection(reflectType)
		if reflectType.Kind() != reflect.Struct || isTime(reflectType) {
			return
		}
		if persisted[reflectType] {
			return
		}
		persisted[reflectType] = true
		worklist = append(worklist, reflectType)
	}

	for _, root := range c.Roots {
		enqueue(root)
	}

	for len(worklist) > 0 {
		reflectType := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]

		typeDeclaration, ok := c.TypeDeclarations[reflectType]
		if !ok {
			continue
		}
		interfaceDeclaration, ok := typeDeclaration.(*type_declaration.InterfaceDeclaration)
		if !ok {
			continue
		}

		for _, property := range interfaceDeclaration.Properties {
			if property == nil || property.Field == nil {
				continue
			}

			if postgresTag := tag.New(property.Field.Tag.Get("postgres")); postgresTag != nil && postgresTag.Skip {
				continue
			}

			for _, referencedType := range referencedStructTypes(property.Field.Type) {
				enqueue(referencedType)
			}
		}
	}

	return persisted
}

func (c *Context) Render() (string, error) {
	persisted := c.persistedTableTypes()

	declarationToType := map[type_declaration.TypeDeclaration]reflect.Type{}
	for reflectType, typeDeclaration := range c.TypeDeclarations {
		declarationToType[typeDeclaration] = reflectType
	}

	var interfaceDeclarations []*InterfaceDeclaration

	for _, typeDeclaration := range c.TypeDeclarationsInOrder {
		switch v := any(typeDeclaration).(type) {
		case *type_declaration.InterfaceDeclaration:
			// Omit struct types reachable only through skipped fields; they are
			// registered for other producers but are not persisted tables.
			if reflectType, ok := declarationToType[typeDeclaration]; ok && !persisted[reflectType] {
				continue
			}

			interfaceDeclarations = append(
				interfaceDeclarations,
				&InterfaceDeclaration{InterfaceDeclaration: v, c: c},
			)
		}
	}

	var stringBuilder strings.Builder

	for i, interfaceDeclaration := range interfaceDeclarations {
		if i > 0 {
			stringBuilder.WriteString("\n")
		}
		d, err := interfaceDeclaration.String()
		if err != nil {
			return "", motmedelErrors.New(fmt.Errorf("interface declaration string: %w", err), interfaceDeclaration)
		}
		stringBuilder.WriteString(d)
		stringBuilder.WriteString("\n")
	}

	return stringBuilder.String(), nil
}

type InterfaceDeclaration struct {
	*type_declaration.InterfaceDeclaration
	c *Context
}

func (t *InterfaceDeclaration) String() (string, error) {
	genericTypeInfo := t.GenericTypeInfo
	if genericTypeInfo != nil {
		return "", motmedelErrors.NewWithTrace(postgresErrors.ErrGenericTypesUnsupported)
	}

	var associativeTables []string
	var indices []string

	var propertyLines []string
	var uniqueCompositeFields []string
	var primaryKeyObserved bool

	for _, property := range t.Properties {
		if property == nil {
			continue
		}

		field := property.Field
		if field == nil {
			return "", motmedelErrors.NewWithTrace(nil_error.New("property field"), property)
		}

		// A field tagged `postgres:"-"` is ignored entirely: it produces neither
		// a column nor an associative/reference table. This must be checked
		// before the field type is resolved so that skipping also suppresses the
		// associative table a struct slice would otherwise emit below.
		postgresTag := tag.New(field.Tag.Get("postgres"))
		if postgresTag != nil && postgresTag.Skip {
			continue
		}

		fieldType := field.Type
		postgresType, err := t.c.GetPostgresType(fieldType)
		if err != nil {
			return "", motmedelErrors.New(fmt.Errorf("context get postgres type: %w", err), fieldType)
		}
		if utils.IsNil(postgresType) {
			return "", motmedelErrors.NewWithTrace(nil_error.New("postgres type"))
		}

		if associativeTable, ok := postgresType.(*AssociativeTable); ok {
			associativeTable.Source = t
			tableString, err := associativeTable.String()
			if err != nil {
				return "", fmt.Errorf("type string: %w", err)
			}

			associativeTables = append(associativeTables, tableString)
			continue
		}

		typeString, err := postgresType.String()
		if err != nil {
			return "", fmt.Errorf("type string: %w", err)
		}

		identifier := property.Identifier
		var attributes []string
		optional := property.Optional

		if postgresTag != nil {
			if name := postgresTag.Name; name != "" {
				identifier = name
			}

			if tagType := postgresTag.Type; tagType != "" {
				typeString = tagType
			}

			if postgresTag.Nullable {
				optional = true
			}

			if postgresTag.UniqueComposite {
				uniqueCompositeFields = append(uniqueCompositeFields, identifier)
			}

			if postgresTag.Indexed {
				indices = append(
					indices,
					fmt.Sprintf("CREATE INDEX %[1]s_%[2]s_idx ON %[1]s(%[2]s);", t.QualifiedName(), identifier),
				)
			}

			if postgresTag.PrimaryKey {
				primaryKeyObserved = true
				attributes = append(attributes, "PRIMARY KEY")
			}

			if _, ok := postgresType.(*TypeReference); ok {
				if onUpdate := postgresTag.OnUpdate; onUpdate != "" {
					attributes = append(attributes, fmt.Sprintf("ON UPDATE %s", onUpdate))
				}

				if onDelete := postgresTag.OnDelete; onDelete != "" {
					attributes = append(attributes, fmt.Sprintf("ON DELETE %s", onDelete))
				}
			}

			if postgresTag.Default != "" {
				attributes = append(attributes, fmt.Sprintf("DEFAULT %s", postgresTag.Default))
			}

			if unique := postgresTag.Unique; unique {
				attributes = append(attributes, "UNIQUE")
			}

			if generated := postgresTag.Generated; generated != "" {
				attributes = append(attributes, fmt.Sprintf("GENERATED ALWAYS AS (%s)", generated))
			}

			if generatedStored := postgresTag.GeneratedStored; generatedStored != "" {
				attributes = append(attributes, fmt.Sprintf("GENERATED ALWAYS AS (%s) STORED", generatedStored))
			}

			if check := postgresTag.Check; check != "" {
				attributes = append(attributes, fmt.Sprintf("CHECK (%s)", check))
			}
		}

		if !optional {
			attributes = append(attributes, "NOT NULL")
		}

		var attributesString string
		if len(attributes) > 0 {
			attributesString = fmt.Sprintf(" %s", strings.Join(attributes, " "))
		}

		propertyLines = append(
			propertyLines,
			fmt.Sprintf("\t%s %s%s", identifier, typeString, attributesString),
		)
	}

	if len(uniqueCompositeFields) > 0 {
		propertyLines = append(
			propertyLines,
			fmt.Sprintf("\tUNIQUE (%s)", strings.Join(uniqueCompositeFields, ", ")),
		)
	}

	if !primaryKeyObserved {
		propertyLines = append(
			propertyLines,
			"\tid uuid PRIMARY KEY DEFAULT gen_random_uuid()",
		)
	}

	table := fmt.Sprintf(
		"CREATE TABLE %s (\n%s\n);",
		t.QualifiedName(),
		strings.Join(propertyLines, ",\n"),
	)

	return strings.Join(slices.Concat([]string{table}, associativeTables, indices), "\n\n"), nil
}

func (t *InterfaceDeclaration) QualifiedName() string {
	return toSnakeCase(t.Identifier)
}

func (t *InterfaceDeclaration) TypeReference() *TypeReference {
	return &TypeReference{TypeDeclaration: t}
}
