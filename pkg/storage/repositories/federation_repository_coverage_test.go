package repositories

import (
	"context"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	appConfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/monitoring"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

type federationRepositoryRoundTripper struct{}

func (f *federationRepositoryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Path, "/.well-known/nodeinfo") {
		body := `{"links":[{"rel":"http://nodeinfo.diaspora.software/ns/schema/2.0","href":"https://` + req.URL.Host + `/nodeinfo/2.0"}]}`
		return newMockResponse(http.StatusOK, body), nil
	}
	if strings.Contains(req.URL.Path, "/.well-known/webfinger") {
		return newMockResponse(http.StatusOK, `{"subject":"acct:test@`+req.URL.Host+`"}`), nil
	}
	return newMockResponse(http.StatusOK, "{}"), nil
}

func setupPermissiveFederationRepoMocks(mockDB *mocks.MockDB, mockQuery *mocks.MockQuery, baseTime time.Time) {
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Between", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		populateFederationSliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	// Wave #1469 page-capped walks (GetFederationStatistics/GetCostProjections/
	// GetAffectedRelationships/GetDetailedFederationMetrics) iterate with
	// AllPaginated instead of a bare All.
	mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
		populateFederationSliceForCoverage(args.Get(0), baseTime)
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Maybe()

	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		populateFederationSliceForCoverage(args.Get(0), baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		populateFederationStructForCoverage(args.Get(0), 0, baseTime)
	}).Return(nil).Maybe()

	mockQuery.On("Create").Return(nil).Maybe()
	mockQuery.On("CreateOrUpdate").Return(nil).Maybe()
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Delete").Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(1), nil).Maybe()
}

func populateFederationSliceForCoverage(target any, baseTime time.Time) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Ptr || value.Elem().Kind() != reflect.Slice {
		return
	}

	slice := value.Elem()
	elemType := slice.Type().Elem()

	// Avoid trying to populate interface slices; keep them empty to prevent type assertion pitfalls.
	if elemType.Kind() == reflect.Interface {
		return
	}

	baseType := elemType
	if baseType.Kind() == reflect.Ptr {
		baseType = baseType.Elem()
	}

	count := 2
	switch baseType {
	case reflect.TypeOf(models.FederationCost{}):
		count = 4 // exercise top-driver trimming
	case reflect.TypeOf(models.FederationEdge{}):
		count = 3 // exercise sorting and pagination branches safely
	case reflect.TypeOf(models.FederationCostActivity{}):
		count = 2 // exercise success/error and time-range filtering
	case reflect.TypeOf(models.FederationAnalyticsTimeSeries{}):
		count = 2 // exercise grouping by domain
	case reflect.TypeOf(models.Follow{}):
		count = 2 // exercise suffix filtering
	case reflect.TypeOf(models.InstanceConnection{}):
		count = 2 // exercise cutoff filtering
	}

	for i := range count {
		var element reflect.Value
		if elemType.Kind() == reflect.Ptr {
			element = reflect.New(baseType)
			populateFederationStructForCoverage(element.Interface(), i, baseTime)
		} else {
			elemPtr := reflect.New(baseType)
			populateFederationStructForCoverage(elemPtr.Interface(), i, baseTime)
			element = elemPtr.Elem()
		}
		slice = reflect.Append(slice, element)
	}

	value.Elem().Set(slice)
}

func populateFederationStructForCoverage(target any, idx int, baseTime time.Time) {
	now := baseTime.Add(time.Duration(idx) * time.Minute)

	switch model := target.(type) {
	case *models.FederationInstance:
		model.Domain = "example.com"
		model.Software = "mastodon"
		model.Version = "4.2.0"
		model.FirstSeen = now.Add(-24 * time.Hour)
		model.LastSeen = now
		model.PublicKey = "pk"
		model.SharedInbox = "https://example.com/inbox"
		model.TrustScore = 0.9
		model.ActiveUsers = 42
		model.TotalMessages = 1234
		model.UpdateKeys()

	case *models.FederationCostActivity:
		model.ID = "activity-" + strconv.Itoa(idx+1)
		model.Domain = "example.com"
		model.Type = "ingress"
		model.ActivityType = "Create"
		model.ByteSize = 1024
		model.Success = idx%2 == 0
		model.ResponseTime = 6000
		if !model.Success {
			model.ErrorMessage = "error"
		}
		model.Timestamp = now
		_ = model.UpdateKeys()

	case *models.FederationCost:
		model.Domain = "domain-" + strconv.Itoa(idx+1) + ".com"
		model.Period = "monthly"
		model.IngressBytes = 1024
		model.EgressBytes = 2048
		model.RequestCount = 10
		model.ErrorCount = 1
		model.ErrorRate = 0.1
		model.AvgResponseTime = 250
		model.EstimatedCostUSD = float64(idx+1) * 1.23
		model.LastUpdated = now
		model.UpdateKeys()

	case *models.FederationNode:
		model.Domain = "node-" + strconv.Itoa(idx+1) + ".com"
		model.Software = "mastodon"
		model.Version = "4.2.0"
		model.FirstSeen = now.Add(-7 * 24 * time.Hour)
		model.LastSeen = now
		model.Health = string(monitoring.HealthStatusHealthy)
		model.ErrorRate = 0.05
		model.ResponseTime = 120
		model.ConnectionType = "direct"
		model.Metadata = map[string]any{"k": "v"}
		model.UpdateKeys()

	case *models.FederationEdge:
		model.SourceDomain = "node-1.com"
		model.TargetDomain = "node-" + strconv.Itoa(idx+2) + ".com"
		model.ConnectionType = "follows"
		model.VolumeIn = 100
		model.VolumeOut = 200
		model.Strength = 0.9 - float64(idx)*0.1
		model.LastActivity = now
		model.SharedUsers = 10
		model.SuccessRate = 0.95
		model.UpdateKeys()

	case *models.InstanceMetadata:
		model.Domain = "example.com"
		model.Software = "mastodon"
		model.Version = "4.2.0"
		model.LastUpdated = now
		model.UpdateKeys()

	case *models.InstanceCluster:
		model.ClusterID = "cluster-1"
		model.Name = "cluster"
		model.Instances = []string{"example.com", "node-1.com"}
		model.CenterNode = "example.com"
		model.Cohesion = 0.75
		model.Size = len(model.Instances)
		model.Description = "desc"
		model.UpdatedAt = now
		model.UpdateKeys()

	case *models.Follow:
		model.PK = "follow#user-1"
		model.SK = "following#1"
		if idx%2 == 0 {
			model.FollowedUsername = "someone@example.com"
		} else {
			model.FollowedUsername = "someone@other.com"
		}
		model.CreatedAt = now
		model.UpdatedAt = now

	case *models.InstanceConnection:
		model.Domain = "example.com"
		model.TargetDomain = "remote.com"
		model.ConnectionType = "follows"
		model.Direction = "outbound"
		model.LastActivity = now
		if idx%2 == 0 {
			model.LastActivity = time.Now()
		} else {
			model.LastActivity = time.Now().Add(-48 * time.Hour)
		}

	case *models.FederationIssue:
		model.Domain = "example.com"
		model.IssueType = "timeout"
		model.Timestamp = now
		model.Severity = "high"
		model.Resolved = false
		model.UpdateKeys()

	case *models.ReconnectionAttempt:
		model.UserID = "user-1"
		model.Domain = "example.com"
		model.AttemptedAt = now
		model.Method = "manual"
		model.UpdateKeys()

	case *models.DeliveryStatus:
		model.ActivityID = "activity-1"
		model.TargetDomain = "example.com"
		model.Status = StatusPending
		model.Attempts = 1
		model.CreatedAt = now
		model.LastAttempt = now
		model.DeliveredAt = now
		model.NextRetry = now
		model.UpdateKeys()

	case *models.SeveredRelationship:
		model.LocalInstance = "local.test"
		model.RemoteInstance = "example.com"
		model.Reason = models.SeveranceReasonOther
		model.DetectedAt = now
		model.Reversible = true
		model.Status = models.SeveranceStatusActive
		_ = model.UpdateKeys()

	case *models.FederationAnalyticsTimeSeries:
		// Use one domain twice to cover "exists" path in aggregation helpers.
		model.Domain = "example.com"
		if idx > 0 {
			model.Domain = "example.com"
		}
		model.Period = models.Period5Min
		model.Timestamp = now
		model.HealthScore = 30.0
		model.InstanceReachability = 0.4
		model.InboxDeliveryP95 = 6000
		model.QueueDepth = 20000
		model.UpdateKeys()

	default:
		fillStructDefaults(reflect.ValueOf(target), now, idx)
	}
}

func fillStructDefaults(value reflect.Value, now time.Time, idx int) {
	if value.Kind() != reflect.Ptr || value.IsNil() {
		return
	}
	value = value.Elem()
	if value.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		fieldType := value.Type().Field(i)

		if !field.CanSet() {
			continue
		}

		switch field.Kind() {
		case reflect.String:
			if fieldType.Name == "Domain" {
				field.SetString("example.com")
			} else {
				field.SetString(fieldType.Name + "-" + strconv.Itoa(idx+1))
			}
		case reflect.Bool:
			field.SetBool(true)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			// Prefer plausible values for time-based fields
			if field.Type() == reflect.TypeOf(time.Time{}) {
				field.Set(reflect.ValueOf(now))
				continue
			}
			field.SetInt(int64(idx + 1))
		case reflect.Float32, reflect.Float64:
			field.SetFloat(float64(idx+1) * 0.1)
		case reflect.Slice:
			// Leave slices empty by default
		case reflect.Map:
			// Leave maps nil by default
		case reflect.Struct:
			if field.Type() == reflect.TypeOf(time.Time{}) {
				field.Set(reflect.ValueOf(now))
			}
		}
	}
}

func TestFederationRepository_CoverageSweep(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	http.DefaultTransport = &federationRepositoryRoundTripper{}

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	setupPermissiveFederationRepoMocks(mockDB, mockQuery, baseTime)

	cfg := &appConfig.Config{Domain: "local.test"}
	repo := NewFederationRepository(mockDB, "test-table", zap.NewNop(), nil, cfg)

	start := baseTime.Add(-2 * time.Hour)
	end := baseTime.Add(2 * time.Hour)

	_, err := repo.GetFederationStatistics(ctx, start, end)
	require.NoError(t, err)

	_, err = repo.GetInstanceStats(ctx, "example.com")
	require.NoError(t, err)

	_, _, err = repo.GetFederationCosts(ctx, start, end, 10, "")
	require.NoError(t, err)

	_, err = repo.GetInstanceHealthReport(ctx, "example.com", time.Hour)
	require.NoError(t, err)

	_, err = repo.GetCostProjections(ctx, "monthly")
	require.NoError(t, err)

	_, err = repo.GetFederationNodes(ctx, 1)
	require.NoError(t, err)

	_, err = repo.GetFederationNodesByHealth(ctx, string(monitoring.HealthStatusHealthy), 10)
	require.NoError(t, err)

	_, err = repo.GetFederationEdges(ctx, []string{"example.com", "remote.com"})
	require.NoError(t, err)

	_, err = repo.GetInstanceMetadata(ctx, "example.com")
	require.NoError(t, err)

	_, err = repo.CalculateFederationClusters(ctx)
	require.NoError(t, err)

	_, err = repo.GetInstanceConnections(ctx, "example.com", ConnectionTypeAll)
	require.NoError(t, err)

	repo.updateAggregatedCosts(ctx, &storage.FederationActivity{
		Domain:       "example.com",
		Type:         "ingress",
		ByteSize:     2048,
		Success:      false,
		ResponseTime: 6000,
		Timestamp:    baseTime,
	})

	require.NoError(t, repo.AcknowledgeSeverance(ctx, "user-1", "example.com"))

	require.NoError(t, repo.TrackFederationIssue(ctx, "example.com", StatusTimeout))

	_, err = repo.GetUserSeveredRelationships(ctx, "user-1")
	require.NoError(t, err)

	_, err = repo.GetAffectedRelationships(ctx, "user-1", "example.com")
	require.NoError(t, err)

	_, err = repo.GetRecentInstanceConnections(ctx, "example.com", time.Hour)
	require.NoError(t, err)

	require.NoError(t, repo.UpdateFederationNode(ctx, &storage.FederationNode{
		Domain:       "example.com",
		Software:     "mastodon",
		Version:      "4.2.0",
		Health:       string(monitoring.HealthStatusHealthy),
		FirstSeen:    baseTime.Add(-24 * time.Hour),
		LastSeen:     baseTime,
		Metadata:     map[string]any{"k": "v"},
		ErrorRate:    0.05,
		ResponseTime: 100,
	}))

	require.NoError(t, repo.UpdateFederationEdge(ctx, &storage.FederationEdge{
		SourceDomain:   "example.com",
		TargetDomain:   "remote.com",
		ConnectionType: "follows",
		Strength:       0.9,
		LastActivity:   baseTime,
	}))

	require.NoError(t, repo.UpdateInstanceMetadata(ctx, &storage.InstanceMetadata{
		Domain:      "example.com",
		LastUpdated: baseTime,
	}))

	require.NoError(t, repo.StoreFederationTimeSeries(ctx, &storage.FederationTimeSeries{
		Domain:         "example.com",
		Period:         models.Period5Min,
		Timestamp:      baseTime,
		ErrorRate:      0.2,
		ResponseTime:   6000,
		InboundVolume:  1000,
		OutboundVolume: 2000,
		ActivePeers:    5,
	}))

	require.NoError(t, repo.StoreInstanceCluster(ctx, &storage.InstanceCluster{
		ClusterID:  "cluster-1",
		Name:       "cluster",
		Instances:  []string{"example.com", "remote.com"},
		CenterNode: "example.com",
		Cohesion:   0.75,
	}))

	require.NoError(t, repo.CreateSeveredRelationship(ctx, &models.SeveredRelationship{
		LocalInstance:     "local.test",
		RemoteInstance:    "example.com",
		Reason:            models.SeveranceReasonOther,
		DetectedAt:        baseTime,
		Reversible:        true,
		AffectedFollowers: 1,
		AffectedFollowing: 2,
		Status:            models.SeveranceStatusActive,
	}))

	_, _, err = repo.GetSeveredRelationships(ctx, "local.test", 2, "")
	require.NoError(t, err)

	_, err = repo.GetSeveredRelationship(ctx, "local.test", "example.com")
	require.NoError(t, err)

	require.NoError(t, repo.UpdateSeveredRelationship(ctx, &models.SeveredRelationship{
		LocalInstance:  "local.test",
		RemoteInstance: "example.com",
		Reason:         models.SeveranceReasonOther,
		Reversible:     true,
		Details:        "details",
	}))

	require.NoError(t, repo.ReverseSeverance(ctx, "local.test", "example.com"))

	_, err = repo.GetSeveranceHistory(ctx, "local.test", "example.com", 2)
	require.NoError(t, err)

	require.NoError(t, repo.RecordDeliveryAttempt(ctx, "activity-1", "example.com", false, "failed"))
	require.NoError(t, repo.RecordDeliveryAttempt(ctx, "activity-1", "example.com", true, ""))

	_, err = repo.GetDeliveryStatus(ctx, "activity-1", "example.com")
	require.NoError(t, err)

	_, err = repo.ListFailedDeliveries(ctx, 2)
	require.NoError(t, err)

	require.NoError(t, repo.RetryDelivery(ctx, "activity-1", "example.com"))

	_, err = repo.CleanupOldDeliveries(ctx, time.Hour)
	require.NoError(t, err)

	activity := &activitypub.Activity{}
	activity.ID = "act-1"
	activity.Type = "Create"
	require.NoError(t, repo.AddToInbox(ctx, "actor-1", activity))
	require.NoError(t, repo.AddToOutbox(ctx, "actor-1", activity, true))

	_, _, err = repo.GetInboxItems(ctx, "actor-1", 2, "")
	require.NoError(t, err)

	_, _, err = repo.GetPublicOutbox(ctx, "actor-1", 2, "")
	require.NoError(t, err)

	_, _, err = repo.GetOutboxItems(ctx, "actor-1", 2, "")
	require.NoError(t, err)

	_, err = repo.GetStrongestConnectionsByType(ctx, ConnectionTypeAll, 3)
	require.NoError(t, err)

	require.NoError(t, repo.StoreDetailedFederationMetrics(ctx, &models.FederationAnalyticsTimeSeries{
		Domain:               "example.com",
		Period:               models.Period5Min,
		Timestamp:            baseTime,
		InstanceReachability: 0.4,
		InboxDeliveryP95:     6000,
		QueueDepth:           20000,
		ErrorRate:            0.2,
		LastSuccessfulContact: func() *time.Time {
			t := baseTime.Add(-time.Minute)
			return &t
		}(),
	}))

	_, err = repo.GetDetailedFederationMetrics(ctx, "example.com", "", start, end)
	require.NoError(t, err)

	_, err = repo.GetDetailedMetricsByPeriod(ctx, "", start, end, 2)
	require.NoError(t, err)

	_, err = repo.GetDomainHealthScore(ctx, "example.com")
	require.NoError(t, err)

	_, err = repo.GetUnhealthyDomains(ctx, 0)
	require.NoError(t, err)

	require.NoError(t, repo.AggregateFederationMetrics(ctx, "example.com", models.Period5Min, "5min", baseTime))

	_, err = repo.GetFederationAlertsData(ctx)
	require.NoError(t, err)

	_, err = repo.GetAffectedFollowersCount(ctx, "user-1", "example.com")
	require.NoError(t, err)

	_, err = repo.GetAffectedFollowingCount(ctx, "user-1", "example.com")
	require.NoError(t, err)

	_, err = repo.GetFederationCostsByUser(ctx, "user-1", start, end, 2, 0)
	require.NoError(t, err)

	_, err = repo.GetAllFederationEdges(ctx, 1)
	require.NoError(t, err)

	_, err = repo.GetFederationClusters(ctx, 2)
	require.NoError(t, err)

	_, err = repo.GetFederationActivitiesByTimeRange(ctx, start, end, 1)
	require.NoError(t, err)

	require.NoError(t, repo.RecordFederationActivity(ctx, &storage.FederationActivity{
		Domain:       "example.com",
		Type:         "ingress",
		ActivityType: "Create",
		ByteSize:     1024,
		Success:      true,
		ResponseTime: 123,
		Timestamp:    baseTime,
	}))

	require.NoError(t, repo.AttemptReconnection(ctx, "user-1", "example.com"))
}
