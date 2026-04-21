package surface

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/equaltoai/lesser/pkg/activitypub"
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

// ApplyLocalActorIdentifiers sets canonical local identifiers derived from the manifest.
func ApplyLocalActorIdentifiers(actor *activitypub.Actor, baseURL, username string) {
	if actor == nil {
		return
	}

	base := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	canonicalUsername := strings.TrimSpace(username)
	if base == "" || canonicalUsername == "" {
		return
	}

	actor.ID = fmt.Sprintf("%s/users/%s", base, canonicalUsername)
	actor.URL = fmt.Sprintf("%s/@%s", base, canonicalUsername)
	actor.Inbox = fmt.Sprintf("%s/users/%s/inbox", base, canonicalUsername)
	actor.Outbox = fmt.Sprintf("%s/users/%s/outbox", base, canonicalUsername)
	actor.Followers = fmt.Sprintf("%s/users/%s/followers", base, canonicalUsername)
	actor.Following = fmt.Sprintf("%s/users/%s/following", base, canonicalUsername)
	actor.Liked = fmt.Sprintf("%s/users/%s/liked", base, canonicalUsername)

	if actor.Endpoints == nil {
		actor.Endpoints = &activitypub.Endpoints{}
	}
	actor.Endpoints.SharedInbox = SharedInboxURL(base)

	actor.PreferredUsername = canonicalUsername

	if actor.PublicKey != nil {
		actor.PublicKey.Owner = actor.ID
		actor.PublicKey.ID = actor.ID + "#main-key"
	}
}
