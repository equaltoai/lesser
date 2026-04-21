// Package surface defines the deploy-time ActivityPub federation surface contract.
package surface

import (
	"net/http"
	"strings"
)

// MethodPolicy describes how a served federation endpoint handles an HTTP method.
type MethodPolicy struct {
	Method     string
	Served     bool
	StatusCode int
}

// EndpointManifest describes the deploy-time contract for one federation endpoint.
type EndpointManifest struct {
	Path       string
	Advertised bool
	Policies   []MethodPolicy
}

// FederationManifest is the source of truth for deploy-time federation surface support.
type FederationManifest struct {
	SharedInbox EndpointManifest
}

var manifest = FederationManifest{
	SharedInbox: EndpointManifest{
		Path:       "/inbox",
		Advertised: true,
		Policies: []MethodPolicy{
			{Method: http.MethodPost, Served: true},
			{Method: http.MethodGet, Served: true, StatusCode: http.StatusMethodNotAllowed},
		},
	},
}

// Current returns the active deploy-time federation surface manifest.
func Current() FederationManifest {
	return manifest
}

// SharedInbox returns the shared inbox endpoint contract.
func SharedInbox() EndpointManifest {
	return Current().SharedInbox
}

// ServedMethods returns the ordered list of explicitly served methods for the endpoint.
func (e EndpointManifest) ServedMethods() []string {
	methods := make([]string, 0, len(e.Policies))
	seen := make(map[string]struct{}, len(e.Policies))
	for _, policy := range e.Policies {
		if !policy.Served {
			continue
		}
		if _, ok := seen[policy.Method]; ok {
			continue
		}
		seen[policy.Method] = struct{}{}
		methods = append(methods, policy.Method)
	}
	return methods
}

// ServesMethod reports whether the endpoint is explicitly served for the given method.
func (e EndpointManifest) ServesMethod(method string) bool {
	_, ok := e.policy(method)
	return ok
}

// AllowsMethod reports whether the method is served as an allowed success path.
func (e EndpointManifest) AllowsMethod(method string) bool {
	policy, ok := e.policy(method)
	return ok && policy.StatusCode == 0
}

// MethodStatus returns the explicit served status for a method when one is defined.
func (e EndpointManifest) MethodStatus(method string) (int, bool) {
	policy, ok := e.policy(method)
	if !ok || policy.StatusCode == 0 {
		return 0, false
	}
	return policy.StatusCode, true
}

func (e EndpointManifest) policy(method string) (MethodPolicy, bool) {
	normalized := strings.ToUpper(strings.TrimSpace(method))
	for _, policy := range e.Policies {
		if policy.Served && policy.Method == normalized {
			return policy, true
		}
	}
	return MethodPolicy{}, false
}

// SharedInboxURL returns the canonical shared inbox URL when the surface advertises it.
func SharedInboxURL(baseURL string) string {
	endpoint := SharedInbox()
	if !endpoint.Advertised {
		return ""
	}
	base := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return ""
	}
	return base + endpoint.Path
}
