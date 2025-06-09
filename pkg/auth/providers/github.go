package providers

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/aron23/lesser/pkg/common"
)

// GitHubProvider implements OAuth for GitHub
type GitHubProvider struct {
	baseProvider
}

// NewGitHubProvider creates a new GitHub OAuth provider
func NewGitHubProvider(clientID, clientSecret string) Provider {
	return &GitHubProvider{
		baseProvider: baseProvider{
			name: "github",
			config: Config{
				ClientID:     clientID,
				ClientSecret: clientSecret,
				AuthURL:      "https://github.com/login/oauth/authorize",
				TokenURL:     "https://github.com/login/oauth/access_token",
				UserInfoURL:  "https://api.github.com/user",
				Scopes:       []string{"read:user", "user:email"},
			},
			client: http.DefaultClient,
		},
	}
}

// GetUserInfo fetches user information from GitHub
func (p *GitHubProvider) GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	// Get basic user info
	userData, err := p.makeAPIRequest(ctx, "GET", p.config.UserInfoURL, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	var githubUser struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}

	if err := common.ParseHTTPResponse(bytes.NewReader(userData), &githubUser); err != nil {
		return nil, fmt.Errorf("failed to parse user info: %w", err)
	}

	// If email is not public, fetch from emails endpoint
	if githubUser.Email == "" {
		emailData, err := p.makeAPIRequest(ctx, "GET", "https://api.github.com/user/emails", accessToken)
		if err == nil {
			var emails []struct {
				Email    string `json:"email"`
				Primary  bool   `json:"primary"`
				Verified bool   `json:"verified"`
			}

			if err := common.ParseHTTPResponse(bytes.NewReader(emailData), &emails); err == nil {
				// Find primary verified email
				for _, email := range emails {
					if email.Primary && email.Verified {
						githubUser.Email = email.Email
						break
					}
				}
			}
		}
	}

	// Use login as name if name is empty
	if githubUser.Name == "" {
		githubUser.Name = githubUser.Login
	}

	return &UserInfo{
		ID:         strconv.FormatInt(githubUser.ID, 10),
		Username:   githubUser.Login,
		Email:      githubUser.Email,
		Name:       githubUser.Name,
		AvatarURL:  githubUser.AvatarURL,
		Provider:   "github",
		ProviderID: strconv.FormatInt(githubUser.ID, 10),
	}, nil
}
