package handlers

import (
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHandler_handleAuthServiceError_CoversBranches(t *testing.T) {
	h := &Handler{
		cfg:    &config.Config{Domain: "example.com"},
		logger: zap.NewNop(),
	}

	ctx, err := round10NewLiftContext("GET", "/api/v1/auth/error", nil, nil, nil)
	require.NoError(t, err)

	resp, err := h.handleAuthServiceError(ctx, nil, "noop")
	require.NoError(t, err)
	require.Nil(t, resp)

	cases := []struct {
		name       string
		inputErr   error
		wantStatus int
	}{
		{"webauthn_not_configured", auth.ErrWebAuthnNotConfigured, http.StatusInternalServerError},
		{"challenge_not_found", auth.ErrChallengeNotFound, http.StatusBadRequest},
		{"user_has_no_credentials", auth.ErrUserHasNoCredentials, http.StatusBadRequest},
		{"invalid_credential", auth.ErrInvalidCredential, http.StatusUnauthorized},
		{"invalid_credentials", auth.ErrInvalidCredentials, http.StatusUnauthorized},
		{"user_not_found", auth.ErrUserNotFound, http.StatusBadRequest},
		{"user_suspended", auth.ErrUserSuspended, http.StatusForbidden},
		{"user_not_approved", auth.ErrUserNotApproved, http.StatusForbidden},
		{"app_error_unauthorized", apperrors.Unauthorized("auth required"), http.StatusUnauthorized},
		{"app_error_insufficient_scope", apperrors.InsufficientScope("write"), http.StatusForbidden},
		{"default", assertError("some other error"), http.StatusInternalServerError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := round10NewLiftContext("GET", "/api/v1/auth/error", nil, nil, nil)
			require.NoError(t, err)

			requireStatus(t, tc.wantStatus)(h.handleAuthServiceError(ctx, tc.inputErr, "test-operation"))
		})
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }
