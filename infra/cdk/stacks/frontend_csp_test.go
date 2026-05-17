package stacks

import (
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestFrontendStaticCSPIsStrictAndBehaviorScoped(t *testing.T) {
	resources := synthClientFrontendResources(t)
	authPolicyLogicalID, authPolicy, clientPolicyLogicalID, clientPolicy := findFrontendResponseHeadersPolicies(t, resources)
	csp := extractResponseHeadersPolicyCSP(t, authPolicy)
	clientCSP := extractResponseHeadersPolicyCSP(t, clientPolicy)

	if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
		t.Fatalf("CSP must not include unsafe directives: %s", csp)
	}
	requireContainsAll(t, csp, []string{
		"'sha256-QzWFZi+FLIx23tnm9SBU4aEgx4x8DsuASP07mfqol/c='",
		"'sha256-SaCkFfPruIdTXT8/97JArQmGxiJAL2o4bBDvSgJ5y3Q='",
		"'sha256-eIXWvAmxkr251LJZkjniEK5LcPF3NkapbJepohwYRIc='",
		"'sha256-IV0HjYu959C/EiJIL2l/9Ty8PA4757JXhA/g112YXVE='",
		"'sha256-vv9IoKo7BSLbWcUHr3tNmfNVmm5L/9Cfn2H6LMk7/ow='",
	})
	requireContainsAll(t, clientCSP, []string{
		"default-src 'self'",
		"frame-ancestors 'none'",
		"object-src 'none'",
		"connect-src 'self' https: wss:",
	})
	if strings.Contains(clientCSP, "unsafe-eval") {
		t.Fatalf("client SSR fallback CSP must not include unsafe-eval: %s", clientCSP)
	}
	if strings.Contains(clientCSP, "unsafe-inline") {
		t.Fatalf("client SSR fallback CSP must not include unsafe-inline: %s", clientCSP)
	}

	dist := findSingleCloudFrontDistribution(t, resources)
	defaultBehavior := extractDefaultCacheBehavior(t, dist)
	if _, ok := defaultBehavior["ResponseHeadersPolicyId"]; ok {
		t.Fatalf("default behavior must not attach ResponseHeadersPolicyId (origin CSP should remain authoritative)")
	}

	cacheBehaviors := extractCacheBehaviors(t, dist)
	authBehaviors := map[string]struct{}{
		"/auth":   {},
		"/auth/*": {},
	}
	clientBehaviors := map[string]struct{}{
		"/l":           {},
		"/l/*":         {},
		"/l/_assets/*": {},
	}
	for _, behavior := range cacheBehaviors {
		pathPattern, _ := behavior["PathPattern"].(string)
		raw, has := behavior["ResponseHeadersPolicyId"]
		if _, shouldHave := authBehaviors[pathPattern]; shouldHave {
			if !has {
				t.Fatalf("behavior %q missing ResponseHeadersPolicyId", pathPattern)
			}
			if !isRefTo(raw, authPolicyLogicalID) {
				t.Fatalf("behavior %q ResponseHeadersPolicyId does not reference %s", pathPattern, authPolicyLogicalID)
			}
			continue
		}
		if _, shouldHave := clientBehaviors[pathPattern]; shouldHave {
			if !has {
				t.Fatalf("behavior %q missing ResponseHeadersPolicyId", pathPattern)
			}
			if !isRefTo(raw, clientPolicyLogicalID) {
				t.Fatalf("behavior %q ResponseHeadersPolicyId does not reference %s", pathPattern, clientPolicyLogicalID)
			}
			continue
		}
		if has && strings.HasPrefix(pathPattern, "/auth/wallet") {
			t.Fatalf("behavior %q must not attach ResponseHeadersPolicyId (API-owned route)", pathPattern)
		}
	}
}

func TestFrontendStaticCSPMatchesBuiltAuthUIDist(t *testing.T) {
	repoRoot := findRepoRootForAuthUITest(t)
	distDir := filepath.Join(repoRoot, "auth-ui", "dist")
	if _, err := os.Stat(filepath.Join(distDir, "index.html")); err != nil {
		if os.Getenv("LESSER_REQUIRE_AUTH_UI_CSP_DIST") == "1" {
			t.Fatalf("auth-ui/dist is required for release CSP verification; run `corepack pnpm --dir auth-ui -s build`: %v", err)
		}
		t.Skipf("auth-ui/dist is not built: %v", err)
	}

	distHashes := collectAuthUIInlineHashes(t, distDir)
	resources := synthClientFrontendResources(t)
	_, authPolicy, _, _ := findFrontendResponseHeadersPolicies(t, resources)
	csp := extractResponseHeadersPolicyCSP(t, authPolicy)

	assertCSPHashSet(t, csp, "script-src", distHashes.scripts)
	assertCSPHashSet(t, csp, "style-src", distHashes.styles)
}

func findFrontendResponseHeadersPolicies(t *testing.T, resources map[string]any) (string, map[string]any, string, map[string]any) {
	t.Helper()
	var authLogicalID string
	var authPolicy map[string]any
	var clientLogicalID string
	var clientPolicy map[string]any

	for id, raw := range resources {
		typed, ok := raw.(map[string]any)
		if !ok || typed["Type"] != "AWS::CloudFront::ResponseHeadersPolicy" {
			continue
		}
		comment := responseHeadersPolicyComment(typed)
		if strings.Contains(comment, "static site") {
			if authLogicalID != "" {
				t.Fatalf("expected one auth ResponseHeadersPolicy, found multiple (%s, %s)", authLogicalID, id)
			}
			authLogicalID = id
			authPolicy = typed
			continue
		}
		if !strings.Contains(comment, "SSR client") {
			continue
		}
		if clientLogicalID != "" {
			t.Fatalf("expected one client ResponseHeadersPolicy, found multiple (%s, %s)", clientLogicalID, id)
		}
		clientLogicalID = id
		clientPolicy = typed
	}
	if authLogicalID == "" || clientLogicalID == "" {
		t.Fatalf("expected auth and client ResponseHeadersPolicy resources")
	}
	return authLogicalID, authPolicy, clientLogicalID, clientPolicy
}

func responseHeadersPolicyComment(policy map[string]any) string {
	props, ok := policy["Properties"].(map[string]any)
	if !ok {
		return ""
	}
	cfg, ok := props["ResponseHeadersPolicyConfig"].(map[string]any)
	if !ok {
		return ""
	}
	comment, _ := cfg["Comment"].(string)
	return comment
}

func extractResponseHeadersPolicyCSP(t *testing.T, policy map[string]any) string {
	t.Helper()
	props, ok := policy["Properties"].(map[string]any)
	if !ok {
		t.Fatalf("ResponseHeadersPolicy Properties missing or wrong type")
	}
	cfg, ok := props["ResponseHeadersPolicyConfig"].(map[string]any)
	if !ok {
		t.Fatalf("ResponseHeadersPolicyConfig missing or wrong type")
	}
	sec, ok := cfg["SecurityHeadersConfig"].(map[string]any)
	if !ok {
		t.Fatalf("SecurityHeadersConfig missing or wrong type")
	}
	csp, ok := sec["ContentSecurityPolicy"].(map[string]any)
	if !ok {
		t.Fatalf("ContentSecurityPolicy missing or wrong type")
	}
	value, ok := csp["ContentSecurityPolicy"].(string)
	if !ok || strings.TrimSpace(value) == "" {
		t.Fatalf("ContentSecurityPolicy string missing or wrong type")
	}
	override, ok := csp["Override"].(bool)
	if !ok {
		t.Fatalf("ContentSecurityPolicy Override missing or wrong type")
	}
	if override {
		t.Fatalf("ContentSecurityPolicy Override must be false to preserve any origin-provided CSP")
	}
	return value
}

func hasResponseHeadersPolicyCSP(policy map[string]any) bool {
	props, ok := policy["Properties"].(map[string]any)
	if !ok {
		return false
	}
	cfg, ok := props["ResponseHeadersPolicyConfig"].(map[string]any)
	if !ok {
		return false
	}
	sec, ok := cfg["SecurityHeadersConfig"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = sec["ContentSecurityPolicy"]
	return ok
}

func findSingleCloudFrontDistribution(t *testing.T, resources map[string]any) map[string]any {
	t.Helper()
	var dist map[string]any
	for _, raw := range resources {
		typed, ok := raw.(map[string]any)
		if !ok || typed["Type"] != "AWS::CloudFront::Distribution" {
			continue
		}
		if dist != nil {
			t.Fatalf("expected one CloudFront Distribution, found multiple")
		}
		dist = typed
	}
	if dist == nil {
		t.Fatalf("expected CloudFront Distribution resource")
	}
	return dist
}

func extractDefaultCacheBehavior(t *testing.T, dist map[string]any) map[string]any {
	t.Helper()
	props, ok := dist["Properties"].(map[string]any)
	if !ok {
		t.Fatalf("Distribution Properties missing or wrong type")
	}
	cfg, ok := props["DistributionConfig"].(map[string]any)
	if !ok {
		t.Fatalf("DistributionConfig missing or wrong type")
	}
	behavior, ok := cfg["DefaultCacheBehavior"].(map[string]any)
	if !ok {
		t.Fatalf("DefaultCacheBehavior missing or wrong type")
	}
	return behavior
}

func extractCacheBehaviors(t *testing.T, dist map[string]any) []map[string]any {
	t.Helper()
	props, ok := dist["Properties"].(map[string]any)
	if !ok {
		t.Fatalf("Distribution Properties missing or wrong type")
	}
	cfg, ok := props["DistributionConfig"].(map[string]any)
	if !ok {
		t.Fatalf("DistributionConfig missing or wrong type")
	}

	raw, ok := cfg["CacheBehaviors"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		behavior, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, behavior)
	}
	return out
}

func isRefTo(raw any, want string) bool {
	ref, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	got, ok := ref["Ref"].(string)
	if !ok {
		return false
	}
	return got == want
}

func requireContainsAll(t *testing.T, haystack string, needles []string) {
	t.Helper()
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			t.Fatalf("expected CSP to contain %q", needle)
		}
	}
}

type authUIInlineHashes struct {
	scripts map[string]struct{}
	styles  map[string]struct{}
}

func findRepoRootForAuthUITest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "auth-ui", "package.json")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatalf("could not find repo root containing auth-ui/package.json from %s", wd)
		}
	}
}

func collectAuthUIInlineHashes(t *testing.T, distDir string) authUIInlineHashes {
	t.Helper()
	result := authUIInlineHashes{
		scripts: map[string]struct{}{},
		styles:  map[string]struct{}{},
	}
	scriptRe := regexp.MustCompile(`(?is)<script([^>]*)>(.*?)</script>`)
	styleRe := regexp.MustCompile(`(?is)<style([^>]*)>(.*?)</style>`)

	err := filepath.WalkDir(distDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		html := string(data)
		for _, match := range scriptRe.FindAllStringSubmatch(html, -1) {
			if len(match) < 3 || strings.Contains(strings.ToLower(match[1]), "src=") {
				continue
			}
			result.scripts[cspSHA256Source(match[2])] = struct{}{}
		}
		for _, match := range styleRe.FindAllStringSubmatch(html, -1) {
			if len(match) < 3 {
				continue
			}
			result.styles[cspSHA256Source(match[2])] = struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk auth-ui dist: %v", err)
	}
	if len(result.scripts) == 0 {
		t.Fatalf("auth-ui dist has no inline scripts to verify")
	}
	return result
}

func cspSHA256Source(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

func assertCSPHashSet(t *testing.T, csp string, directive string, want map[string]struct{}) {
	t.Helper()
	got := map[string]struct{}{}
	for _, source := range cspDirectiveSources(csp, directive) {
		if strings.HasPrefix(source, "'sha256-") {
			got[source] = struct{}{}
		}
	}
	if !sameStringSet(got, want) {
		t.Fatalf("%s CSP hashes do not match built auth-ui/dist\nmissing from CSP: %v\nstale in CSP: %v",
			directive,
			setDifference(want, got),
			setDifference(got, want),
		)
	}
}

func cspDirectiveSources(csp string, directive string) []string {
	for _, part := range strings.Split(csp, ";") {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) == 0 || fields[0] != directive {
			continue
		}
		return fields[1:]
	}
	return nil
}

func sameStringSet(left map[string]struct{}, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if _, ok := right[key]; !ok {
			return false
		}
	}
	return true
}

func setDifference(left map[string]struct{}, right map[string]struct{}) []string {
	var diff []string
	for key := range left {
		if _, ok := right[key]; !ok {
			diff = append(diff, key)
		}
	}
	sort.Strings(diff)
	return diff
}
