package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/assert"
)

// TestCSR049_CanViewCMSPrivateAttribution_UnauthenticatedRejected verifies
// that unauthenticated viewers cannot see CMS workflow attribution actors
// (generatedBy, reviewedBy, publishedBy). These are private workflow
// metadata distinct from the public Author (attributedTo) byline.
func TestCSR049_CanViewCMSPrivateAttribution_UnauthenticatedRejected(t *testing.T) {
	r := &Resolver{}

	// No auth claims in context — simulates public (unauthenticated) viewer.
	ctx := context.Background()
	ok := r.canViewCMSPrivateAttribution(ctx, "https://lesser.example/users/alice")
	assert.False(t, ok, "public viewers must not see CMS private attribution")
}

// TestCSR049_CanViewCMSPrivateAttribution_NilClaimsRejected verifies that
// nil auth claims (no authentication) result in denied access.
func TestCSR049_CanViewCMSPrivateAttribution_NilClaimsRejected(t *testing.T) {
	r := &Resolver{}

	// Claims stored in context but nil.
	ctx := context.WithValue(context.Background(), common.ContextKeyClaims, (*auth.Claims)(nil))
	ok := r.canViewCMSPrivateAttribution(ctx, "https://lesser.example/users/alice")
	assert.False(t, ok, "nil claims must be treated as unauthenticated")
}

// TestCSR049_CanViewCMSPrivateAttribution_EmptyAttributedToRejected verifies
// that an empty attributedTo always returns false (defense in depth).
func TestCSR049_CanViewCMSPrivateAttribution_EmptyAttributedToRejected(t *testing.T) {
	r := &Resolver{}

	ctx := context.WithValue(context.Background(), common.ContextKeyClaims, &auth.Claims{
		Username: "alice",
	})
	ok := r.canViewCMSPrivateAttribution(ctx, "")
	assert.False(t, ok, "empty attributedTo must not grant access")
}

// TestCSR049_ResolveCMSPrivateAttributionActor_EmptyActorIDReturnsNil verifies
// that an empty or whitespace-only actor ID returns nil without any lookup.
func TestCSR049_ResolveCMSPrivateAttributionActor_EmptyActorIDReturnsNil(t *testing.T) {
	r := &Resolver{}

	tests := []struct {
		name    string
		actorID string
	}{
		{"empty string", ""},
		{"whitespace only", "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.resolveCMSPrivateAttributionActor(
				context.Background(),
				"https://lesser.example/users/alice",
				tt.actorID,
			)
			assert.Nil(t, result, "empty actor ID must return nil")
		})
	}
}

// TestCSR049_ResolveCMSPrivateAttributionActor_UnauthenticatedReturnsNil
// verifies that public (unauthenticated) viewers always get nil for
// CMS attribution actors regardless of the stored actor ID.
func TestCSR049_ResolveCMSPrivateAttributionActor_UnauthenticatedReturnsNil(t *testing.T) {
	r := &Resolver{}

	ctx := context.Background() // no auth claims

	result := r.resolveCMSPrivateAttributionActor(
		ctx,
		"https://lesser.example/users/alice",
		"https://lesser.example/users/editor-bot",
	)
	assert.Nil(t, result,
		"CSR-049: unauthenticated viewers must not see CMS attribution actors")
}
