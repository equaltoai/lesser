package models

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelGSITheoryDBTagsUseOmitEmpty(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)

	dir := filepath.Dir(file)
	set := token.NewFileSet()
	pkgs, err := parser.ParseDir(set, dir, func(info fs.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.ParseComments)
	require.NoError(t, err)

	modelsPkg, ok := pkgs["models"]
	require.True(t, ok)

	var failures []string

	for filename, f := range modelsPkg.Files {
		ast.Inspect(f, func(node ast.Node) bool {
			field, ok := node.(*ast.Field)
			if !ok || field.Tag == nil {
				return true
			}

			tagLiteral := strings.Trim(field.Tag.Value, "`")
			if tagLiteral == "" {
				return true
			}

			structTag := reflect.StructTag(tagLiteral)
			theoryTag := structTag.Get("theorydb")
			if !strings.Contains(theoryTag, "index:gsi") {
				return true
			}
			if strings.Contains(theoryTag, "omitempty") {
				return true
			}

			names := make([]string, 0, len(field.Names))
			for _, name := range field.Names {
				names = append(names, name.Name)
			}
			if len(names) == 0 {
				names = append(names, "<embedded>")
			}

			failures = append(failures, filename+": "+strings.Join(names, ", ")+" missing theorydb omitempty")
			return true
		})
	}

	require.Empty(t, failures, strings.Join(failures, "\n"))
}
