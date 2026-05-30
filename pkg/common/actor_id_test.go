package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalActorID(t *testing.T) {
	tests := []struct {
		name    string
		actorID string
		want    string
		wantErr bool
	}{
		{
			name:    "lowercases scheme and host",
			actorID: "HTTPS://REMOTE.EXAMPLE/users/alice",
			want:    "https://remote.example/users/alice",
		},
		{
			name:    "trims trailing slash",
			actorID: "https://remote.example/users/alice/",
			want:    "https://remote.example/users/alice",
		},
		{
			name:    "preserves path case",
			actorID: "https://remote.example/users/Alice",
			want:    "https://remote.example/users/Alice",
		},
		{
			name:    "rejects handle",
			actorID: "alice@remote.example",
			wantErr: true,
		},
		{
			name:    "rejects query",
			actorID: "https://remote.example/users/alice?next=https://evil.example",
			wantErr: true,
		},
		{
			name:    "rejects fragment",
			actorID: "https://remote.example/users/alice#main-key",
			wantErr: true,
		},
		{
			name:    "rejects userinfo",
			actorID: "https://alice@remote.example/users/alice",
			wantErr: true,
		},
		{
			name:    "rejects unsafe segment",
			actorID: "https://remote.example/users/../admin",
			wantErr: true,
		},
		{
			name:    "rejects control rune",
			actorID: "https://remote.example/users/alice\nHost: evil.example",
			wantErr: true,
		},
		{
			name:    "rejects overlong url",
			actorID: "https://remote.example/users/" + strings.Repeat("a", 2001),
			wantErr: true,
		},
		{
			name:    "rejects generic users path",
			actorID: "https://remote.example/users",
			wantErr: true,
		},
		{
			name:    "rejects generic at path",
			actorID: "https://remote.example/@",
			wantErr: true,
		},
		{
			name:    "rejects multi segment users actor path",
			actorID: "https://remote.example/users/alice/mallory",
			wantErr: true,
		},
		{
			name:    "rejects multi segment at actor path",
			actorID: "https://remote.example/@alice/mallory",
			wantErr: true,
		},
		{
			name:    "rejects invalid users actor username",
			actorID: "https://remote.example/users/-alice",
			wantErr: true,
		},
		{
			name:    "rejects root path",
			actorID: "https://remote.example/",
			wantErr: true,
		},
		{
			name:    "rejects empty",
			actorID: "",
			wantErr: true,
		},
		{
			name:    "rejects bad escaping",
			actorID: "https://remote.example/users/%zz",
			wantErr: true,
		},
		{
			name:    "rejects encoded unsafe segment",
			actorID: "https://remote.example/users/%2e%2e/admin",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CanonicalActorID(tt.actorID)
			if tt.wantErr {
				require.Error(t, err)
				require.Empty(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSameCanonicalActorID(t *testing.T) {
	require.True(t, SameCanonicalActorID(
		"HTTPS://REMOTE.EXAMPLE/users/alice/",
		"https://remote.example/users/alice",
	))
	require.False(t, SameCanonicalActorID(
		"https://remote.example/users/alice",
		"https://remote.example/users/alice/anything",
	))
	require.False(t, SameCanonicalActorID(
		"https://remote.example/users/Alice",
		"https://remote.example/users/alice",
	))
	require.False(t, SameCanonicalActorID(
		"alice@remote.example",
		"https://remote.example/users/alice",
	))
}

func TestActorUsernameFromID(t *testing.T) {
	tests := []struct {
		name    string
		actorID string
		want    string
		wantErr bool
	}{
		{
			name:    "users URL",
			actorID: "https://example.com/users/alice",
			want:    "alice",
		},
		{
			name:    "users URL with trailing slash",
			actorID: "https://example.com/users/alice/",
			want:    "alice",
		},
		{
			name:    "at URL",
			actorID: "https://example.com/@bob",
			want:    "bob",
		},
		{
			name:    "plain username",
			actorID: "carol",
			want:    "carol",
		},
		{
			name:    "handle username",
			actorID: "@dave",
			want:    "dave",
		},
		{
			name:    "rejects crafted users path",
			actorID: "https://example.com/users/admin/mallory",
			wantErr: true,
		},
		{
			name:    "rejects crafted at path",
			actorID: "https://example.com/@admin/mallory",
			wantErr: true,
		},
		{
			name:    "rejects unsupported actor path",
			actorID: "https://example.com/api/v1/actors/alice",
			wantErr: true,
		},
		{
			name:    "rejects invalid username",
			actorID: "https://example.com/users/-alice",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ActorUsernameFromID(tt.actorID)
			if tt.wantErr {
				require.Error(t, err)
				require.Empty(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCachedRemoteActorIDMatchesLookup(t *testing.T) {
	tests := []struct {
		name          string
		cachedActorID string
		lookup        string
		want          bool
	}{
		{
			name:          "exact canonical url matches",
			cachedActorID: "https://remote.example/users/victim",
			lookup:        "HTTPS://REMOTE.EXAMPLE/users/victim/",
			want:          true,
		},
		{
			name:          "crafted trailing path does not match",
			cachedActorID: "https://remote.example/users/victim",
			lookup:        "https://remote.example/users/victim/anything",
			want:          false,
		},
		{
			name:          "different actor does not match",
			cachedActorID: "https://remote.example/users/victim",
			lookup:        "https://remote.example/users/other",
			want:          false,
		},
		{
			name:          "invalid cached actor cannot satisfy canonical lookup",
			cachedActorID: "victim@remote.example",
			lookup:        "https://remote.example/users/victim",
			want:          false,
		},
		{
			name:          "url lookup with query cannot satisfy cached actor",
			cachedActorID: "https://remote.example/users/victim",
			lookup:        "https://remote.example/users/victim?spoof=https://evil.example/users/root",
			want:          false,
		},
		{
			name:          "handle lookup remains allowed",
			cachedActorID: "https://remote.example/users/victim",
			lookup:        "victim@remote.example",
			want:          true,
		},
		{
			name:          "blank lookup remains allowed for non-url cache consumers",
			cachedActorID: "https://remote.example/users/victim",
			lookup:        "",
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, CachedRemoteActorIDMatchesLookup(tt.cachedActorID, tt.lookup))
		})
	}
}
