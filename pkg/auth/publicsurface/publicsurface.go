// Package publicsurface owns Lesser's importable public-surface decision.
package publicsurface

import (
	"net/http"
	"strings"
)

const apiV1AppsPath = "/api/v1/apps"

// RuleMatch describes how a public-surface rule matches paths.
type RuleMatch string

const (
	// RuleMatchExact matches one exact path.
	RuleMatchExact RuleMatch = "exact"
	// RuleMatchPrefix matches every path with the configured prefix.
	RuleMatchPrefix RuleMatch = "prefix"
	// RuleMatchStatusRead matches public status-read paths except sensitive
	// status subresources.
	RuleMatchStatusRead RuleMatch = "status_read"
	// RuleMatchAccountContent matches public account statuses/notes reads.
	RuleMatchAccountContent RuleMatch = "account_content"
	// RuleMatchSingleSegment matches a prefix followed by exactly one path
	// segment. It is used for public profile/detail routes without opening
	// sibling collection or credential-management routes under the same prefix.
	RuleMatchSingleSegment RuleMatch = "single_segment"
	// RuleMatchAgentAccessLeaseProof matches the agent access-lease proof
	// exchange routes that are intentionally reachable without an OAuth bearer;
	// those handlers enforce lease/challenge/signature proof in the request body.
	RuleMatchAgentAccessLeaseProof RuleMatch = "agent_access_lease_proof"
	// RuleMatchSkills matches the public skills catalog with one exact exclusion.
	RuleMatchSkills RuleMatch = "skills_catalog"
)

// PublicRule is one source-of-truth entry in Lesser's anonymous public surface.
// The runtime gate, generated docs, and reconciliation tests all derive from
// these rules.
type PublicRule struct {
	Methods          []string
	Path             string
	Match            RuleMatch
	Description      string
	ExceptExactPaths []string
	ExceptSuffixes   []string
	RequiredContains []string
}

// ContractAuthClass describes auth requirements that are enforced outside the
// API gateway public-surface middleware but still need to be reflected in the
// generated public contract.
type ContractAuthClass string

const (
	// ContractAuthSetupBearer uses the temporary setup-session bearer token.
	ContractAuthSetupBearer ContractAuthClass = "setup_bearer"
	// ContractAuthBearerRequired uses the normal OAuth bearer-token posture.
	ContractAuthBearerRequired ContractAuthClass = "bearer_required"
	// ContractAuthInternalOnly is handler-enforced with internal instance keys.
	ContractAuthInternalOnly ContractAuthClass = "internal_only"
)

// ContractAuthRule is one handler-enforced contract-auth override for a route
// that remains gate-reachable through IsPublic.
type ContractAuthRule struct {
	Method      string
	Path        string
	Class       ContractAuthClass
	Description string
}

// ClassificationKind identifies how publicsurface resolves a route.
type ClassificationKind string

const (
	// ClassificationAnonymous means the route is in the anonymous public surface.
	ClassificationAnonymous ClassificationKind = "anonymous"
	// ClassificationContractAuth means the gate is reachable but handlers enforce
	// a non-anonymous auth class that the generated contract must advertise.
	ClassificationContractAuth ClassificationKind = "contract_auth"
	// ClassificationAuthRequired is the default-deny classification for routes
	// outside the anonymous allowlist.
	ClassificationAuthRequired ClassificationKind = "auth_required"
	// ClassificationUnknown means the route could not be classified because the
	// method or path is empty.
	ClassificationUnknown ClassificationKind = "unknown"
)

// Classification is publicsurface's resolved auth posture for a method/path.
type Classification struct {
	Kind              ClassificationKind
	Public            bool
	Rule              *PublicRule
	ContractAuthClass ContractAuthClass
}

var publicRules = []PublicRule{
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/",
		Match:       RuleMatchExact,
		Description: "root document",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/robots.txt",
		Match:       RuleMatchExact,
		Description: "robots metadata",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/.well-known/oauth-authorization-server",
		Match:       RuleMatchExact,
		Description: "OAuth authorization-server metadata",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/.well-known/nodeinfo",
		Match:       RuleMatchExact,
		Description: "NodeInfo discovery",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/.well-known/lesser-soul-agent",
		Match:       RuleMatchExact,
		Description: "Lesser Soul agent discovery",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/nodeinfo/2.0",
		Match:       RuleMatchExact,
		Description: "NodeInfo document",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/.well-known/reputation-keys",
		Match:       RuleMatchExact,
		Description: "reputation key discovery",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/health",
		Match:       RuleMatchExact,
		Description: "legacy liveness health check",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/health/live",
		Match:       RuleMatchExact,
		Description: "liveness health check",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/health/ready",
		Match:       RuleMatchExact,
		Description: "readiness health check",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/auth/device",
		Match:       RuleMatchExact,
		Description: "device authorization status",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/oembed",
		Match:       RuleMatchExact,
		Description: "oEmbed lookup",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/instance",
		Match:       RuleMatchExact,
		Description: "Mastodon instance metadata",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v2/instance",
		Match:       RuleMatchExact,
		Description: "Mastodon v2 instance metadata",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/custom_emojis",
		Match:       RuleMatchExact,
		Description: "custom emoji catalog",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/directory",
		Match:       RuleMatchExact,
		Description: "public profile directory",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/announcements",
		Match:       RuleMatchExact,
		Description: "public announcements",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/timelines/public",
		Match:       RuleMatchExact,
		Description: "public timeline",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/timelines/link",
		Match:       RuleMatchExact,
		Description: "public link timeline",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v2/search",
		Match:       RuleMatchExact,
		Description: "Mastodon v2 search endpoint",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v2/suggestions",
		Match:       RuleMatchExact,
		Description: "public suggestions endpoint",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/setup/status",
		Match:       RuleMatchExact,
		Description: "setup status check",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/oauth/authorize",
		Match:       RuleMatchExact,
		Description: "OAuth authorization entrypoint",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/trust/jwks.json",
		Match:       RuleMatchExact,
		Description: "lesser.host trust-proxy JWKS",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/trust/attestations",
		Match:       RuleMatchExact,
		Description: "lesser.host trust-proxy attestation index",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/embed/",
		Match:       RuleMatchPrefix,
		Description: "public embeds",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/instance/",
		Match:       RuleMatchPrefix,
		Description: "instance metadata subresources",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/trends",
		Match:       RuleMatchPrefix,
		Description: "Mastodon v1 trends endpoints",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v2/trends",
		Match:       RuleMatchPrefix,
		Description: "Mastodon v2 trends endpoints",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/timelines/tag/",
		Match:       RuleMatchPrefix,
		Description: "public hashtag timelines",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/trust/attestations/",
		Match:       RuleMatchPrefix,
		Description: "lesser.host trust-proxy attestation reads",
	},
	{
		Methods:        []string{http.MethodGet, http.MethodHead},
		Path:           "/api/v1/statuses/",
		Match:          RuleMatchStatusRead,
		Description:    "public status reads; visibility remains handler/service enforced",
		ExceptSuffixes: []string{"/source", "/favourited_by", "/reblogged_by"},
	},
	{
		Methods:          []string{http.MethodGet, http.MethodHead},
		Path:             "/api/v1/accounts/",
		Match:            RuleMatchAccountContent,
		Description:      "public account statuses/notes reads",
		RequiredContains: []string{"/statuses", "/notes"},
	},
	{
		Methods: []string{http.MethodGet, http.MethodHead},
		Path:    "/api/v1/accounts/",
		Match:   RuleMatchSingleSegment,
		Description: "public account profile lookup by Mastodon id, username, or actor URL; " +
			"credential/relationship siblings remain private",
		ExceptExactPaths: []string{
			"/api/v1/accounts/lookup",
			"/api/v1/accounts/relationships",
			"/api/v1/accounts/search",
			"/api/v1/accounts/verify_credentials",
			"/api/v1/accounts/quote_permissions",
		},
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/accounts/lookup",
		Match:       RuleMatchExact,
		Description: "Mastodon account lookup by acct handle",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/accounts/search",
		Match:       RuleMatchPrefix,
		Description: "public account search and suggestions",
	},
	{
		Methods:          []string{http.MethodGet, http.MethodHead},
		Path:             "/api/v1/skills",
		Match:            RuleMatchSkills,
		Description:      "public skills catalog; exact resolver remains private",
		ExceptExactPaths: []string{"/api/v1/skills/resolve"},
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/search/statuses",
		Match:       RuleMatchPrefix,
		Description: "public status-search read path; route-level guard may still require OAuth",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/notes/",
		Match:       RuleMatchPrefix,
		Description: "public community-note reads",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        apiV1AppsPath,
		Match:       RuleMatchExact,
		Description: "OAuth app registration",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/oauth/register",
		Match:       RuleMatchExact,
		Description: "legacy OAuth app registration",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/api/v1/accounts",
		Match:       RuleMatchExact,
		Description: "account registration with wallet/WebAuthn proof",
	},
	{
		Methods:     []string{http.MethodGet, http.MethodHead},
		Path:        "/api/v1/agents",
		Match:       RuleMatchExact,
		Description: "public agent directory; private fields are handler-redacted for anonymous viewers",
	},
	{
		Methods: []string{http.MethodGet, http.MethodHead},
		Path:    "/api/v1/agents/",
		Match:   RuleMatchSingleSegment,
		Description: "public agent profile lookup; private fields are handler-redacted for anonymous viewers, " +
			"management subresources remain private",
		ExceptExactPaths: []string{
			"/api/v1/agents/memory",
		},
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/api/v1/agents/register/challenge",
		Match:       RuleMatchExact,
		Description: "self-sovereign agent registration challenge",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/api/v1/agents/register",
		Match:       RuleMatchExact,
		Description: "self-sovereign agent registration using signed challenge proof",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/api/v1/agents/auth/challenge",
		Match:       RuleMatchExact,
		Description: "self-sovereign agent auth challenge",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/api/v1/agents/auth/token",
		Match:       RuleMatchExact,
		Description: "self-sovereign agent token exchange using signed challenge proof",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/api/v1/agents/",
		Match:       RuleMatchAgentAccessLeaseProof,
		Description: "agent access-lease session and renewal proof exchanges; handlers enforce lease/challenge signatures",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/api/v1/notifications/deliver",
		Match:       RuleMatchExact,
		Description: "gate-reachable notification delivery; handler enforces internal instance key",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/oauth/token",
		Match:       RuleMatchExact,
		Description: "OAuth token exchange",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/oauth/revoke",
		Match:       RuleMatchExact,
		Description: "OAuth token revocation",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/oauth/consent",
		Match:       RuleMatchExact,
		Description: "OAuth consent submission",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/oauth/device/code",
		Match:       RuleMatchExact,
		Description: "OAuth device-code start",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/oauth/device/verify",
		Match:       RuleMatchExact,
		Description: "OAuth device verification",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/setup/bootstrap/challenge",
		Match:       RuleMatchExact,
		Description: "setup bootstrap challenge",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/setup/bootstrap/verify",
		Match:       RuleMatchExact,
		Description: "setup bootstrap verification",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/setup/admin",
		Match:       RuleMatchExact,
		Description: "gate-reachable setup admin creation; handler enforces setup bearer",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/setup/finalize",
		Match:       RuleMatchExact,
		Description: "gate-reachable setup finalization; handler enforces OAuth bearer/admin scope",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/api/v1/auth/webauthn/login/begin",
		Match:       RuleMatchExact,
		Description: "WebAuthn login begin",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/api/v1/auth/webauthn/login/finish",
		Match:       RuleMatchExact,
		Description: "WebAuthn login finish",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/auth/wallet/challenge",
		Match:       RuleMatchExact,
		Description: "wallet challenge",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/auth/wallet/verify",
		Match:       RuleMatchExact,
		Description: "wallet verification",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/auth/wallet/login",
		Match:       RuleMatchExact,
		Description: "wallet login",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/auth/wallet/link",
		Match:       RuleMatchExact,
		Description: "wallet link during registration flow",
	},
	{
		Methods:     []string{http.MethodPost},
		Path:        "/api/v1/search/statuses",
		Match:       RuleMatchExact,
		Description: "status-search write-compatible method; route-level guard may still require OAuth",
	},
}

var contractAuthRules = []ContractAuthRule{
	{
		Method:      http.MethodPost,
		Path:        "/setup/admin",
		Class:       ContractAuthSetupBearer,
		Description: "setup-session bearer token enforced in handler",
	},
	{
		Method:      http.MethodPost,
		Path:        "/setup/finalize",
		Class:       ContractAuthBearerRequired,
		Description: "OAuth bearer/admin setup finalization enforced in handler",
	},
	{
		Method:      http.MethodPost,
		Path:        "/api/v1/notifications/deliver",
		Class:       ContractAuthInternalOnly,
		Description: "internal instance-key bearer validation enforced in handler",
	},
}

// PublicRules returns a copy of Lesser's anonymous public-surface rules.
func PublicRules() []PublicRule {
	out := make([]PublicRule, len(publicRules))
	for i, rule := range publicRules {
		out[i] = clonePublicRule(rule)
	}
	return out
}

// ContractAuthRules returns a copy of Lesser's handler-enforced contract auth
// overrides.
func ContractAuthRules() []ContractAuthRule {
	out := make([]ContractAuthRule, len(contractAuthRules))
	copy(out, contractAuthRules)
	return out
}

// IsPublic reports whether the method/path pair is in Lesser's explicitly
// allowlisted anonymous API surface.
//
// The default is deny: method/path pairs missing from this allowlist are not
// public.
func IsPublic(method, path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}

	for _, rule := range publicRules {
		if rule.matches(method, path) {
			return true
		}
	}
	return false
}

// Classify resolves a method/path through publicsurface so tests and tools can
// prove every route is intentionally public, contract-auth, or auth-required.
func Classify(method, path string) Classification {
	if strings.TrimSpace(method) == "" || strings.TrimSpace(path) == "" {
		return Classification{Kind: ClassificationUnknown}
	}
	if class, ok := ContractAuth(method, path); ok {
		return Classification{
			Kind:              ClassificationContractAuth,
			Public:            true,
			ContractAuthClass: class,
		}
	}
	for _, rule := range publicRules {
		if rule.matches(method, path) {
			ruleCopy := clonePublicRule(rule)
			return Classification{Kind: ClassificationAnonymous, Public: true, Rule: &ruleCopy}
		}
	}
	return Classification{Kind: ClassificationAuthRequired}
}

// ContractAuth returns handler-enforced contract auth requirements for routes
// that remain gate-reachable through IsPublic but must not be advertised as
// anonymous in the generated OpenAPI contract.
//
// This is additive contract metadata only. It intentionally does not change
// IsPublic's gate decision.
func ContractAuth(method, path string) (ContractAuthClass, bool) {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = normalizeContractPath(path)

	for _, rule := range contractAuthRules {
		if rule.Method == method && rule.Path == path {
			return rule.Class, true
		}
	}
	return "", false
}

func clonePublicRule(rule PublicRule) PublicRule {
	rule.Methods = append([]string(nil), rule.Methods...)
	rule.ExceptExactPaths = append([]string(nil), rule.ExceptExactPaths...)
	rule.ExceptSuffixes = append([]string(nil), rule.ExceptSuffixes...)
	rule.RequiredContains = append([]string(nil), rule.RequiredContains...)
	return rule
}

func (rule PublicRule) matches(method, path string) bool {
	if !rule.matchesMethod(method) {
		return false
	}

	switch rule.Match {
	case RuleMatchExact:
		return path == rule.Path
	case RuleMatchPrefix:
		return strings.HasPrefix(path, rule.Path)
	case RuleMatchStatusRead:
		if !strings.HasPrefix(path, rule.Path) {
			return false
		}
		for _, suffix := range rule.ExceptSuffixes {
			if strings.HasSuffix(path, suffix) {
				return false
			}
		}
		return true
	case RuleMatchAccountContent:
		if !strings.HasPrefix(path, rule.Path) {
			return false
		}
		for _, required := range rule.RequiredContains {
			if strings.Contains(path, required) {
				return true
			}
		}
		return false
	case RuleMatchSingleSegment:
		if rule.excludes(path) {
			return false
		}
		return hasExactlyOneSegmentAfterPrefix(path, rule.Path)
	case RuleMatchAgentAccessLeaseProof:
		return matchesAgentAccessLeaseProofPath(path)
	case RuleMatchSkills:
		if path != rule.Path && !strings.HasPrefix(path, rule.Path+"/") {
			return false
		}
		for _, excluded := range rule.ExceptExactPaths {
			if path == excluded {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (rule PublicRule) excludes(path string) bool {
	for _, excluded := range rule.ExceptExactPaths {
		if path == excluded {
			return true
		}
	}
	return false
}

func (rule PublicRule) matchesMethod(method string) bool {
	for _, candidate := range rule.Methods {
		if method == candidate {
			return true
		}
	}
	return false
}

func normalizeContractPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	if len(path) > 1 {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

func hasExactlyOneSegmentAfterPrefix(path, prefix string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	rest := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	return rest != "" && !strings.Contains(rest, "/")
}

func matchesAgentAccessLeaseProofPath(path string) bool {
	const prefix = "/api/v1/agents/"
	if !strings.HasPrefix(path, prefix) {
		return false
	}

	segments := splitNonEmpty(strings.TrimPrefix(path, prefix))
	if len(segments) < 4 || segments[1] != "access-leases" {
		return false
	}

	switch {
	case len(segments) == 4 && segments[3] == "session-key":
		return true
	case len(segments) == 4 && segments[3] == "token":
		return true
	case len(segments) == 5 && segments[3] == "session-key" && segments[4] == "challenge":
		return true
	case len(segments) == 5 && segments[3] == "renew" && segments[4] == "challenge":
		return true
	default:
		return false
	}
}

func splitNonEmpty(path string) []string {
	raw := strings.Split(path, "/")
	out := make([]string, 0, len(raw))
	for _, segment := range raw {
		segment = strings.TrimSpace(segment)
		if segment != "" {
			out = append(out, segment)
		}
	}
	return out
}
