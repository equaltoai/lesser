package graph

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	storagetypes "github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound12AdminParity_QueriesAndMutations(t *testing.T) {
	resolver, store := newRound12GraphResolver(t)

	ctx := round12AuthContext("admin")
	mut := resolver.Mutation()
	qry := resolver.Query()

	// Seed users via admin API.
	moderatorRole := adminRoleModerator
	adminRole := adminRoleAdmin

	_, err := mut.AdminCreateUser(ctx, model.AdminCreateUserInput{
		Username: "bob",
	})
	require.NoError(t, err)

	_, err = mut.AdminCreateUser(ctx, model.AdminCreateUserInput{
		Username: "mod",
		Role:     &moderatorRole,
	})
	require.NoError(t, err)

	_, err = mut.AdminCreateUser(ctx, model.AdminCreateUserInput{
		Username: "other-admin",
		Role:     &adminRole,
	})
	require.NoError(t, err)

	first := 5
	accounts, err := qry.AdminAccounts(ctx, &first, nil)
	require.NoError(t, err)
	require.NotNil(t, accounts)

	account, err := qry.AdminAccount(ctx, "bob")
	require.NoError(t, err)
	require.NotNil(t, account)

	// Account actions.
	_, err = mut.AdminAccountAction(ctx, model.AdminAccountActionInput{ID: "bob", Type: "nope"})
	require.Error(t, err)

	_, err = mut.AdminAccountAction(ctx, model.AdminAccountActionInput{ID: "bob", Type: AdminAccountActionUnsensitive})
	require.NoError(t, err)

	_, err = mut.AdminAccountAction(ctx, model.AdminAccountActionInput{ID: "bob", Type: AdminAccountActionSuspend})
	require.NoError(t, err)

	_, err = mut.AdminAccountAction(ctx, model.AdminAccountActionInput{ID: "bob", Type: ModerationActionApprove})
	require.NoError(t, err)

	_, err = mut.AdminAccountAction(ctx, model.AdminAccountActionInput{ID: "bob", Type: ModerationActionReject})
	require.NoError(t, err)

	// Announcements.
	announcement, err := mut.AdminCreateAnnouncement(ctx, model.AdminCreateAnnouncementInput{Text: "hello"})
	require.NoError(t, err)
	require.NotNil(t, announcement)

	// Domain allows/blocks/email domain blocks.
	allow, err := mut.AdminCreateDomainAllow(ctx, "https://example.com/path")
	require.NoError(t, err)
	require.NotNil(t, allow)

	_, _ = mut.AdminDeleteDomainAllow(ctx, allow.ID)

	silence := "silence"
	block, err := mut.AdminCreateDomainBlock(ctx, model.AdminDomainBlockCreateInput{
		Domain:      "bad.example",
		Severity:    &silence,
		RejectMedia: ptrBool(true),
		Obfuscate:   ptrBool(true),
	})
	require.NoError(t, err)
	require.NotNil(t, block)

	suspend := "suspend"
	updatedBlock, err := mut.AdminUpdateDomainBlock(ctx, block.ID, model.AdminDomainBlockUpdateInput{
		Severity: &suspend,
	})
	_ = updatedBlock
	_ = err

	_, _ = mut.AdminDeleteDomainBlock(ctx, block.ID)

	emailBlock, err := mut.AdminCreateEmailDomainBlock(ctx, "@spam.example")
	require.NoError(t, err)
	require.NotNil(t, emailBlock)

	_, _ = mut.AdminDeleteEmailDomainBlock(ctx, emailBlock.ID)

	// Admin domain queries.
	_, err = qry.AdminDomainAllows(ctx, &first, nil)
	require.NoError(t, err)
	_, err = qry.AdminDomainBlocks(ctx, &first, nil)
	require.NoError(t, err)
	_, err = qry.AdminEmailDomainBlocks(ctx, &first, nil)
	require.NoError(t, err)
	_, _ = qry.AdminDomainBlock(ctx, "block-1")

	// Federation queries (empty results are acceptable in this harness).
	_, err = qry.AdminFederationInstances(ctx, &first, nil)
	require.NoError(t, err)
	_, err = qry.AdminFederationInstance(ctx, "example.org")
	require.NoError(t, err)
	_, err = qry.AdminFederationStatistics(ctx, nil, nil)
	require.NoError(t, err)

	// Moderation event override + query.
	event := &storagetypes.ModerationEvent{
		ID:        "evt-1",
		EventType: "status",
		Category:  "spam",
		Severity:  "3",
		ObjectID:  "status-1",
		ActorID:   "bob",
		Reason:    "test",
	}
	require.NoError(t, store.Moderation().CreateModerationEvent(ctx, event))

	overrideReason := "override"
	override, err := mut.AdminOverrideModerationEvent(ctx, model.AdminModerationEventOverrideInput{
		EventID:  event.ID,
		Decision: "reject",
		Reason:   &overrideReason,
	})
	require.NoError(t, err)
	require.NotNil(t, override)

	eventType := "status"
	eventsConn, err := qry.AdminModerationEvents(ctx, &model.AdminModerationEventFilter{EventType: &eventType}, &first, nil)
	require.NoError(t, err)
	require.NotNil(t, eventsConn)

	// Reviewer list (uses role-based user queries and reviewer stats).
	_, err = qry.AdminModerationReviewers(ctx)
	require.NoError(t, err)

	// Trust graph + admin trust update.
	seedTrust := &storagetypes.TrustRelationship{
		TrusterID:  "bob",
		TrusteeID:  "carol",
		Category:   storagetypes.TrustCategory("general"),
		Score:      0.5,
		Confidence: 1.0,
		Created:    time.Now(),
		Updated:    time.Now(),
	}
	require.NoError(t, store.Trust().CreateTrustRelationship(ctx, seedTrust))

	_, err = qry.AdminTrustGraph(ctx, &first)
	require.NoError(t, err)

	category := "general"
	_, err = mut.AdminUpdateTrust(ctx, model.AdminUpdateTrustInput{
		FromActorID: "bob",
		ToActorID:   "carol",
		Trust:       0.2,
		Category:    &category,
	})
	require.NoError(t, err)

	// Promote/demote reviewers.
	_, err = mut.AdminPromoteReviewer(ctx, "bob")
	require.NoError(t, err)

	_, err = mut.AdminDemoteReviewer(ctx, "bob")
	require.NoError(t, err)

	_, err = mut.AdminDemoteReviewer(ctx, "admin")
	require.Error(t, err)

	// Seed a status and report to exercise report + status admin paths.
	status := &storagemodels.Status{
		StatusID:   "status-1",
		Content:    "hello",
		Visibility: "public",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	require.NoError(t, store.Status().CreateStatus(ctx, status))

	report := &storagetypes.Report{
		ID:              "rep-1",
		ReporterID:      "bob",
		TargetAccountID: "carol",
		StatusIDs:       []string{"status-1"},
		Category:        "spam",
		Comment:         "report comment",
	}
	require.NoError(t, store.Moderation().CreateReport(ctx, report))

	_, err = qry.AdminReports(ctx, nil, &first, nil)
	require.NoError(t, err)

	_, err = qry.AdminReport(ctx, report.ID)
	require.NoError(t, err)

	_, err = mut.AdminReportAction(ctx, report.ID, model.AdminReportActionAssignToSelf)
	require.NoError(t, err)

	_, err = mut.AdminReportAction(ctx, report.ID, model.AdminReportActionResolve)
	require.NoError(t, err)

	_, err = qry.AdminStatuses(ctx, nil, &first, nil)
	require.NoError(t, err)

	_, err = qry.AdminStatus(ctx, status.StatusID)
	require.NoError(t, err)

	_, err = mut.AdminSetStatusSensitive(ctx, status.StatusID, true)
	require.NoError(t, err)

	ok, err := mut.AdminDeleteStatus(ctx, status.StatusID)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestRound12AdminParity_HelperCoverage(t *testing.T) {
	now := time.Now()
	sessions := []*storagetypes.Session{
		{SessionID: "s1", IPAddress: "1.1.1.1", LastActivity: now.Add(-time.Hour)},
		{SessionID: "s2", IPAddress: "2.2.2.2", LastActivity: now},
		{SessionID: "s3", IPAddress: "1.1.1.1", LastActivity: now.Add(-2 * time.Hour)},
		nil,
	}

	lastIP, history := deriveAdminIPInfo(sessions)
	require.NotNil(t, lastIP)
	require.NotEmpty(t, history)

	accountID, username := normalizeAdminAccountID("user-bob")
	require.Equal(t, "user-bob", accountID)
	require.Equal(t, "bob", username)

	accountID, username = normalizeAdminAccountID("bob")
	require.Equal(t, "user-bob", accountID)
	require.Equal(t, "bob", username)

	require.Nil(t, toJSONPointer(nil))
	require.Nil(t, toJSONPointer(func() {}))
	require.NotNil(t, toJSONPointer(map[string]any{"a": 1}))
}

func ptrBool(v bool) *bool { return &v }
