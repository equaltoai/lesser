package main

import (
	"time"

	"github.com/equaltoai/lesser/pkg/ratelimit"
	"github.com/pay-theory/lift/pkg/lift"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func configureModerationRoutesAppTheory(app *apptheory.App) {
	baseMiddleware := standardLiftMiddlewaresForAppTheory()

	// Moderation + reports.
	app.Post("/api/v1/moderation/flag", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleModerationFlagLift),
			30, time.Hour, logger,
		),
		baseMiddleware,
	))
	app.Get("/api/v1/moderation/queue", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleModerationQueueLift),
		baseMiddleware,
	))
	app.Post("/api/v1/moderation/review", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleModerationReviewLift),
			60, time.Hour, logger,
		),
		baseMiddleware,
	))
	app.Get("/api/v1/moderation/history/{object_id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleModerationHistoryLift),
		baseMiddleware,
	))
	app.Get("/api/v1/moderation/consensus/{event_id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetConsensusLift),
		baseMiddleware,
	))
	app.Get("/api/v1/moderation/trust", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetTrustRelationshipsLift),
		baseMiddleware,
	))
	app.Put("/api/v1/moderation/trust", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleUpdateTrustLift),
		baseMiddleware,
	))
	app.Get("/api/v1/moderation/trust/{actor_id}/score", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetTrustScoreLift),
		baseMiddleware,
	))
	app.Post("/api/v1/reports", liftHandlerToAppTheory(
		ratelimit.ApplyRateLimit(
			lift.HandlerFunc(liftHandler.HandleCreateReportLift),
			30, time.Hour, logger,
		),
		baseMiddleware,
	))

	// Reputation + vouches.
	app.Get("/api/v1/reputation/{actor_id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetReputationLift),
		baseMiddleware,
	))
	app.Post("/api/v1/reputation/export", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleExportReputationLift),
		baseMiddleware,
	))
	app.Post("/api/v1/reputation/import", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleImportReputationLift),
		baseMiddleware,
	))
	app.Post("/api/v1/reputation/verify", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleVerifyReputationLift),
		baseMiddleware,
	))
	app.Post("/api/v1/vouches", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleCreateVouchLift),
		baseMiddleware,
	))
	app.Get("/api/v1/vouches/{actor_id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetVouchesLift),
		baseMiddleware,
	))
	app.Delete("/api/v1/vouches/{vouch_id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleRevokeVouchLift),
		baseMiddleware,
	))

	// Admin endpoints.
	app.Get("/api/v1/admin/accounts", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminGetAccountsLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/accounts", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminCreateUserLift),
		baseMiddleware,
	))
	app.Get("/api/v1/admin/accounts/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminGetAccountLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/accounts/{id}/action", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminAccountActionLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/accounts/{id}/approve", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminApproveAccountLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/accounts/{id}/reject", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminRejectAccountLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/accounts/{id}/enable", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminEnableAccountLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/accounts/{id}/unsilence", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminUnsilenceAccountLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/accounts/{id}/unsuspend", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminUnsuspendAccountLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/accounts/{id}/unsensitive", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminUnsensitiveAccountLift),
		baseMiddleware,
	))

	app.Get("/api/v1/admin/reports", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminGetReportsLift),
		baseMiddleware,
	))
	app.Get("/api/v1/admin/reports/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminGetReportLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/reports/{id}/resolve", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminResolveReportLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/reports/{id}/reopen", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminReopenReportLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/reports/{id}/assign_to_self", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminAssignReportLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/reports/{id}/unassign", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminUnassignReportLift),
		baseMiddleware,
	))

	app.Get("/api/v1/admin/statuses", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminGetStatusesLift),
		baseMiddleware,
	))
	app.Get("/api/v1/admin/statuses/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminGetStatusLift),
		baseMiddleware,
	))
	app.Delete("/api/v1/admin/statuses/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminDeleteStatusLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/statuses/{id}/sensitive", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminMarkStatusSensitiveLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/statuses/{id}/unsensitive", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminUnmarkStatusSensitiveLift),
		baseMiddleware,
	))

	app.Get("/api/v1/admin/domain_blocks", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetAdminDomainBlocksLift),
		baseMiddleware,
	))
	app.Get("/api/v1/admin/domain_blocks/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetAdminDomainBlockLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/domain_blocks", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleCreateAdminDomainBlockLift),
		baseMiddleware,
	))
	app.Put("/api/v1/admin/domain_blocks/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleUpdateAdminDomainBlockLift),
		baseMiddleware,
	))
	app.Delete("/api/v1/admin/domain_blocks/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleDeleteAdminDomainBlockLift),
		baseMiddleware,
	))

	app.Get("/api/v1/admin/domain_allows", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetAdminDomainAllowsLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/domain_allows", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleCreateAdminDomainAllowLift),
		baseMiddleware,
	))
	app.Delete("/api/v1/admin/domain_allows/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleDeleteAdminDomainAllowLift),
		baseMiddleware,
	))

	app.Get("/api/v1/admin/email_domain_blocks", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetEmailDomainBlocksLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/email_domain_blocks", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleCreateEmailDomainBlockLift),
		baseMiddleware,
	))
	app.Delete("/api/v1/admin/email_domain_blocks/{id}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleDeleteEmailDomainBlockLift),
		baseMiddleware,
	))

	app.Get("/api/v1/admin/federation/instances", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetFederationInstancesLift),
		baseMiddleware,
	))
	app.Get("/api/v1/admin/federation/instance/{domain}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetFederationInstanceLift),
		baseMiddleware,
	))
	app.Get("/api/v1/admin/federation/statistics", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleGetFederationStatisticsLift),
		baseMiddleware,
	))

	app.Post("/api/v1/admin/announcements", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleCreateAnnouncementLift),
		baseMiddleware,
	))

	app.Post("/api/v1/admin/custom_emojis", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleCreateCustomEmojiLift),
		baseMiddleware,
	))
	app.Put("/api/v1/admin/custom_emojis/{shortcode}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleUpdateCustomEmojiLift),
		baseMiddleware,
	))
	app.Delete("/api/v1/admin/custom_emojis/{shortcode}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleDeleteCustomEmojiLift),
		baseMiddleware,
	))

	app.Get("/api/v1/admin/moderation/overview", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminModerationOverviewLift),
		baseMiddleware,
	))
	app.Get("/api/v1/admin/moderation/events", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminGetModerationEventsLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/moderation/events/{id}/override", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminOverrideModerationEventLift),
		baseMiddleware,
	))

	app.Get("/api/v1/admin/moderation/trust/graph", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminGetTrustGraphLift),
		baseMiddleware,
	))
	app.Put("/api/v1/admin/moderation/trust/{from}/{to}", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminUpdateTrustLift),
		baseMiddleware,
	))

	app.Get("/api/v1/admin/moderation/reviewers", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminGetReviewersLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/moderation/reviewers/{id}/promote", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminPromoteModeratorLift),
		baseMiddleware,
	))
	app.Post("/api/v1/admin/moderation/reviewers/{id}/demote", liftHandlerToAppTheory(
		lift.HandlerFunc(liftHandler.HandleAdminDemoteModeratorLift),
		baseMiddleware,
	))
}

