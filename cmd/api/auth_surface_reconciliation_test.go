package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth/publicsurface"
	"gopkg.in/yaml.v3"
)

const (
	authSurfaceGoldenRelPath     = "cmd/api/testdata/auth_surface_golden.json"
	authSurfaceExceptionsRelPath = "cmd/api/testdata/auth_surface_exceptions.json"
)

var updateAuthSurfaceGolden = flag.Bool(
	"update-auth-surface-golden",
	false,
	"rewrite the auth-surface reconciliation golden snapshot",
)

type authSurfaceGuard string

const (
	authSurfaceGuardNone     authSurfaceGuard = "none"
	authSurfaceGuardOptional authSurfaceGuard = "optional"
	authSurfaceGuardRequired authSurfaceGuard = "required"
)

type authSurfacePosture string

const (
	authSurfacePostureAnonymous    authSurfacePosture = "anonymous"
	authSurfacePostureAuthRequired authSurfacePosture = "auth-required"
)

type authSurfaceSecurity string

const (
	authSurfaceSecurityPublic   authSurfaceSecurity = "public"
	authSurfaceSecurityOptional authSurfaceSecurity = "optional"
	authSurfaceSecurityRequired authSurfaceSecurity = "required"
)

type authSurfaceVerdict string

const (
	authSurfaceVerdictAgree     authSurfaceVerdict = "agree"
	authSurfaceVerdictOverMark  authSurfaceVerdict = "overMark"
	authSurfaceVerdictUnderMark authSurfaceVerdict = "underMark"
	authSurfaceVerdictFailOpen  authSurfaceVerdict = "failOpen"
	authSurfaceVerdictDocDrift  authSurfaceVerdict = "docDrift"
)

type authSurfaceRoute struct {
	Method string
	Path   string
	Lambda string
	Guard  authSurfaceGuard
	Source string
}

type authSurfaceSnapshotEntry struct {
	Method          string              `json:"method"`
	Path            string              `json:"path"`
	RuntimePosture  authSurfacePosture  `json:"runtimePosture"`
	OpenAPISecurity authSurfaceSecurity `json:"openapiSecurity"`
	Verdict         authSurfaceVerdict  `json:"verdict"`
}

type authSurfaceException struct {
	Method  string             `json:"method"`
	Path    string             `json:"path"`
	Verdict authSurfaceVerdict `json:"verdict"`
	Owner   string             `json:"owner"`
	Expiry  string             `json:"expiry"`
	Note    string             `json:"note"`
}

type authSurfaceOpenAPIDoc struct {
	Paths map[string]map[string]authSurfaceOpenAPIOperation `yaml:"paths"`
}

type authSurfaceOpenAPIOperation struct {
	Security     []map[string][]string `yaml:"security"`
	LesserLambda string                `yaml:"x-lesser-lambda"`
}

func TestAuthSurfaceReconciliationGolden(t *testing.T) {
	repoRoot := authSurfaceRepoRoot(t)
	routes := enumerateAuthSurfaceRoutes(t, repoRoot)
	actual := authSurfaceClassifyRoutes(t, repoRoot, routes)

	if *updateAuthSurfaceGolden {
		writeAuthSurfaceGolden(t, filepath.Join(repoRoot, authSurfaceGoldenRelPath), actual)
		return
	}

	golden := readAuthSurfaceGolden(t, filepath.Join(repoRoot, authSurfaceGoldenRelPath))
	if failures := validateAuthSurfaceRouteCoverage(routes, golden); len(failures) > 0 {
		t.Fatalf("auth surface route coverage drift:\n%s", strings.Join(failures, "\n"))
	}
	if failures := compareAuthSurfaceSnapshots(golden, actual); len(failures) > 0 {
		t.Fatalf("auth surface golden drift:\n%s", strings.Join(failures, "\n"))
	}

	exceptions := readAuthSurfaceExceptions(t, filepath.Join(repoRoot, authSurfaceExceptionsRelPath))
	if failures := validateAuthSurfaceExceptions(actual, exceptions); len(failures) > 0 {
		t.Fatalf("auth surface exception drift:\n%s", strings.Join(failures, "\n"))
	}

	t.Logf("auth surface verdict counts: %s", formatAuthSurfaceCounts(actual, exceptions))
}

func TestAuthSurfaceReconcilerDetectsSyntheticDrift(t *testing.T) {
	golden := []authSurfaceSnapshotEntry{{
		Method:          http.MethodGet,
		Path:            "/synthetic/drift",
		RuntimePosture:  authSurfacePostureAnonymous,
		OpenAPISecurity: authSurfaceSecurityPublic,
		Verdict:         authSurfaceVerdictAgree,
	}}
	flipped := []authSurfaceSnapshotEntry{{
		Method:          http.MethodGet,
		Path:            "/synthetic/drift",
		RuntimePosture:  authSurfacePostureAuthRequired,
		OpenAPISecurity: authSurfaceSecurityPublic,
		Verdict:         authSurfaceVerdictUnderMark,
	}}

	failures := compareAuthSurfaceSnapshots(golden, flipped)
	if len(failures) == 0 {
		t.Fatal("synthetic auth-surface drift was not detected")
	}
}

func authSurfaceClassifyRoutes(
	t *testing.T,
	repoRoot string,
	routes []authSurfaceRoute,
) []authSurfaceSnapshotEntry {
	t.Helper()

	operations := readAuthSurfaceOpenAPIOperations(t, repoRoot)

	snapshots := make([]authSurfaceSnapshotEntry, 0, len(routes))
	for _, route := range routes {
		operation := operations[authSurfaceRouteKey(route.Method, route.Path)]
		if operation.LesserLambda != "" && operation.LesserLambda != route.Lambda {
			t.Fatalf(
				"OpenAPI x-lesser-lambda mismatch for %s: route=%s openapi=%s",
				authSurfaceRouteKey(route.Method, route.Path),
				route.Lambda,
				operation.LesserLambda,
			)
		}
		runtimePosture := runtimeEffectiveAuthSurfacePosture(route)
		openAPISecurity := authSurfaceOpenAPISecurity(operation)
		verdict := classifyAuthSurfaceVerdict(route, runtimePosture, openAPISecurity)
		snapshots = append(snapshots, authSurfaceSnapshotEntry{
			Method:          route.Method,
			Path:            route.Path,
			RuntimePosture:  runtimePosture,
			OpenAPISecurity: openAPISecurity,
			Verdict:         verdict,
		})
	}

	sortAuthSurfaceSnapshots(snapshots)
	return snapshots
}

func enumerateAuthSurfaceRoutes(t *testing.T, repoRoot string) []authSurfaceRoute {
	t.Helper()

	routes := make([]authSurfaceRoute, 0, 340)
	routes = append(routes, parseAuthSurfaceAppRoutes(t, repoRoot, "cmd/api/routes.go", "api")...)
	routes = append(routes, parseAuthSurfaceAppRoutes(t, repoRoot, "cmd/api/main.go", "api")...)
	routes = append(routes, parseAuthSurfaceAppRoutes(t, repoRoot, "cmd/sse/main.go", "sse")...)
	routes = append(routes, parseWebFingerInventoryRoutes(t, repoRoot)...)

	seen := map[string]authSurfaceRoute{}
	for _, route := range routes {
		key := authSurfaceRouteKey(route.Method, route.Path)
		if previous, ok := seen[key]; ok {
			t.Fatalf("duplicate auth-surface route %s from %s and %s", key, previous.Source, route.Source)
		}
		seen[key] = route
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	return routes
}

func parseAuthSurfaceAppRoutes(t *testing.T, repoRoot, relPath, lambda string) []authSurfaceRoute {
	t.Helper()

	absPath := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, absPath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", relPath, err)
	}

	var routes []authSurfaceRoute
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		route, ok := authSurfaceRouteFromCall(call, lambda, relPath)
		if ok {
			routes = append(routes, route)
		}
		return true
	})
	return routes
}

func authSurfaceRouteFromCall(call *ast.CallExpr, lambda, source string) (authSurfaceRoute, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel == nil {
		return authSurfaceRoute{}, false
	}
	receiver, ok := selector.X.(*ast.Ident)
	if !ok || receiver.Name != "app" {
		return authSurfaceRoute{}, false
	}

	if selector.Sel.Name == "Handle" {
		return authSurfaceRouteFromHandleArgs(call.Args, lambda, source)
	}

	method := strings.ToUpper(selector.Sel.Name)
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead:
		return authSurfaceRouteFromArgs(method, call.Args, 1, lambda, source)
	default:
		return authSurfaceRoute{}, false
	}
}

func authSurfaceRouteFromHandleArgs(args []ast.Expr, lambda, source string) (authSurfaceRoute, bool) {
	if len(args) < 3 {
		return authSurfaceRoute{}, false
	}
	method, ok := authSurfaceStringLiteral(args[0])
	if !ok {
		return authSurfaceRoute{}, false
	}
	return authSurfaceRouteFromArgs(strings.ToUpper(method), args[1:], 1, lambda, source)
}

func authSurfaceRouteFromArgs(
	method string,
	args []ast.Expr,
	handlerIndex int,
	lambda string,
	source string,
) (authSurfaceRoute, bool) {
	if len(args) <= handlerIndex {
		return authSurfaceRoute{}, false
	}
	path, ok := authSurfaceStringLiteral(args[0])
	if !ok {
		return authSurfaceRoute{}, false
	}
	return authSurfaceRoute{
		Method: method,
		Path:   path,
		Lambda: lambda,
		Guard:  inferAuthSurfaceGuard(args[handlerIndex:]),
		Source: source,
	}, true
}

func parseWebFingerInventoryRoutes(t *testing.T, repoRoot string) []authSurfaceRoute {
	t.Helper()

	const relPath = "cmd/webfinger/main.go"
	absPath := filepath.Join(repoRoot, filepath.FromSlash(relPath))
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, absPath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", relPath, err)
	}

	var routes []authSurfaceRoute
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "webfingerRouteInventory" || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			method, path, ok := authSurfaceWebFingerInventoryEntry(literal)
			if ok {
				routes = append(routes, authSurfaceRoute{
					Method: method,
					Path:   path,
					Lambda: "webfinger",
					Guard:  authSurfaceGuardNone,
					Source: relPath,
				})
			}
			return true
		})
	}

	if len(routes) == 0 {
		t.Fatalf("no webfinger inventory routes found in %s", relPath)
	}
	return routes
}

func authSurfaceWebFingerInventoryEntry(literal *ast.CompositeLit) (string, string, bool) {
	var method, path string
	for _, elt := range literal.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch key.Name {
		case "Method":
			method = authSurfaceHTTPMethodExpr(kv.Value)
		case "Path":
			path, _ = authSurfaceStringLiteral(kv.Value)
		}
	}
	if method == "" || path == "" {
		return "", "", false
	}
	return method, path, true
}

func authSurfaceHTTPMethodExpr(expr ast.Expr) string {
	if value, ok := authSurfaceStringLiteral(expr); ok {
		return strings.ToUpper(value)
	}
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel == nil {
		return ""
	}
	receiver, ok := selector.X.(*ast.Ident)
	if !ok || receiver.Name != "http" {
		return ""
	}
	switch selector.Sel.Name {
	case "MethodGet":
		return http.MethodGet
	case "MethodPost":
		return http.MethodPost
	case "MethodPut":
		return http.MethodPut
	case "MethodPatch":
		return http.MethodPatch
	case "MethodDelete":
		return http.MethodDelete
	case "MethodHead":
		return http.MethodHead
	default:
		return ""
	}
}

func inferAuthSurfaceGuard(args []ast.Expr) authSurfaceGuard {
	guard := authSurfaceGuardNone
	for _, arg := range args {
		if candidate := inferAuthSurfaceGuardExpr(arg); authSurfaceGuardPriority(candidate) > authSurfaceGuardPriority(guard) {
			guard = candidate
		}
	}
	return guard
}

func inferAuthSurfaceGuardExpr(expr ast.Expr) authSurfaceGuard {
	switch value := expr.(type) {
	case *ast.Ident:
		return authSurfaceGuardToken(value.Name)
	case *ast.SelectorExpr:
		if value.Sel == nil {
			return authSurfaceGuardNone
		}
		return authSurfaceGuardToken(value.Sel.Name)
	case *ast.CallExpr:
		guard := inferAuthSurfaceGuardExpr(value.Fun)
		for _, arg := range value.Args {
			candidate := inferAuthSurfaceGuardExpr(arg)
			if authSurfaceGuardPriority(candidate) > authSurfaceGuardPriority(guard) {
				guard = candidate
			}
		}
		return guard
	default:
		return authSurfaceGuardNone
	}
}

func authSurfaceGuardToken(name string) authSurfaceGuard {
	switch {
	case name == "optionalAuth" || name == "OptionalAuth":
		return authSurfaceGuardOptional
	case strings.HasPrefix(name, "require") || strings.HasPrefix(name, "Require"):
		return authSurfaceGuardRequired
	default:
		return authSurfaceGuardNone
	}
}

func authSurfaceGuardPriority(guard authSurfaceGuard) int {
	switch guard {
	case authSurfaceGuardRequired:
		return 2
	case authSurfaceGuardOptional:
		return 1
	default:
		return 0
	}
}

func authSurfaceStringLiteral(expr ast.Expr) (string, bool) {
	literal, ok := expr.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

func readAuthSurfaceOpenAPIOperations(
	t *testing.T,
	repoRoot string,
) map[string]authSurfaceOpenAPIOperation {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(repoRoot, "docs/contracts/openapi.yaml")) // #nosec G304 -- repo testdata.
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	var doc authSurfaceOpenAPIDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi: %v", err)
	}

	operations := map[string]authSurfaceOpenAPIOperation{}
	for path, item := range doc.Paths {
		for method, operation := range item {
			upperMethod := strings.ToUpper(method)
			switch upperMethod {
			case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead:
				operations[authSurfaceRouteKey(upperMethod, path)] = operation
			}
		}
	}
	return operations
}

func runtimeEffectiveAuthSurfacePosture(route authSurfaceRoute) authSurfacePosture {
	switch route.Lambda {
	case "api":
		if apiRequestIsPublic(route.Method, route.Path) && route.Guard != authSurfaceGuardRequired {
			return authSurfacePostureAnonymous
		}
		return authSurfacePostureAuthRequired
	case "sse":
		if route.Path == "/api/v1/streaming" || route.Path == "/api/v1/streaming/health" {
			return authSurfacePostureAnonymous
		}
		return authSurfacePostureAuthRequired
	case "webfinger":
		return authSurfacePostureAnonymous
	default:
		return authSurfacePostureAuthRequired
	}
}

func authSurfaceOpenAPISecurity(operation authSurfaceOpenAPIOperation) authSurfaceSecurity {
	if len(operation.Security) == 0 {
		return authSurfaceSecurityPublic
	}

	hasAnonymousAlternative := false
	hasBearerAlternative := false
	for _, alternative := range operation.Security {
		if len(alternative) == 0 {
			hasAnonymousAlternative = true
			continue
		}
		if _, ok := alternative["bearerAuth"]; ok {
			hasBearerAlternative = true
		}
		if _, ok := alternative["setupBearer"]; ok {
			hasBearerAlternative = true
		}
	}

	if hasAnonymousAlternative && hasBearerAlternative {
		return authSurfaceSecurityOptional
	}
	if hasBearerAlternative {
		return authSurfaceSecurityRequired
	}
	if hasAnonymousAlternative {
		return authSurfaceSecurityPublic
	}
	return authSurfaceSecurityRequired
}

func classifyAuthSurfaceVerdict(
	route authSurfaceRoute,
	runtimePosture authSurfacePosture,
	openAPISecurity authSurfaceSecurity,
) authSurfaceVerdict {
	if runtimePosture == authSurfacePostureAnonymous && isAuthSurfaceUnexpectedPublicMutation(route) {
		return authSurfaceVerdictFailOpen
	}
	if runtimePosture == authSurfacePostureAnonymous && openAPISecurity == authSurfaceSecurityRequired {
		return authSurfaceVerdictOverMark
	}
	if runtimePosture == authSurfacePostureAuthRequired && openAPISecurity != authSurfaceSecurityRequired {
		return authSurfaceVerdictUnderMark
	}
	return authSurfaceVerdictAgree
}

func isAuthSurfaceUnexpectedPublicMutation(route authSurfaceRoute) bool {
	if route.Method == http.MethodGet || route.Method == http.MethodHead {
		return false
	}
	return !authSurfaceExpectedPublicMutations[authSurfaceRouteKey(route.Method, route.Path)]
}

var authSurfaceExpectedPublicMutations = map[string]bool{
	"POST /api/v1/accounts":                                                        true,
	"POST /api/v1/apps":                                                            true,
	"POST /api/v1/agents/auth/challenge":                                           true,
	"POST /api/v1/agents/auth/token":                                               true,
	"POST /api/v1/agents/register":                                                 true,
	"POST /api/v1/agents/register/challenge":                                       true,
	"POST /api/v1/auth/webauthn/login/begin":                                       true,
	"POST /api/v1/auth/webauthn/login/finish":                                      true,
	"POST /api/v1/search/statuses":                                                 true,
	"POST /api/v1/agents/{username}/access-leases/{leaseID}/renew/challenge":       true,
	"POST /api/v1/agents/{username}/access-leases/{leaseID}/session-key":           true,
	"POST /api/v1/agents/{username}/access-leases/{leaseID}/session-key/challenge": true,
	"POST /api/v1/agents/{username}/access-leases/{leaseID}/token":                 true,
	"POST /auth/wallet/challenge":                                                  true,
	"POST /auth/wallet/link":                                                       true,
	"POST /auth/wallet/login":                                                      true,
	"POST /auth/wallet/verify":                                                     true,
	"POST /oauth/consent":                                                          true,
	"POST /oauth/device/code":                                                      true,
	"POST /oauth/device/consent":                                                   true,
	"POST /oauth/device/verify":                                                    true,
	"POST /oauth/register":                                                         true,
	"POST /oauth/revoke":                                                           true,
	"POST /oauth/token":                                                            true,
	"POST /setup/admin":                                                            true,
	"POST /setup/bootstrap/challenge":                                              true,
	"POST /setup/bootstrap/verify":                                                 true,
	"POST /setup/finalize":                                                         true,
}

func validateAuthSurfaceRouteCoverage(
	routes []authSurfaceRoute,
	golden []authSurfaceSnapshotEntry,
) []string {
	goldenByRoute := authSurfaceSnapshotMap(golden)
	routesByKey := make(map[string]authSurfaceRoute, len(routes))
	var failures []string

	for _, route := range routes {
		key := authSurfaceRouteKey(route.Method, route.Path)
		routesByKey[key] = route
		if _, ok := goldenByRoute[key]; !ok {
			failures = append(failures, fmt.Sprintf("registered route %s from %s is missing from the golden snapshot", key, route.Source))
		}

		classification := publicsurface.Classify(route.Method, route.Path)
		if classification.Kind == publicsurface.ClassificationUnknown {
			failures = append(failures, fmt.Sprintf("registered route %s from %s did not resolve through publicsurface", key, route.Source))
			continue
		}
		if route.Lambda == "api" {
			gatePublic := apiRequestIsPublic(route.Method, route.Path)
			if gatePublic != classification.Public {
				failures = append(failures, fmt.Sprintf(
					"api route %s publicsurface/gate mismatch: classify public=%t gate public=%t",
					key,
					classification.Public,
					gatePublic,
				))
			}
		}
	}

	for key := range goldenByRoute {
		if _, ok := routesByKey[key]; !ok {
			failures = append(failures, fmt.Sprintf("golden route %s is no longer registered", key))
		}
	}
	sort.Strings(failures)
	return failures
}

func compareAuthSurfaceSnapshots(
	golden []authSurfaceSnapshotEntry,
	actual []authSurfaceSnapshotEntry,
) []string {
	goldenByRoute := authSurfaceSnapshotMap(golden)
	actualByRoute := authSurfaceSnapshotMap(actual)

	var failures []string
	for key, actualEntry := range actualByRoute {
		goldenEntry, ok := goldenByRoute[key]
		if !ok {
			failures = append(failures, fmt.Sprintf("new route %s appears outside the golden snapshot", key))
			continue
		}
		if actualEntry.RuntimePosture != goldenEntry.RuntimePosture ||
			actualEntry.OpenAPISecurity != goldenEntry.OpenAPISecurity ||
			actualEntry.Verdict != goldenEntry.Verdict {
			failures = append(failures, fmt.Sprintf(
				"%s changed: golden runtime=%s openapi=%s verdict=%s; actual runtime=%s openapi=%s verdict=%s",
				key,
				goldenEntry.RuntimePosture,
				goldenEntry.OpenAPISecurity,
				goldenEntry.Verdict,
				actualEntry.RuntimePosture,
				actualEntry.OpenAPISecurity,
				actualEntry.Verdict,
			))
		}
	}
	for key := range goldenByRoute {
		if _, ok := actualByRoute[key]; !ok {
			failures = append(failures, fmt.Sprintf("golden route %s is no longer registered", key))
		}
	}
	sort.Strings(failures)
	return failures
}

func validateAuthSurfaceExceptions(
	actual []authSurfaceSnapshotEntry,
	exceptions []authSurfaceException,
) []string {
	actualByRoute := authSurfaceSnapshotMap(actual)
	exceptionsByRoute := map[string]authSurfaceException{}
	docDriftExceptions := map[string]authSurfaceException{}
	var failures []string

	for _, exception := range exceptions {
		key := authSurfaceRouteKey(exception.Method, exception.Path)
		failures = append(failures, validateAuthSurfaceExceptionMetadata(key, exception)...)
		if exception.Verdict == authSurfaceVerdictDocDrift {
			if _, ok := docDriftExceptions[key]; ok {
				failures = append(failures, fmt.Sprintf("duplicate docDrift exception for %s", key))
				continue
			}
			docDriftExceptions[key] = exception
			continue
		}
		if _, ok := exceptionsByRoute[key]; ok {
			failures = append(failures, fmt.Sprintf("duplicate exception for %s", key))
			continue
		}
		exceptionsByRoute[key] = exception
		actualEntry, ok := actualByRoute[key]
		if !ok {
			failures = append(failures, fmt.Sprintf("exception %s no longer matches a registered route", key))
			continue
		}
		if actualEntry.Verdict != exception.Verdict {
			failures = append(failures, fmt.Sprintf(
				"exception %s expected verdict %s, actual %s",
				key,
				exception.Verdict,
				actualEntry.Verdict,
			))
		}
	}

	for _, actualEntry := range actual {
		if actualEntry.Verdict == authSurfaceVerdictAgree {
			continue
		}
		key := authSurfaceRouteKey(actualEntry.Method, actualEntry.Path)
		if _, ok := exceptionsByRoute[key]; !ok {
			failures = append(failures, fmt.Sprintf("non-agree route %s lacks an exception", key))
		}
	}
	sort.Strings(failures)
	return failures
}

func validateAuthSurfaceExceptionMetadata(
	key string,
	exception authSurfaceException,
) []string {
	var failures []string
	if strings.TrimSpace(exception.Owner) == "" {
		failures = append(failures, fmt.Sprintf("exception %s is missing owner", key))
	}
	if strings.TrimSpace(exception.Expiry) == "" {
		failures = append(failures, fmt.Sprintf("exception %s is missing expiry", key))
	}
	if strings.TrimSpace(exception.Note) == "" {
		failures = append(failures, fmt.Sprintf("exception %s is missing note", key))
	}
	return failures
}

func authSurfaceSnapshotMap(entries []authSurfaceSnapshotEntry) map[string]authSurfaceSnapshotEntry {
	out := make(map[string]authSurfaceSnapshotEntry, len(entries))
	for _, entry := range entries {
		out[authSurfaceRouteKey(entry.Method, entry.Path)] = entry
	}
	return out
}

func readAuthSurfaceGolden(t *testing.T, path string) []authSurfaceSnapshotEntry {
	t.Helper()
	var entries []authSurfaceSnapshotEntry
	readAuthSurfaceJSON(t, path, &entries)
	return entries
}

func readAuthSurfaceExceptions(t *testing.T, path string) []authSurfaceException {
	t.Helper()
	var entries []authSurfaceException
	readAuthSurfaceJSON(t, path, &entries)
	return entries
}

func readAuthSurfaceJSON(t *testing.T, path string, target any) {
	t.Helper()
	file, err := os.Open(path) // #nosec G304 -- repo testdata.
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			t.Fatalf("close %s: %v", path, closeErr)
		}
	}()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("decode %s: trailing JSON content", path)
	}
}

func writeAuthSurfaceGolden(t *testing.T, path string, entries []authSurfaceSnapshotEntry) {
	t.Helper()
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil { // #nosec G306 -- repo testdata.
		t.Fatalf("write %s: %v", path, err)
	}
}

func formatAuthSurfaceCounts(entries []authSurfaceSnapshotEntry, exceptions []authSurfaceException) string {
	counts := map[authSurfaceVerdict]int{}
	for _, entry := range entries {
		counts[entry.Verdict]++
	}
	for _, exception := range exceptions {
		if exception.Verdict == authSurfaceVerdictDocDrift {
			counts[authSurfaceVerdictDocDrift]++
		}
	}
	return fmt.Sprintf(
		"totalRoutes=%d agree=%d overMark=%d underMark=%d failOpen=%d docDrift=%d",
		len(entries),
		counts[authSurfaceVerdictAgree],
		counts[authSurfaceVerdictOverMark],
		counts[authSurfaceVerdictUnderMark],
		counts[authSurfaceVerdictFailOpen],
		counts[authSurfaceVerdictDocDrift],
	)
}

func sortAuthSurfaceSnapshots(entries []authSurfaceSnapshotEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path == entries[j].Path {
			return entries[i].Method < entries[j].Method
		}
		return entries[i].Path < entries[j].Path
	})
}

func authSurfaceRouteKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + strings.TrimSpace(path)
}

func authSurfaceRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "cmd/api/routes.go")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("unable to locate repo root")
		}
		dir = parent
	}
}
