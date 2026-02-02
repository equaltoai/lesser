// Package main provides an offline verifier for Lesser's GraphQL coverage inventory.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type gqlgenConfig struct {
	Schema []string `yaml:"schema"`
}

type coverageSpec struct {
	Version    int              `yaml:"version"`
	Exemptions []coverageExempt `yaml:"exemptions"`
	Routes     []coverageRoute  `yaml:"routes"`
}

type coverageExempt struct {
	ID     string         `yaml:"id"`
	Reason string         `yaml:"reason"`
	Match  exemptionMatch `yaml:"match"`
}

type exemptionMatch struct {
	Exact    []string `yaml:"exact,omitempty"`
	Prefixes []string `yaml:"prefixes,omitempty"`
	Contains []string `yaml:"contains,omitempty"`
	Methods  []string `yaml:"methods,omitempty"`
}

type coverageRoute struct {
	Method     string   `yaml:"method"`
	Path       string   `yaml:"path"`
	Policy     string   `yaml:"policy"`
	ExemptedBy string   `yaml:"exemptedBy,omitempty"`
	Capability string   `yaml:"capability,omitempty"`
	Status     string   `yaml:"status,omitempty"`  // covered|missing (informational until strict mode)
	GraphQL    []string `yaml:"graphql,omitempty"` // e.g. Query.timeline, Mutation.createNote
	Notes      string   `yaml:"notes,omitempty"`
}

type routeDef struct {
	Method string
	Path   string
}

const (
	coverageSpecVersion   = 2
	policyGraphQLRequired = "graphql_required"
	policyRestOnly        = "rest_only"

	statusCovered = "covered"
	statusMissing = "missing"
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

type runOptions struct {
	SpecPath string
	Write    bool
	Check    bool
	Strict   bool
}

func parseOptions() runOptions {
	var opts runOptions

	flag.StringVar(&opts.SpecPath, "spec", "docs/specs/graphql_coverage.yaml", "path to coverage spec yaml")
	flag.BoolVar(&opts.Write, "write", false, "update the spec file in place")
	flag.BoolVar(&opts.Check, "check", false, "verify spec matches current routes + schema")
	flag.BoolVar(&opts.Strict, "strict", false, "fail if any graphql_required route remains missing")
	flag.Parse()

	if !opts.Write && !opts.Check {
		opts.Check = true
	}

	return opts
}

func run(repoRoot string, opts runOptions) error {
	absSpec := filepath.Join(repoRoot, filepath.FromSlash(opts.SpecPath))
	spec, err := readCoverageSpec(absSpec)
	if err != nil {
		return err
	}

	currentRoutes, err := extractConfiguredRoutes(repoRoot)
	if err != nil {
		return err
	}

	currentRoutes = sanitizeRoutes(currentRoutes)
	sortRoutes(currentRoutes)

	updated, missing, stale := syncSpecRoutes(spec, currentRoutes)
	spec.Routes = updated

	if opts.Write {
		if err := writeCoverageSpecInPlace(repoRoot, absSpec, opts.SpecPath, spec); err != nil {
			return err
		}
	}

	if opts.Check {
		if err := verifyCoverageSpec(repoRoot, opts, spec, missing, stale); err != nil {
			return err
		}
		fmt.Printf("ok: %s (%d routes)\n", opts.SpecPath, len(spec.Routes))
	}

	return nil
}

func writeCoverageSpecInPlace(repoRoot, absSpecPath, displayPath string, spec *coverageSpec) error {
	if spec == nil {
		return errors.New("spec is nil")
	}

	if spec.Version < coverageSpecVersion {
		spec.Version = coverageSpecVersion
	}
	applyDerivedRouteFields(spec)

	if err := validateRoutePolicies(spec.Routes, spec.Exemptions); err != nil {
		return err
	}
	if err := validateCoverageMappings(repoRoot, spec.Routes); err != nil {
		return err
	}
	if err := writeCoverageSpec(absSpecPath, spec); err != nil {
		return err
	}

	fmt.Println("updated:", displayPath)
	return nil
}

func verifyCoverageSpec(repoRoot string, opts runOptions, spec *coverageSpec, missing []routeDef, stale []coverageRoute) error {
	if spec == nil {
		return errors.New("spec is nil")
	}
	if spec.Version < coverageSpecVersion {
		return fmt.Errorf("coverage spec %s is version %d; run `lesser generate graphql-coverage` to upgrade", opts.SpecPath, spec.Version)
	}
	if !opts.Write {
		if err := reportRouteDrift(opts.SpecPath, missing, stale); err != nil {
			return err
		}
	}
	if err := validateRoutePolicies(spec.Routes, spec.Exemptions); err != nil {
		return err
	}
	if err := validateCoverageMappings(repoRoot, spec.Routes); err != nil {
		return err
	}
	if opts.Strict {
		return validateStrictCoverage(spec.Routes)
	}
	return nil
}

func validateCoverageMappings(repoRoot string, routes []coverageRoute) error {
	schemaOps, err := loadGraphQLOperations(repoRoot)
	if err != nil {
		return err
	}
	return validateGraphQLMappings(routes, schemaOps)
}

func validateStrictCoverage(routes []coverageRoute) error {
	var problems []string
	for _, entry := range routes {
		entry = normalizeCoverageRoute(entry)
		if entry.Policy != policyGraphQLRequired {
			continue
		}
		if entry.Status != statusCovered {
			problems = append(problems, fmt.Sprintf("%s %s: status is %q", entry.Method, entry.Path, entry.Status))
			continue
		}
		if len(entry.GraphQL) == 0 {
			problems = append(problems, fmt.Sprintf("%s %s: missing graphql mapping", entry.Method, entry.Path))
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return errors.New("strict coverage failed:\n" + strings.Join(problems, "\n"))
	}
	return nil
}

func syncSpecRoutes(spec *coverageSpec, routes []routeDef) (updated []coverageRoute, missing []routeDef, stale []coverageRoute) {
	specRoutesByKey := make(map[string]coverageRoute, len(spec.Routes))
	for _, entry := range spec.Routes {
		specRoutesByKey[routeKey(entry.Method, entry.Path)] = entry
	}

	for _, current := range routes {
		key := routeKey(current.Method, current.Path)
		if existing, ok := specRoutesByKey[key]; ok {
			updated = append(updated, normalizeCoverageRoute(existing))
			continue
		}

		missing = append(missing, current)
		updated = append(updated, coverageRoute{
			Method: current.Method,
			Path:   current.Path,
			Status: statusMissing,
		})
	}

	currentKeys := make(map[string]struct{}, len(routes))
	for _, current := range routes {
		currentKeys[routeKey(current.Method, current.Path)] = struct{}{}
	}
	for _, entry := range spec.Routes {
		if _, ok := currentKeys[routeKey(entry.Method, entry.Path)]; !ok {
			stale = append(stale, entry)
		}
	}

	return updated, missing, stale
}

func applyDerivedRouteFields(spec *coverageSpec) {
	if spec == nil {
		return
	}

	for i, entry := range spec.Routes {
		entry = normalizeCoverageRoute(entry)
		matches := matchingExemptionIDs(entry.Method, entry.Path, spec.Exemptions)
		if len(matches) > 0 {
			entry.Policy = policyRestOnly
			if entry.ExemptedBy == "" || !containsString(matches, entry.ExemptedBy) {
				entry.ExemptedBy = matches[0]
			}
			entry.Status = ""
			entry.GraphQL = nil
		} else {
			entry.Policy = policyGraphQLRequired
			entry.ExemptedBy = ""
			if entry.Status == "" {
				entry.Status = statusMissing
			}
		}
		spec.Routes[i] = entry
	}
}

func validateRoutePolicies(routes []coverageRoute, exemptions []coverageExempt) error {
	exemptionsByID, err := indexExemptions(exemptions)
	if err != nil {
		return err
	}

	var problems []string
	for _, entry := range routes {
		problems = append(problems, validateRoutePolicy(entry, exemptions, exemptionsByID)...)
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	return nil
}

func indexExemptions(exemptions []coverageExempt) (map[string]coverageExempt, error) {
	exemptionsByID := make(map[string]coverageExempt, len(exemptions))
	for _, ex := range exemptions {
		id := strings.TrimSpace(ex.ID)
		if id == "" {
			return nil, errors.New("exemption id cannot be empty")
		}
		if _, ok := exemptionsByID[id]; ok {
			return nil, fmt.Errorf("duplicate exemption id %q", id)
		}
		exemptionsByID[id] = ex
	}
	return exemptionsByID, nil
}

func validateRoutePolicy(entry coverageRoute, exemptions []coverageExempt, exemptionsByID map[string]coverageExempt) []string {
	entry = normalizeCoverageRoute(entry)
	if entry.Method == "" || entry.Path == "" {
		return []string{"route entry missing method or path"}
	}

	matches := matchingExemptionIDs(entry.Method, entry.Path, exemptions)
	hasExemption := len(matches) > 0

	switch entry.Policy {
	case policyGraphQLRequired:
		return validateGraphQLRequiredRoute(entry, hasExemption, matches)
	case policyRestOnly:
		return validateRestOnlyRoute(entry, hasExemption, matches, exemptionsByID)
	default:
		return []string{fmt.Sprintf("%s %s: policy must be %q or %q (got %q)", entry.Method, entry.Path, policyGraphQLRequired, policyRestOnly, entry.Policy)}
	}
}

func validateGraphQLRequiredRoute(entry coverageRoute, hasExemption bool, matches []string) []string {
	var problems []string
	if hasExemption {
		problems = append(problems, fmt.Sprintf("%s %s: policy %q but matches exemption(s) %s", entry.Method, entry.Path, entry.Policy, strings.Join(matches, ", ")))
	}
	if entry.ExemptedBy != "" {
		problems = append(problems, fmt.Sprintf("%s %s: exemptedBy must be empty for policy %q", entry.Method, entry.Path, entry.Policy))
	}
	if entry.Status != statusCovered && entry.Status != statusMissing {
		problems = append(problems, fmt.Sprintf("%s %s: status must be %q or %q for policy %q", entry.Method, entry.Path, statusCovered, statusMissing, entry.Policy))
	}
	return problems
}

func validateRestOnlyRoute(entry coverageRoute, hasExemption bool, matches []string, exemptionsByID map[string]coverageExempt) []string {
	var problems []string
	if !hasExemption {
		problems = append(problems, fmt.Sprintf("%s %s: policy %q but no exemptions match", entry.Method, entry.Path, entry.Policy))
	}
	if entry.ExemptedBy == "" {
		problems = append(problems, fmt.Sprintf("%s %s: exemptedBy is required for policy %q", entry.Method, entry.Path, entry.Policy))
	} else if ex, ok := exemptionsByID[entry.ExemptedBy]; !ok {
		problems = append(problems, fmt.Sprintf("%s %s: exemptedBy %q does not match any exemption id", entry.Method, entry.Path, entry.ExemptedBy))
	} else if !exemptionMatches(entry.Method, entry.Path, ex.Match) {
		problems = append(problems, fmt.Sprintf("%s %s: exemptedBy %q does not match route (matching: %s)", entry.Method, entry.Path, entry.ExemptedBy, strings.Join(matches, ", ")))
	}
	if entry.Status != "" {
		problems = append(problems, fmt.Sprintf("%s %s: status must be empty for policy %q", entry.Method, entry.Path, entry.Policy))
	}
	if len(entry.GraphQL) > 0 {
		problems = append(problems, fmt.Sprintf("%s %s: graphql must be empty for policy %q", entry.Method, entry.Path, entry.Policy))
	}
	return problems
}

func reportRouteDrift(specPath string, missing []routeDef, stale []coverageRoute) error {
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
		if fileExists(filepath.Join(dir, "go.mod")) && fileExists(filepath.Join(dir, "cmd", "api", "routes.go")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", errors.New("unable to locate repo root (expected go.mod and cmd/api/routes.go)")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func readCoverageSpec(path string) (*coverageSpec, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is an operator-supplied local file
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var spec coverageSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if spec.Version == 0 {
		return nil, fmt.Errorf("invalid %s: missing version", path)
	}
	return &spec, nil
}

func writeCoverageSpec(path string, spec *coverageSpec) error {
	if spec == nil {
		return errors.New("spec is nil")
	}

	spec.Routes = dedupeCoverageRoutes(spec.Routes)
	sort.Slice(spec.Routes, func(i, j int) bool {
		if spec.Routes[i].Path == spec.Routes[j].Path {
			return spec.Routes[i].Method < spec.Routes[j].Method
		}
		return spec.Routes[i].Path < spec.Routes[j].Path
	})

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

func dedupeCoverageRoutes(in []coverageRoute) []coverageRoute {
	seen := make(map[string]struct{}, len(in))
	var out []coverageRoute
	for _, entry := range in {
		entry = normalizeCoverageRoute(entry)
		key := routeKey(entry.Method, entry.Path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, entry)
	}
	return out
}

func normalizeCoverageRoute(r coverageRoute) coverageRoute {
	r.Method = strings.ToUpper(strings.TrimSpace(r.Method))
	r.Path = strings.TrimSpace(r.Path)
	r.Policy = strings.ToLower(strings.TrimSpace(r.Policy))
	r.ExemptedBy = strings.TrimSpace(r.ExemptedBy)
	r.Capability = strings.TrimSpace(r.Capability)
	r.Status = strings.ToLower(strings.TrimSpace(r.Status))
	r.Notes = strings.TrimSpace(r.Notes)
	if len(r.GraphQL) > 0 {
		var cleaned []string
		for _, op := range r.GraphQL {
			if s := strings.TrimSpace(op); s != "" {
				cleaned = append(cleaned, s)
			}
		}
		r.GraphQL = cleaned
	}
	return r
}

func routeKey(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + strings.TrimSpace(path)
}

func extractConfiguredRoutes(repoRoot string) ([]routeDef, error) {
	files := []string{
		filepath.Join(repoRoot, "cmd", "api", "routes.go"),
		filepath.Join(repoRoot, "cmd", "api", "main.go"),
	}

	var routes []routeDef
	seen := make(map[string]struct{})

	callRE := regexp.MustCompile(`\bapp\.(GET|POST|PUT|PATCH|DELETE|Get|Post|Put|Patch|Delete)\("([^"]+)"`)
	handleRE := regexp.MustCompile(`\bapp\.Handle\("([^"]+)",\s*"([^"]+)"`)

	for _, path := range files {
		data, err := os.ReadFile(path) // #nosec G304 -- local source file path
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if m := callRE.FindStringSubmatch(line); len(m) == 3 {
				method := m[1]
				routePath := m[2]
				key := routeKey(method, routePath)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				routes = append(routes, routeDef{Method: method, Path: routePath})
			}
			if m := handleRE.FindStringSubmatch(line); len(m) == 3 {
				method := m[1]
				routePath := m[2]
				key := routeKey(method, routePath)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				routes = append(routes, routeDef{Method: method, Path: routePath})
			}
		}
	}

	return routes, nil
}

func sanitizeRoutes(routes []routeDef) []routeDef {
	var out []routeDef
	for _, r := range routes {
		method := strings.ToUpper(strings.TrimSpace(r.Method))
		path := strings.TrimSpace(r.Path)
		if method == "" || path == "" {
			continue
		}
		if method == "OPTIONS" || method == "HEAD" {
			continue
		}
		out = append(out, routeDef{Method: method, Path: path})
	}
	return out
}

func matchingExemptionIDs(method, path string, exemptions []coverageExempt) []string {
	var matches []string
	for _, ex := range exemptions {
		if exemptionMatches(method, path, ex.Match) {
			matches = append(matches, ex.ID)
		}
	}
	return matches
}

func containsString(in []string, needle string) bool {
	for _, s := range in {
		if s == needle {
			return true
		}
	}
	return false
}

func exemptionMatches(method, path string, match exemptionMatch) bool {
	if len(match.Methods) > 0 {
		allowed := false
		for _, m := range match.Methods {
			if strings.EqualFold(method, m) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	for _, exact := range match.Exact {
		if strings.TrimSpace(exact) == path {
			return true
		}
	}
	for _, prefix := range match.Prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix != "" && strings.HasPrefix(path, prefix) {
			return true
		}
	}
	for _, needle := range match.Contains {
		needle = strings.TrimSpace(needle)
		if needle != "" && strings.Contains(path, needle) {
			return true
		}
	}
	return false
}

func sortRoutes(routes []routeDef) {
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
}

type graphqlOperations struct {
	Query        map[string]struct{}
	Mutation     map[string]struct{}
	Subscription map[string]struct{}
}

func loadGraphQLOperations(repoRoot string) (*graphqlOperations, error) {
	cfgPath := filepath.Join(repoRoot, "gqlgen.yml")
	data, err := os.ReadFile(cfgPath) // #nosec G304 -- local file
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", cfgPath, err)
	}

	var cfg gqlgenConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", cfgPath, err)
	}
	if len(cfg.Schema) == 0 {
		return nil, fmt.Errorf("no schema entries found in %s", cfgPath)
	}

	var files []string
	for _, pattern := range cfg.Schema {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		abs := filepath.Join(repoRoot, filepath.FromSlash(pattern))
		matches, err := filepath.Glob(abs)
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", pattern, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("schema entry %q did not match any files", pattern)
		}
		files = append(files, matches...)
	}

	op := &graphqlOperations{
		Query:        map[string]struct{}{},
		Mutation:     map[string]struct{}{},
		Subscription: map[string]struct{}{},
	}

	for _, path := range files {
		content, err := os.ReadFile(path) // #nosec G304 -- local schema file
		if err != nil {
			return nil, fmt.Errorf("read schema %s: %w", path, err)
		}
		addSchemaOps(op, string(content))
	}

	return op, nil
}

func addSchemaOps(op *graphqlOperations, content string) {
	addRootOps(op.Query, content, "Query")
	addRootOps(op.Mutation, content, "Mutation")
	addRootOps(op.Subscription, content, "Subscription")
}

func addRootOps(dest map[string]struct{}, content string, rootType string) {
	lines := strings.Split(content, "\n")
	startRE := regexp.MustCompile(fmt.Sprintf(`^\s*(type|extend type)\s+%s\b`, regexp.QuoteMeta(rootType)))
	fieldRE := regexp.MustCompile(`^\s*([_A-Za-z][_0-9A-Za-z]*)\s*(\(|:)`)

	inBlock := false
	braceDepth := 0
	parenDepth := 0

	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "#") {
			continue
		}

		if !inBlock && startRE.MatchString(trim) {
			inBlock = true
			braceDepth = 0
			parenDepth = 0
			braceDepth += strings.Count(line, "{")
			braceDepth -= strings.Count(line, "}")
			continue
		}

		if !inBlock {
			continue
		}

		if braceDepth > 0 && parenDepth == 0 && trim != "" && !strings.HasPrefix(trim, "}") {
			if m := fieldRE.FindStringSubmatch(line); len(m) == 3 {
				name := strings.TrimSpace(m[1])
				if name != "" {
					dest[name] = struct{}{}
				}
			}
		}

		braceDepth += strings.Count(line, "{")
		braceDepth -= strings.Count(line, "}")
		parenDepth += strings.Count(line, "(")
		parenDepth -= strings.Count(line, ")")

		if braceDepth <= 0 && strings.Contains(line, "}") {
			inBlock = false
			braceDepth = 0
			parenDepth = 0
		}
	}
}

func validateGraphQLMappings(routes []coverageRoute, ops *graphqlOperations) error {
	if ops == nil {
		return errors.New("graphql operations is nil")
	}

	var problems []string
	for _, r := range routes {
		r = normalizeCoverageRoute(r)
		if r.Policy == policyRestOnly {
			continue
		}
		for _, mapping := range r.GraphQL {
			typ, field, ok := strings.Cut(mapping, ".")
			if !ok || strings.TrimSpace(typ) == "" || strings.TrimSpace(field) == "" {
				problems = append(problems, fmt.Sprintf("%s %s: invalid graphql mapping %q (expected Query.<field>)", r.Method, r.Path, mapping))
				continue
			}

			field = strings.TrimSpace(field)
			switch strings.TrimSpace(typ) {
			case "Query":
				if _, ok := ops.Query[field]; !ok {
					problems = append(problems, fmt.Sprintf("%s %s: graphql mapping %q not found in schema", r.Method, r.Path, mapping))
				}
			case "Mutation":
				if _, ok := ops.Mutation[field]; !ok {
					problems = append(problems, fmt.Sprintf("%s %s: graphql mapping %q not found in schema", r.Method, r.Path, mapping))
				}
			case "Subscription":
				if _, ok := ops.Subscription[field]; !ok {
					problems = append(problems, fmt.Sprintf("%s %s: graphql mapping %q not found in schema", r.Method, r.Path, mapping))
				}
			default:
				problems = append(problems, fmt.Sprintf("%s %s: invalid graphql mapping type %q (expected Query|Mutation|Subscription)", r.Method, r.Path, typ))
			}
		}
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	return nil
}
