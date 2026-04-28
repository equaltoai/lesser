package common

import (
	"testing"
)

func TestIsLocalActorID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		actorID     string
		localDomain string
		want        bool
	}{
		{"exact match", "https://example.com/users/alice", "example.com", true},
		{"case insensitive", "https://EXAMPLE.COM/users/alice", "example.com", true},
		{"different domain", "https://remote.example/users/alice", "example.com", false},
		{"substring trap", "https://evil-example.com/users/alice", "example.com", false},
		{"subdomain", "https://social.example.com/users/alice", "example.com", false},
		{"empty actorID", "", "example.com", false},
		{"empty domain", "https://example.com/users/alice", "", false},
		{"both empty", "", "", false},
		{"bare username", "alice", "example.com", false},
		{"handle format", "alice@example.com", "example.com", false},
		{"trailing slash", "https://example.com/users/alice/", "example.com", true},
		{"with port", "https://example.com:443/users/alice", "example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsLocalActorID(tt.actorID, tt.localDomain)
			if got != tt.want {
				t.Errorf("IsLocalActorID(%q, %q) = %v, want %v", tt.actorID, tt.localDomain, got, tt.want)
			}
		})
	}
}

func TestExtractDomainFromActorID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		actorID string
		want    string
	}{
		{"standard URL", "https://example.com/users/alice", "example.com"},
		{"http URL", "http://example.com/users/alice", "example.com"},
		{"mixed case", "https://Example.COM/users/alice", "example.com"},
		{"with path", "https://example.com/users/alice/inbox", "example.com"},
		{"trailing slash", "https://example.com/users/alice/", "example.com"},
		{"subdomain", "https://social.example.com/users/alice", "social.example.com"},
		{"empty string", "", ""},
		{"bare username", "alice", ""},
		{"handle", "alice@example.com", ""},
		{"not a URL", "/users/alice", ""},
		{"ftp scheme", "ftp://example.com/users/alice", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractDomainFromActorID(tt.actorID)
			if got != tt.want {
				t.Errorf("ExtractDomainFromActorID(%q) = %q, want %q", tt.actorID, got, tt.want)
			}
		})
	}
}

func TestValidateActorDomainConsistency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		fetchURL   string
		declaredID string
		wantErr    bool
	}{
		{"matching domains", "https://example.com/users/alice", "https://example.com/users/alice", false},
		{"different schemes", "http://example.com/users/alice", "https://example.com/users/alice", false},
		{"different domains", "https://evil.example/users/bob", "https://victim.example/users/bob", true},
		{"invalid fetch URL", "/not-a-url", "https://example.com/users/alice", true},
		{"invalid declared ID", "https://example.com/users/alice", "/not-a-url", true},
		{"both invalid", "/not-a-url", "/not-a-url", true},
		{"case insensitive", "https://Example.COM/users/alice", "https://example.COM/users/alice", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateActorDomainConsistency(tt.fetchURL, tt.declaredID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateActorDomainConsistency(%q, %q) error = %v, wantErr = %v", tt.fetchURL, tt.declaredID, err, tt.wantErr)
			}
		})
	}
}
