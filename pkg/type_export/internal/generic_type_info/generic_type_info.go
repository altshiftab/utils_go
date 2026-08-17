package generic_type_info

import (
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	goTypes "go/types"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/altshiftab/utils_go/pkg/type_export/types/generic_type_info"
	"github.com/altshiftab/utils_go/pkg/type_export/types/shape"

	altshiftErrors "github.com/altshiftab/utils_go/pkg/errors"
	"github.com/altshiftab/utils_go/pkg/errors/types/empty_error"
	"github.com/altshiftab/utils_go/pkg/errors/types/nil_error"
	altshiftReflect "github.com/altshiftab/utils_go/pkg/reflect"
)

var (
	ErrNotTypeName = errors.New("not a type name")
	ErrNotNamed    = errors.New("not a named")
	ErrNotStruct   = errors.New("not a struct")
	ErrNotGeneric  = errors.New("not a generic type")
)

type goTypesMatch struct {
	param *goTypes.TypeParam
	kind  shape.Kind
}

func detectShapeTypes(
	t goTypes.Type,
	paramSet map[*goTypes.TypeParam]struct{},
) []goTypesMatch {
	switch tt := t.(type) {
	case *goTypes.TypeParam:
		if _, ok := paramSet[tt]; ok {
			return []goTypesMatch{{param: tt, kind: shape.KindDirect}}
		}
	case *goTypes.Pointer:
		var matches []goTypesMatch
		for _, m := range detectShapeTypes(tt.Elem(), paramSet) {
			matches = append(matches, goTypesMatch{param: m.param, kind: shape.KindPointer})
		}
		return matches
	case *goTypes.Slice:
		var matches []goTypesMatch
		for _, m := range detectShapeTypes(tt.Elem(), paramSet) {
			matches = append(matches, goTypesMatch{param: m.param, kind: shape.KindSlice})
		}
		return matches
	case *goTypes.Array:
		var matches []goTypesMatch
		for _, m := range detectShapeTypes(tt.Elem(), paramSet) {
			matches = append(matches, goTypesMatch{param: m.param, kind: shape.KindArray})
		}
		return matches
	case *goTypes.Map:
		var matches []goTypesMatch
		for _, m := range detectShapeTypes(tt.Elem(), paramSet) {
			matches = append(matches, goTypesMatch{param: m.param, kind: shape.KindMapValue})
		}
		for _, m := range detectShapeTypes(tt.Key(), paramSet) {
			matches = append(matches, goTypesMatch{param: m.param, kind: shape.KindMapKey})
		}
		return matches
	}

	return nil
}

func discoverUsingTypesImporter(pkgPath string, typeName string) (*generic_type_info.GenericTypeInfo, error) {
	defaultImporter := importer.Default()
	if defaultImporter == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("default importer"))
	}

	pkg, err := defaultImporter.Import(pkgPath)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("go importer default import: %w", err))
	}

	return discoverInTypesPackage(pkg, typeName)
}

func discoverInTypesPackage(pkg *goTypes.Package, typeName string) (*generic_type_info.GenericTypeInfo, error) {
	if pkg == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("package"))
	}

	pkgScope := pkg.Scope()
	if pkgScope == nil {
		return nil, altshiftErrors.NewWithTrace(nil_error.New("scope"))
	}

	object := pkgScope.Lookup(typeName)
	if object == nil {
		return nil, nil
	}

	objectWithName, ok := object.(*goTypes.TypeName)
	if !ok {
		return nil, altshiftErrors.NewWithTrace(ErrNotTypeName)
	}

	namedType, ok := objectWithName.Type().(*goTypes.Named)
	if !ok {
		return nil, altshiftErrors.NewWithTrace(ErrNotNamed)
	}

	structType, ok := namedType.Underlying().(*goTypes.Struct)
	if !ok {
		return nil, altshiftErrors.NewWithTrace(ErrNotStruct)
	}

	typeParameters := namedType.TypeParams()
	if typeParameters.Len() == 0 {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("type parameters"))
	}

	parameterNamesSet := map[*goTypes.TypeParam]struct{}{}
	parameterNames := make([]string, typeParameters.Len())
	for i := range typeParameters.Len() {
		typeParameter := typeParameters.At(i)
		parameterNamesSet[typeParameter] = struct{}{}
		parameterNames[i] = typeParameter.Obj().Name()
	}

	fieldNameToShapes := map[string][]shape.Shape{}
	paramToField := map[string]string{}
	for field := range structType.Fields() {
		matches := detectShapeTypes(field.Type(), parameterNamesSet)
		if len(matches) == 0 {
			continue
		}

		name := field.Name()
		for _, m := range matches {
			paramName := m.param.Obj().Name()
			fieldNameToShapes[name] = append(fieldNameToShapes[name], shape.Shape{Param: paramName, Kind: m.kind})
			if _, exists := paramToField[paramName]; !exists {
				paramToField[paramName] = name
			}
		}
	}

	return &generic_type_info.GenericTypeInfo{
		TypeParameterNames:           parameterNames,
		FieldNameToShapes:            fieldNameToShapes,
		TypeParameterNameToFieldName: paramToField,
	}, nil
}

type astMatch struct {
	param string
	kind  shape.Kind
}

func detectShapeAst(e ast.Expr, paramSet map[string]struct{}) []astMatch {
	switch ee := e.(type) {
	case *ast.Ident:
		if _, ok := paramSet[ee.Name]; ok {
			return []astMatch{{param: ee.Name, kind: shape.KindDirect}}
		}
	case *ast.StarExpr:
		var matches []astMatch
		for _, m := range detectShapeAst(ee.X, paramSet) {
			matches = append(matches, astMatch{param: m.param, kind: shape.KindPointer})
		}
		return matches
	case *ast.ArrayType:
		kind := shape.KindSlice
		if ee.Len != nil {
			kind = shape.KindArray
		}
		var matches []astMatch
		for _, m := range detectShapeAst(ee.Elt, paramSet) {
			matches = append(matches, astMatch{param: m.param, kind: kind})
		}
		return matches
	case *ast.MapType:
		var matches []astMatch
		for _, m := range detectShapeAst(ee.Value, paramSet) {
			matches = append(matches, astMatch{param: m.param, kind: shape.KindMapValue})
		}
		for _, m := range detectShapeAst(ee.Key, paramSet) {
			matches = append(matches, astMatch{param: m.param, kind: shape.KindMapKey})
		}
		return matches
	}

	return nil
}

func discoverInWorkingDir(typeName string) (*generic_type_info.GenericTypeInfo, error) {
	workingDirectoryPath, err := os.Getwd()
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(fmt.Errorf("os getwd: %w", err))
	}

	entries, err := os.ReadDir(workingDirectoryPath)
	if err != nil {
		return nil, altshiftErrors.NewWithTrace(
			fmt.Errorf("os read dir: %w", err),
			workingDirectoryPath,
		)
	}

	fileSet := token.NewFileSet()
	var files []*ast.File
	for _, entry := range entries {
		entryName := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(entryName, ".go") {
			continue
		}

		filePath := filepath.Join(workingDirectoryPath, entryName)
		file, err := parser.ParseFile(fileSet, filePath, nil, 0)
		if err != nil {
			return nil, altshiftErrors.NewWithTrace(
				fmt.Errorf("go parser parse file: %w", err),
				filePath,
			)
		}
		files = append(files, file)
	}

	for _, file := range files {
		for _, topLevelDeclaration := range file.Decls {
			genericDeclarationNode, ok := topLevelDeclaration.(*ast.GenDecl)
			if !ok || genericDeclarationNode.Tok != token.TYPE {
				continue
			}

			for _, spec := range genericDeclarationNode.Specs {
				// Find the type spec for the base type.

				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || typeSpec.Name == nil || typeSpec.Name.Name != typeName {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}

				// Extract the type parameters

				var paramNames []string
				paramSet := map[string]struct{}{}
				if typeParams := typeSpec.TypeParams; typeParams != nil {
					for _, field := range typeParams.List {
						for _, identifier := range field.Names {
							paramNames = append(paramNames, identifier.Name)
							paramSet[identifier.Name] = struct{}{}
						}
					}
				}
				// TODO: No parameters? Can this happen?
				if len(paramNames) == 0 {
					continue
				}

				fieldShapes := map[string][]shape.Shape{}
				paramToField := map[string]string{}
				for _, field := range structType.Fields.List {
					if len(field.Names) == 0 {
						continue
					}

					matches := detectShapeAst(field.Type, paramSet)
					if len(matches) == 0 {
						continue
					}

					for _, identifier := range field.Names {
						for _, m := range matches {
							fieldShapes[identifier.Name] = append(
								fieldShapes[identifier.Name],
								shape.Shape{Param: m.param, Kind: m.kind},
							)
							if _, exists := paramToField[m.param]; !exists {
								paramToField[m.param] = identifier.Name
							}
						}
					}
				}

				return &generic_type_info.GenericTypeInfo{
					TypeParameterNames:           paramNames,
					FieldNameToShapes:            fieldShapes,
					TypeParameterNameToFieldName: paramToField,
				}, nil
			}
		}
	}

	return nil, nil
}

func fullTypeName(t reflect.Type) string {
	name := t.Name()
	if name == "" {
		return t.String()
	}
	if pkgPath := t.PkgPath(); pkgPath != "" {
		return pkgPath + "." + name
	}
	return name
}

func parseTypeArgs(name string) []string {
	idx := strings.Index(name, "[")
	if idx == -1 {
		return nil
	}
	inner := name[idx+1 : len(name)-1]

	var args []string
	depth := 0
	start := 0
	for i, c := range inner {
		switch c {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(inner[start:i]))
				start = i + 1
			}
		}
	}
	args = append(args, strings.TrimSpace(inner[start:]))
	return args
}

type reflectMatch struct {
	argIdx int
	kind   shape.Kind
}

func matchTypeArg(fieldType reflect.Type, typeArgs []string) []reflectMatch {
	ftn := fullTypeName(fieldType)
	for i, arg := range typeArgs {
		if ftn == arg {
			return []reflectMatch{{argIdx: i, kind: shape.KindDirect}}
		}
	}

	var matches []reflectMatch
	//exhaustive:ignore
	switch fieldType.Kind() {
	case reflect.Pointer:
		ftn = fullTypeName(fieldType.Elem())
		for i, arg := range typeArgs {
			if ftn == arg {
				matches = append(matches, reflectMatch{argIdx: i, kind: shape.KindPointer})
			}
		}
	case reflect.Slice:
		ftn = fullTypeName(fieldType.Elem())
		for i, arg := range typeArgs {
			if ftn == arg {
				matches = append(matches, reflectMatch{argIdx: i, kind: shape.KindSlice})
			}
		}
	case reflect.Array:
		ftn = fullTypeName(fieldType.Elem())
		for i, arg := range typeArgs {
			if ftn == arg {
				matches = append(matches, reflectMatch{argIdx: i, kind: shape.KindArray})
			}
		}
	case reflect.Map:
		ftn = fullTypeName(fieldType.Elem())
		for i, arg := range typeArgs {
			if ftn == arg {
				matches = append(matches, reflectMatch{argIdx: i, kind: shape.KindMapValue})
			}
		}
		ftn = fullTypeName(fieldType.Key())
		for i, arg := range typeArgs {
			if ftn == arg {
				matches = append(matches, reflectMatch{argIdx: i, kind: shape.KindMapKey})
			}
		}
	}

	return matches
}

func discoverUsingReflection(structType reflect.Type) (*generic_type_info.GenericTypeInfo, error) {
	typeArgs := parseTypeArgs(structType.Name())
	if len(typeArgs) == 0 {
		return nil, nil
	}

	paramNames := make([]string, len(typeArgs))
	for i := range typeArgs {
		paramNames[i] = fmt.Sprintf("T%d", i)
	}

	fieldNameToShapes := map[string][]shape.Shape{}
	paramToField := map[string]string{}

	for field := range structType.Fields() {
		matches := matchTypeArg(field.Type, typeArgs)
		if len(matches) == 0 {
			continue
		}

		for _, m := range matches {
			paramName := paramNames[m.argIdx]
			fieldNameToShapes[field.Name] = append(
				fieldNameToShapes[field.Name],
				shape.Shape{Param: paramName, Kind: m.kind},
			)
			if _, exists := paramToField[paramName]; !exists {
				paramToField[paramName] = field.Name
			}
		}
	}

	return &generic_type_info.GenericTypeInfo{
		TypeParameterNames:           paramNames,
		FieldNameToShapes:            fieldNameToShapes,
		TypeParameterNameToFieldName: paramToField,
	}, nil
}

func GetGenericTypeInfo(structType reflect.Type) (*generic_type_info.GenericTypeInfo, error) {
	structType = altshiftReflect.RemoveIndirection(structType)
	if structType.Kind() != reflect.Struct {
		return nil, altshiftErrors.NewWithTrace(ErrNotStruct)
	}

	typeName, isGenericType := altshiftReflect.GetTypeName(structType)
	if typeName == "" {
		return nil, altshiftErrors.NewWithTrace(empty_error.New("type name"))
	}
	if !isGenericType {
		return nil, altshiftErrors.NewWithTrace(ErrNotGeneric)
	}

	var genericTypeInfo *generic_type_info.GenericTypeInfo

	var workingDirErr error
	var importerErr error
	var reflectionErr error
	genericTypeInfo, workingDirErr = discoverInWorkingDir(typeName)

	if genericTypeInfo == nil {
		genericTypeInfo, importerErr = discoverUsingTypesImporter(structType.PkgPath(), typeName)
	}

	if genericTypeInfo == nil {
		genericTypeInfo, reflectionErr = discoverUsingReflection(structType)
	}

	if genericTypeInfo == nil {
		return nil, errors.Join(workingDirErr, importerErr, reflectionErr)
	}

	return genericTypeInfo, nil
}
