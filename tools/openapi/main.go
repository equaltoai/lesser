// Package main provides a drift-checked OpenAPI generator for Lesser's REST surface.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type runOptions struct {
	SpecPath string
	Write    bool
	Check    bool
}

type routeDef struct {
	Method  string
	Path    string
	Lambda  string
	Sources []string
}

type openAPISpec struct {
	OpenAPI    string               `yaml:"openapi"`
	Info       openAPIInfo          `yaml:"info"`
	Components openAPIComponents    `yaml:"components,omitempty"`
	Paths      map[string]*pathItem `yaml:"paths"`
}

type openAPIInfo struct {
	Title       string `yaml:"title"`
	Version     string `yaml:"version"`
	Description string `yaml:"description,omitempty"`
}

type openAPIComponents struct {
	SecuritySchemes map[string]securityScheme `yaml:"securitySchemes,omitempty"`
	Schemas         map[string]any            `yaml:"schemas,omitempty"`
}

type securityScheme struct {
	Type         string      `yaml:"type"`
	Scheme       string      `yaml:"scheme,omitempty"`
	BearerFormat string      `yaml:"bearerFormat,omitempty"`
	Description  string      `yaml:"description,omitempty"`
	Flows        *oauthFlows `yaml:"flows,omitempty"`
}

type oauthFlows struct {
	AuthorizationCode *oauthFlow `yaml:"authorizationCode,omitempty"`
}

type oauthFlow struct {
	AuthorizationURL string            `yaml:"authorizationUrl"`
	TokenURL         string            `yaml:"tokenUrl"`
	Scopes           map[string]string `yaml:"scopes"`
}

type pathItem struct {
	Get    *operation `yaml:"get,omitempty"`
	Post   *operation `yaml:"post,omitempty"`
	Put    *operation `yaml:"put,omitempty"`
	Patch  *operation `yaml:"patch,omitempty"`
	Delete *operation `yaml:"delete,omitempty"`
}

type operation struct {
	OperationID string                `yaml:"operationId,omitempty"`
	Summary     string                `yaml:"summary,omitempty"`
	Description string                `yaml:"description,omitempty"`
	Tags        []string              `yaml:"tags,omitempty"`
	Security    []map[string][]string `yaml:"security,omitempty"`
	Parameters  []parameter           `yaml:"parameters,omitempty"`
	RequestBody *requestBody          `yaml:"requestBody,omitempty"`
	Responses   map[string]response   `yaml:"responses"`
	Extensions  map[string]any        `yaml:",inline,omitempty"`
}

type parameter struct {
	Name        string    `yaml:"name"`
	In          string    `yaml:"in"`
	Required    bool      `yaml:"required,omitempty"`
	Description string    `yaml:"description,omitempty"`
	Schema      schemaRef `yaml:"schema"`
}

type schemaRef struct {
	Type                 string `yaml:"type,omitempty"`
	Format               string `yaml:"format,omitempty"`
	Ref                  string `yaml:"$ref,omitempty"`
	AdditionalProperties any    `yaml:"additionalProperties,omitempty"`
}

type requestBody struct {
	Required bool                 `yaml:"required,omitempty"`
	Content  map[string]mediaType `yaml:"content"`
}

type mediaType struct {
	Schema schemaRef `yaml:"schema"`
}

type response struct {
	Description string `yaml:"description"`
}

const (
	methodGET    = "GET"
	methodPOST   = "POST"
	methodPUT    = "PUT"
	methodPATCH  = "PATCH"
	methodDELETE = "DELETE"
)

func main() {
	opts := parseOptions()

	repoRoot, err := findRepoRoot()
	if err != nil {
		fatal(err)
	}

	if err := run(repoRoot, opts); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}

func parseOptions() runOptions {
	var opts runOptions

	flag.StringVar(&opts.SpecPath, "spec", "docs/specs/openapi.yaml", "path to OpenAPI spec yaml")
	flag.BoolVar(&opts.Write, "write", false, "update the spec file in place")
	flag.BoolVar(&opts.Check, "check", false, "verify spec matches current routes")
	flag.Parse()

	if !opts.Write && !opts.Check {
		opts.Check = true
	}

	return opts
}

func run(repoRoot string, opts runOptions) error {
	absSpec := filepath.Join(repoRoot, filepath.FromSlash(opts.SpecPath))

	spec, err := readOrInitSpec(absSpec, opts.Write)
	if err != nil {
		return err
	}

	currentRoutes, err := extractConfiguredRoutes(repoRoot)
	if err != nil {
		return err
	}
	currentRoutes = sanitizeRoutes(currentRoutes)
	sortRoutes(currentRoutes)

	missing, stale := syncSpec(spec, currentRoutes)

	if opts.Write {
		if err := writeSpec(absSpec, spec); err != nil {
			return err
		}
		fmt.Println("updated:", opts.SpecPath)
	}

	if opts.Check {
		if err := reportRouteDrift(opts.SpecPath, missing, stale); err != nil {
			return err
		}
		fmt.Printf("ok: %s (%d paths)\n", opts.SpecPath, len(spec.Paths))
	}

	return nil
}

func readOrInitSpec(path string, allowInit bool) (*openAPISpec, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is an operator-supplied local file
	if err != nil {
		if os.IsNotExist(err) && allowInit {
			return defaultSpec(), nil
		}
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("read %s: missing file; run `make generate-openapi`", path)
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var spec openAPISpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if strings.TrimSpace(spec.OpenAPI) == "" {
		return nil, fmt.Errorf("invalid %s: missing openapi field", path)
	}
	if spec.Paths == nil {
		spec.Paths = map[string]*pathItem{}
	}

	ensureSecuritySchemes(&spec)
	return &spec, nil
}

func defaultSpec() *openAPISpec {
	spec := &openAPISpec{
		OpenAPI: "3.1.0",
		Info: openAPIInfo{
			Title:       "Lesser REST API",
			Version:     "0.1.0",
			Description: "Auto-generated route skeleton; fill request/response schemas over time. Do not serve this file at runtime; use it for build-time client generation.",
		},
		Components: openAPIComponents{},
		Paths:      map[string]*pathItem{},
	}
	ensureSecuritySchemes(spec)
	return spec
}

func ensureSecuritySchemes(spec *openAPISpec) {
	if spec == nil {
		return
	}
	if spec.Components.SecuritySchemes == nil {
		spec.Components.SecuritySchemes = map[string]securityScheme{}
	}

	if _, ok := spec.Components.SecuritySchemes["bearerAuth"]; !ok {
		spec.Components.SecuritySchemes["bearerAuth"] = securityScheme{
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
			Description:  "OAuth access token (JWT): `Authorization: Bearer <access_token>`.",
		}
	}

	if _, ok := spec.Components.SecuritySchemes["setupBearer"]; !ok {
		spec.Components.SecuritySchemes["setupBearer"] = securityScheme{
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "opaque",
			Description:  "Temporary setup session token: `Authorization: Bearer <setup_token>` (issued by `/setup/bootstrap/verify`).",
		}
	}

	if _, ok := spec.Components.SecuritySchemes["oauth2"]; !ok {
		spec.Components.SecuritySchemes["oauth2"] = securityScheme{
			Type: "oauth2",
			Flows: &oauthFlows{
				AuthorizationCode: &oauthFlow{
					AuthorizationURL: "/oauth/authorize",
					TokenURL:         "/oauth/token",
					Scopes: map[string]string{
						"read":   "Read access",
						"write":  "Write access",
						"follow": "Follow-related access",
						"push":   "Push notification access",
						"admin":  "Administrative access",
					},
				},
			},
		}
	}
}

func syncSpec(spec *openAPISpec, routes []routeDef) (missing []routeDef, stale []routeDef) {
	configured := make(map[string]routeDef, len(routes))
	for _, r := range routes {
		configured[routeKey(r.Method, r.Path)] = r
	}

	// Add missing routes.
	for _, r := range routes {
		item := spec.Paths[r.Path]
		if item == nil {
			item = &pathItem{}
			spec.Paths[r.Path] = item
		}
		if getOperation(item, r.Method) == nil {
			missing = append(missing, r)
			setOperation(item, r.Method, newOperation(r))
		} else {
			// Ensure basic invariants for existing operations.
			op := getOperation(item, r.Method)
			ensureOperationDefaults(op, r)
		}
	}

	// Remove stale routes.
	for path, item := range spec.Paths {
		if item == nil {
			continue
		}

		for _, method := range []string{methodGET, methodPOST, methodPUT, methodPATCH, methodDELETE} {
			op := getOperation(item, method)
			if op == nil {
				continue
			}
			if _, ok := configured[routeKey(method, path)]; !ok {
				stale = append(stale, routeDef{Method: method, Path: path})
				setOperation(item, method, nil)
				continue
			}
			ensureOperationDefaults(op, configured[routeKey(method, path)])
		}

		if getOperation(item, methodGET) == nil &&
			getOperation(item, methodPOST) == nil &&
			getOperation(item, methodPUT) == nil &&
			getOperation(item, methodPATCH) == nil &&
			getOperation(item, methodDELETE) == nil {
			delete(spec.Paths, path)
		}
	}

	sortRoutes(missing)
	sortRoutes(stale)
	return missing, stale
}

func ensureOperationDefaults(op *operation, route routeDef) {
	if op == nil {
		return
	}

	ensureGeneratedExtensions(op, route)

	if strings.TrimSpace(op.OperationID) == "" {
		op.OperationID = buildOperationID(route.Method, route.Path)
	}
	if len(op.Responses) == 0 {
		op.Responses = map[string]response{"200": {Description: "OK"}}
	}

	// Ensure path parameters exist.
	params := extractPathParams(route.Path)
	if len(params) > 0 {
		existing := make(map[string]parameter, len(op.Parameters))
		for _, p := range op.Parameters {
			if strings.EqualFold(p.In, "path") && strings.TrimSpace(p.Name) != "" {
				existing[p.Name] = p
			}
		}
		for _, name := range params {
			if _, ok := existing[name]; ok {
				continue
			}
			op.Parameters = append(op.Parameters, parameter{
				Name:     name,
				In:       "path",
				Required: true,
				Schema:   schemaRef{Type: "string"},
			})
		}
		sort.Slice(op.Parameters, func(i, j int) bool {
			if op.Parameters[i].In == op.Parameters[j].In {
				return op.Parameters[i].Name < op.Parameters[j].Name
			}
			return op.Parameters[i].In < op.Parameters[j].In
		})
	}

	// Ensure a placeholder request body for write methods (optional by default).
	if route.Method == methodPOST || route.Method == methodPUT || route.Method == methodPATCH {
		if op.RequestBody == nil {
			op.RequestBody = &requestBody{
				Required: false,
				Content: map[string]mediaType{
					"application/json": {Schema: schemaRef{Type: "object", AdditionalProperties: true}},
				},
			}
		}
	}
}

func newOperation(route routeDef) *operation {
	op := &operation{
		OperationID: buildOperationID(route.Method, route.Path),
		Tags:        deriveTags(route.Path),
		Responses:   map[string]response{"200": {Description: "OK"}},
	}

	// Security defaults where we can be confident.
	switch {
	case strings.HasPrefix(route.Path, "/api/v1/admin/"):
		op.Security = []map[string][]string{{"bearerAuth": {}}}
	case route.Path == "/setup/admin":
		op.Security = []map[string][]string{{"setupBearer": {}}}
	case route.Path == "/setup/finalize":
		op.Security = []map[string][]string{{"bearerAuth": {}}}
	case strings.HasPrefix(route.Path, "/api/v1/auth/webauthn/register"):
		op.Security = []map[string][]string{{"bearerAuth": {}}}
	case strings.HasPrefix(route.Path, "/api/v1/auth/webauthn/credentials"):
		op.Security = []map[string][]string{{"bearerAuth": {}}}
	}

	ensureOperationDefaults(op, route)
	return op
}

func deriveTags(path string) []string {
	trimmed := strings.TrimPrefix(path, "/")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return []string{"root"}
	}

	parts := strings.Split(trimmed, "/")
	switch parts[0] {
	case "api":
		if len(parts) == 2 && parts[1] == "graphql" {
			return []string{"graphql"}
		}
		if len(parts) >= 3 && parts[1] != "" {
			if parts[2] == "admin" {
				return []string{"admin"}
			}
			return []string{parts[2]}
		}
		return []string{"api"}
	case "oauth":
		return []string{"oauth"}
	case "auth":
		return []string{"auth"}
	case "setup":
		return []string{"setup"}
	case ".well-known":
		return []string{"well-known"}
	case "nodeinfo":
		return []string{"nodeinfo"}
	case "users":
		return []string{"activitypub"}
	case "health":
		return []string{"health"}
	case "embed":
		return []string{"embed"}
	default:
		return []string{parts[0]}
	}
}

func buildOperationID(method, path string) string {
	method = strings.ToLower(strings.TrimSpace(method))
	path = strings.TrimSpace(path)

	// Convert "/api/v1/accounts/{id}" -> "api_v1_accounts_by_id"
	normalized := strings.TrimPrefix(path, "/")
	normalized = strings.ReplaceAll(normalized, "/", "_")
	normalized = strings.ReplaceAll(normalized, "{", "by_")
	normalized = strings.ReplaceAll(normalized, "}", "")
	normalized = regexp.MustCompile(`[^a-zA-Z0-9_]+`).ReplaceAllString(normalized, "_")
	normalized = strings.Trim(normalized, "_")
	if normalized == "" {
		normalized = "root"
	}
	return method + "_" + normalized
}

func extractPathParams(path string) []string {
	re := regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)
	matches := re.FindAllStringSubmatch(path, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, m := range matches {
		if len(m) != 2 {
			continue
		}
		name := m[1]
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func getOperation(item *pathItem, method string) *operation {
	if item == nil {
		return nil
	}
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case methodGET:
		return item.Get
	case methodPOST:
		return item.Post
	case methodPUT:
		return item.Put
	case methodPATCH:
		return item.Patch
	case methodDELETE:
		return item.Delete
	default:
		return nil
	}
}

func setOperation(item *pathItem, method string, op *operation) {
	if item == nil {
		return
	}
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case methodGET:
		item.Get = op
	case methodPOST:
		item.Post = op
	case methodPUT:
		item.Put = op
	case methodPATCH:
		item.Patch = op
	case methodDELETE:
		item.Delete = op
	}
}

func writeSpec(path string, spec *openAPISpec) error {
	if spec == nil {
		return errors.New("spec is nil")
	}
	ensureSecuritySchemes(spec)

	out, err := yaml.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshal yaml: %w", err)
	}
	out = append(out, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	return os.Rename(tmp, path)
}

func reportRouteDrift(specPath string, missing []routeDef, stale []routeDef) error {
	var problems []string
	if len(missing) > 0 {
		var lines []string
		for _, r := range missing {
			lines = append(lines, fmt.Sprintf("  - %s %s", r.Method, r.Path))
		}
		problems = append(problems, fmt.Sprintf("missing routes in %s:\n%s", specPath, strings.Join(lines, "\n")))
	}
	if len(stale) > 0 {
		var lines []string
		for _, r := range stale {
			lines = append(lines, fmt.Sprintf("  - %s %s", r.Method, r.Path))
		}
		problems = append(problems, fmt.Sprintf("stale routes in %s (no longer configured):\n%s", specPath, strings.Join(lines, "\n")))
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n\n"))
	}
	return nil
}

func findRepoRoot() (string, error) {
	start, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := start
	for {
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, "cmd", "api", "routes_lift.go")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("unable to locate repo root (expected go.mod and cmd/api/routes_lift.go)")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func extractConfiguredRoutes(repoRoot string) ([]routeDef, error) {
	type routeAgg struct {
		Method        string
		Path          string
		Lambda        string
		LambdaScore   int
		RouteSources  map[string]struct{}
		SeenFromFiles map[string]struct{}
	}

	callRE := regexp.MustCompile(`\bapp\.(GET|POST|PUT|PATCH|DELETE)\("([^"]+)"`)
	handleRE := regexp.MustCompile(`\bapp\.Handle\("([^"]+)",\s*"([^"]+)"`)

	routesByKey := make(map[string]*routeAgg)

	addRoute := func(method, path, lambda, source string) {
		method = strings.ToUpper(strings.TrimSpace(method))
		path = normalizePath(path)
		if method == "" || path == "" {
			return
		}
		if method != methodGET && method != methodPOST && method != methodPUT && method != methodPATCH && method != methodDELETE {
			return
		}

		key := routeKey(method, path)
		entry := routesByKey[key]
		if entry == nil {
			entry = &routeAgg{
				Method:       method,
				Path:         path,
				Lambda:       lambda,
				LambdaScore:  lambdaPriority(lambda),
				RouteSources: map[string]struct{}{},
			}
			routesByKey[key] = entry
		}

		if source != "" {
			entry.RouteSources[source] = struct{}{}
		}
		if score := lambdaPriority(lambda); score > entry.LambdaScore {
			entry.Lambda = lambda
			entry.LambdaScore = score
		}
	}

	addRoutesFromFile := func(relPath, lambda string) error {
		absPath := filepath.Join(repoRoot, filepath.FromSlash(relPath))
		data, err := os.ReadFile(absPath) //nolint:gosec // local source file path
		if err != nil {
			return fmt.Errorf("read %s: %w", absPath, err)
		}

		source := filepath.ToSlash(relPath)

		for _, line := range strings.Split(string(data), "\n") {
			if m := callRE.FindStringSubmatch(line); len(m) == 3 {
				addRoute(m[1], m[2], lambda, source)
			}
			if m := handleRE.FindStringSubmatch(line); len(m) == 3 {
				addRoute(m[1], m[2], lambda, source)
			}
		}
		return nil
	}

	// API (Lift) routes.
	if err := addRoutesFromFile("cmd/api/routes_lift.go", "api"); err != nil {
		return nil, err
	}
	if err := addRoutesFromFile("cmd/api/main.go", "api"); err != nil {
		return nil, err
	}

	// SSE streaming routes.
	if err := addRoutesFromFile("cmd/sse/main.go", "sse"); err != nil {
		return nil, err
	}

	// Inventory-driven HTTP lambdas (federation + webfinger + GraphQL gateway).
	inventoryRoutes, err := extractInventoryHTTPRoutes(repoRoot)
	if err != nil {
		return nil, err
	}
	for _, r := range inventoryRoutes {
		source := filepath.ToSlash("infra/cdk/inventory/lambdas.go") + ":" + strings.TrimSpace(r.Lambda)
		addRoute(r.Method, r.Path, r.Lambda, source)
	}

	var routes []routeDef
	for _, entry := range routesByKey {
		sources := make([]string, 0, len(entry.RouteSources))
		for s := range entry.RouteSources {
			sources = append(sources, s)
		}
		sort.Strings(sources)

		routes = append(routes, routeDef{
			Method:  entry.Method,
			Path:    entry.Path,
			Lambda:  strings.TrimSpace(entry.Lambda),
			Sources: sources,
		})
	}

	sortRoutes(routes)
	return routes, nil
}

func sanitizeRoutes(routes []routeDef) []routeDef {
	var out []routeDef
	for _, r := range routes {
		r.Method = strings.ToUpper(strings.TrimSpace(r.Method))
		r.Path = normalizePath(r.Path)
		r.Lambda = strings.TrimSpace(r.Lambda)
		if r.Method == "" || r.Path == "" {
			continue
		}
		if r.Method != methodGET && r.Method != methodPOST && r.Method != methodPUT && r.Method != methodPATCH && r.Method != methodDELETE {
			continue
		}
		out = append(out, r)
	}
	return out
}

func sortRoutes(routes []routeDef) {
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
}

func routeKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + strings.TrimSpace(path)
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	parts := strings.Split(path, "/")
	for i, part := range parts {
		if len(part) > 1 && strings.HasPrefix(part, ":") {
			name := strings.TrimSpace(strings.TrimPrefix(part, ":"))
			if name != "" {
				parts[i] = "{" + name + "}"
			}
		}
	}

	normalized := strings.Join(parts, "/")
	for strings.Contains(normalized, "//") {
		normalized = strings.ReplaceAll(normalized, "//", "/")
	}
	if len(normalized) > 1 {
		normalized = strings.TrimSuffix(normalized, "/")
	}
	return normalized
}

func lambdaPriority(lambda string) int {
	lambda = strings.TrimSpace(lambda)
	switch lambda {
	case "":
		return 0
	case "api":
		return 10
	case "sse":
		return 20
	case "graphql":
		return 20
	default:
		return 30
	}
}

func ensureGeneratedExtensions(op *operation, route routeDef) {
	if op == nil {
		return
	}
	if op.Extensions == nil {
		op.Extensions = map[string]any{}
	}

	lambda := strings.TrimSpace(route.Lambda)
	if lambda == "" {
		delete(op.Extensions, "x-lesser-lambda")
	} else {
		op.Extensions["x-lesser-lambda"] = lambda
	}

	if len(route.Sources) == 0 {
		delete(op.Extensions, "x-lesser-routeSources")
		return
	}

	sources := append([]string(nil), route.Sources...)
	sort.Strings(sources)
	op.Extensions["x-lesser-routeSources"] = sources
}

func extractInventoryHTTPRoutes(repoRoot string) ([]routeDef, error) {
	path := filepath.Join(repoRoot, "infra", "cdk", "inventory", "lambdas.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	inventoryLit, err := findInventoryComposite(file)
	if err != nil {
		return nil, fmt.Errorf("extract inventory routes: %w", err)
	}

	lambdasLit, err := findCompositeField(inventoryLit, "Lambdas")
	if err != nil {
		return nil, fmt.Errorf("extract inventory routes: %w", err)
	}

	var routes []routeDef
	for _, elt := range lambdasLit.Elts {
		lambdaLit, ok := elt.(*ast.CompositeLit)
		if !ok {
			continue
		}

		name, ok := findStringField(lambdaLit, "Name")
		if !ok {
			return nil, fmt.Errorf("extract inventory routes: lambda spec missing Name in %s", path)
		}

		httpRoutesLit, err := findCompositeFieldOptional(lambdaLit, "HTTPRoutes")
		if err != nil {
			return nil, fmt.Errorf("extract inventory routes: %w", err)
		}
		if httpRoutesLit == nil {
			continue
		}

		for _, routeElt := range httpRoutesLit.Elts {
			routeLit, ok := routeElt.(*ast.CompositeLit)
			if !ok {
				continue
			}
			method, ok := findStringField(routeLit, "Method")
			if !ok {
				return nil, fmt.Errorf("extract inventory routes: HTTPRoute missing Method for lambda %q", name)
			}
			routePath, ok := findStringField(routeLit, "Path")
			if !ok {
				return nil, fmt.Errorf("extract inventory routes: HTTPRoute missing Path for lambda %q", name)
			}

			method = strings.ToUpper(strings.TrimSpace(method))
			routePath = normalizePath(routePath)
			if method == "ANY" {
				continue
			}
			if strings.Contains(routePath, "{proxy+}") {
				continue
			}

			routes = append(routes, routeDef{
				Method: method,
				Path:   routePath,
				Lambda: name,
			})
		}
	}

	sortRoutes(routes)
	return routes, nil
}

func findInventoryComposite(file *ast.File) (*ast.CompositeLit, error) {
	if file == nil {
		return nil, errors.New("inventory file is nil")
	}

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			valSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range valSpec.Names {
				if name == nil || name.Name != "LambdaInventory" {
					continue
				}
				if i >= len(valSpec.Values) {
					continue
				}
				if lit := unwrapComposite(valSpec.Values[i]); lit != nil {
					return lit, nil
				}
			}
		}
	}
	return nil, errors.New("LambdaInventory variable not found")
}

func unwrapComposite(expr ast.Expr) *ast.CompositeLit {
	switch v := expr.(type) {
	case *ast.CompositeLit:
		return v
	case *ast.UnaryExpr:
		if v.Op == token.AND {
			if lit, ok := v.X.(*ast.CompositeLit); ok {
				return lit
			}
		}
	}
	return nil
}

func findCompositeField(lit *ast.CompositeLit, field string) (*ast.CompositeLit, error) {
	value, ok := findKeyValueExpr(lit, field)
	if !ok {
		return nil, fmt.Errorf("missing %s field", field)
	}
	comp := unwrapComposite(value)
	if comp == nil {
		return nil, fmt.Errorf("%s field is not a composite literal", field)
	}
	return comp, nil
}

func findCompositeFieldOptional(lit *ast.CompositeLit, field string) (*ast.CompositeLit, error) {
	value, ok := findKeyValueExpr(lit, field)
	if !ok {
		return nil, nil
	}
	comp := unwrapComposite(value)
	if comp == nil {
		return nil, fmt.Errorf("%s field is not a composite literal", field)
	}
	return comp, nil
}

func findKeyValueExpr(lit *ast.CompositeLit, field string) (ast.Expr, bool) {
	if lit == nil {
		return nil, false
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyIdent, ok := kv.Key.(*ast.Ident)
		if !ok || keyIdent.Name != field {
			continue
		}
		return kv.Value, true
	}
	return nil, false
}

func findStringField(lit *ast.CompositeLit, field string) (string, bool) {
	value, ok := findKeyValueExpr(lit, field)
	if !ok {
		return "", false
	}

	switch v := value.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		parsed, err := strconv.Unquote(v.Value)
		if err != nil {
			return "", false
		}
		return parsed, true
	}
	return "", false
}
