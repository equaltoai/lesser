package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	storageinterfaces "github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	tablemocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type graphQLAuthTestRepos struct {
	account *repositories.AccountRepository
}

func (r graphQLAuthTestRepos) Account() *repositories.AccountRepository {
	return r.account
}

func (graphQLAuthTestRepos) Actor() storageinterfaces.ActorRepository {
	return nil
}

func (graphQLAuthTestRepos) Activity() storageinterfaces.ActivityRepository {
	return nil
}

func (graphQLAuthTestRepos) Notification() storageinterfaces.NotificationRepository {
	return nil
}

func (graphQLAuthTestRepos) Recovery() *repositories.RecoveryRepository {
	return nil
}

func (graphQLAuthTestRepos) Audit() *repositories.AuditRepository {
	return nil
}

func TestGraphQLAuthMiddlewareRejectsRevokedSessionTokens(t *testing.T) {
	const secret = "a-very-strong-jwt-key-without-weak-patterns-9876543210"

	db := new(tablemocks.MockDB)
	query := new(tablemocks.MockQuery)
	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(query)
	query.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(query)
	query.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*storagemodels.Session)
		*dest = storagemodels.Session{
			SessionID:  "sid-revoked",
			UserID:     "USER#alice",
			CreatedAt:  time.Now().Add(-2 * time.Hour),
			LastUsedAt: time.Now().Add(-30 * time.Minute),
			UpdatedAt:  time.Now().Add(-30 * time.Minute),
			ExpiresAt:  time.Now().Add(time.Hour).Unix(),
			IsRevoked:  true,
		}
	})

	repos := graphQLAuthTestRepos{
		account: repositories.NewAccountRepository(db, "test-table", "example.com", zap.NewNop()),
	}
	cfg := &config.Config{JWTSecret: secret, Domain: "example.com"}
	authService, err := auth.NewAuthService(cfg, repos)
	require.NoError(t, err)
	oauthService := auth.NewOAuthService(secret, cfg, repos, nil)

	now := time.Now()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "alice",
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		Username:  "alice",
		ClientID:  "web",
		Scopes:    []string{auth.ScopeRead},
		SessionID: "sid-revoked",
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	require.NoError(t, err)

	ctx := &apptheory.Context{
		Request: apptheory.Request{
			Method: http.MethodPost,
			Path:   "/graphql",
			Headers: map[string][]string{
				"authorization": {"Bearer " + token},
			},
		},
	}

	mw := createAuthMiddlewareWithServices(authService, oauthService, zap.NewNop())
	resp, err := mw(func(c *apptheory.Context) (*apptheory.Response, error) {
		require.Nil(t, c.AuthPrincipal)
		require.False(t, auth.IsAuthenticated(c))
		return &apptheory.Response{Status: http.StatusOK}, nil
	})(ctx)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.Status)
}

var _ auth.StorageProvider = graphQLAuthTestRepos{}
