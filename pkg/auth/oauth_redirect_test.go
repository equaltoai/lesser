package auth

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestOAuthServiceValidateRedirectURILoopbackPorts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		registered   string
		presented    string
		clientClass  string
		confidential bool
		wantErr      bool
	}{
		{
			name:        "ipv4 loopback accepts a different port",
			registered:  "http://127.0.0.1:37371/callback",
			presented:   "http://127.0.0.1:3118/callback",
			clientClass: ClientClassCLI,
		},
		{
			name:        "ipv4 loopback keeps the path exact",
			registered:  "http://127.0.0.1:37371/callback",
			presented:   "http://127.0.0.1:3118/other-path",
			clientClass: ClientClassCLI,
			wantErr:     true,
		},
		{
			name:        "localhost accepts a different port",
			registered:  "http://localhost:37371/callback",
			presented:   "http://localhost:3118/callback",
			clientClass: ClientClassCLI,
		},
		{
			name:        "localhost does not match an ipv4 literal",
			registered:  "http://localhost:37371/callback",
			presented:   "http://127.0.0.1:3118/callback",
			clientClass: ClientClassCLI,
			wantErr:     true,
		},
		{
			name:        "ipv4 loopback addresses are not wildcarded",
			registered:  "http://127.0.0.1:37371/callback",
			presented:   "http://127.0.0.2:3118/callback",
			clientClass: ClientClassCLI,
			wantErr:     true,
		},
		{
			name:        "alternate ipv4 loopback accepts a different port",
			registered:  "http://127.0.0.2:37371/callback",
			presented:   "http://127.0.0.2:3118/callback",
			clientClass: ClientClassCLI,
		},
		{
			name:        "ipv6 loopback accepts a different port",
			registered:  "http://[::1]:37371/callback",
			presented:   "http://[::1]:3118/callback",
			clientClass: ClientClassCLI,
		},
		{
			name:        "ipv6 loopback accepts an omitted registered port",
			registered:  "http://[::1]/callback",
			presented:   "http://[::1]:3118/callback",
			clientClass: ClientClassCLI,
		},
		{
			name:        "ipv4 and ipv6 literals are not interchangeable",
			registered:  "http://127.0.0.1:37371/callback",
			presented:   "http://[::1]:3118/callback",
			clientClass: ClientClassCLI,
			wantErr:     true,
		},
		{
			name:        "ipv6 loopback keeps the path exact",
			registered:  "http://[::1]:37371/callback",
			presented:   "http://[::1]:3118/other-path",
			clientClass: ClientClassCLI,
			wantErr:     true,
		},
		{
			name:        "loopback keeps the scheme exact",
			registered:  "http://127.0.0.1:37371/callback",
			presented:   "https://127.0.0.1:3118/callback",
			clientClass: ClientClassCLI,
			wantErr:     true,
		},
		{
			name:        "https loopback keeps exact port matching",
			registered:  "https://127.0.0.1:37371/callback",
			presented:   "https://127.0.0.1:3118/callback",
			clientClass: ClientClassCLI,
			wantErr:     true,
		},
		{
			name:        "loopback keeps the query exact",
			registered:  "http://127.0.0.1:37371/callback?source=registered",
			presented:   "http://127.0.0.1:3118/callback?source=presented",
			clientClass: ClientClassCLI,
			wantErr:     true,
		},
		{
			name:        "non-loopback port mismatch is rejected",
			registered:  "https://example.com:37371/callback",
			presented:   "https://example.com:3118/callback",
			clientClass: ClientClassCLI,
			wantErr:     true,
		},
		{
			name:        "non-loopback host mismatch is rejected",
			registered:  "https://example.com/callback",
			presented:   "https://other.example/callback",
			clientClass: ClientClassCLI,
			wantErr:     true,
		},
		{
			name:        "public web client keeps exact matching",
			registered:  "http://127.0.0.1:37371/callback",
			presented:   "http://127.0.0.1:3118/callback",
			clientClass: ClientClassWeb,
			wantErr:     true,
		},
		{
			name:         "confidential cli client keeps exact matching",
			registered:   "http://127.0.0.1:37371/callback",
			presented:    "http://127.0.0.1:3118/callback",
			clientClass:  ClientClassCLI,
			confidential: true,
			wantErr:      true,
		},
		{
			name:        "non-loopback exact match remains accepted",
			registered:  "https://example.com/callback",
			presented:   "https://example.com/callback",
			clientClass: ClientClassWeb,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			const clientID = "client-1"
			repo := &oauthAccountRepoStub{
				clients: map[string]*storage.OAuthClient{
					clientID: {
						ClientID:     clientID,
						RedirectURIs: []string{tt.registered},
						ClientClass:  tt.clientClass,
						Confidential: tt.confidential,
					},
				},
			}
			svc := &OAuthService{accountRepo: repo}

			err := svc.ValidateRedirectURI(context.Background(), clientID, tt.presented)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrInvalidRequest)
				return
			}
			require.NoError(t, err)
		})
	}
}
