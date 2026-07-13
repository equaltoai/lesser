package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormmocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

type rateLimitSelectiveErrRepo struct {
	*rateLimitRepoStub
	countErrByID map[string]error
	limitErrByID map[string]error
}

func (r *rateLimitSelectiveErrRepo) IsRateLimited(_ context.Context, identifier string) (bool, time.Time, error) {
	if err := r.limitErrByID[identifier]; err != nil {
		return false, time.Time{}, err
	}
	if r.rateLimitRepoStub == nil {
		return false, time.Time{}, nil
	}
	return r.rateLimitRepoStub.IsRateLimited(context.Background(), identifier)
}

func (r *rateLimitSelectiveErrRepo) GetLoginAttemptCount(_ context.Context, identifier string, _ time.Time) (int, error) {
	if err := r.countErrByID[identifier]; err != nil {
		return 0, err
	}
	if r.rateLimitRepoStub == nil {
		return 0, nil
	}
	return r.rateLimitRepoStub.GetLoginAttemptCount(context.Background(), identifier, time.Time{})
}

func TestScopePolicy_EdgeCases(t *testing.T) {
	t.Parallel()

	scopes := CanonicalOAuthScopes()
	scopes[0] = "mutated"
	require.Equal(t, []string{ScopeRead, ScopeWrite, ScopeFollow, ScopePush}, CanonicalOAuthScopes())

	require.NoError(t, ValidatePublicOAuthScopes(nil))
	require.NoError(t, ValidatePublicOAuthScopes([]string{" READ "}))
	require.False(t, ScopeGrantAllows("", ScopeRead))
	require.False(t, ScopeGrantAllows(ScopeRead, ""))
	require.True(t, ScopeGrantAllows(ScopeAdmin, "admin:write"))
	require.True(t, ScopeSetAllows([]string{ScopeRead}, []string{"", "read:notifications"}))

	require.True(t, isRecognizedOAuthScope(" PUSH "))
	require.True(t, isRecognizedOAuthScope("admin:write"))
	require.False(t, isRecognizedOAuthScope(""))
	require.False(t, isRecognizedOAuthScope("custom:scope"))
}

func TestMCPAccessHelper_EdgeCases(t *testing.T) {
	t.Parallel()

	bundle := BuildPublicMCPAccessBundle(" https://example.com/ ", "   ")
	require.Empty(t, bundle.MCPURL)
	require.Empty(t, bundle.ProtectedResourceURL)
	require.Len(t, bundle.Guidance, 5)

	require.Equal(t, "https://api.example.com:8443", canonicalMCPResourceBaseURL("https://example.com:8443/"))
	require.Equal(t, "not a url", canonicalMCPResourceBaseURL("not a url"))
	require.True(t, isLocalMCPHostname("127.0.0.1"))
	require.True(t, isLocalMCPHostname("service.localhost"))
	require.True(t, isLocalMCPHostname("::1"))
	require.False(t, isLocalMCPHostname(""))
}

func TestOAuthService_HelperEarlyReturns(t *testing.T) {
	t.Parallel()

	require.Nil(t, getOAuthAccountRepo(nil))
	require.Nil(t, getOAuthAccountRepo(reposStub{}))

	var nilService *OAuthService
	require.NoError(t, nilService.validateAccessTokenNotRevoked(&Claims{
		RegisteredClaims: jwt.RegisteredClaims{ID: "jti"},
	}))

	svc := &OAuthService{}
	require.NoError(t, svc.validateAccessTokenNotRevoked(nil))
	require.NoError(t, svc.validateAccessTokenNotRevoked(&Claims{}))

	svc = &OAuthService{repos: reposStub{}}
	require.NoError(t, svc.validateAccessTokenNotRevoked(&Claims{
		RegisteredClaims: jwt.RegisteredClaims{ID: "jti"},
	}))
}

func TestOAuthService_RevocationLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()

	jti := "jti-read-error"
	mockDB := new(dynamormmocks.MockDB)
	mockQuery := new(dynamormmocks.MockQuery)
	accountRepo := repositories.NewAccountRepository(mockDB, "test-table", "example.com", zap.NewNop())
	svc := &OAuthService{repos: reposWithAccount{account: accountRepo}}

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.MatchedBy(func(model *models.RevokedAccessToken) bool {
		return model != nil
	})).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "REVOKEDTOKEN#"+jti).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", repositories.SKToken).Return(mockQuery)
	mockQuery.On("ConsistentRead").Return(mockQuery)
	mockQuery.On("First", mock.MatchedBy(func(dest *models.RevokedAccessToken) bool {
		return dest != nil
	})).Return(errors.New("read failed"))

	err := svc.validateAccessTokenNotRevoked(&Claims{
		RegisteredClaims: jwt.RegisteredClaims{ID: jti},
	})
	require.ErrorIs(t, err, ErrInvalidToken)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestRateLimiter_ErrorBranches(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(reposStub{})
	require.Nil(t, rl.accountRepo)

	ctx := context.Background()
	baseStub := &rateLimitRepoStub{
		limited: map[string]bool{},
		until:   map[string]time.Time{},
		counts:  map[string]int{},
	}
	stub := &rateLimitSelectiveErrRepo{
		rateLimitRepoStub: baseStub,
		countErrByID:      map[string]error{},
		limitErrByID:      map[string]error{},
	}
	rl = &RateLimiter{accountRepo: stub}

	stub.limitErrByID[RateLimitTypeIP+":192.0.2.1"] = errors.New("limit failed")
	err := rl.CheckRateLimit(ctx, "", "192.0.2.1")
	require.ErrorIs(t, err, ErrIPRateLimitCheck)

	delete(stub.limitErrByID, RateLimitTypeIP+":192.0.2.1")
	stub.limitErrByID[RateLimitTypeAccount+":alice"] = errors.New("account limit failed")
	err = rl.CheckRateLimit(ctx, "alice", "192.0.2.1")
	require.ErrorIs(t, err, ErrAccountRateLimitCheck)
	delete(stub.limitErrByID, RateLimitTypeAccount+":alice")

	stub.countErrByID[RateLimitTypeIP+":192.0.2.1"] = errors.New("ip count failed")
	err = rl.RecordAttempt(ctx, "", "192.0.2.1", false)
	require.ErrorIs(t, err, ErrGetIPAttemptCount)
	delete(stub.countErrByID, RateLimitTypeIP+":192.0.2.1")

	baseStub.counts[RateLimitTypeIP+":192.0.2.1"] = 0
	baseStub.counts[RateLimitTypeAccount+":alice"] = 0
	stub.countErrByID[RateLimitTypeAccount+":alice"] = errors.New("account count failed")
	err = rl.RecordAttempt(ctx, "alice", "192.0.2.1", false)
	require.ErrorIs(t, err, ErrGetAccountAttemptCount)
	delete(stub.countErrByID, RateLimitTypeAccount+":alice")

	baseStub.clearErr = errors.New("clear failed")
	require.Error(t, rl.ClearAccountLockout(ctx, "alice"))

	baseStub.clearErr = nil
	stub.limitErrByID[RateLimitTypeAccount+":alice"] = errors.New("status failed")
	_, err = rl.GetAccountStatus(ctx, "alice")
	require.Error(t, err)
}
