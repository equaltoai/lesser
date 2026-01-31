package main

import (
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/ratelimit"
	"github.com/pay-theory/lift/pkg/lift"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func configureAuthRoutesAppTheory(app *apptheory.App) {
	baseMiddleware := []lift.Middleware{
		createLoggingMiddleware(logger),
		common.CreateAPIErrorMiddleware(logger),
	}

	// OAuth app registration (public, no auth required)
	app.Post("/api/v1/apps", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAppRegistrationLift),
		baseMiddleware,
	))

	// Wallet authentication endpoints (public, for passwordless login)
	app.Post("/auth/wallet/challenge", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleCreateChallengeLift),
			20, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/auth/wallet/verify", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleVerifySignatureLift),
			20, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/auth/wallet/login", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleLoginWalletLift),
			20, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/auth/wallet/link", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleLinkWalletLift),
		baseMiddleware,
	))
	app.Delete("/auth/wallet/unlink/{address}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleUnlinkWalletLift),
		baseMiddleware,
	))
	app.Get("/auth/wallet/list", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetWalletsLift),
		baseMiddleware,
	))

	// WebAuthn (passkey) endpoints
	// Registration begin/finish requires auth (binds a passkey to the logged-in user).
	app.Post("/api/v1/auth/webauthn/register/begin", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleBeginWebAuthnRegistrationLift),
			20, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/auth/webauthn/register/finish", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleFinishWebAuthnRegistrationLift),
			20, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	// Login begin/finish is public (username provided), but rate limited.
	app.Post("/api/v1/auth/webauthn/login/begin", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleBeginWebAuthnLoginLift),
			20, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/auth/webauthn/login/finish", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleFinishWebAuthnLoginLift),
			20, 5*time.Minute, logger,
		),
		baseMiddleware,
	))

	app.Get("/api/v1/auth/webauthn/credentials", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleListWebAuthnCredentialsLift),
		baseMiddleware,
	))
	app.Delete("/api/v1/auth/webauthn/credentials/{credentialId}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleDeleteWebAuthnCredentialLift),
		baseMiddleware,
	))
	app.Put("/api/v1/auth/webauthn/credentials/{credentialId}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleUpdateWebAuthnCredentialNameLift),
		baseMiddleware,
	))

	// OAuth endpoints with native Lift implementation + rate limiting
	app.Get("/oauth/authorize", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleOAuthAuthorizeLift),
			20, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/oauth/consent", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleOAuthConsentLift),
			20, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/oauth/token", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleOAuthTokenLift),
			10, time.Minute, logger,
		),
		baseMiddleware,
	))

	// Instance setup endpoints (locked-by-default bootstrapping)
	app.Get("/setup/status", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleSetupStatusLift),
		baseMiddleware,
	))
	app.Post("/setup/bootstrap/challenge", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleSetupBootstrapChallengeLift),
			20, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/setup/bootstrap/verify", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleSetupBootstrapVerifyLift),
			20, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/setup/admin", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleSetupCreateAdminLift),
		baseMiddleware,
	))
	app.Post("/setup/finalize", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleSetupFinalizeLift),
		baseMiddleware,
	))
}
