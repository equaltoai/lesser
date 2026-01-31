package main

import (
	"time"

	"github.com/equaltoai/lesser/pkg/ratelimit"
	"github.com/pay-theory/lift/pkg/lift"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func configureAccountsRoutesAppTheory(app *apptheory.App) {
	baseMiddleware := standardLiftMiddlewaresForAppTheory()

	// Account verification/update endpoints.
	app.Get("/api/v1/accounts/verify_credentials", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleVerifyCredentialsLift),
		baseMiddleware,
	))
	app.Patch("/api/v1/accounts/update_credentials", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleUpdateCredentialsLift),
			10, time.Hour, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/accounts", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleRegistrationLift),
			10, time.Hour, logger,
		),
		baseMiddleware,
	))

	// Relationships and account surfaces.
	app.Get("/api/v1/accounts/relationships", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetRelationshipsLift),
		baseMiddleware,
	))
	app.Get("/api/v1/accounts/{id}/notes", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetUserNotesLift),
		baseMiddleware,
	))
	app.Get("/api/v1/accounts/{id}/statuses", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetAccountStatusesLift),
		baseMiddleware,
	))

	// Search endpoints with privacy enforcement.
	app.Get("/api/v1/accounts/search", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleAccountSearchLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Get("/api/v1/accounts/search/suggestions", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetSearchSuggestionsLift),
		baseMiddleware,
	))

	// Relationship interactions.
	app.Post("/api/v1/accounts/{id}/follow", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleFollowLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/accounts/{id}/unfollow", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleUnfollowLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/accounts/{id}/block", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleBlockLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/accounts/{id}/unblock", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleUnblockLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/accounts/{id}/mute", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleMuteAccountLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Post("/api/v1/accounts/{id}/unmute", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleUnmuteAccountLift),
			30, 5*time.Minute, logger,
		),
		baseMiddleware,
	))
	app.Get("/api/v1/blocks", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleGetBlocksLift),
			60, time.Hour, logger,
		),
		baseMiddleware,
	))
	app.Get("/api/v1/mutes", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleGetMutedAccountsLift),
			60, time.Hour, logger,
		),
		baseMiddleware,
	))

	// Follow requests.
	app.Get("/api/v1/follow_requests", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetFollowRequestsLift),
		baseMiddleware,
	))
	app.Post("/api/v1/follow_requests/{account_id}/authorize", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAuthorizeFollowRequestLift),
		baseMiddleware,
	))
	app.Post("/api/v1/follow_requests/{account_id}/reject", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleRejectFollowRequestLift),
		baseMiddleware,
	))

	// Domain blocks (user-level).
	app.Get("/api/v1/domain_blocks", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetDomainBlocksLift),
		baseMiddleware,
	))
	app.Post("/api/v1/domain_blocks", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleCreateDomainBlockLift),
			20, time.Hour, logger,
		),
		baseMiddleware,
	))
	app.Delete("/api/v1/domain_blocks", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleDeleteDomainBlockLift),
			20, time.Hour, logger,
		),
		baseMiddleware,
	))

	// Data exports.
	app.Post("/api/v1/exports", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleCreateExportLift),
			5, 24*time.Hour, logger,
		),
		baseMiddleware,
	))
	app.Get("/api/v1/exports/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetExportStatusLift),
		baseMiddleware,
	))
	app.Get("/api/v1/exports/{id}/download", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleDownloadExportLift),
		baseMiddleware,
	))
	app.Get("/api/v1/exports", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleListExportsLift),
		baseMiddleware,
	))

	// Data imports.
	app.Post("/api/v1/imports", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleCreateImportLift),
			5, 24*time.Hour, logger,
		),
		baseMiddleware,
	))
	app.Get("/api/v1/imports/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetImportStatusLift),
		baseMiddleware,
	))
	app.Delete("/api/v1/imports/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleCancelImportLift),
		baseMiddleware,
	))
	app.Get("/api/v1/imports", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleListImportsLift),
		baseMiddleware,
	))

	// Quote permissions.
	app.Get("/api/v1/accounts/{id}/quote_permissions", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetQuotePermissionsLift),
		baseMiddleware,
	))
	app.Put("/api/v1/accounts/quote_permissions", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleUpdateQuotePermissionsLift),
		baseMiddleware,
	))
}

