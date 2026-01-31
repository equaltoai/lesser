package main

import (
	"crypto/sha1" // #nosec G505 -- used only for deterministic schema component names
	"fmt"
	"go/types"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

type schemaBuilder struct {
	spec   *openAPISpec
	pkgs   map[string]*packages.Package
	config map[string]schemaPackageConfig

	inProgress map[string]struct{}
}

type schemaPackageConfig struct {
	Prefix string
}

const (
	packagePathModels        = "github.com/equaltoai/lesser/cmd/api/models"
	packagePathAuth          = "github.com/equaltoai/lesser/pkg/auth"
	packagePathMastodon      = "github.com/equaltoai/lesser/pkg/mastodon"
	packagePathReputation    = "github.com/equaltoai/lesser/pkg/reputation"
	packagePathStorage       = "github.com/equaltoai/lesser/pkg/storage"
	packagePathStorageModels = "github.com/equaltoai/lesser/pkg/storage/models"

	schemaAnyObject = "AnyObject"
	schemaAnyArray  = "AnyArray"
)

func newSchemaBuilder(spec *openAPISpec, pkgs map[string]*packages.Package, config map[string]schemaPackageConfig) *schemaBuilder {
	return &schemaBuilder{
		spec:       spec,
		pkgs:       pkgs,
		config:     config,
		inProgress: map[string]struct{}{},
	}
}

func ensureGeneratedSchemas(spec *openAPISpec, repoRoot string) (*schemaBuilder, error) {
	if spec == nil {
		return nil, nil
	}

	pkgs, err := loadPackages(repoRoot, "./cmd/api/models", "./pkg/auth", "./pkg/mastodon", "./pkg/reputation", "./pkg/storage", "./pkg/storage/models", "./cmd/api/handlers")
	if err != nil {
		return nil, err
	}

	config := map[string]schemaPackageConfig{
		packagePathModels:        {Prefix: ""},
		packagePathAuth:          {Prefix: "Auth"},
		packagePathMastodon:      {Prefix: "Mastodon"},
		packagePathReputation:    {Prefix: "Reputation"},
		packagePathStorage:       {Prefix: "Storage"},
		packagePathStorageModels: {Prefix: "StorageModels"},
	}

	builder := newSchemaBuilder(spec, pkgs, config)
	if err := builder.generateAllExported(packagePathModels); err != nil {
		return nil, err
	}

	ensureGenericSchemas(spec)
	return builder, nil
}

func (b *schemaBuilder) generateAllExported(pkgPath string) error {
	pkg := b.pkgs[pkgPath]
	if pkg == nil || pkg.Types == nil || pkg.Types.Scope() == nil {
		return fmt.Errorf("generate schemas: missing package %q", pkgPath)
	}

	for _, name := range pkg.Types.Scope().Names() {
		obj := pkg.Types.Scope().Lookup(name)
		typeName, ok := obj.(*types.TypeName)
		if !ok || typeName == nil {
			continue
		}
		if !typeName.Exported() {
			continue
		}

		named, ok := types.Unalias(typeName.Type()).(*types.Named)
		if !ok {
			continue
		}
		if _, err := b.ensureNamedSchema(named); err != nil {
			return err
		}
	}

	return nil
}

func ensureGenericSchemas(spec *openAPISpec) {
	if spec == nil {
		return
	}
	if spec.Components.Schemas == nil {
		spec.Components.Schemas = map[string]any{}
	}
	if _, ok := spec.Components.Schemas[schemaAnyObject]; !ok {
		spec.Components.Schemas[schemaAnyObject] = map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		}
	}
	if _, ok := spec.Components.Schemas[schemaAnyArray]; !ok {
		spec.Components.Schemas[schemaAnyArray] = map[string]any{
			"type":  "array",
			"items": map[string]any{},
		}
	}
}

func (b *schemaBuilder) schemaKeyForPayloadType(t types.Type) (string, error) {
	if b == nil {
		return "", nil
	}
	if t == nil {
		return "", nil
	}

	t = types.Unalias(t)
	for {
		ptr, ok := t.(*types.Pointer)
		if !ok {
			break
		}
		t = types.Unalias(ptr.Elem())
	}

	switch tt := t.(type) {
	case *types.Named:
		if isTimeType(tt) {
			return "RFC3339DateTime", nil
		}
		if isURIType(tt) {
			return "URI", nil
		}
		name, err := b.ensureNamedSchema(tt)
		if err != nil {
			return "", err
		}
		if name == "" {
			return schemaAnyObject, nil
		}
		return name, nil
	case *types.Slice:
		return b.ensureListSchemaFor(tt.Elem())
	case *types.Array:
		return b.ensureListSchemaFor(tt.Elem())
	case *types.Map:
		return b.ensureMapSchemaFor(tt)
	case *types.Struct:
		return schemaAnyObject, nil
	case *types.Basic:
		return schemaAnyObject, nil
	case *types.Interface:
		return schemaAnyObject, nil
	default:
		return schemaAnyObject, nil
	}
}

func (b *schemaBuilder) ensureListSchemaFor(elem types.Type) (string, error) {
	if b == nil {
		return "", nil
	}

	elem = types.Unalias(elem)
	for {
		ptr, ok := elem.(*types.Pointer)
		if !ok {
			break
		}
		elem = types.Unalias(ptr.Elem())
	}

	var listName string
	itemsSchema, err := b.schemaForType(elem)
	if err != nil {
		return "", err
	}

	switch et := elem.(type) {
	case *types.Named:
		baseName, err := b.ensureNamedSchema(et)
		if err != nil {
			return "", err
		}
		if baseName == "" {
			listName = schemaAnyArray
		} else {
			listName = baseName + "List"
		}
	case *types.Basic:
		listName = basicTypeListName(et) + "List"
	default:
		listName = schemaAnyArray
	}

	if listName == schemaAnyArray {
		return listName, nil
	}

	if b.spec.Components.Schemas == nil {
		b.spec.Components.Schemas = map[string]any{}
	}

	b.spec.Components.Schemas[listName] = map[string]any{
		"type":  "array",
		"items": itemsSchema,
	}
	return listName, nil
}

func (b *schemaBuilder) ensureMapSchemaFor(mt *types.Map) (string, error) {
	if b == nil {
		return "", nil
	}
	if mt == nil {
		return schemaAnyObject, nil
	}

	key := types.Unalias(mt.Key())
	if basic, ok := key.(*types.Basic); !ok || basic.Kind() != types.String {
		return schemaAnyObject, nil
	}

	elem := types.Unalias(mt.Elem())
	for {
		ptr, ok := elem.(*types.Pointer)
		if !ok {
			break
		}
		elem = types.Unalias(ptr.Elem())
	}

	mapName := ""
	switch et := elem.(type) {
	case *types.Named:
		baseName, err := b.ensureNamedSchema(et)
		if err != nil {
			return "", err
		}
		if baseName != "" {
			mapName = baseName + "Map"
		}
	case *types.Basic:
		mapName = basicTypeListName(et) + "Map"
	}

	if mapName == "" {
		hash := sha1.Sum([]byte(types.TypeString(mt, func(p *types.Package) string { // #nosec G401 -- deterministic schema component names, not cryptographic
			if p == nil {
				return ""
			}
			return p.Path()
		})))
		mapName = fmt.Sprintf("Map%x", hash)[:11]
	}

	if b.spec.Components.Schemas == nil {
		b.spec.Components.Schemas = map[string]any{}
	}
	if _, ok := b.spec.Components.Schemas[mapName]; ok {
		return mapName, nil
	}

	valueSchema, err := b.schemaForType(elem)
	if err != nil {
		return "", err
	}

	b.spec.Components.Schemas[mapName] = map[string]any{
		"type":                 "object",
		"additionalProperties": valueSchema,
	}

	return mapName, nil
}

func basicTypeListName(basic *types.Basic) string {
	if basic == nil {
		return "Any"
	}
	switch basic.Kind() {
	case types.String:
		return "String"
	case types.Bool:
		return "Boolean"
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
		return "Integer"
	case types.Float32, types.Float64:
		return "Number"
	default:
		return "Any"
	}
}

func (b *schemaBuilder) schemaNameForNamed(t *types.Named) string {
	if t == nil || t.Obj() == nil || t.Obj().Pkg() == nil {
		return ""
	}
	base := strings.TrimSpace(t.Obj().Name())
	if base == "" {
		return ""
	}
	cfg, ok := b.config[t.Obj().Pkg().Path()]
	if !ok {
		return ""
	}
	return cfg.Prefix + base
}

func (b *schemaBuilder) ensureNamedSchema(t *types.Named) (string, error) {
	if b == nil || b.spec == nil || t == nil {
		return "", nil
	}
	name := b.schemaNameForNamed(t)
	if name == "" {
		return "", nil
	}

	if _, ok := b.inProgress[name]; ok {
		return name, nil
	}

	b.inProgress[name] = struct{}{}
	schema, err := b.schemaForType(t.Underlying())
	if err != nil {
		delete(b.inProgress, name)
		return "", err
	}
	delete(b.inProgress, name)

	if b.spec.Components.Schemas == nil {
		b.spec.Components.Schemas = map[string]any{}
	}
	b.spec.Components.Schemas[name] = schema
	return name, nil
}

func (b *schemaBuilder) schemaForType(t types.Type) (any, error) {
	if b == nil {
		return map[string]any{}, nil
	}
	if t == nil {
		return map[string]any{}, nil
	}

	t = types.Unalias(t)
	switch tt := t.(type) {
	case *types.Named:
		if isTimeType(tt) {
			return map[string]any{"$ref": "#/components/schemas/RFC3339DateTime"}, nil
		}
		if isURIType(tt) {
			return map[string]any{"$ref": "#/components/schemas/URI"}, nil
		}
		if isJSONRawMessageType(tt) {
			return map[string]any{}, nil
		}
		if name, err := b.ensureNamedSchema(tt); err != nil {
			return nil, err
		} else if name != "" {
			return map[string]any{"$ref": "#/components/schemas/" + name}, nil
		}
		return b.schemaForType(tt.Underlying())
	case *types.Basic:
		return schemaForBasic(tt), nil
	case *types.Pointer:
		inner, err := b.schemaForType(tt.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"anyOf": []any{
				inner,
				map[string]any{"type": "null"},
			},
		}, nil
	case *types.Slice:
		items, err := b.schemaForType(tt.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"type":  "array",
			"items": items,
		}, nil
	case *types.Array:
		items, err := b.schemaForType(tt.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"type":  "array",
			"items": items,
		}, nil
	case *types.Map:
		valueSchema, err := b.schemaForType(tt.Elem())
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"type":                 "object",
			"additionalProperties": valueSchema,
		}, nil
	case *types.Interface:
		return map[string]any{}, nil
	case *types.Struct:
		return b.schemaForStruct(tt)
	default:
		return map[string]any{}, nil
	}
}

func (b *schemaBuilder) schemaForStruct(st *types.Struct) (any, error) {
	if st == nil {
		return map[string]any{}, nil
	}

	type property struct {
		name   string
		schema any
	}

	var props []property
	required := map[string]struct{}{}

	for i := 0; i < st.NumFields(); i++ {
		field := st.Field(i)
		if field == nil || !field.Exported() {
			continue
		}

		jsonName, omitEmpty := jsonFieldName(field.Name(), st.Tag(i))
		if jsonName == "" {
			continue
		}

		schema, err := b.schemaForType(field.Type())
		if err != nil {
			return nil, err
		}

		props = append(props, property{name: jsonName, schema: schema})
		if !omitEmpty && !isPointer(field.Type()) {
			required[jsonName] = struct{}{}
		}
	}

	sort.Slice(props, func(i, j int) bool { return props[i].name < props[j].name })
	properties := map[string]any{}
	for _, p := range props {
		properties[p.name] = p.schema
	}

	var requiredList []string
	for name := range required {
		requiredList = append(requiredList, name)
	}
	sort.Strings(requiredList)

	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(requiredList) > 0 {
		schema["required"] = requiredList
	}
	return schema, nil
}

func schemaForBasic(basic *types.Basic) any {
	if basic == nil {
		return map[string]any{}
	}

	switch basic.Kind() {
	case types.String:
		return map[string]any{"type": "string"}
	case types.Bool:
		return map[string]any{"type": "boolean"}
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
		return map[string]any{"type": "integer"}
	case types.Float32, types.Float64:
		return map[string]any{"type": "number"}
	case types.UntypedNil:
		return map[string]any{"type": "null"}
	default:
		return map[string]any{}
	}
}

func jsonFieldName(fallback, rawTag string) (string, bool) {
	tag := reflect.StructTag(strings.TrimSpace(rawTag))
	jsonTag := tag.Get("json")
	if jsonTag == "-" {
		return "", true
	}

	name := strings.TrimSpace(jsonTag)
	if name == "" {
		return fallback, false
	}

	parts := strings.Split(name, ",")
	fieldName := strings.TrimSpace(parts[0])
	if fieldName == "" {
		fieldName = fallback
	}

	omitEmpty := false
	for _, opt := range parts[1:] {
		if strings.TrimSpace(opt) == "omitempty" {
			omitEmpty = true
			break
		}
	}

	return fieldName, omitEmpty
}

func isPointer(t types.Type) bool {
	_, ok := types.Unalias(t).(*types.Pointer)
	return ok
}

func isTimeType(named *types.Named) bool {
	if named == nil || named.Obj() == nil || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Name() == "Time" && named.Obj().Pkg().Path() == "time"
}

func isURIType(named *types.Named) bool {
	if named == nil || named.Obj() == nil || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Name() == "URL" && named.Obj().Pkg().Path() == "net/url"
}

func isJSONRawMessageType(named *types.Named) bool {
	if named == nil || named.Obj() == nil || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Name() == "RawMessage" && named.Obj().Pkg().Path() == "encoding/json"
}

func loadPackages(repoRoot string, patterns ...string) (map[string]*packages.Package, error) {
	cfg := &packages.Config{
		Dir:  repoRoot,
		Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps,
	}
	loaded, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}
	if packages.PrintErrors(loaded) > 0 {
		return nil, fmt.Errorf("load packages: type errors present")
	}

	out := map[string]*packages.Package{}
	for _, pkg := range loaded {
		if pkg == nil {
			continue
		}
		out[pkg.PkgPath] = pkg
	}
	return out, nil
}
