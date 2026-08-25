// Package main provides static audit gates used by `lesser verify audit`.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type baseline struct {
	Version int `yaml:"version"`

	DisabledGoFiles []string `yaml:"disabledGoFiles"`

	GoReadAllRespBody    map[string]int `yaml:"goReadAllRespBody"`
	GoInsecureSkipVerify map[string]int `yaml:"goInsecureSkipVerify"`
	GoDynamoDBQueryScan  map[string]int `yaml:"goDynamoDBQueryScan"`
	GoDynamoDBBadPKWhere map[string]int `yaml:"goDynamoDBBadPKWhere"`
	GoDynamoDBAllNoKey   map[string]int `yaml:"goDynamoDBAllNoKey"`

	InfraCdkCspUnsafeInline map[string]int `yaml:"infraCdkCspUnsafeInline"`
	InfraCdkCspUnsafeEval   map[string]int `yaml:"infraCdkCspUnsafeEval"`
}

type options struct {
	Check        bool
	BaselinePath string
	DumpDynamoDB bool
}

func main() {
	var opts options
	flag.BoolVar(&opts.Check, "check", false, "run audit gates in check mode (exit non-zero on failures)")
	flag.StringVar(&opts.BaselinePath, "baseline", "tools/audit_gates/baseline.yml", "baseline file path (repo-relative)")
	flag.BoolVar(&opts.DumpDynamoDB, "dump-dynamodb-baseline", false, "print current DynamoDB scan/pk-misuse baseline YAML to stdout")
	flag.Parse()

	if opts.DumpDynamoDB {
		if err := dumpDynamoDBBaseline(); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		return
	}

	if !opts.Check {
		fmt.Fprintln(os.Stderr, "error: --check is required (or use --dump-dynamodb-baseline)")
		os.Exit(2)
	}

	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(opts options) error {
	b, err := loadBaseline(opts.BaselinePath)
	if err != nil {
		return err
	}

	var problems []string

	if err := checkStaticcheckGoVersion("go.mod", ".golangci.yml"); err != nil {
		problems = append(problems, err.Error())
	}

	if err := checkDisabledGoFiles(b); err != nil {
		problems = append(problems, err.Error())
	}

	if err := checkGoReadAllRespBody(b); err != nil {
		problems = append(problems, err.Error())
	}

	if err := checkGoInsecureSkipVerify(b); err != nil {
		problems = append(problems, err.Error())
	}

	if err := checkGoDynamoDBQueryScan(b); err != nil {
		problems = append(problems, err.Error())
	}

	if err := checkGoDynamoDBBadPKWhere(b); err != nil {
		problems = append(problems, err.Error())
	}

	if err := checkGoDynamoDBAllNoKey(b); err != nil {
		problems = append(problems, err.Error())
	}

	if err := checkInfraCdkCspUnsafe(b); err != nil {
		problems = append(problems, err.Error())
	}

	if err := checkGraphQLResolverRoleGates(); err != nil {
		problems = append(problems, err.Error())
	}

	if err := checkGraphQLResolverIgnoredContext(); err != nil {
		problems = append(problems, err.Error())
	}

	if err := checkHTMLResponsesHaveCSP(); err != nil {
		problems = append(problems, err.Error())
	}

	if err := checkSecurityStubInventory(); err != nil {
		problems = append(problems, err.Error())
	}

	if err := checkNoDefaultInitStorageContinuation(); err != nil {
		problems = append(problems, err.Error())
	}

	if len(problems) > 0 {
		return errors.New("audit gates failed:\n- " + strings.Join(problems, "\n- "))
	}

	fmt.Println("✓ audit gates passed")
	return nil
}

func checkNoDefaultInitStorageContinuation() error {
	matches, err := findDefaultInitStorageContinuationMatches("cmd")
	if err != nil {
		return err
	}
	if len(matches) > 0 {
		return fmt.Errorf("production lambdas must use pkg/lambdastorage instead of continuing after common.InitializeWithDefaults failures:\n  %s", strings.Join(matches, "\n  "))
	}
	return nil
}

func findDefaultInitStorageContinuationMatches(root string) ([]string, error) {
	forbidden := []string{
		"failed to initialize with defaults",
		"InitializeWithDefaults(",
		"initializeWithDefaults",
	}
	var matches []string
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open audit root: %w", err)
	}
	defer func() { _ = rootFS.Close() }()

	err = fs.WalkDir(rootFS.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(path, ".git") || strings.Contains(path, "/testdata/") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, readErr := rootFS.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, needle := range forbidden {
			if bytes.Contains(content, []byte(needle)) {
				displayPath := filepath.Join(root, filepath.FromSlash(path))
				matches = append(matches, fmt.Sprintf("%s contains %q", displayPath, needle))
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan lambda default-init usage: %w", err)
	}
	sort.Strings(matches)
	return matches, nil
}

func dumpDynamoDBBaseline() error {
	skips := defaultSkips()
	skips["tools"] = struct{}{} // allowlist cmd/tools one-time backfills

	scanCounts, err := countGoSelectorCallsWithMinArgs([]string{"cmd", "pkg", "graph"}, "Scan", 1, scanOptions{
		IncludeTests: false,
		Skips:        skips,
	})
	if err != nil {
		return err
	}

	badPKCounts, err := countGoWhereMisusedPartitionKey([]string{"cmd", "pkg", "graph"}, scanOptions{
		IncludeTests: false,
		Skips:        skips,
	})
	if err != nil {
		return err
	}

	allNoKeyCounts, err := countGoUnkeyedAllCalls([]string{"cmd", "pkg", "graph"}, scanOptions{
		IncludeTests: false,
		Skips:        skips,
	})
	if err != nil {
		return err
	}

	fmt.Println("goDynamoDBQueryScan:")
	printYAMLCountMap(scanCounts)
	fmt.Println()
	fmt.Println("goDynamoDBBadPKWhere:")
	printYAMLCountMap(badPKCounts)
	fmt.Println()
	fmt.Println("goDynamoDBAllNoKey:")
	printYAMLCountMap(allNoKeyCounts)
	return nil
}

func printYAMLCountMap(counts map[string]int) {
	keys := make([]string, 0, len(counts))
	for path := range counts {
		keys = append(keys, path)
	}
	sort.Strings(keys)

	if len(keys) == 0 {
		fmt.Println("  {}")
		return
	}

	for _, path := range keys {
		fmt.Printf("  %s: %d\n", path, counts[path])
	}
}

func loadBaseline(path string) (baseline, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- baseline is a repo-local file
	if err != nil {
		return baseline{}, fmt.Errorf("failed to read baseline file %q: %w", path, err)
	}

	var b baseline
	if err := yaml.Unmarshal(content, &b); err != nil {
		return baseline{}, fmt.Errorf("failed to parse baseline file %q: %w", path, err)
	}

	if b.Version == 0 {
		return baseline{}, fmt.Errorf("baseline file %q missing version", path)
	}

	return b, nil
}

func checkStaticcheckGoVersion(goModPath string, golangciPath string) error {
	goVersion, err := readGoModVersion(goModPath)
	if err != nil {
		return err
	}

	staticcheckGo, err := readGolangciStaticcheckGoVersion(golangciPath)
	if err != nil {
		return err
	}

	if goVersion != staticcheckGo {
		return fmt.Errorf("staticcheck go version mismatch: go.mod=%q .golangci.yml=%q", goVersion, staticcheckGo)
	}

	return nil
}

func readGoModVersion(path string) (string, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- repo-local file path
	if err != nil {
		return "", fmt.Errorf("failed to read %q: %w", path, err)
	}

	re := regexp.MustCompile(`(?m)^go\s+([0-9]+\.[0-9]+)(?:\.[0-9]+)?\s*$`)
	match := re.FindSubmatch(content)
	if len(match) != 2 {
		return "", fmt.Errorf("failed to parse go version from %q", path)
	}

	return string(match[1]), nil
}

type golangciConfig struct {
	LintersSettings struct {
		Staticcheck struct {
			Go string `yaml:"go"`
		} `yaml:"staticcheck"`
	} `yaml:"linters-settings"`
}

func readGolangciStaticcheckGoVersion(path string) (string, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- repo-local file path
	if err != nil {
		return "", fmt.Errorf("failed to read %q: %w", path, err)
	}

	var cfg golangciConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return "", fmt.Errorf("failed to parse %q: %w", path, err)
	}

	version := strings.TrimSpace(cfg.LintersSettings.Staticcheck.Go)
	if version == "" {
		return "", fmt.Errorf("missing linters-settings.staticcheck.go in %q", path)
	}

	return version, nil
}

func checkDisabledGoFiles(b baseline) error {
	want := make(map[string]struct{}, len(b.DisabledGoFiles))
	for _, path := range b.DisabledGoFiles {
		want[normalizePath(path)] = struct{}{}
	}

	actual, err := findFilesWithSuffix(".", ".go.disabled", defaultSkips())
	if err != nil {
		return err
	}

	var unexpected []string
	for _, path := range actual {
		if _, ok := want[normalizePath(path)]; !ok {
			unexpected = append(unexpected, path)
		}
	}

	var missing []string
	actualSet := make(map[string]struct{}, len(actual))
	for _, path := range actual {
		actualSet[normalizePath(path)] = struct{}{}
	}
	for path := range want {
		if _, ok := actualSet[path]; !ok {
			missing = append(missing, path)
		}
	}

	sort.Strings(unexpected)
	sort.Strings(missing)

	if len(unexpected) == 0 && len(missing) == 0 {
		return nil
	}

	var lines []string
	if len(unexpected) > 0 {
		lines = append(lines, fmt.Sprintf("unexpected *.go.disabled files: %s", strings.Join(unexpected, ", ")))
	}
	if len(missing) > 0 {
		lines = append(lines, fmt.Sprintf("baseline lists missing *.go.disabled files: %s", strings.Join(missing, ", ")))
	}
	lines = append(lines, "update tools/audit_gates/baseline.yml when intentionally changing these")
	return errors.New(strings.Join(lines, "; "))
}

func checkGoReadAllRespBody(b baseline) error {
	const needle = "io.ReadAll(resp.Body)"

	actual, err := countSubstringOccurrences([]string{"cmd", "pkg", "graph"}, needle, scanOptions{
		IncludeTests: false,
		Skips:        defaultSkips(),
	})
	if err != nil {
		return err
	}

	return compareCounts("io.ReadAll(resp.Body) occurrences", actual, b.GoReadAllRespBody)
}

func checkGoInsecureSkipVerify(b baseline) error {
	const needle = "InsecureSkipVerify: true"

	actual, err := countSubstringOccurrences([]string{"cmd", "pkg", "graph"}, needle, scanOptions{
		IncludeTests: false,
		Skips:        defaultSkips(),
	})
	if err != nil {
		return err
	}

	return compareCounts("InsecureSkipVerify: true occurrences", actual, b.GoInsecureSkipVerify)
}

func checkGoDynamoDBQueryScan(b baseline) error {
	skips := defaultSkips()
	skips["tools"] = struct{}{} // allowlist cmd/tools one-time backfills

	actual, err := countGoSelectorCallsWithMinArgs([]string{"cmd", "pkg", "graph"}, "Scan", 1, scanOptions{
		IncludeTests: false,
		Skips:        skips,
	})
	if err != nil {
		return err
	}

	return compareCounts("DynamoDB Query.Scan(...) occurrences", actual, b.GoDynamoDBQueryScan)
}

func checkGoDynamoDBBadPKWhere(b baseline) error {
	skips := defaultSkips()
	skips["tools"] = struct{}{} // allowlist cmd/tools one-time backfills

	actual, err := countGoWhereMisusedPartitionKey([]string{"cmd", "pkg", "graph"}, scanOptions{
		IncludeTests: false,
		Skips:        skips,
	})
	if err != nil {
		return err
	}

	return compareCounts("DynamoDB partition-key misuse in Where(...)", actual, b.GoDynamoDBBadPKWhere)
}

// checkGoDynamoDBAllNoKey catches key-less TableTheory All(...) calls — All
// without a preceding Where key condition — which compile to DynamoDB Scans.
// These are the seed-once migration scans and offline maintenance tools; they
// are baselined in baseline.yml with the deliberate one-time justification in
// docs/architecture/dynamodb-scan-inventory.md.
func checkGoDynamoDBAllNoKey(b baseline) error {
	skips := defaultSkips()
	skips["tools"] = struct{}{} // allowlist cmd/tools one-time backfills

	actual, err := countGoUnkeyedAllCalls([]string{"cmd", "pkg", "graph"}, scanOptions{
		IncludeTests: false,
		Skips:        skips,
	})
	if err != nil {
		return err
	}

	return compareCounts("key-less TableTheory All(...) occurrences", actual, b.GoDynamoDBAllNoKey)
}

// countGoUnkeyedAllCalls counts All(...) calls on freshly-built query chains
// that contain no Where(...) call — i.e. queries compiled to a DynamoDB Scan
// with no key condition. All(...) chained onto a pre-built query variable
// (query.Limit(n).All(...)) or a field receiver is statically indeterminate
// and is deliberately NOT flagged; the gate targets new inline key-less scan
// callsites like the instance-count seed scans.
func countGoUnkeyedAllCalls(roots []string, opts scanOptions) (map[string]int, error) {
	counts := make(map[string]int)
	for _, root := range roots {
		if err := walkGoFiles(root, opts, func(path string) error {
			n, err := countGoUnkeyedAllCallsInFile(path)
			if err != nil {
				return err
			}
			if n > 0 {
				counts[normalizePath(path)] = n
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return counts, nil
}

func countGoUnkeyedAllCallsInFile(path string) (int, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- repo-local file path (audit scan)
	if err != nil {
		return 0, err
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %q: %w", path, err)
	}

	n := 0
	ast.Inspect(f, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil || sel.Sel.Name != "All" {
			return true
		}
		if isUnkeyedFreshChain(sel.X) {
			n++
		}
		return true
	})
	return n, nil
}

// isUnkeyedFreshChain reports whether the All receiver is a freshly-built query
// chain: it contains a Model(...) or WithContext(...) construction call and no
// Where(...) call anywhere in the chain, so it compiles to an unkeyed Scan.
func isUnkeyedFreshChain(receiver ast.Expr) bool {
	hasConstruct := false
	hasWhere := false
	ast.Inspect(receiver, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil {
			return true
		}
		switch sel.Sel.Name {
		case "Model", "WithContext":
			hasConstruct = true
		case "Where":
			hasWhere = true
			return false
		}
		return true
	})
	return hasConstruct && !hasWhere
}

func checkInfraCdkCspUnsafe(b baseline) error {
	inlineActual, err := countSubstringOccurrences([]string{"infra/cdk"}, "'unsafe-inline'", scanOptions{
		IncludeTests: false,
		Skips:        defaultSkips(),
	})
	if err != nil {
		return err
	}

	evalActual, err := countSubstringOccurrences([]string{"infra/cdk"}, "'unsafe-eval'", scanOptions{
		IncludeTests: false,
		Skips:        defaultSkips(),
	})
	if err != nil {
		return err
	}

	if err := compareCounts("infra/cdk CSP 'unsafe-inline' occurrences", inlineActual, b.InfraCdkCspUnsafeInline); err != nil {
		return err
	}
	if err := compareCounts("infra/cdk CSP 'unsafe-eval' occurrences", evalActual, b.InfraCdkCspUnsafeEval); err != nil {
		return err
	}

	return nil
}

type gqlRoleGateRule struct {
	File     string
	Receiver string
	Gate     string
	Methods  []string
}

func checkGraphQLResolverRoleGates() error {
	rules := []gqlRoleGateRule{
		// Moderation: mod/admin only.
		{File: "graph/query_resolvers_moderation.go", Receiver: "queryResolver", Gate: "requireModeratorOrAdmin"},
		{File: "graph/subscription_resolvers_moderation.go", Receiver: "subscriptionResolver", Gate: "requireModeratorOrAdmin"},
		{File: "graph/mutation_resolvers_moderation.go", Receiver: "mutationResolver", Gate: "requireModeratorOrAdmin", Methods: []string{
			"CreateModerationPattern",
			"UpdateModerationPattern",
			"DeleteModerationPattern",
		}},

		// Admin-only ops/insights.
		{File: "graph/query_resolvers_cost.go", Receiver: "queryResolver", Gate: "requireAdmin"},
		{File: "graph/subscription_resolvers_cost.go", Receiver: "subscriptionResolver", Gate: "requireAdmin"},
		{File: "graph/query_resolvers_federation.go", Receiver: "queryResolver", Gate: "requireAdmin"},
		{File: "graph/subscription_resolvers_federation.go", Receiver: "subscriptionResolver", Gate: "requireAdmin"},
		{File: "graph/query_resolvers_ai.go", Receiver: "queryResolver", Gate: "requireAdmin"},
		{File: "graph/subscription_resolvers_ai.go", Receiver: "subscriptionResolver", Gate: "requireAdmin"},

		// Admin-only control plane (even when partially stubbed).
		{File: "graph/mutation_resolvers_federation.go", Receiver: "mutationResolver", Gate: "requireAdmin"},
	}

	var problems []string
	for _, rule := range rules {
		missing, err := resolverMethodsMissingGate(rule)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		for _, method := range missing {
			problems = append(problems, fmt.Sprintf("graphql role gate missing: %s %s.%s should call %s", rule.File, rule.Receiver, method, rule.Gate))
		}
	}

	sort.Strings(problems)
	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "; "))
}

func resolverMethodsMissingGate(rule gqlRoleGateRule) ([]string, error) {
	if rule.File == "" || rule.Receiver == "" || rule.Gate == "" {
		return nil, fmt.Errorf("internal error: invalid gql role gate rule: %+v", rule)
	}

	content, err := os.ReadFile(rule.File) // #nosec G304 -- repo-local file path (audit scan)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read %q: %w", rule.File, err)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rule.File, content, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %q: %w", rule.File, err)
	}

	methodAllow := make(map[string]struct{}, len(rule.Methods))
	for _, name := range rule.Methods {
		methodAllow[name] = struct{}{}
	}

	var missing []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Body == nil || fn.Name == nil {
			continue
		}
		if receiverTypeName(fn) != rule.Receiver {
			continue
		}

		if !ast.IsExported(fn.Name.Name) {
			continue
		}

		if len(methodAllow) > 0 {
			if _, ok := methodAllow[fn.Name.Name]; !ok {
				continue
			}
		}

		if !funcDeclHasGateCall(fn, rule.Gate) {
			missing = append(missing, fn.Name.Name)
		}
	}

	sort.Strings(missing)
	return missing, nil
}

func receiverTypeName(fn *ast.FuncDecl) string {
	if fn == nil || fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}

	receiverType := fn.Recv.List[0].Type
	switch t := receiverType.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	}

	return ""
}

func funcDeclHasGateCall(fn *ast.FuncDecl, gateName string) bool {
	if fn == nil || fn.Body == nil {
		return false
	}

	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if fun.Sel != nil && fun.Sel.Name == gateName {
				found = true
				return false
			}
		case *ast.Ident:
			if fun.Name == gateName {
				found = true
				return false
			}
		}

		return true
	})

	return found
}

func checkGraphQLResolverIgnoredContext() error {
	re := regexp.MustCompile(`func\s+\(r\s+\*(queryResolver|mutationResolver|subscriptionResolver)\)\s+\w+\(_\s+context\.Context`)
	actual, err := countRegexpOccurrences([]string{"graph"}, re, scanOptions{
		IncludeTests: false,
		Skips:        defaultSkips(),
	})
	if err != nil {
		return err
	}

	if len(actual) == 0 {
		return nil
	}

	var problems []string
	for path, count := range actual {
		problems = append(problems, fmt.Sprintf("graphql resolver ignores context: %s (%d)", path, count))
	}
	sort.Strings(problems)
	return errors.New(strings.Join(problems, "; "))
}

func checkHTMLResponsesHaveCSP() error {
	const htmlContentTypeHeader = `"content-type": {"text/html`
	const cspHeaderKey = "content-security-policy"

	var offenders []string
	err := walkGoFiles("cmd", scanOptions{IncludeTests: false, Skips: defaultSkips()}, func(path string) error {
		data, err := os.ReadFile(path) // #nosec G304 -- repo-local file path (audit scan)
		if err != nil {
			return err
		}

		content := string(data)
		if !strings.Contains(content, htmlContentTypeHeader) {
			return nil
		}

		if !strings.Contains(strings.ToLower(content), cspHeaderKey) {
			offenders = append(offenders, normalizePath(path))
		}
		return nil
	})
	if err != nil {
		return err
	}

	sort.Strings(offenders)
	if len(offenders) == 0 {
		return nil
	}

	return errors.New("html responses missing CSP header: " + strings.Join(offenders, ", "))
}

func checkSecurityStubInventory() error {
	const needle = "This is a placeholder"

	docPath := filepath.FromSlash("docs/security-stubs-and-placeholders.md")
	docContent, err := os.ReadFile(docPath) // #nosec G304 -- repo-local doc path
	if err != nil {
		return fmt.Errorf("failed to read %q: %w", docPath, err)
	}

	docText := string(docContent)
	var offenders []string
	for _, root := range []string{"cmd", "graph", "pkg/auth", "pkg/security"} {
		err := walkGoFiles(root, scanOptions{IncludeTests: false, Skips: defaultSkips()}, func(path string) error {
			data, err := os.ReadFile(path) // #nosec G304 -- repo-local file path (audit scan)
			if err != nil {
				return err
			}
			if !bytes.Contains(data, []byte(needle)) {
				return nil
			}

			normalized := normalizePath(path)
			if !strings.Contains(docText, normalized) {
				offenders = append(offenders, normalized)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	sort.Strings(offenders)
	if len(offenders) == 0 {
		return nil
	}

	return errors.New("placeholder inventory missing entries in docs/security-stubs-and-placeholders.md: " + strings.Join(offenders, ", "))
}

type scanOptions struct {
	IncludeTests bool
	Skips        map[string]struct{}
}

func countSubstringOccurrences(roots []string, needle string, opts scanOptions) (map[string]int, error) {
	counts := make(map[string]int)
	if needle == "" {
		return counts, fmt.Errorf("internal error: empty needle")
	}

	for _, root := range roots {
		if err := walkGoFiles(root, opts, func(path string) error {
			n, err := countNeedleInFile(path, needle)
			if err != nil {
				return err
			}
			if n == 0 {
				return nil
			}
			counts[normalizePath(path)] = n
			return nil
		}); err != nil {
			return nil, err
		}
	}

	return counts, nil
}

func countRegexpOccurrences(roots []string, needle *regexp.Regexp, opts scanOptions) (map[string]int, error) {
	counts := make(map[string]int)
	if needle == nil {
		return counts, fmt.Errorf("internal error: nil regexp")
	}

	for _, root := range roots {
		if err := walkGoFiles(root, opts, func(path string) error {
			data, err := os.ReadFile(path) // #nosec G304 -- repo-local file path (audit scan)
			if err != nil {
				return err
			}

			n := len(needle.FindAllIndex(data, -1))
			if n == 0 {
				return nil
			}
			counts[normalizePath(path)] = n
			return nil
		}); err != nil {
			return nil, err
		}
	}

	return counts, nil
}

func countGoSelectorCallsWithMinArgs(roots []string, selector string, minArgs int, opts scanOptions) (map[string]int, error) {
	counts := make(map[string]int)

	selector = strings.TrimSpace(selector)
	if selector == "" {
		return counts, fmt.Errorf("internal error: empty selector")
	}
	if minArgs < 0 {
		return counts, fmt.Errorf("internal error: minArgs must be >= 0")
	}

	for _, root := range roots {
		if err := walkGoFiles(root, opts, func(path string) error {
			n, err := countGoSelectorCallsWithMinArgsInFile(path, selector, minArgs)
			if err != nil {
				return err
			}
			if n > 0 {
				counts[normalizePath(path)] = n
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}

	return counts, nil
}

func countGoSelectorCallsWithMinArgsInFile(path string, selector string, minArgs int) (int, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- repo-local file path (audit scan)
	if err != nil {
		return 0, err
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %q: %w", path, err)
	}

	n := 0
	ast.Inspect(f, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isSelectorCallWithMinArgs(call, selector, minArgs) {
			n++
		}
		return true
	})

	return n, nil
}

func isSelectorCallWithMinArgs(call *ast.CallExpr, selector string, minArgs int) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return false
	}
	if sel.Sel.Name != selector {
		return false
	}
	return len(call.Args) >= minArgs
}

func countGoWhereMisusedPartitionKey(roots []string, opts scanOptions) (map[string]int, error) {
	counts := make(map[string]int)
	badOps := badPartitionKeyWhereOps()

	for _, root := range roots {
		if err := walkGoFiles(root, opts, func(path string) error {
			n, err := countGoWhereMisusedPartitionKeyInFile(path, badOps)
			if err != nil {
				return err
			}
			if n > 0 {
				counts[normalizePath(path)] = n
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}

	return counts, nil
}

func badPartitionKeyWhereOps() map[string]struct{} {
	return map[string]struct{}{
		"begins_with": {},
		"BEGINS_WITH": {},
		">":           {},
		">=":          {},
		"<":           {},
		"<=":          {},
	}
}

func countGoWhereMisusedPartitionKeyInFile(path string, badOps map[string]struct{}) (int, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- repo-local file path (audit scan)
	if err != nil {
		return 0, err
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %q: %w", path, err)
	}

	n := 0
	ast.Inspect(f, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isMisusedPartitionKeyWhere(call, badOps) {
			n++
		}
		return true
	})

	return n, nil
}

func isMisusedPartitionKeyWhere(call *ast.CallExpr, badOps map[string]struct{}) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil || sel.Sel.Name != "Where" {
		return false
	}
	if len(call.Args) < 2 {
		return false
	}

	field, ok := goStringLiteral(call.Args[0])
	if !ok || !isPartitionKeyField(field) {
		return false
	}

	op, ok := goStringLiteral(call.Args[1])
	if !ok {
		return false
	}
	_, bad := badOps[op]
	return bad
}

func goStringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	decoded, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return decoded, true
}

func isPartitionKeyField(field string) bool {
	if field == "PK" {
		return true
	}

	if !strings.HasPrefix(field, "gsi") || !strings.HasSuffix(field, "PK") {
		return false
	}

	middle := strings.TrimSuffix(strings.TrimPrefix(field, "gsi"), "PK")
	if middle == "" {
		return false
	}

	for _, ch := range middle {
		if ch < '0' || ch > '9' {
			return false
		}
	}

	return true
}

func walkGoFiles(root string, opts scanOptions, fn func(path string) error) error {
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if shouldSkipDir(path, opts.Skips) {
				return fs.SkipDir
			}
			return nil
		}

		if filepath.Ext(path) != ".go" {
			return nil
		}
		if !opts.IncludeTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}

		return fn(path)
	})
}

func countNeedleInFile(path string, needle string) (int, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- local source file path (repo scan)
	if err != nil {
		return 0, err
	}
	return strings.Count(string(data), needle), nil
}

func compareCounts(label string, actual map[string]int, expected map[string]int) error {
	expectedNormalized := make(map[string]int, len(expected))
	for path, count := range expected {
		expectedNormalized[normalizePath(path)] = count
	}

	var problems []string

	for path, count := range actual {
		want, ok := expectedNormalized[normalizePath(path)]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: unexpected %q (%d)", label, path, count))
			continue
		}
		if count != want {
			problems = append(problems, fmt.Sprintf("%s: %q expected %d got %d", label, path, want, count))
		}
	}

	for path, want := range expectedNormalized {
		if _, ok := actual[path]; !ok {
			problems = append(problems, fmt.Sprintf("%s: baseline expects %q (%d) but found 0", label, path, want))
		}
		if _, err := os.Stat(filepath.FromSlash(path)); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				problems = append(problems, fmt.Sprintf("%s: baseline references missing file %q", label, path))
				continue
			}
			problems = append(problems, fmt.Sprintf("%s: failed to stat %q: %v", label, path, err))
		}
	}

	sort.Strings(problems)

	if len(problems) == 0 {
		return nil
	}

	problems = append(problems, "update tools/audit_gates/baseline.yml when intentionally changing these")
	return errors.New(strings.Join(problems, "; "))
}

func normalizePath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func defaultSkips() map[string]struct{} {
	return map[string]struct{}{
		".git":         {},
		"bin":          {},
		"build":        {},
		"cdk.out":      {},
		"dist":         {},
		"node_modules": {},
		"report":       {},
		"tmp":          {},
		"vendor":       {},
	}
}

func shouldSkipDir(path string, skips map[string]struct{}) bool {
	base := filepath.Base(path)
	if base == "" || base == "." {
		return false
	}
	if _, ok := skips[base]; ok {
		return true
	}
	return false
}

func findFilesWithSuffix(root string, suffix string, skips map[string]struct{}) ([]string, error) {
	var matches []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipDir(path, skips) {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, suffix) {
			matches = append(matches, normalizePath(strings.TrimPrefix(path, "./")))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}
