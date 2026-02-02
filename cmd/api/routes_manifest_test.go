package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigureRoutes_RouteManifestMatchesSnapshot(t *testing.T) {
	actual := extractRouteManifest(t)

	_, testFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	dir := filepath.Dir(testFile)
	snapshotPath := filepath.Join(dir, "testdata", "routes_manifest.txt")

	if os.Getenv("UPDATE_ROUTE_MANIFEST") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(snapshotPath), 0o755))
		require.NoError(t, os.WriteFile(snapshotPath, []byte(strings.Join(actual, "\n")+"\n"), 0o644))
		t.Skip("updated route manifest snapshot")
	}

	expectedBytes, err := os.ReadFile(snapshotPath)
	require.NoErrorf(t, err, "missing snapshot file; generate with UPDATE_ROUTE_MANIFEST=1")

	expected := splitLines(string(expectedBytes))
	if len(expected) > 0 && expected[len(expected)-1] == "" {
		expected = expected[:len(expected)-1]
	}

	if !equalStringSlices(expected, actual) {
		missing, extra := diffSets(expected, actual)
		t.Fatalf(
			"routes manifest mismatch\nmissing (%d): %s\nextra (%d): %s\nregenerate with UPDATE_ROUTE_MANIFEST=1",
			len(missing),
			strings.Join(missing, ", "),
			len(extra),
			strings.Join(extra, ", "),
		)
	}
}

func extractRouteManifest(t *testing.T) []string {
	t.Helper()

	_, testFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	dir := filepath.Dir(testFile)
	routeFile := filepath.Join(dir, "routes.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, routeFile, nil, 0)
	require.NoError(t, err)

	var target *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		if fn.Name.Name == "configureRoutes" {
			target = fn
			break
		}
	}
	require.NotNil(t, target, "configureRoutes not found in routes.go")
	require.NotNil(t, target.Body, "configureRoutes body missing")

	var routes []string
	seen := map[string]struct{}{}

	ast.Inspect(target.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || call == nil {
			return true
		}

		method, path, ok := extractRouteFromCall(call)
		if !ok {
			return true
		}

		key := method + " " + path
		if _, exists := seen[key]; exists {
			t.Fatalf("duplicate route registration: %s", key)
		}
		seen[key] = struct{}{}
		routes = append(routes, key)
		return true
	})

	sort.Strings(routes)
	return routes
}

func extractRouteFromCall(call *ast.CallExpr) (string, string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return "", "", false
	}
	recv, ok := sel.X.(*ast.Ident)
	if !ok || recv.Name != "app" {
		return "", "", false
	}

	switch sel.Sel.Name {
	case "Get", "Post", "Put", "Patch", "Delete":
		path, ok := evalStringLiteral(call.Args, 0)
		if !ok {
			return "", "", false
		}
		return strings.ToUpper(sel.Sel.Name), path, true
	case "Handle":
		method, ok := evalStringLiteral(call.Args, 0)
		if !ok {
			return "", "", false
		}
		path, ok := evalStringLiteral(call.Args, 1)
		if !ok {
			return "", "", false
		}
		return strings.ToUpper(method), path, true
	default:
		return "", "", false
	}
}

func evalStringLiteral(args []ast.Expr, index int) (string, bool) {
	if len(args) <= index {
		return "", false
	}
	lit, ok := args[index].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func diffSets(expected, actual []string) (missing []string, extra []string) {
	expectedSet := make(map[string]struct{}, len(expected))
	for _, e := range expected {
		expectedSet[e] = struct{}{}
	}
	actualSet := make(map[string]struct{}, len(actual))
	for _, a := range actual {
		actualSet[a] = struct{}{}
	}

	for e := range expectedSet {
		if _, ok := actualSet[e]; !ok {
			missing = append(missing, e)
		}
	}
	for a := range actualSet {
		if _, ok := expectedSet[a]; !ok {
			extra = append(extra, a)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	// Keep output concise on failure.
	const max = 12
	if len(missing) > max {
		missing = append(missing[:max], "...") // sentinel
	}
	if len(extra) > max {
		extra = append(extra[:max], "...") // sentinel
	}
	return missing, extra
}
