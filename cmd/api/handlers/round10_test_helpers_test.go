package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
	dynamormcore "github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

type round10Where struct {
	field string
	op    string
	value any
}

type round10VAPIDSecretsClient struct {
	mu     sync.Mutex
	secret string
}

func round10NewVAPIDSecretsClient(state *round10QueryState) *round10VAPIDSecretsClient {
	client := &round10VAPIDSecretsClient{}
	if state != nil && state.forceVapidNotFound {
		return client
	}
	keys := &storage.VAPIDKeys{
		PublicKey:  "pub",
		PrivateKey: "priv",
		Subject:    "mailto:test@example.com",
		CreatedAt:  time.Now().Add(-24 * time.Hour),
		UpdatedAt:  time.Now(),
	}
	if state != nil && state.vapidKeys != nil {
		keys = state.vapidKeys
	}
	payload, err := json.Marshal(map[string]any{
		"public_key":  keys.PublicKey,
		"private_key": keys.PrivateKey,
		"subject":     keys.Subject,
		"created_at":  keys.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  keys.UpdatedAt.UTC().Format(time.RFC3339),
	})
	if err == nil {
		client.secret = string(payload)
	}
	return client
}

func (c *round10VAPIDSecretsClient) GetSecretValue(
	_ context.Context,
	_ *secretsmanager.GetSecretValueInput,
	_ ...func(*secretsmanager.Options),
) (*secretsmanager.GetSecretValueOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.secret == "" {
		return nil, fmt.Errorf("test VAPID secret is absent")
	}
	return &secretsmanager.GetSecretValueOutput{SecretString: aws.String(c.secret)}, nil
}

func (c *round10VAPIDSecretsClient) PutSecretValue(
	_ context.Context,
	input *secretsmanager.PutSecretValueInput,
	_ ...func(*secretsmanager.Options),
) (*secretsmanager.PutSecretValueOutput, error) {
	if input == nil || input.SecretString == nil {
		return nil, fmt.Errorf("test VAPID secret value is absent")
	}
	c.mu.Lock()
	c.secret = aws.ToString(input.SecretString)
	c.mu.Unlock()
	return &secretsmanager.PutSecretValueOutput{}, nil
}

type round10QueryState struct {
	sharedMu       *sync.Mutex
	wheres         []round10Where
	model          any
	sets           map[string]any
	limit          int
	orderField     string
	orderDirection string

	usersByUsername               map[string]storagemodels.User
	actorsByUser                  map[string]storagemodels.Actor
	actorList                     []storagemodels.Actor
	activitiesByID                map[string]*storagemodels.Activity
	statusByID                    map[string]storagemodels.Status
	statusList                    []storagemodels.Status
	objectsByID                   map[string]storagemodels.Object
	objectList                    []storagemodels.Object
	tombstonesByObjectID          map[string]storagemodels.Tombstone
	reportsByID                   map[string]storagemodels.Report
	eventsByID                    map[string]storagemodels.ModerationEvent
	userMediaByUser               map[string][]*storagemodels.Media
	sessionsByID                  map[string]storagemodels.Session
	filtersByID                   map[string]storagemodels.Filter
	filterKeywords                map[string][]storagemodels.FilterKeyword
	filterStatuses                map[string][]storagemodels.FilterStatus
	importsByID                   map[string]storagemodels.Import
	importsByUser                 map[string][]storagemodels.Import
	importBudgetsByPKSK           map[string]storagemodels.ImportBudget
	pushSubscriptionsByUser       map[string][]storagemodels.PushSubscription
	webAuthnCredentialsByUser     map[string][]storagemodels.WebAuthnCredential
	webAuthnCredentialByID        map[string]storagemodels.WebAuthnCredential
	webAuthnChallengesByID        map[string]storagemodels.WebAuthnChallenge
	passkeyRegistrationProofsByID map[string]storagemodels.PasskeyRegistrationProof
	oauthClientsByID              map[string]storagemodels.OAuthClient
	authorizationCodesByCode      map[string]storagemodels.AuthorizationCode
	refreshTokensByToken          map[string]storagemodels.RefreshToken
	refreshAuthoritiesByKey       map[string]storagemodels.OAuthRefreshAuthority
	refreshArtifactsByKey         map[string]storagemodels.OAuthRefreshSuccessorArtifact
	refreshWalkBudgetsByKey       map[string]storagemodels.OAuthRefreshWalkBudget
	revokedAccessTokensByJTI      map[string]storagemodels.RevokedAccessToken
	oauthDeviceSessionsByHash     map[string]storagemodels.OAuthDeviceSession
	oauthDeviceSessionsByUserCode map[string]storagemodels.OAuthDeviceSession
	setupSessionsByID             map[string]storagemodels.SetupSession
	pollsByID                     map[string]storagemodels.Poll
	pollVotesByKey                map[string]storagemodels.PollVote
	costRecords                   []*storagemodels.DynamoDBCostRecord
	costAggregations              []*storagemodels.DynamoDBCostAggregation
	metricRecords                 []storagemodels.MetricRecord
	instanceHistories             []storagemodels.InstanceHistory
	instanceMetrics               map[string]storagemodels.InstanceMetrics
	instanceState                 *storagemodels.InstanceState
	soulBodyBindingsByAgentID     map[string]storagemodels.InstanceSoulBodyBinding
	soulBodyBindingUsernames      map[string]storagemodels.InstanceSoulBodyBindingUsername
	agentInstanceConfig           *storagemodels.AgentInstanceConfig
	quoteRelationships            []storagemodels.QuoteRelationship
	announcesByKey                map[string]storagemodels.Announce
	oauthStates                   map[string]storagemodels.OAuthState
	notificationsByID             map[string]storagemodels.Notification
	domainBlocks                  []storagemodels.InstanceDomainBlock
	domainAllows                  []storagemodels.DomainAllow
	emailDomainBlocks             []storagemodels.EmailDomainBlock
	federationInstancesByDomain   map[string]storagemodels.FederationInstance
	federationInstances           []storagemodels.FederationInstance
	moderationReviews             []storagemodels.ModerationReview
	moderationDecisionsByObject   map[string]storagemodels.ModerationDecision
	exportsByID                   map[string]storagemodels.Export
	exportList                    []storagemodels.Export
	communityNotesByGSI3PK        map[string][]storagemodels.CommunityNote

	relationshipRecords []storagemodels.RelationshipRecord
	trustRelationships  []storagemodels.TrustRelationship
	instanceRules       []storagemodels.InstanceRule

	reputationsByPK map[string][]storagemodels.Reputation
	vouchModels     []*storagemodels.Vouch
	vouchModelsByID map[string]*storagemodels.Vouch

	vapidKeys *storage.VAPIDKeys

	walletChallengesByID       map[string]storagemodels.WalletChallenge
	walletCredentialsByUser    map[string][]storagemodels.WalletCredential
	walletCredentialsByAddress map[string]storagemodels.WalletCredential

	trusteesByUser             map[string][]storagemodels.Trustee
	recoveryRequestsByID       map[string]storagemodels.RecoveryRequest
	recoveryCodesByUser        map[string][]storagemodels.RecoveryCode
	devicesByID                map[string]storagemodels.Device
	featuredTagsByUser         map[string][]storagemodels.FeaturedTag
	hashtagFollowsByUser       map[string][]storagemodels.HashtagFollow
	announcementByID           map[string]storagemodels.Announcement
	announcementDismissalsByID map[string][]storagemodels.AnnouncementDismissal
	announcementReactionsByID  map[string][]storagemodels.AnnouncementReaction
	trendingHashtags           []storagemodels.HashtagTrend
	trendingStatuses           []storagemodels.StatusTrend
	trendingLinks              []storagemodels.LinkTrend

	agentKeyChallengesByID    map[string]storagemodels.AgentKeyChallenge
	agentAccessLeasesByKey    map[string]storagemodels.AgentAccessLease
	agentAccessChallengesByID map[string]storagemodels.AgentAccessLeaseChallenge
	agentGovernanceByUsername map[string]storagemodels.AgentGovernanceState
	quotePermissionsByUser    map[string]storagemodels.QuotePermissions
	statusMetadataByStatus    map[string]storagemodels.StatusMetadata
	agentMemoryEventsByAgent  map[string][]storagemodels.AgentMemoryEvent
	remoteActorsByPK          map[string]storagemodels.RemoteActor
	auditLogsByUser           map[string][]*storagemodels.AuthAuditLog

	forceVapidNotFound bool
	disableAuditRepo   bool

	notFoundPKs    map[string]bool
	notFoundPKSK   map[string]bool
	notFoundGSI3PK map[string]bool

	allErrorOnce            error
	allErrorByType          map[string]error
	scanErrorOnce           error
	firstErrorOnce          error
	firstErrorByType        map[string]error
	updateErrorOnce         error
	createErrorOnce         error
	createOrUpdateErrorOnce error
	deleteErrorOnce         error
	executeErrorOnce        error
	transactionErrorOnce    error
	transactionErrors       []error

	firstErrorPK     map[string]error
	firstErrorGSI3PK map[string]error
}

func (s *round10QueryState) reset() {
	s.wheres = nil
	s.sets = nil
	s.limit = 0
	s.orderField = ""
	s.orderDirection = ""
}

func (s *round10QueryState) whereValue(field string) (any, bool) {
	for i := len(s.wheres) - 1; i >= 0; i-- {
		if s.wheres[i].field == field {
			return s.wheres[i].value, true
		}
	}
	return nil, false
}

func (s *round10QueryState) whereString(field string) (string, bool) {
	v, ok := s.whereValue(field)
	if !ok {
		return "", false
	}
	str, ok := v.(string)
	return str, ok
}

func round10CanonicalizeWebAuthnCredential(cred storagemodels.WebAuthnCredential) storagemodels.WebAuthnCredential {
	if strings.TrimSpace(cred.PK) == "" && strings.TrimSpace(cred.UserID) != "" {
		cred.PK = "USER#" + cred.UserID
	}
	if strings.TrimSpace(cred.SK) == "" && strings.TrimSpace(cred.ID) != "" {
		cred.SK = "WEBAUTHN_CRED#" + cred.ID
	}
	if strings.TrimSpace(cred.GSI1PK) == "" && strings.TrimSpace(cred.ID) != "" {
		cred.GSI1PK = "WEBAUTHN_CREDENTIAL#" + cred.ID
	}
	if strings.TrimSpace(cred.GSI1SK) == "" && strings.TrimSpace(cred.UserID) != "" {
		cred.GSI1SK = "USER#" + cred.UserID
	}
	return cred
}

func round10UpsertWebAuthnCredential(state *round10QueryState, cred storagemodels.WebAuthnCredential) {
	if state == nil {
		return
	}

	cred = round10CanonicalizeWebAuthnCredential(cred)

	if state.webAuthnCredentialByID == nil {
		state.webAuthnCredentialByID = map[string]storagemodels.WebAuthnCredential{}
	}
	state.webAuthnCredentialByID[cred.ID] = cred

	if strings.TrimSpace(cred.UserID) == "" {
		return
	}

	if state.webAuthnCredentialsByUser == nil {
		state.webAuthnCredentialsByUser = map[string][]storagemodels.WebAuthnCredential{}
	}

	creds := state.webAuthnCredentialsByUser[cred.UserID]
	for i := range creds {
		if creds[i].ID == cred.ID {
			creds[i] = cred
			state.webAuthnCredentialsByUser[cred.UserID] = creds
			return
		}
	}
	state.webAuthnCredentialsByUser[cred.UserID] = append(creds, cred)
}

func round10DeleteWebAuthnCredential(state *round10QueryState, credentialID string) {
	if state == nil || strings.TrimSpace(credentialID) == "" {
		return
	}

	cred, ok := state.webAuthnCredentialByID[credentialID]
	if ok {
		delete(state.webAuthnCredentialByID, credentialID)
		creds := state.webAuthnCredentialsByUser[cred.UserID]
		filtered := creds[:0]
		for _, candidate := range creds {
			if candidate.ID == credentialID {
				continue
			}
			filtered = append(filtered, candidate)
		}
		if len(filtered) == 0 {
			delete(state.webAuthnCredentialsByUser, cred.UserID)
			return
		}
		state.webAuthnCredentialsByUser[cred.UserID] = filtered
		return
	}

	for username, creds := range state.webAuthnCredentialsByUser {
		filtered := creds[:0]
		removed := false
		for _, candidate := range creds {
			if candidate.ID == credentialID {
				removed = true
				continue
			}
			filtered = append(filtered, candidate)
		}
		if !removed {
			continue
		}
		if len(filtered) == 0 {
			delete(state.webAuthnCredentialsByUser, username)
		} else {
			state.webAuthnCredentialsByUser[username] = filtered
		}
		return
	}
}

func round10CommunityNoteGSI3SK(note storagemodels.CommunityNote) string {
	if note.GSI3SK != "" {
		return note.GSI3SK
	}
	return note.CreatedAt.Format(time.RFC3339) + "#" + note.ID
}

func round10ApplyCommunityNoteAuthorQuery(state *round10QueryState, notes []storagemodels.CommunityNote) []storagemodels.CommunityNote {
	items := append([]storagemodels.CommunityNote(nil), notes...)
	for i := range items {
		if items[i].GSI3SK == "" {
			items[i].GSI3SK = round10CommunityNoteGSI3SK(items[i])
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		left := round10CommunityNoteGSI3SK(items[i])
		right := round10CommunityNoteGSI3SK(items[j])
		if strings.EqualFold(state.orderField, "gsi3SK") && strings.EqualFold(state.orderDirection, "DESC") {
			return left > right
		}
		return left < right
	})

	if cursor, ok := state.whereString("gsi3SK"); ok && cursor != "" {
		filtered := items[:0]
		for _, note := range items {
			if round10CommunityNoteGSI3SK(note) < cursor {
				filtered = append(filtered, note)
			}
		}
		items = filtered
	}

	if state.limit > 0 && len(items) > state.limit {
		items = items[:state.limit]
	}
	return items
}

func round10ResolveUserForRead(state *round10QueryState) storagemodels.User {
	if state == nil {
		now := time.Now()
		return storagemodels.User{Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-time.Hour), UpdatedAt: now}
	}

	username := ""
	if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "USER#") {
		username = strings.TrimPrefix(pk, "USER#")
	} else if gsi5pk, ok := state.whereString("gsi5PK"); ok && strings.HasPrefix(gsi5pk, "USER_HANDLE_PREFIX#") {
		if gsi5sk, ok := state.whereString("gsi5SK"); ok {
			for candidate, user := range state.usersByUsername {
				if strings.EqualFold(strings.TrimSpace(user.Username), gsi5sk) || strings.EqualFold(strings.TrimSpace(candidate), gsi5sk) {
					return user
				}
			}
			username = gsi5sk
		}
	}

	if user, ok := state.usersByUsername[username]; ok {
		return user
	}
	for candidate, user := range state.usersByUsername {
		if strings.EqualFold(strings.TrimSpace(user.Username), username) || strings.EqualFold(strings.TrimSpace(candidate), username) {
			return user
		}
	}

	role := "user"
	if strings.EqualFold(username, "admin") {
		role = "admin"
	}
	now := time.Now()
	user := storagemodels.User{
		Username:  username,
		Role:      role,
		Approved:  true,
		Version:   1,
		CreatedAt: now.Add(-time.Hour),
		UpdatedAt: now,
	}
	if strings.HasPrefix(strings.ToLower(username), "agent") {
		user.IsAgent = true
		user.AgentType = "service"
	}
	for candidate, actor := range state.actorsByUser {
		if !strings.EqualFold(candidate, username) && !strings.EqualFold(actor.Username, username) {
			continue
		}
		if actor.Actor != nil && strings.EqualFold(strings.TrimSpace(actor.Actor.Type), "Service") {
			user.IsAgent = true
		}
		break
	}
	return user
}

func round10HasUserForRead(state *round10QueryState) bool {
	if state == nil || state.usersByUsername == nil {
		return false
	}
	user := round10ResolveUserForRead(state)
	if strings.TrimSpace(user.Username) == "" {
		return false
	}
	for candidate, existing := range state.usersByUsername {
		if strings.EqualFold(strings.TrimSpace(existing.Username), user.Username) || strings.EqualFold(strings.TrimSpace(candidate), user.Username) {
			return true
		}
	}
	return false
}

func round10PopulateAccountProjection(dest any, state *round10QueryState) bool {
	typeName := reflect.TypeOf(dest).String()
	if typeName != "*repositories.userCoreProjection" && typeName != "*repositories.userMetadataProjection" {
		return false
	}

	user := round10ResolveUserForRead(state)
	value := reflect.ValueOf(dest).Elem()
	round10SetField(value, "Table", "test-table")
	round10SetField(value, "PK", "USER#"+user.Username)
	round10SetField(value, "SK", storagemodels.SKMetadata)

	if typeName == "*repositories.userMetadataProjection" {
		round10SetField(value, "Metadata", user.Metadata)
		return true
	}

	round10SetField(value, "Username", user.Username)
	round10SetField(value, "Email", user.Email)
	round10SetField(value, "PasswordHash", user.PasswordHash)
	round10SetField(value, "DisplayName", user.DisplayName)
	round10SetField(value, "Note", user.Note)
	round10SetField(value, "Avatar", user.Avatar)
	round10SetField(value, "Header", user.Header)
	round10SetField(value, "URL", user.URL)
	round10SetField(value, "Locked", user.Locked)
	round10SetField(value, "Discoverable", user.Discoverable)
	round10SetField(value, "Fields", user.Fields)
	round10SetField(value, "CreatedAt", user.CreatedAt)
	round10SetField(value, "UpdatedAt", user.UpdatedAt)
	round10SetField(value, "Approved", user.Approved)
	round10SetField(value, "Suspended", user.Suspended)
	round10SetField(value, "Silenced", user.Silenced)
	round10SetField(value, "Role", user.Role)
	round10SetField(value, "Locale", user.Locale)
	round10SetField(value, "RecoveryMethods", user.RecoveryMethods)
	round10SetField(value, "AllowNSFW", user.AllowNSFW)
	round10SetField(value, "RequireNSFWWarning", user.RequireNSFWWarning)
	round10SetField(value, "IsAgent", user.IsAgent)
	round10SetField(value, "AgentType", user.AgentType)
	round10SetField(value, "AgentCapabilities", user.AgentCapabilities)
	round10SetField(value, "AgentVersion", user.AgentVersion)
	round10SetField(value, "AgentOwner", user.AgentOwner)
	round10SetField(value, "AgentCreatedBy", user.AgentCreatedBy)
	round10SetField(value, "AgentPublicKey", user.AgentPublicKey)
	round10SetField(value, "AgentKeyType", user.AgentKeyType)
	round10SetField(value, "Version", user.Version)
	return true
}

func round10SetField(target reflect.Value, name string, value any) {
	field := target.FieldByName(name)
	if !field.IsValid() || !field.CanSet() {
		return
	}
	if value == nil {
		field.Set(reflect.Zero(field.Type()))
		return
	}
	incoming := reflect.ValueOf(value)
	if incoming.Type().AssignableTo(field.Type()) {
		field.Set(incoming)
		return
	}
	if incoming.Type().ConvertibleTo(field.Type()) {
		field.Set(incoming.Convert(field.Type()))
	}
}

func round10NewLiftContext(method, path string, headers, query map[string]string, body any) (*apptheory.Context, error) {
	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyBytes = b
	}

	return round10NewLiftContextWithBodyBytes(method, path, headers, query, bodyBytes), nil
}

func round10NewLiftContextWithBodyBytes(method, path string, headers, query map[string]string, body []byte) *apptheory.Context {
	parsedPath := path
	queryValues := url.Values{}
	if idx := strings.Index(path, "?"); idx >= 0 {
		parsedPath = path[:idx]
		if parsed, err := url.ParseQuery(path[idx+1:]); err == nil {
			queryValues = parsed
		}
	}

	hdr := map[string][]string{}
	for k, v := range headers {
		if strings.TrimSpace(k) == "" {
			continue
		}
		hdr[strings.ToLower(strings.TrimSpace(k))] = []string{v}
	}
	if len(body) > 0 {
		if _, ok := hdr["content-type"]; !ok {
			defaultContentType := "application/json; charset=utf-8"
			if parsedPath == "/oauth/token" {
				defaultContentType = "application/x-www-form-urlencoded"
			}
			hdr["content-type"] = []string{defaultContentType}
		}
	}

	q := map[string][]string{}
	for k, values := range queryValues {
		if strings.TrimSpace(k) == "" {
			continue
		}
		if len(values) == 0 {
			continue
		}
		out := make([]string, 0, len(values))
		for _, value := range values {
			out = append(out, value)
		}
		q[k] = out
	}
	for k, v := range query {
		if strings.TrimSpace(k) == "" {
			continue
		}
		q[k] = []string{v}
	}

	return &apptheory.Context{
		Request: apptheory.Request{
			Method:  method,
			Path:    parsedPath,
			Headers: hdr,
			Query:   q,
			Body:    body,
		},
		Params: map[string]string{},
	}
}

type round10DynamoHarness struct {
	db     *round10TransactionalDB
	query  *mocks.MockQuery
	update *mocks.MockUpdateBuilder
	state  *round10QueryState
}

type round10TransactionalDB struct {
	inner *mocks.MockDB
	state *round10QueryState
}

func (db *round10TransactionalDB) Model(model any) dynamormcore.Query {
	return db.inner.Model(model)
}

func (db *round10TransactionalDB) Migrate() error {
	return db.inner.Migrate()
}

func (db *round10TransactionalDB) AutoMigrate(models ...any) error {
	return db.inner.AutoMigrate(models...)
}

func (db *round10TransactionalDB) Close() error {
	return db.inner.Close()
}

func (db *round10TransactionalDB) WithContext(context.Context) dynamormcore.DB {
	return db
}

func (db *round10TransactionalDB) Transact() dynamormcore.TransactionBuilder {
	return &round10TransactionBuilder{state: db.state}
}

func (db *round10TransactionalDB) TransactWrite(_ context.Context, fn func(dynamormcore.TransactionBuilder) error) error {
	builder := &round10TransactionBuilder{state: db.state}
	if err := fn(builder); err != nil {
		return err
	}
	return builder.Execute()
}

type round10TransactionBuilder struct {
	state      *round10QueryState
	operations []round10TransactionOperation
}

type round10TransactionOperation struct {
	kind       string
	model      any
	conditions []dynamormcore.TransactCondition
}

func (b *round10TransactionBuilder) Put(any, ...dynamormcore.TransactCondition) dynamormcore.TransactionBuilder {
	return b
}

func (b *round10TransactionBuilder) Create(model any, conditions ...dynamormcore.TransactCondition) dynamormcore.TransactionBuilder {
	b.operations = append(b.operations, round10TransactionOperation{kind: "create", model: model, conditions: append([]dynamormcore.TransactCondition(nil), conditions...)})
	return b
}

func (b *round10TransactionBuilder) Update(model any, _ []string, conditions ...dynamormcore.TransactCondition) dynamormcore.TransactionBuilder {
	b.operations = append(b.operations, round10TransactionOperation{kind: "update", model: model, conditions: append([]dynamormcore.TransactCondition(nil), conditions...)})
	return b
}

func (b *round10TransactionBuilder) UpdateWithBuilder(model any, updateFn func(dynamormcore.UpdateBuilder) error, conditions ...dynamormcore.TransactCondition) dynamormcore.TransactionBuilder {
	if updateFn != nil {
		_ = updateFn(&round10TransactionUpdateBuilder{})
	}
	b.operations = append(b.operations, round10TransactionOperation{kind: "update", model: model, conditions: append([]dynamormcore.TransactCondition(nil), conditions...)})
	return b
}

func (b *round10TransactionBuilder) Delete(model any, conditions ...dynamormcore.TransactCondition) dynamormcore.TransactionBuilder {
	b.operations = append(b.operations, round10TransactionOperation{
		kind:       "delete",
		model:      model,
		conditions: append([]dynamormcore.TransactCondition(nil), conditions...),
	})
	return b
}

func (b *round10TransactionBuilder) ConditionCheck(model any, conditions ...dynamormcore.TransactCondition) dynamormcore.TransactionBuilder {
	b.operations = append(b.operations, round10TransactionOperation{
		kind:       "condition_check",
		model:      model,
		conditions: append([]dynamormcore.TransactCondition(nil), conditions...),
	})
	return b
}

func (b *round10TransactionBuilder) WithContext(context.Context) dynamormcore.TransactionBuilder {
	return b
}

func (b *round10TransactionBuilder) Execute() error {
	if b.state.sharedMu != nil {
		b.state.sharedMu.Lock()
		defer b.state.sharedMu.Unlock()
	}
	if b.state.transactionErrorOnce != nil {
		err := b.state.transactionErrorOnce
		b.state.transactionErrorOnce = nil
		return err
	}
	if len(b.state.transactionErrors) > 0 {
		err := b.state.transactionErrors[0]
		b.state.transactionErrors = b.state.transactionErrors[1:]
		if err != nil {
			return err
		}
	}
	for idx, operation := range b.operations {
		if !round10TransactionConditionsMet(b.state, operation.model, operation.conditions) {
			return &dynamormerrors.TransactionError{
				Err:            dynamormerrors.ErrConditionFailed,
				Operation:      operation.kind,
				OperationIndex: idx,
				Reason:         "ConditionalCheckFailed",
			}
		}
	}

	for idx, operation := range b.operations {
		var err error
		switch operation.kind {
		case "create":
			if b.state.createErrorOnce != nil {
				err, b.state.createErrorOnce = b.state.createErrorOnce, nil
			}
		case "update":
			if b.state.updateErrorOnce != nil {
				err, b.state.updateErrorOnce = b.state.updateErrorOnce, nil
			}
		case "delete":
			if b.state.deleteErrorOnce != nil {
				err, b.state.deleteErrorOnce = b.state.deleteErrorOnce, nil
			}
		}
		if err != nil {
			return &dynamormerrors.TransactionError{Err: err, Operation: operation.kind, OperationIndex: idx, Reason: err.Error()}
		}
	}

	for idx, operation := range b.operations {
		if !round10TransactionApplyOperation(b.state, operation) {
			return &dynamormerrors.TransactionError{
				Err:            dynamormerrors.ErrConditionFailed,
				Operation:      operation.kind,
				OperationIndex: idx,
				Reason:         "ConditionalCheckFailed",
			}
		}
	}

	return nil
}

func (b *round10TransactionBuilder) ExecuteWithContext(context.Context) error {
	return b.Execute()
}

func round10TransactionConditionsMet(state *round10QueryState, model any, conditions []dynamormcore.TransactCondition) bool {
	for _, condition := range conditions {
		switch condition.Kind {
		case dynamormcore.TransactConditionKindPrimaryKeyExists:
			if !round10TransactionModelExists(state, model) {
				return false
			}
		case dynamormcore.TransactConditionKindPrimaryKeyNotExists:
			if round10TransactionModelExists(state, model) {
				return false
			}
		case dynamormcore.TransactConditionKindField:
			if !round10TransactionFieldConditionMet(state, model, condition) {
				return false
			}
		case dynamormcore.TransactConditionKindVersionEquals:
			if !round10TransactionVersionConditionMet(state, model, condition.Value) {
				return false
			}
		case dynamormcore.TransactConditionKindExpression:
			if !round10TransactionExpressionConditionMet(state, model, condition) {
				return false
			}
		}
	}
	return true
}

func round10TransactionExpressionConditionMet(state *round10QueryState, model any, condition dynamormcore.TransactCondition) bool {
	typed, ok := model.(*storagemodels.RefreshToken)
	if !ok || typed == nil {
		return true
	}
	stored, exists := state.refreshTokensByToken[typed.Token]
	if !exists {
		return false
	}
	expression := condition.Expression
	if strings.Contains(expression, "familyID") && stored.FamilyID != "" {
		return false
	}
	if strings.Contains(expression, "retryRedeemedAt") && !stored.RetryRedeemedAt.IsZero() {
		return false
	}
	if strings.Contains(expression, "version") {
		want, _ := condition.Values[":expectedVersion"].(int)
		if stored.Version != want {
			return false
		}
	}
	if strings.Contains(expression, "revoked") && stored.Revoked {
		return false
	}
	return true
}

func round10TransactionModelExists(state *round10QueryState, model any) bool {
	switch typed := model.(type) {
	case *storagemodels.WebAuthnCredential:
		if typed == nil {
			return false
		}
		credential := round10CanonicalizeWebAuthnCredential(*typed)
		if _, ok := state.webAuthnCredentialByID[credential.ID]; ok {
			return true
		}
		for _, creds := range state.webAuthnCredentialsByUser {
			for _, candidate := range creds {
				if candidate.ID == credential.ID {
					return true
				}
			}
		}
		return false
	case *storagemodels.WalletCredential:
		if typed == nil {
			return false
		}
		username := strings.TrimSpace(typed.Username)
		address := strings.ToLower(strings.TrimSpace(typed.Address))
		if _, ok := state.walletCredentialsByAddress[address]; ok {
			return true
		}
		for _, wallet := range state.walletCredentialsByUser[username] {
			if strings.EqualFold(wallet.Address, address) {
				return true
			}
		}
		return false
	case *storagemodels.AuthorizationCode:
		if typed == nil {
			return false
		}
		code := strings.TrimSpace(typed.Code)
		if _, ok := state.authorizationCodesByCode[code]; ok {
			return true
		}
		return !state.notFoundPKs["AUTHCODE#"+code]
	case *storagemodels.RefreshToken:
		if typed == nil {
			return false
		}
		_, ok := state.refreshTokensByToken[strings.TrimSpace(typed.Token)]
		return ok
	case *storagemodels.OAuthDeviceSession:
		if typed == nil {
			return false
		}
		_, ok := state.oauthDeviceSessionsByHash[strings.TrimSpace(typed.DeviceCodeHash)]
		return ok
	case *storagemodels.OAuthRefreshAuthority:
		_, ok := state.refreshAuthoritiesByKey[typed.PK+"#"+typed.SK]
		return ok
	case *storagemodels.OAuthRefreshSuccessorArtifact:
		_, ok := state.refreshArtifactsByKey[typed.PK+"#"+typed.SK]
		return ok
	case *storagemodels.OAuthRefreshWalkBudget:
		_, ok := state.refreshWalkBudgetsByKey[typed.PK+"#"+typed.SK]
		return ok
	default:
		return false
	}
}

func round10TransactionApplyOperation(state *round10QueryState, operation round10TransactionOperation) bool {
	switch operation.kind {
	case "create", "update":
		switch typed := operation.model.(type) {
		case *storagemodels.RefreshToken:
			if typed == nil {
				return false
			}
			if state.refreshTokensByToken == nil {
				state.refreshTokensByToken = map[string]storagemodels.RefreshToken{}
			}
			updated := *typed
			if operation.kind == "update" {
				updated.Version = state.refreshTokensByToken[typed.Token].Version + 1
			}
			state.refreshTokensByToken[typed.Token] = updated
			return true
		case *storagemodels.OAuthDeviceSession:
			if typed == nil {
				return false
			}
			if state.oauthDeviceSessionsByHash == nil {
				state.oauthDeviceSessionsByHash = map[string]storagemodels.OAuthDeviceSession{}
			}
			state.oauthDeviceSessionsByHash[typed.DeviceCodeHash] = *typed
			return true
		case *storagemodels.OAuthRefreshAuthority:
			if state.refreshAuthoritiesByKey == nil {
				state.refreshAuthoritiesByKey = map[string]storagemodels.OAuthRefreshAuthority{}
			}
			updated := *typed
			if operation.kind == "update" {
				updated.Revision = state.refreshAuthoritiesByKey[typed.PK+"#"+typed.SK].Revision + 1
			}
			state.refreshAuthoritiesByKey[typed.PK+"#"+typed.SK] = updated
			return true
		case *storagemodels.OAuthRefreshSuccessorArtifact:
			if state.refreshArtifactsByKey == nil {
				state.refreshArtifactsByKey = map[string]storagemodels.OAuthRefreshSuccessorArtifact{}
			}
			state.refreshArtifactsByKey[typed.PK+"#"+typed.SK] = *typed
			return true
		case *storagemodels.OAuthRefreshWalkBudget:
			if state.refreshWalkBudgetsByKey == nil {
				state.refreshWalkBudgetsByKey = map[string]storagemodels.OAuthRefreshWalkBudget{}
			}
			updated := *typed
			if operation.kind == "update" {
				updated.Version = state.refreshWalkBudgetsByKey[typed.PK+"#"+typed.SK].Version + 1
			}
			state.refreshWalkBudgetsByKey[typed.PK+"#"+typed.SK] = updated
			return true
		default:
			return false
		}
	case "delete":
		if code, ok := operation.model.(*storagemodels.AuthorizationCode); ok && code != nil {
			delete(state.authorizationCodesByCode, code.Code)
			return true
		}
		return round10TransactionDeleteModel(state, operation.model)
	case "condition_check":
		return true
	default:
		return false
	}
}

func round10TransactionFieldConditionMet(state *round10QueryState, model any, condition dynamormcore.TransactCondition) bool {
	if condition.Operator != "=" {
		return true
	}
	switch typed := model.(type) {
	case *storagemodels.OAuthDeviceSession:
		want, ok := condition.Value.(string)
		if !ok || typed == nil {
			return false
		}
		stored, ok := state.oauthDeviceSessionsByHash[typed.DeviceCodeHash]
		return ok && condition.Field == "Status" && stored.Status == want
	case *storagemodels.RefreshToken:
		if typed == nil {
			return false
		}
		stored, ok := state.refreshTokensByToken[typed.Token]
		if !ok {
			return false
		}
		switch condition.Field {
		case "Revoked":
			want, ok := condition.Value.(bool)
			return ok && stored.Revoked == want
		case "Current":
			want, ok := condition.Value.(bool)
			return ok && stored.Current == want
		}
	}
	return true
}

func round10TransactionVersionConditionMet(state *round10QueryState, model any, value any) bool {
	want, ok := value.(int64)
	if !ok {
		return false
	}
	switch typed := model.(type) {
	case *storagemodels.RefreshToken:
		stored, exists := state.refreshTokensByToken[typed.Token]
		return exists && strconv.Itoa(stored.Version) == strconv.FormatInt(want, 10)
	case *storagemodels.OAuthRefreshAuthority:
		stored, exists := state.refreshAuthoritiesByKey[typed.PK+"#"+typed.SK]
		return exists && strconv.Itoa(stored.Revision) == strconv.FormatInt(want, 10)
	case *storagemodels.OAuthRefreshWalkBudget:
		stored, exists := state.refreshWalkBudgetsByKey[typed.PK+"#"+typed.SK]
		return exists && strconv.Itoa(stored.Version) == strconv.FormatInt(want, 10)
	default:
		return false
	}
}

type round10TransactionUpdateBuilder struct{}

func (b *round10TransactionUpdateBuilder) Set(string, any) dynamormcore.UpdateBuilder { return b }
func (b *round10TransactionUpdateBuilder) SetIfNotExists(string, any, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *round10TransactionUpdateBuilder) Add(string, any) dynamormcore.UpdateBuilder    { return b }
func (b *round10TransactionUpdateBuilder) Increment(string) dynamormcore.UpdateBuilder   { return b }
func (b *round10TransactionUpdateBuilder) Decrement(string) dynamormcore.UpdateBuilder   { return b }
func (b *round10TransactionUpdateBuilder) Remove(string) dynamormcore.UpdateBuilder      { return b }
func (b *round10TransactionUpdateBuilder) Delete(string, any) dynamormcore.UpdateBuilder { return b }
func (b *round10TransactionUpdateBuilder) AppendToList(string, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *round10TransactionUpdateBuilder) PrependToList(string, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *round10TransactionUpdateBuilder) RemoveFromListAt(string, int) dynamormcore.UpdateBuilder {
	return b
}
func (b *round10TransactionUpdateBuilder) SetListElement(string, int, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *round10TransactionUpdateBuilder) Condition(string, string, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *round10TransactionUpdateBuilder) OrCondition(string, string, any) dynamormcore.UpdateBuilder {
	return b
}
func (b *round10TransactionUpdateBuilder) ConditionExists(string) dynamormcore.UpdateBuilder {
	return b
}
func (b *round10TransactionUpdateBuilder) ConditionNotExists(string) dynamormcore.UpdateBuilder {
	return b
}
func (b *round10TransactionUpdateBuilder) ConditionVersion(int64) dynamormcore.UpdateBuilder {
	return b
}
func (b *round10TransactionUpdateBuilder) ReturnValues(string) dynamormcore.UpdateBuilder { return b }
func (b *round10TransactionUpdateBuilder) Execute() error                                 { return nil }
func (b *round10TransactionUpdateBuilder) ExecuteWithResult(any) error                    { return nil }

func round10TransactionDeleteModel(state *round10QueryState, model any) bool {
	switch typed := model.(type) {
	case *storagemodels.WebAuthnCredential:
		if typed == nil {
			return false
		}
		credential := round10CanonicalizeWebAuthnCredential(*typed)
		if !round10TransactionModelExists(state, typed) {
			return false
		}
		round10DeleteWebAuthnCredential(state, credential.ID)
		return true
	case *storagemodels.WalletCredential:
		if typed == nil {
			return false
		}
		username := strings.TrimSpace(typed.Username)
		address := strings.ToLower(strings.TrimSpace(typed.Address))
		if !round10TransactionModelExists(state, typed) {
			return false
		}
		delete(state.walletCredentialsByAddress, address)
		wallets := state.walletCredentialsByUser[username]
		filtered := wallets[:0]
		for _, wallet := range wallets {
			if strings.EqualFold(wallet.Address, address) {
				continue
			}
			filtered = append(filtered, wallet)
		}
		if len(filtered) == 0 {
			delete(state.walletCredentialsByUser, username)
		} else {
			state.walletCredentialsByUser[username] = filtered
		}
		return true
	default:
		return false
	}
}

func round10NewDynamoHarness(t *testing.T, state *round10QueryState) *round10DynamoHarness {
	t.Helper()

	if state == nil {
		state = &round10QueryState{}
	}

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdate := new(mocks.MockUpdateBuilder)

	// DB wiring
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Run(func(args mock.Arguments) {
		state.reset()
		state.model = args.Get(0)
	}).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()

	// Chainable query methods (loose matching for unit tests)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Run(func(args mock.Arguments) {
		state.wheres = append(state.wheres, round10Where{
			field: args.String(0),
			op:    args.String(1),
			value: args.Get(2),
		})
	}).Maybe()
	mockQuery.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrFilter", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Run(func(args mock.Arguments) {
		switch value := args.Get(0).(type) {
		case int:
			state.limit = value
		case int32:
			state.limit = int(value)
		case int64:
			state.limit = int(value)
		}
	}).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Run(func(args mock.Arguments) {
		state.orderField, _ = args.Get(0).(string)
		state.orderDirection, _ = args.Get(1).(string)
	}).Maybe()
	mockQuery.On("ConsistentRead").Return(mockQuery).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("IfNotExists").Return(mockQuery).Maybe()
	mockQuery.On("IfExists").Return(mockQuery).Maybe()
	mockQuery.On("BatchGet", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(1)
		switch out := dest.(type) {
		case *[]*storagemodels.Status:
			keys, _ := args.Get(0).([]any)
			results := make([]*storagemodels.Status, 0, len(keys))
			for _, rawKey := range keys {
				keyModel, ok := rawKey.(*storagemodels.Status)
				if !ok {
					continue
				}
				statusID := strings.TrimPrefix(keyModel.PK, "status#")
				if status, ok := state.statusByID[statusID]; ok {
					statusCopy := status
					results = append(results, &statusCopy)
				}
			}
			*out = results
		}
	}).Maybe()

	// UpdateBuilder support
	mockQuery.On("UpdateBuilder").Return(mockUpdate).Maybe()
	mockUpdate.On("Set", mock.Anything, mock.Anything).Return(mockUpdate).Run(func(args mock.Arguments) {
		if state.sets == nil {
			state.sets = map[string]any{}
		}
		state.sets[args.String(0)] = args.Get(1)
	}).Maybe()
	mockUpdate.On("Add", mock.Anything, mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("SetIfNotExists", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("Remove", mock.Anything).Return(mockUpdate).Run(func(args mock.Arguments) {
		if state.sets == nil {
			state.sets = map[string]any{}
		}
		state.sets[args.String(0)] = nil
	}).Maybe()
	mockUpdate.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("ConditionExists", mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("ConditionNotExists", mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("ConditionVersion", mock.Anything).Return(mockUpdate).Maybe()
	// O(1) instance-count maintenance uses ExecuteWithResult for the domain
	// release path (see instance_counts.go); report a drained domain so the
	// empty-domain delete + global decrement run.
	mockUpdate.On("ExecuteWithResult", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		if dest, ok := args.Get(0).(*storagemodels.DomainCounter); ok {
			dest.Value = 0
		}
	}).Maybe()
	if state.executeErrorOnce != nil {
		mockUpdate.On("Execute").Return(state.executeErrorOnce).Once()
	} else if state.updateErrorOnce != nil {
		mockUpdate.On("Execute").Return(state.updateErrorOnce).Once()
	}
	mockUpdate.On("Execute").Return(nil).Run(func(_ mock.Arguments) {
		switch state.model.(type) {
		case *storagemodels.AgentGovernanceState:
			m, _ := state.model.(*storagemodels.AgentGovernanceState)
			if m == nil {
				return
			}
			if state.agentGovernanceByUsername == nil {
				state.agentGovernanceByUsername = map[string]storagemodels.AgentGovernanceState{}
			}
			state.agentGovernanceByUsername[strings.ToLower(strings.TrimSpace(m.Username))] = *m
		case *storagemodels.OAuthClient:
			pk, ok := state.whereString("PK")
			if !ok || !strings.HasPrefix(pk, "OAUTH_CLIENT#") {
				return
			}
			clientID := strings.TrimPrefix(pk, "OAUTH_CLIENT#")
			client, exists := state.oauthClientsByID[clientID]
			if !exists {
				return
			}
			if secret, ok := state.sets["ClientSecret"].(string); ok {
				client.ClientSecret = secret
			}
			if previousSecret, ok := state.sets["PreviousClientSecret"].(string); ok {
				client.PreviousClientSecret = previousSecret
			}
			if graceExpiresAt, ok := state.sets["PreviousClientSecretGraceExpiresAt"].(time.Time); ok {
				client.PreviousClientSecretGraceExpiresAt = graceExpiresAt
			}
			if rotatedAt, ok := state.sets["SecretRotatedAt"].(time.Time); ok {
				client.SecretRotatedAt = rotatedAt
			}
			if rotatedBy, ok := state.sets["SecretRotatedBy"].(string); ok {
				client.SecretRotatedBy = rotatedBy
			}
			if updatedAt, ok := state.sets["UpdatedAt"].(time.Time); ok {
				client.UpdatedAt = updatedAt
			}
			state.oauthClientsByID[clientID] = client
		case *storagemodels.WebAuthnCredential:
			model, _ := state.model.(*storagemodels.WebAuthnCredential)
			if model == nil {
				return
			}

			credential := round10CanonicalizeWebAuthnCredential(*model)
			if existing, ok := state.webAuthnCredentialByID[credential.ID]; ok {
				credential = round10CanonicalizeWebAuthnCredential(existing)
			}
			if name, ok := state.sets["Name"].(string); ok {
				credential.Name = name
			}
			if signCount, ok := state.sets["SignCount"].(uint32); ok {
				credential.SignCount = signCount
			}
			if cloneWarning, ok := state.sets["CloneWarning"].(bool); ok {
				credential.CloneWarning = cloneWarning
			}
			if backupState, ok := state.sets["BackupState"].(bool); ok {
				credential.BackupState = backupState
			}
			if lastUsedAt, ok := state.sets["LastUsedAt"].(time.Time); ok {
				credential.LastUsedAt = lastUsedAt
			}
			round10UpsertWebAuthnCredential(state, credential)
		case *storagemodels.PasskeyRegistrationProof:
			model, _ := state.model.(*storagemodels.PasskeyRegistrationProof)
			if model == nil {
				return
			}

			proof := *model
			if existing, ok := state.passkeyRegistrationProofsByID[proof.ID]; ok {
				proof = existing
			}
			if consumed, ok := state.sets["Consumed"].(bool); ok {
				proof.Consumed = consumed
			}
			if consumedAt, ok := state.sets["ConsumedAt"].(time.Time); ok {
				proof.ConsumedAt = consumedAt
			}
			if state.passkeyRegistrationProofsByID == nil {
				state.passkeyRegistrationProofsByID = map[string]storagemodels.PasskeyRegistrationProof{}
			}
			state.passkeyRegistrationProofsByID[proof.ID] = proof
		}
	}).Maybe()

	// Mutations
	if state.createErrorOnce != nil {
		mockQuery.On("Create").Return(state.createErrorOnce).Once()
	}
	if state.createOrUpdateErrorOnce != nil {
		mockQuery.On("CreateOrUpdate").Return(state.createOrUpdateErrorOnce).Once()
	}
	mockQuery.On("CreateOrUpdate").Return(nil).Maybe()
	mockQuery.On("Create").Return(nil).Run(func(_ mock.Arguments) {
		switch m := state.model.(type) {
		case *storagemodels.User:
			if m == nil {
				return
			}
			if state.usersByUsername == nil {
				state.usersByUsername = map[string]storagemodels.User{}
			}
			state.usersByUsername[strings.ToLower(strings.TrimSpace(m.Username))] = *m
		case *storagemodels.Actor:
			if m == nil {
				return
			}
			if state.actorsByUser == nil {
				state.actorsByUser = map[string]storagemodels.Actor{}
			}
			state.actorsByUser[strings.ToLower(strings.TrimSpace(m.Username))] = *m
		case *storagemodels.AgentGovernanceState:
			if m == nil {
				return
			}
			if state.agentGovernanceByUsername == nil {
				state.agentGovernanceByUsername = map[string]storagemodels.AgentGovernanceState{}
			}
			state.agentGovernanceByUsername[strings.ToLower(strings.TrimSpace(m.Username))] = *m
		case *storagemodels.QuotePermissions:
			if m == nil {
				return
			}
			if state.quotePermissionsByUser == nil {
				state.quotePermissionsByUser = map[string]storagemodels.QuotePermissions{}
			}
			state.quotePermissionsByUser[m.Username] = *m
		case *storagemodels.OAuthClient:
			if m == nil {
				return
			}
			if state.oauthClientsByID == nil {
				state.oauthClientsByID = map[string]storagemodels.OAuthClient{}
			}
			state.oauthClientsByID[m.ClientID] = *m
		case *storagemodels.OAuthState:
			if m == nil {
				return
			}
			if state.oauthStates == nil {
				state.oauthStates = map[string]storagemodels.OAuthState{}
			}
			state.oauthStates[m.State] = *m
		case *storagemodels.AuthorizationCode:
			if m == nil {
				return
			}
			if state.authorizationCodesByCode == nil {
				state.authorizationCodesByCode = map[string]storagemodels.AuthorizationCode{}
			}
			state.authorizationCodesByCode[m.Code] = *m
		case *storagemodels.RefreshToken:
			if m == nil || strings.TrimSpace(m.Token) == "" {
				return
			}
			if state.refreshTokensByToken == nil {
				state.refreshTokensByToken = map[string]storagemodels.RefreshToken{}
			}
			state.refreshTokensByToken[m.Token] = *m
		case *storagemodels.OAuthDeviceSession:
			if m == nil {
				return
			}
			if state.oauthDeviceSessionsByHash == nil {
				state.oauthDeviceSessionsByHash = map[string]storagemodels.OAuthDeviceSession{}
			}
			if state.oauthDeviceSessionsByUserCode == nil {
				state.oauthDeviceSessionsByUserCode = map[string]storagemodels.OAuthDeviceSession{}
			}
			state.oauthDeviceSessionsByHash[m.DeviceCodeHash] = *m
			state.oauthDeviceSessionsByUserCode[m.UserCode] = *m
		case *storagemodels.WebAuthnCredential:
			if m == nil {
				return
			}
			round10UpsertWebAuthnCredential(state, *m)
		case *storagemodels.AuthAuditLog:
			if m == nil {
				return
			}
			if state.auditLogsByUser == nil {
				state.auditLogsByUser = map[string][]*storagemodels.AuthAuditLog{}
			}
			username := strings.TrimSpace(m.Username)
			state.auditLogsByUser[username] = append(state.auditLogsByUser[username], m)
		case *storagemodels.RevokedAccessToken:
			if m == nil {
				return
			}
			jti := strings.TrimSpace(m.JTI)
			if jti == "" {
				return
			}
			if state.revokedAccessTokensByJTI == nil {
				state.revokedAccessTokensByJTI = map[string]storagemodels.RevokedAccessToken{}
			}
			state.revokedAccessTokensByJTI[jti] = *m
		case *storagemodels.AgentAccessLease:
			if m == nil {
				return
			}
			if state.agentAccessLeasesByKey == nil {
				state.agentAccessLeasesByKey = map[string]storagemodels.AgentAccessLease{}
			}
			state.agentAccessLeasesByKey[m.PK+"#"+m.SK] = *m
		case *storagemodels.AgentAccessLeaseChallenge:
			if m == nil {
				return
			}
			if state.agentAccessChallengesByID == nil {
				state.agentAccessChallengesByID = map[string]storagemodels.AgentAccessLeaseChallenge{}
			}
			state.agentAccessChallengesByID[m.ID] = *m
		case *storagemodels.AgentKeyChallenge:
			if m == nil {
				return
			}
			if state.agentKeyChallengesByID == nil {
				state.agentKeyChallengesByID = map[string]storagemodels.AgentKeyChallenge{}
			}
			state.agentKeyChallengesByID[m.ID] = *m
		case *storagemodels.WalletCredential:
			if m == nil {
				return
			}
			address := strings.ToLower(strings.TrimSpace(m.Address))
			username := strings.TrimSpace(m.Username)
			if username == "" || address == "" {
				return
			}
			if state.walletCredentialsByAddress == nil {
				state.walletCredentialsByAddress = map[string]storagemodels.WalletCredential{}
			}
			if state.walletCredentialsByUser == nil {
				state.walletCredentialsByUser = map[string][]storagemodels.WalletCredential{}
			}
			state.walletCredentialsByAddress[address] = *m
			wallets := state.walletCredentialsByUser[username]
			replaced := false
			for i := range wallets {
				if strings.EqualFold(wallets[i].Address, address) {
					wallets[i] = *m
					replaced = true
					break
				}
			}
			if !replaced {
				wallets = append(wallets, *m)
			}
			state.walletCredentialsByUser[username] = wallets
		}
	}).Maybe()

	if state.deleteErrorOnce != nil {
		mockQuery.On("Delete").Return(state.deleteErrorOnce).Once()
	}
	mockQuery.On("Delete").Return(nil).Run(func(_ mock.Arguments) {
		switch state.model.(type) {
		case *storagemodels.WebAuthnCredential:
			credentialID := ""
			if model, ok := state.model.(*storagemodels.WebAuthnCredential); ok && model != nil && strings.TrimSpace(model.ID) != "" {
				credentialID = strings.TrimSpace(model.ID)
			}
			if credentialID == "" {
				if sk, ok := state.whereString("SK"); ok && strings.HasPrefix(sk, "WEBAUTHN_CRED#") {
					credentialID = strings.TrimPrefix(sk, "WEBAUTHN_CRED#")
				}
			}
			round10DeleteWebAuthnCredential(state, credentialID)
		case *storagemodels.WalletCredential:
			pk, okPK := state.whereString("PK")
			sk, okSK := state.whereString("SK")
			if !okPK || !okSK || !strings.HasPrefix(pk, "USER#") || !strings.HasPrefix(sk, "WALLET#") {
				return
			}
			username := strings.TrimPrefix(pk, "USER#")
			address := strings.ToLower(strings.TrimPrefix(sk, "WALLET#"))
			delete(state.walletCredentialsByAddress, address)
			wallets := state.walletCredentialsByUser[username]
			filtered := wallets[:0]
			for _, wallet := range wallets {
				if strings.EqualFold(wallet.Address, address) {
					continue
				}
				filtered = append(filtered, wallet)
			}
			if len(filtered) == 0 {
				delete(state.walletCredentialsByUser, username)
				return
			}
			state.walletCredentialsByUser[username] = filtered
		default:
			if state.refreshTokensByToken == nil {
				return
			}
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "REFRESHTOKEN#") {
				delete(state.refreshTokensByToken, strings.TrimPrefix(pk, "REFRESHTOKEN#"))
			}
		}
	}).Maybe()

	if state.updateErrorOnce != nil {
		mockQuery.On("Update", mock.Anything).Return(state.updateErrorOnce).Once()
	}
	mockQuery.On("Update", mock.Anything).Return(nil).Run(func(_ mock.Arguments) {
		switch m := state.model.(type) {
		case *storagemodels.User:
			if m == nil {
				return
			}
			if state.usersByUsername == nil {
				state.usersByUsername = map[string]storagemodels.User{}
			}
			state.usersByUsername[strings.ToLower(strings.TrimSpace(m.Username))] = *m
		case *storagemodels.Actor:
			if m == nil {
				return
			}
			if state.actorsByUser == nil {
				state.actorsByUser = map[string]storagemodels.Actor{}
			}
			state.actorsByUser[strings.ToLower(strings.TrimSpace(m.Username))] = *m
		case *storagemodels.AgentGovernanceState:
			if m == nil {
				return
			}
			if state.agentGovernanceByUsername == nil {
				state.agentGovernanceByUsername = map[string]storagemodels.AgentGovernanceState{}
			}
			state.agentGovernanceByUsername[strings.ToLower(strings.TrimSpace(m.Username))] = *m
		case *storagemodels.QuotePermissions:
			if m == nil {
				return
			}
			if state.quotePermissionsByUser == nil {
				state.quotePermissionsByUser = map[string]storagemodels.QuotePermissions{}
			}
			state.quotePermissionsByUser[m.Username] = *m
		case *storagemodels.RefreshToken:
			if m == nil || strings.TrimSpace(m.Token) == "" {
				return
			}
			if state.refreshTokensByToken == nil {
				state.refreshTokensByToken = map[string]storagemodels.RefreshToken{}
			}
			state.refreshTokensByToken[m.Token] = *m
		case *storagemodels.OAuthDeviceSession:
			if m == nil {
				return
			}
			if state.oauthDeviceSessionsByHash == nil {
				state.oauthDeviceSessionsByHash = map[string]storagemodels.OAuthDeviceSession{}
			}
			if state.oauthDeviceSessionsByUserCode == nil {
				state.oauthDeviceSessionsByUserCode = map[string]storagemodels.OAuthDeviceSession{}
			}
			state.oauthDeviceSessionsByHash[m.DeviceCodeHash] = *m
			state.oauthDeviceSessionsByUserCode[m.UserCode] = *m
		case *storagemodels.WalletChallenge:
			if m == nil {
				return
			}
			if state.walletChallengesByID == nil {
				state.walletChallengesByID = map[string]storagemodels.WalletChallenge{}
			}
			state.walletChallengesByID[m.ID] = *m
		case *storagemodels.WebAuthnChallenge:
			if m == nil {
				return
			}
			if state.webAuthnChallengesByID == nil {
				state.webAuthnChallengesByID = map[string]storagemodels.WebAuthnChallenge{}
			}
			state.webAuthnChallengesByID[m.Challenge] = *m
		case *storagemodels.WebAuthnCredential:
			if m == nil {
				return
			}
			round10UpsertWebAuthnCredential(state, *m)
		case *storagemodels.PasskeyRegistrationProof:
			if m == nil {
				return
			}
			if state.passkeyRegistrationProofsByID == nil {
				state.passkeyRegistrationProofsByID = map[string]storagemodels.PasskeyRegistrationProof{}
			}
			state.passkeyRegistrationProofsByID[m.ID] = *m
		}
	}).Maybe()
	mockQuery.On("Count").Return(int64(2), nil).Maybe()

	// Query executions
	if state.forceVapidNotFound {
		mockQuery.On("First", mock.AnythingOfType("*models.VAPIDKeyRecord")).Return(dynamormerrors.ErrItemNotFound).Maybe()
	}

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		if _, ok := dest.(*storagemodels.QuotePermissions); !ok {
			return false
		}
		pk, okPK := state.whereString("PK")
		sk, okSK := state.whereString("SK")
		if !okPK || !okSK || !strings.HasPrefix(pk, "USER#") || sk != "QUOTE_PERMISSIONS" {
			return false
		}
		if _, hasInjectedError := state.firstErrorPK[pk]; hasInjectedError {
			return false
		}
		_, exists := state.quotePermissionsByUser[strings.TrimPrefix(pk, "USER#")]
		return !exists
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()

	if len(state.notFoundPKSK) > 0 {
		mockQuery.On("First", mock.MatchedBy(func(_ any) bool {
			pk, okPK := state.whereString("PK")
			sk, okSK := state.whereString("SK")
			if !okPK || !okSK {
				return false
			}
			return state.notFoundPKSK[pk+"#"+sk]
		})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	}

	if len(state.notFoundPKs) > 0 || len(state.notFoundGSI3PK) > 0 {
		mockQuery.On("First", mock.MatchedBy(func(_ any) bool {
			if pk, ok := state.whereString("PK"); ok && state.notFoundPKs[pk] {
				return true
			}
			if gsi3pk, ok := state.whereString("gsi3PK"); ok && state.notFoundGSI3PK[gsi3pk] {
				return true
			}
			return false
		})).Return(dynamormerrors.ErrItemNotFound).Maybe()
	}

	for pk, err := range state.firstErrorPK {
		pk := pk
		err := err
		mockQuery.On("First", mock.MatchedBy(func(_ any) bool {
			value, ok := state.whereString("PK")
			return ok && value == pk
		})).Return(err).Maybe()
	}

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		if _, ok := dest.(*storagemodels.StatusMetadata); !ok {
			return false
		}
		pk, okPK := state.whereString("PK")
		if !okPK || !strings.HasPrefix(pk, "STATUS_META#") {
			return false
		}
		statusID := strings.TrimPrefix(pk, "STATUS_META#")
		_, exists := state.statusMetadataByStatus[statusID]
		return !exists
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()

	for gsi3pk, err := range state.firstErrorGSI3PK {
		gsi3pk := gsi3pk
		err := err
		mockQuery.On("First", mock.MatchedBy(func(_ any) bool {
			value, ok := state.whereString("gsi3PK")
			return ok && value == gsi3pk
		})).Return(err).Maybe()
	}

	for typeName, err := range state.firstErrorByType {
		typeName := typeName
		err := err
		mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
			return dest != nil && reflect.TypeOf(dest).String() == typeName
		})).Return(err).Once()
	}

	if state.firstErrorOnce != nil {
		mockQuery.On("First", mock.Anything).Return(state.firstErrorOnce).Once()
	}

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		if _, ok := dest.(*storagemodels.InstanceDomainBlock); !ok {
			return false
		}
		pk, okPK := state.whereString("PK")
		sk, okSK := state.whereString("SK")
		if !okPK || !okSK {
			return false
		}
		if !strings.HasPrefix(pk, "DOMAIN_BLOCK#") || !strings.HasPrefix(sk, "DOMAIN_BLOCK#") {
			return false
		}
		domain := strings.TrimPrefix(pk, "DOMAIN_BLOCK#")
		for _, block := range state.domainBlocks {
			if block.Domain == domain || strings.TrimPrefix(block.PK, "DOMAIN_BLOCK#") == domain {
				return false
			}
		}
		return true
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		if _, ok := dest.(*storagemodels.FederationInstance); !ok {
			return false
		}
		pk, okPK := state.whereString("PK")
		sk, okSK := state.whereString("SK")
		if !okPK || !okSK {
			return false
		}
		if !strings.HasPrefix(pk, "INSTANCE#") || !strings.HasPrefix(sk, "INSTANCE#") {
			return false
		}
		domain := strings.TrimPrefix(pk, "INSTANCE#")
		if _, ok := state.federationInstancesByDomain[domain]; ok {
			return false
		}
		for _, instance := range state.federationInstances {
			if instance.Domain == domain {
				return false
			}
		}
		return true
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		if _, ok := dest.(*storagemodels.Tombstone); !ok {
			return false
		}
		pk, okPK := state.whereString("PK")
		sk, okSK := state.whereString("SK")
		if !okPK || !okSK {
			return false
		}
		if !strings.HasPrefix(pk, "OBJECT#") || sk != "TOMBSTONE" {
			return false
		}
		objectID := strings.TrimPrefix(pk, "OBJECT#")
		if _, ok := state.tombstonesByObjectID[objectID]; ok {
			return false
		}
		return true
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		if _, ok := dest.(*storagemodels.RevokedAccessToken); !ok {
			return false
		}
		pk, okPK := state.whereString("PK")
		sk, okSK := state.whereString("SK")
		if !okPK || !okSK {
			return false
		}
		if !strings.HasPrefix(pk, "REVOKEDTOKEN#") || sk != storagemodels.SKToken {
			return false
		}
		jti := strings.TrimPrefix(pk, "REVOKEDTOKEN#")
		if state.revokedAccessTokensByJTI != nil {
			if _, ok := state.revokedAccessTokensByJTI[jti]; ok {
				return false
			}
		}
		return true
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		if _, ok := dest.(*storagemodels.QuoteRelationship); !ok {
			return false
		}
		pk, okPK := state.whereString("PK")
		sk, okSK := state.whereString("SK")
		if !okPK || !okSK {
			return false
		}
		if !strings.HasPrefix(pk, "QUOTE#") || !strings.HasPrefix(sk, "QUOTED#") {
			return false
		}
		for _, rel := range state.quoteRelationships {
			if rel.PK == pk && rel.SK == sk {
				return false
			}
		}
		return true
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		if _, ok := dest.(*storagemodels.RelationshipRecord); !ok {
			return false
		}
		pk, okPK := state.whereString("PK")
		sk, okSK := state.whereString("SK")
		if !okPK || !okSK {
			return false
		}
		if !strings.HasPrefix(pk, "FOLLOW#") || !strings.HasPrefix(sk, "FOLLOWING#") {
			return false
		}

		for _, rel := range state.relationshipRecords {
			if rel.PK == pk && rel.SK == sk {
				return false
			}
		}
		return true
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		if dest == nil {
			return false
		}
		typeName := reflect.TypeOf(dest).String()
		if typeName != "*repositories.userCoreProjection" && typeName != "*repositories.userMetadataProjection" {
			return false
		}
		if pk, ok := state.whereString("PK"); ok && state.notFoundPKs[pk] {
			return true
		}
		if gsi5sk, ok := state.whereString("gsi5SK"); ok && state.notFoundPKs["USER#"+strings.TrimSpace(gsi5sk)] {
			return true
		}
		if state.usersByUsername == nil {
			return false
		}
		return !round10HasUserForRead(state)
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		if _, ok := dest.(*storagemodels.User); !ok {
			return false
		}
		pk, ok := state.whereString("PK")
		if !ok || !strings.HasPrefix(pk, "USER#") || state.usersByUsername == nil {
			return false
		}
		username := strings.TrimPrefix(pk, "USER#")
		_, exactExists := state.usersByUsername[username]
		return !exactExists
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		if _, ok := dest.(*storagemodels.Actor); !ok {
			return false
		}
		pk, ok := state.whereString("PK")
		if !ok || !strings.HasPrefix(pk, "ACTOR#") || state.actorsByUser == nil {
			return false
		}
		username := strings.TrimPrefix(pk, "ACTOR#")
		_, exactExists := state.actorsByUser[username]
		return !exactExists
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		if _, ok := dest.(*storagemodels.AgentGovernanceState); !ok {
			return false
		}
		pk, ok := state.whereString("PK")
		if !ok || !strings.HasPrefix(pk, "USER#") {
			return false
		}
		sk, ok := state.whereString("SK")
		if !ok || sk != storagemodels.SKAgentGovernance {
			return false
		}
		if state.agentGovernanceByUsername == nil {
			return true
		}
		username := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(pk, "USER#")))
		_, exactExists := state.agentGovernanceByUsername[username]
		return !exactExists
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()

	mockQuery.On("First", mock.MatchedBy(func(dest any) bool {
		pk, okPK := state.whereString("PK")
		sk, okSK := state.whereString("SK")
		if !okPK || !okSK {
			return false
		}
		key := pk + "#" + sk
		switch dest.(type) {
		case *storagemodels.OAuthRefreshAuthority:
			_, exists := state.refreshAuthoritiesByKey[key]
			return !exists
		case *storagemodels.OAuthRefreshSuccessorArtifact:
			_, exists := state.refreshArtifactsByKey[key]
			return !exists
		case *storagemodels.OAuthRefreshWalkBudget:
			_, exists := state.refreshWalkBudgetsByKey[key]
			return !exists
		default:
			return false
		}
	})).Return(dynamormerrors.ErrItemNotFound).Maybe()

	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0)
		if round10PopulateAccountProjection(dest, state) {
			return
		}
		switch d := dest.(type) {
		case *storagemodels.QuotePermissions:
			pk, _ := state.whereString("PK")
			*d = state.quotePermissionsByUser[strings.TrimPrefix(pk, "USER#")]
		case *storagemodels.AgentInstanceConfig:
			if state.agentInstanceConfig != nil {
				*d = *state.agentInstanceConfig
				return
			}
			*d = *storagemodels.NewAgentInstanceConfig()
		case *storagemodels.User:
			username := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "USER#") {
				username = strings.TrimPrefix(pk, "USER#")
			} else if gsi5pk, ok := state.whereString("gsi5PK"); ok && strings.HasPrefix(gsi5pk, "USER_HANDLE_PREFIX#") {
				if gsi5sk, ok := state.whereString("gsi5SK"); ok {
					for candidate, user := range state.usersByUsername {
						if strings.EqualFold(strings.TrimSpace(user.Username), gsi5sk) || strings.EqualFold(strings.TrimSpace(candidate), gsi5sk) {
							*d = user
							return
						}
					}
				}
			}
			if user, ok := state.usersByUsername[username]; ok {
				*d = user
				return
			}
			for candidate, user := range state.usersByUsername {
				if strings.EqualFold(strings.TrimSpace(user.Username), username) || strings.EqualFold(strings.TrimSpace(candidate), username) {
					*d = user
					return
				}
			}
			*d = storagemodels.User{Username: username, Role: "user", Approved: true, Version: 1}
		case *storagemodels.AgentGovernanceState:
			username := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "USER#") {
				username = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(pk, "USER#")))
			}
			if governance, ok := state.agentGovernanceByUsername[username]; ok {
				*d = governance
				return
			}
			*d = storagemodels.AgentGovernanceState{Username: username}
		case *storagemodels.Actor:
			username := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "ACTOR#") {
				username = strings.TrimPrefix(pk, "ACTOR#")
			}
			if actor, ok := state.actorsByUser[username]; ok {
				*d = actor
				return
			}
			*d = storagemodels.Actor{
				Username: username,
				Actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   "https://example.com/users/" + username,
						Type: "Person",
					},
					PreferredUsername: username,
					Name:              username,
				},
				CreatedAt: time.Now().Add(-24 * time.Hour),
				UpdatedAt: time.Now().Add(-1 * time.Hour),
			}
		case *storagemodels.Object:
			if url, ok := state.whereString("URL"); ok {
				for _, obj := range state.objectList {
					if obj.URL == url {
						*d = obj
						return
					}
				}
			}
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "object#") {
				id := strings.TrimPrefix(pk, "object#")
				if obj, ok := state.objectsByID[id]; ok {
					*d = obj
					return
				}
				*d = storagemodels.Object{
					ID:      id,
					Type:    activitypub.NoteType,
					Content: "default object content",
				}
				return
			}
			if len(state.objectList) > 0 {
				*d = state.objectList[0]
				return
			}
		case *storagemodels.Article:
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "object#") {
				id := strings.TrimPrefix(pk, "object#")
				if obj, ok := state.objectsByID[id]; ok {
					*d = storagemodels.Article{
						Object:        obj,
						ContentFormat: "markdown",
					}
					return
				}
			}
		case *storagemodels.AgentKeyChallenge:
			pk, _ := state.whereString("PK")
			id := strings.TrimPrefix(pk, "AGENT_KEY_CHALLENGE#")
			if state.agentKeyChallengesByID != nil {
				if challenge, ok := state.agentKeyChallengesByID[id]; ok {
					*d = challenge
					return
				}
			}
			*d = storagemodels.AgentKeyChallenge{PK: pk, SK: "CHALLENGE", ID: id}
		case *storagemodels.AgentAccessLeaseChallenge:
			pk, _ := state.whereString("PK")
			id := strings.TrimPrefix(pk, "AGENT_ACCESS_CHALLENGE#")
			if state.agentAccessChallengesByID != nil {
				if challenge, ok := state.agentAccessChallengesByID[id]; ok {
					*d = challenge
					return
				}
			}
			*d = storagemodels.AgentAccessLeaseChallenge{PK: pk, SK: "CHALLENGE", ID: id}
		case *storagemodels.AgentAccessLease:
			pk, _ := state.whereString("PK")
			sk, _ := state.whereString("SK")
			key := pk + "#" + sk
			if state.agentAccessLeasesByKey != nil {
				if lease, ok := state.agentAccessLeasesByKey[key]; ok {
					*d = lease
					return
				}
			}
			*d = storagemodels.AgentAccessLease{PK: pk, SK: sk}
		case *storagemodels.RemoteActor:
			pk, _ := state.whereString("PK")
			if state.remoteActorsByPK != nil {
				if actor, ok := state.remoteActorsByPK[pk]; ok {
					*d = actor
					return
				}
			}
			*d = storagemodels.RemoteActor{PK: pk, SK: storagemodels.SKProfile}
		case *storagemodels.Tombstone:
			objectID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "OBJECT#") {
				objectID = strings.TrimPrefix(pk, "OBJECT#")
			}
			if tombstone, ok := state.tombstonesByObjectID[objectID]; ok {
				*d = tombstone
				return
			}
			*d = storagemodels.Tombstone{
				ID:         objectID,
				Type:       "Tombstone",
				FormerType: activitypub.NoteType,
				Deleted:    time.Now().Add(-1 * time.Hour),
				CreatedAt:  time.Now().Add(-1 * time.Hour),
			}
		case *storagemodels.Notification:
			id := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "NOTIFICATION#") {
				id = strings.TrimPrefix(pk, "NOTIFICATION#")
			}
			if notif, ok := state.notificationsByID[id]; ok {
				*d = notif
				return
			}
			*d = storagemodels.Notification{
				ID:        id,
				UserID:    "alice",
				ActorID:   "alice",
				Type:      "mention",
				CreatedAt: time.Now().Add(-1 * time.Hour),
			}
		case *storagemodels.OAuthState:
			stateKey := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "OAUTH_STATE#") {
				stateKey = strings.TrimPrefix(pk, "OAUTH_STATE#")
			}
			if oauthState, ok := state.oauthStates[stateKey]; ok {
				*d = oauthState
				return
			}
			*d = storagemodels.OAuthState{
				State:       stateKey,
				ClientID:    "client-1",
				Username:    "alice",
				RedirectURI: "https://client.example/redirect",
				Scopes:      []string{"read"},
			}
		case *storagemodels.AuthorizationCode:
			code := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "AUTHCODE#") {
				code = strings.TrimPrefix(pk, "AUTHCODE#")
			}
			if authCode, ok := state.authorizationCodesByCode[code]; ok {
				*d = authCode
				return
			}
			*d = storagemodels.AuthorizationCode{
				Code:          code,
				ClientID:      "client-1",
				RedirectURI:   "https://example.com/callback",
				Username:      "alice",
				CodeChallenge: "",
				ExpiresAt:     time.Now().Add(10 * time.Minute),
				Scopes:        []string{"read", "write"},
				CreatedAt:     time.Now().Add(-1 * time.Minute),
			}
			_ = d.UpdateKeys()
		case *storagemodels.RefreshToken:
			token := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "REFRESHTOKEN#") {
				token = strings.TrimPrefix(pk, "REFRESHTOKEN#")
			}
			if refreshToken, ok := state.refreshTokensByToken[token]; ok {
				*d = refreshToken
				return
			}
			*d = storagemodels.RefreshToken{
				Token:     token,
				ClientID:  "client-1",
				Username:  "alice",
				ExpiresAt: time.Now().Add(24 * time.Hour),
				Scopes:    []string{"read", "write"},
				CreatedAt: time.Now().Add(-1 * time.Minute),
			}
			_ = d.UpdateKeys()
		case *storagemodels.OAuthRefreshAuthority:
			pk, _ := state.whereString("PK")
			sk, _ := state.whereString("SK")
			*d = state.refreshAuthoritiesByKey[pk+"#"+sk]
		case *storagemodels.OAuthRefreshSuccessorArtifact:
			pk, _ := state.whereString("PK")
			sk, _ := state.whereString("SK")
			*d = state.refreshArtifactsByKey[pk+"#"+sk]
		case *storagemodels.OAuthRefreshWalkBudget:
			pk, _ := state.whereString("PK")
			sk, _ := state.whereString("SK")
			*d = state.refreshWalkBudgetsByKey[pk+"#"+sk]
		case *storagemodels.RevokedAccessToken:
			jti := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "REVOKEDTOKEN#") {
				jti = strings.TrimPrefix(pk, "REVOKEDTOKEN#")
			}
			if state.revokedAccessTokensByJTI != nil {
				if record, ok := state.revokedAccessTokensByJTI[jti]; ok {
					*d = record
					return
				}
			}
		case *storagemodels.OAuthDeviceSession:
			deviceCodeHash := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "OAUTH_DEVICE#") {
				deviceCodeHash = strings.TrimPrefix(pk, "OAUTH_DEVICE#")
			}
			if session, ok := state.oauthDeviceSessionsByHash[deviceCodeHash]; ok {
				*d = session
				return
			}
			*d = storagemodels.OAuthDeviceSession{
				DeviceCodeHash:  deviceCodeHash,
				UserCode:        "ABCD-EFGH",
				ClientID:        "client-1",
				Scopes:          []string{"read", "write"},
				Status:          "pending",
				IntervalSeconds: oauthDevicePollIntervalSeconds,
				PollCount:       0,
				CreatedAt:       time.Now().Add(-1 * time.Minute),
				UpdatedAt:       time.Now().Add(-1 * time.Minute),
				ExpiresAt:       time.Now().Add(10 * time.Minute),
			}
			_ = d.UpdateKeys()
		case *storagemodels.SetupSession:
			sessionID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "SETUP_SESSION#") {
				sessionID = strings.TrimPrefix(pk, "SETUP_SESSION#")
			}
			if session, ok := state.setupSessionsByID[sessionID]; ok {
				*d = session
				return
			}
			now := time.Now().UTC()
			*d = storagemodels.SetupSession{
				ID:           sessionID,
				Purpose:      "bootstrap",
				WalletType:   "ethereum",
				WalletAddr:   "0xabc",
				IssuedAt:     now.Add(-1 * time.Minute),
				ExpiresAt:    now.Add(1 * time.Hour),
				InstanceLock: true,
			}
			_ = d.UpdateKeys()
		case *storagemodels.InstanceState:
			if state.instanceState != nil {
				*d = *state.instanceState
				return
			}
			defaultState := storagemodels.NewDefaultInstanceState()
			*d = *defaultState
			_ = d.UpdateKeys()
		case *storagemodels.InstanceSoulBodyBindingUsername:
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "SOUL_BODY_BINDING_USERNAME#") {
				username := strings.TrimPrefix(pk, "SOUL_BODY_BINDING_USERNAME#")
				if state.soulBodyBindingUsernames != nil {
					if binding, exists := state.soulBodyBindingUsernames[username]; exists {
						*d = binding
						return
					}
				}
			}
		case *storagemodels.InstanceSoulBodyBinding:
			if sk, ok := state.whereString("SK"); ok && strings.HasPrefix(sk, storagemodels.SKSoulBodyBindingPrefix) {
				agentID := strings.TrimPrefix(sk, storagemodels.SKSoulBodyBindingPrefix)
				if state.soulBodyBindingsByAgentID != nil {
					if binding, exists := state.soulBodyBindingsByAgentID[agentID]; exists {
						*d = binding
						return
					}
				}
			}
		case *storagemodels.RelationshipRecord:
			pk, _ := state.whereString("PK")
			sk, _ := state.whereString("SK")
			for _, rel := range state.relationshipRecords {
				if rel.PK == pk && rel.SK == sk {
					*d = rel
					return
				}
			}

			*d = storagemodels.RelationshipRecord{PK: pk, SK: sk, State: storagemodels.RelationshipPending}
		case *storagemodels.StatusMetadata:
			statusID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "STATUS_META#") {
				statusID = strings.TrimPrefix(pk, "STATUS_META#")
			}
			if metadata, ok := state.statusMetadataByStatus[statusID]; ok {
				*d = metadata
				return
			}
			*d = storagemodels.StatusMetadata{StatusID: statusID}
		case *storagemodels.QuoteRelationship:
			pk, _ := state.whereString("PK")
			sk, _ := state.whereString("SK")
			for _, rel := range state.quoteRelationships {
				if rel.PK == pk && rel.SK == sk {
					*d = rel
					return
				}
			}
			*d = storagemodels.QuoteRelationship{
				PK:           pk,
				SK:           sk,
				QuoterNoteID: strings.TrimPrefix(pk, "QUOTE#"),
				TargetNoteID: strings.TrimPrefix(sk, "QUOTED#"),
				QuoterID:     "alice",
				Timestamp:    time.Now().Add(-1 * time.Hour),
			}
		case *storagemodels.Poll:
			pollID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "POLL#") {
				pollID = strings.TrimPrefix(pk, "POLL#")
			}
			if poll, ok := state.pollsByID[pollID]; ok {
				*d = poll
				return
			}
			*d = storagemodels.Poll{
				ID:          pollID,
				StatusID:    "s1",
				CreatedBy:   "alice",
				Options:     []string{"a", "b"},
				Multiple:    false,
				HideTotals:  false,
				ExpiresAt:   time.Now().Add(1 * time.Hour),
				CreatedAt:   time.Now().Add(-5 * time.Minute),
				UpdatedAt:   time.Now().Add(-1 * time.Minute),
				VotesCount:  0,
				VotersCount: 0,
				Votes:       map[string][]int{},
			}
			_ = d.UpdateKeys()
		case *storagemodels.PollVote:
			pk, _ := state.whereString("PK")
			sk, _ := state.whereString("SK")
			if pk != "" && sk != "" {
				if vote, ok := state.pollVotesByKey[pk+"#"+sk]; ok {
					*d = vote
					return
				}
			}

			pollID := ""
			if strings.HasPrefix(pk, "POLL#") {
				pollID = strings.TrimPrefix(pk, "POLL#")
			}
			voterID := ""
			if strings.HasPrefix(sk, "VOTE#") {
				voterID = strings.TrimPrefix(sk, "VOTE#")
			}
			*d = storagemodels.PollVote{
				VoterID: voterID,
				Choices: []int{0},
				VotedAt: time.Now().Add(-1 * time.Minute),
			}
			if pollID != "" {
				d.SetPollID(pollID)
			}
		case *storagemodels.Announce:
			pk, _ := state.whereString("PK")
			sk, _ := state.whereString("SK")
			if pk != "" && sk != "" {
				if announce, ok := state.announcesByKey[pk+"|"+sk]; ok {
					*d = announce
					return
				}
			}
			*d = storagemodels.Announce{
				PK:        pk,
				SK:        sk,
				Actor:     strings.TrimPrefix(sk, "ACTOR#"),
				Object:    strings.TrimSuffix(strings.TrimPrefix(pk, "OBJECT#"), "#ANNOUNCES"),
				ID:        "announce-1",
				Published: time.Now().Add(-2 * time.Hour),
				CreatedAt: time.Now().Add(-2 * time.Hour),
			}
		case *storagemodels.Report:
			id := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "REPORT#") {
				id = strings.TrimPrefix(pk, "REPORT#")
			}
			if report, ok := state.reportsByID[id]; ok {
				*d = report
				return
			}
			*d = storagemodels.Report{
				ID:              id,
				ReporterID:      "admin",
				TargetAccountID: "alice",
				Category:        "spam",
				Status:          "open",
				CreatedAt:       time.Now().Add(-1 * time.Hour),
				UpdatedAt:       time.Now().Add(-30 * time.Minute),
			}
		case *storagemodels.ReportStats:
			*d = storagemodels.ReportStats{TotalReports: 2, ResolvedReports: 1}
		case *storagemodels.ModerationEvent:
			id := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "MOD_EVENT#") {
				id = strings.TrimPrefix(pk, "MOD_EVENT#")
			}
			if event, ok := state.eventsByID[id]; ok {
				*d = event
				return
			}
			*d = storagemodels.ModerationEvent{
				ID:              id,
				EventType:       "flagged",
				ActorID:         "alice",
				ObjectID:        "status-1",
				ObjectType:      "status",
				Category:        "spam",
				Severity:        "4",
				ConfidenceScore: 0.9,
				Created:         time.Now().Add(-2 * time.Hour),
			}
		case *storagemodels.ModerationDecision:
			objectID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "DECISION#") {
				objectID = strings.TrimPrefix(pk, "DECISION#")
			}
			if decision, ok := state.moderationDecisionsByObject[objectID]; ok {
				*d = decision
				return
			}
		case *storagemodels.Status:
			statusID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "status#") {
				statusID = strings.TrimPrefix(pk, "status#")
			}
			if st, ok := state.statusByID[statusID]; ok {
				*d = st
				return
			}
			*d = storagemodels.Status{
				PK:             "status#" + statusID,
				SK:             "status#" + statusID,
				StatusID:       statusID,
				AuthorUsername: "alice",
				AuthorID:       "https://example.com/users/alice",
				Content:        "hello",
				Sensitive:      false,
				CreatedAt:      time.Now().Add(-1 * time.Hour),
				UpdatedAt:      time.Now().Add(-30 * time.Minute),
			}
		case *storagemodels.Session:
			sessionID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "session#") {
				sessionID = strings.TrimPrefix(pk, "session#")
			}
			if session, ok := state.sessionsByID[sessionID]; ok {
				*d = session
				return
			}
			username := "alice"
			if sessionID == "" {
				sessionID = "sess-1"
			}
			*d = storagemodels.Session{
				PK:         "session#" + sessionID,
				SK:         "session#" + sessionID,
				SessionID:  sessionID,
				UserID:     "USER#" + username,
				IPAddress:  "203.0.113.10",
				CreatedAt:  time.Now().Add(-2 * time.Hour),
				LastUsedAt: time.Now().Add(-30 * time.Minute),
				ExpiresAt:  time.Now().Add(2 * time.Hour).Unix(),
			}
		case *storagemodels.WalletChallenge:
			challengeID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "WALLET_CHALLENGE#") {
				challengeID = strings.TrimPrefix(pk, "WALLET_CHALLENGE#")
			}
			if challenge, ok := state.walletChallengesByID[challengeID]; ok {
				*d = challenge
				return
			}
			if challengeID == "" {
				challengeID = "challenge-1"
			}
			*d = storagemodels.WalletChallenge{
				ID:        challengeID,
				Username:  "alice",
				Address:   "0xabc",
				ChainID:   1,
				Nonce:     "nonce",
				Message:   "message",
				IssuedAt:  time.Now().Add(-1 * time.Minute),
				ExpiresAt: time.Now().Add(5 * time.Minute),
			}
		case *storagemodels.WalletCredential:
			address := ""
			if sk, ok := state.whereString("SK"); ok && strings.HasPrefix(sk, "WALLET#") {
				address = strings.TrimPrefix(sk, "WALLET#")
			}
			if address != "" {
				if wallet, ok := state.walletCredentialsByAddress[address]; ok {
					*d = wallet
					return
				}
			}
			username := "alice"
			*d = storagemodels.WalletCredential{
				Username: username,
				Address:  "0xabc",
				ChainID:  1,
				Type:     "ethereum",
				LinkedAt: time.Now().Add(-24 * time.Hour),
				LastUsed: time.Now().Add(-1 * time.Hour),
			}
		case *storagemodels.Trustee:
			username := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "USER#") {
				username = strings.TrimPrefix(pk, "USER#")
			}
			if trustees, ok := state.trusteesByUser[username]; ok && len(trustees) > 0 {
				*d = trustees[0]
				return
			}
			*d = storagemodels.Trustee{
				Username:  username,
				ActorID:   "https://example.com/users/trustee",
				AddedAt:   time.Now().Add(-24 * time.Hour),
				Confirmed: true,
			}
		case *storagemodels.RecoveryRequest:
			requestID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "RECOVERY#") {
				requestID = strings.TrimPrefix(pk, "RECOVERY#")
			}
			if req, ok := state.recoveryRequestsByID[requestID]; ok {
				*d = req
				return
			}
			*d = storagemodels.RecoveryRequest{
				ID:            requestID,
				Username:      "alice",
				InitiatedAt:   time.Now().Add(-2 * time.Hour),
				ExpiresAt:     time.Now().Add(24 * time.Hour),
				RequiredVotes: 2,
				ReceivedVotes: map[string]bool{"trustee-1": true},
				Status:        "pending",
			}
		case *storagemodels.Announcement:
			announcementID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "ANNOUNCEMENT#") {
				announcementID = strings.TrimPrefix(pk, "ANNOUNCEMENT#")
			}
			if announcement, ok := state.announcementByID[announcementID]; ok {
				*d = announcement
				return
			}
			*d = storagemodels.Announcement{
				ID:          announcementID,
				Content:     "announcement content",
				Text:        "announcement content",
				PublishedAt: time.Now().Add(-2 * time.Hour),
				UpdatedAt:   time.Now().Add(-1 * time.Hour),
				AllDay:      false,
			}
		case *storagemodels.Hashtag:
			tagName := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "HASHTAG#") {
				tagName = strings.TrimPrefix(pk, "HASHTAG#")
			}
			if tagName == "" {
				tagName = "go"
			}
			*d = storagemodels.Hashtag{
				Name:       tagName,
				URL:        fmt.Sprintf("https://example.com/tags/%s", tagName),
				UsageCount: 12,
				FirstSeen:  time.Now().Add(-72 * time.Hour),
				LastUsed:   time.Now().Add(-2 * time.Hour),
			}
		case *storagemodels.VAPIDKeyRecord:
			if state.vapidKeys == nil {
				state.vapidKeys = &storage.VAPIDKeys{
					PublicKey:  "pub",
					PrivateKey: "priv",
					Subject:    "mailto:test@example.com",
					CreatedAt:  time.Now().Add(-24 * time.Hour),
					UpdatedAt:  time.Now(),
				}
			}
			d.Data = map[string]any{
				"public_key":  state.vapidKeys.PublicKey,
				"private_key": state.vapidKeys.PrivateKey,
				"subject":     state.vapidKeys.Subject,
				"created_at":  state.vapidKeys.CreatedAt.Format(time.RFC3339Nano),
				"updated_at":  state.vapidKeys.UpdatedAt.Format(time.RFC3339Nano),
			}
			d.PK = "INSTANCE#CONFIG"
			d.SK = "VAPID_KEYS"
		case *storagemodels.Export:
			exportID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "EXPORT#") {
				exportID = strings.TrimPrefix(pk, "EXPORT#")
			}
			if export, ok := state.exportsByID[exportID]; ok {
				*d = export
				return
			}
			*d = storagemodels.Export{
				ID:        exportID,
				Username:  "alice",
				Type:      "archive",
				Format:    "activitypub",
				Status:    "pending",
				CreatedAt: time.Now().Add(-1 * time.Hour),
			}
		case *storagemodels.Filter:
			filterID := ""
			if sk, ok := state.whereString("SK"); ok && strings.HasPrefix(sk, "FILTER#") {
				filterID = strings.TrimPrefix(sk, "FILTER#")
			}
			if filter, ok := state.filtersByID[filterID]; ok {
				*d = filter
				return
			}
			*d = storagemodels.Filter{
				ID:           filterID,
				Username:     "alice",
				Title:        "Test Filter",
				Context:      []string{"home"},
				FilterAction: "warn",
				Severity:     "medium",
				MatchMode:    "keyword",
				CreatedAt:    time.Now().Add(-1 * time.Hour),
				UpdatedAt:    time.Now().Add(-30 * time.Minute),
			}
		case *storagemodels.FilterKeyword:
			keywordID := ""
			if sk, ok := state.whereString("SK"); ok && strings.HasPrefix(sk, "KEYWORD#") {
				keywordID = strings.TrimPrefix(sk, "KEYWORD#")
			}
			*d = storagemodels.FilterKeyword{
				ID:        keywordID,
				FilterID:  "filter-1",
				Keyword:   "spam",
				WholeWord: true,
				CreatedAt: time.Now().Add(-1 * time.Hour),
			}
		case *storagemodels.FilterStatus:
			statusID := ""
			if sk, ok := state.whereString("SK"); ok && strings.HasPrefix(sk, "STATUS#") {
				statusID = strings.TrimPrefix(sk, "STATUS#")
			}
			*d = storagemodels.FilterStatus{
				ID:        "fs-1",
				FilterID:  "filter-1",
				StatusID:  statusID,
				CreatedAt: time.Now().Add(-1 * time.Hour),
			}
		case *storagemodels.Import:
			importID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "IMPORT#") {
				importID = strings.TrimPrefix(pk, "IMPORT#")
			}
			if imp, ok := state.importsByID[importID]; ok {
				*d = imp
				return
			}
			*d = storagemodels.Import{
				ID:        importID,
				Username:  "alice",
				Type:      "followers",
				Mode:      "merge",
				Status:    "pending",
				S3Key:     "imports/alice/" + importID + "/followers.data",
				CreatedAt: time.Now().Add(-1 * time.Hour),
				UpdatedAt: time.Now().Add(-30 * time.Minute),
			}
		case *storagemodels.ImportBudget:
			pk, _ := state.whereString("PK")
			sk, _ := state.whereString("SK")
			if budget, ok := state.importBudgetsByPKSK[pk+"#"+sk]; ok {
				*d = budget
				return
			}
			*d = storagemodels.ImportBudget{
				PK:          pk,
				SK:          sk,
				Username:    "alice",
				Period:      "daily",
				IsActive:    false,
				NextResetAt: time.Now().Add(24 * time.Hour),
			}
		case *storagemodels.WebAuthnChallenge:
			challenge := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "CHALLENGE#") {
				challenge = strings.TrimPrefix(pk, "CHALLENGE#")
			}
			if ch, ok := state.webAuthnChallengesByID[challenge]; ok {
				*d = ch
				return
			}
			*d = storagemodels.WebAuthnChallenge{
				Challenge: challenge,
				UserID:    "alice",
				Type:      "registration",
				ExpiresAt: time.Now().Add(5 * time.Minute),
			}
		case *storagemodels.PasskeyRegistrationProof:
			proofID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "PASSKEY_REGISTRATION_PROOF#") {
				proofID = strings.TrimPrefix(pk, "PASSKEY_REGISTRATION_PROOF#")
			}
			if proof, ok := state.passkeyRegistrationProofsByID[proofID]; ok {
				*d = proof
				return
			}
			*d = storagemodels.PasskeyRegistrationProof{
				ID:           proofID,
				Username:     "alice",
				CeremonyID:   "signup-1",
				CredentialID: "cred-1",
				PublicKey:    []byte{0x01},
				CreatedAt:    time.Now().Add(-1 * time.Minute),
				ExpiresAt:    time.Now().Add(5 * time.Minute),
			}
		case *storagemodels.WebAuthnCredential:
			credentialID := ""
			if gsi1PK, ok := state.whereString("gsi1PK"); ok && strings.HasPrefix(gsi1PK, "WEBAUTHN_CREDENTIAL#") {
				credentialID = strings.TrimPrefix(gsi1PK, "WEBAUTHN_CREDENTIAL#")
			} else if sk, ok := state.whereString("SK"); ok && strings.HasPrefix(sk, "WEBAUTHN_CRED#") {
				credentialID = strings.TrimPrefix(sk, "WEBAUTHN_CRED#")
			}
			if cred, ok := state.webAuthnCredentialByID[credentialID]; ok {
				*d = round10CanonicalizeWebAuthnCredential(cred)
				return
			}
			*d = round10CanonicalizeWebAuthnCredential(storagemodels.WebAuthnCredential{
				ID:         "Y3JlZA==",
				UserID:     "alice",
				PublicKey:  []byte{0x01, 0x02},
				Name:       "Test Key",
				CreatedAt:  time.Now().Add(-2 * time.Hour),
				LastUsedAt: time.Now().Add(-1 * time.Hour),
			})
		case *storagemodels.OAuthClient:
			clientID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "OAUTH_CLIENT#") {
				clientID = strings.TrimPrefix(pk, "OAUTH_CLIENT#")
			}
			if client, ok := state.oauthClientsByID[clientID]; ok {
				*d = client
				return
			}
			*d = storagemodels.OAuthClient{
				ClientID:     clientID,
				ClientSecret: "secret",
				Name:         "Test App",
				RedirectURIs: []string{"https://example.com/callback"},
				Scopes:       []string{"read", "write"},
				Confidential: true,
				CreatedAt:    time.Now().Add(-24 * time.Hour),
			}
		case *storagemodels.InstanceDomainBlock:
			domain := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "DOMAIN_BLOCK#") {
				domain = strings.TrimPrefix(pk, "DOMAIN_BLOCK#")
			}
			for _, block := range state.domainBlocks {
				if block.Domain == domain || strings.TrimPrefix(block.PK, "DOMAIN_BLOCK#") == domain {
					*d = block
					return
				}
			}
			*d = storagemodels.InstanceDomainBlock{Domain: domain}
		case *storagemodels.FederationInstance:
			domain := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "INSTANCE#") {
				domain = strings.TrimPrefix(pk, "INSTANCE#")
			}
			if instance, ok := state.federationInstancesByDomain[domain]; ok {
				*d = instance
				return
			}
			for _, instance := range state.federationInstances {
				if instance.Domain == domain {
					*d = instance
					return
				}
			}
			*d = storagemodels.FederationInstance{Domain: domain}
		case *storagemodels.InstanceMetrics:
			key := ""
			if pk, ok := state.whereString("PK"); ok {
				if sk, ok := state.whereString("SK"); ok {
					key = pk + "#" + sk
				} else {
					key = pk
				}
			}
			if metric, ok := state.instanceMetrics[key]; ok {
				*d = metric
				return
			}
			*d = storagemodels.InstanceMetrics{
				Value:     1024,
				UpdatedAt: time.Now().Add(-10 * time.Minute),
			}
		default:
			// Handle unexported projections (e.g. repositories.userVersionProjection) via reflection when needed.
			rv := reflect.ValueOf(dest)
			if rv.Kind() == reflect.Ptr && rv.Elem().IsValid() {
				if field := rv.Elem().FieldByName("Value"); field.IsValid() && field.CanSet() && field.Kind() == reflect.Int {
					field.SetInt(1)
				}
			}
		}
	}).Maybe()

	if state.allErrorOnce != nil {
		mockQuery.On("All", mock.Anything).Return(state.allErrorOnce).Once()
	}

	for typeName, err := range state.allErrorByType {
		typeName := typeName
		err := err
		mockQuery.On("All", mock.MatchedBy(func(arg any) bool {
			return reflect.TypeOf(arg).String() == typeName
		})).Return(err).Once()
	}

	mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0)
		switch d := dest.(type) {
		case *[]*storagemodels.Activity:
			// New access pattern: ActivityRepository.GetActivity queries GSI2 by activity ID.
			if gsi2pk, ok := state.whereString("gsi2PK"); ok && strings.HasPrefix(gsi2pk, "ACTIVITYID#") {
				activityID := strings.TrimPrefix(gsi2pk, "ACTIVITYID#")
				if activity, ok := state.activitiesByID[activityID]; ok && activity != nil {
					*d = []*storagemodels.Activity{activity}
					return
				}
			}

			// Legacy fallback: tests that still look up by SK.
			if activityID, ok := state.whereString("SK"); ok && activityID != "" {
				if activity, ok := state.activitiesByID[activityID]; ok && activity != nil {
					*d = []*storagemodels.Activity{activity}
					return
				}
			}
			*d = []*storagemodels.Activity{}
		case *[]storagemodels.FederationInstance:
			// FederationRepository.GetKnownInstances / GetFederationStatistics
			// resolve through keyed gsi1 queries (FEDERATION_ACTIVE) after the
			// #1469 batch F scan elimination — the fake must answer .All, not
			// .Scan, for these admin federation endpoints.
			*d = state.federationInstances
		case *[]*storagemodels.AuthAuditLog:
			pk, _ := state.whereString("gsi1PK")
			username := strings.TrimPrefix(pk, "USER#")
			if state.auditLogsByUser != nil {
				if logs, ok := state.auditLogsByUser[username]; ok {
					*d = append([]*storagemodels.AuthAuditLog(nil), logs...)
					return
				}
			}
			*d = []*storagemodels.AuthAuditLog{}
		case *[]storagemodels.AgentMemoryEvent:
			agentPK, _ := state.whereString("gsi1PK")
			agentUsername := strings.TrimPrefix(agentPK, "AGENT#")
			if state.agentMemoryEventsByAgent != nil {
				if events, ok := state.agentMemoryEventsByAgent[agentUsername]; ok {
					*d = append([]storagemodels.AgentMemoryEvent(nil), events...)
					return
				}
			}
			*d = []storagemodels.AgentMemoryEvent{}
		case *[]storagemodels.OAuthDeviceSession:
			gsi1PK, _ := state.whereString("gsi1PK")
			userCode := strings.TrimPrefix(gsi1PK, "OAUTH_DEVICE_USER_CODE#")
			if state.oauthDeviceSessionsByUserCode != nil {
				if session, ok := state.oauthDeviceSessionsByUserCode[userCode]; ok {
					*d = []storagemodels.OAuthDeviceSession{session}
					return
				}
			}
			*d = []storagemodels.OAuthDeviceSession{}
		case *[]*storagemodels.OAuthClient:
			items := make([]*storagemodels.OAuthClient, 0, len(state.oauthClientsByID))
			for _, client := range state.oauthClientsByID {
				clientCopy := client
				items = append(items, &clientCopy)
			}
			*d = items
		case *[]storagemodels.User:
			role, _ := state.whereString("gsi3PK")
			switch role {
			case "ROLE#admin":
				*d = []storagemodels.User{state.usersByUsername["admin"]}
			case "ROLE#moderator":
				*d = []storagemodels.User{state.usersByUsername["mod"]}
			default:
				*d = []storagemodels.User{state.usersByUsername["admin"], state.usersByUsername["alice"]}
			}
		case *[]storagemodels.Report:
			if len(state.reportsByID) == 0 {
				*d = nil
				return
			}
			*d = []storagemodels.Report{state.reportsByID["r1"], state.reportsByID["r2"]}
		case *[]storagemodels.Actor:
			if len(state.actorList) > 0 {
				*d = state.actorList
				return
			}
			actors := make([]storagemodels.Actor, 0, len(state.actorsByUser))
			for _, actor := range state.actorsByUser {
				actors = append(actors, actor)
			}
			*d = actors
		case *[]storagemodels.Status:
			if len(state.statusList) > 0 {
				*d = state.statusList
				return
			}
			statuses := make([]storagemodels.Status, 0, len(state.statusByID))
			for _, status := range state.statusByID {
				statuses = append(statuses, status)
			}
			*d = statuses
		case *[]storagemodels.Object:
			*d = state.objectList
		case *[]storagemodels.Session:
			username := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "USER#") {
				username = strings.TrimPrefix(pk, "USER#")
			}
			*d = []storagemodels.Session{
				{
					PK:         "USER#" + username,
					SK:         "SESSION#1",
					SessionID:  "sess-1",
					UserID:     "USER#" + username,
					IPAddress:  "203.0.113.10",
					CreatedAt:  time.Now().Add(-2 * time.Hour),
					LastUsedAt: time.Now().Add(-1 * time.Hour),
					ExpiresAt:  time.Now().Add(24 * time.Hour).Unix(),
				},
			}
		case *[]storagemodels.RelationshipRecord:
			if len(state.relationshipRecords) > 0 {
				*d = state.relationshipRecords
				return
			}
			*d = []storagemodels.RelationshipRecord{
				{PK: "FOLLOW#alice", SK: "FOLLOWING#bob", GSI1PK: "FOLLOW#alice", GSI1SK: "FOLLOWER#bob", State: storagemodels.RelationshipAccepted},
				{PK: "FOLLOW#alice", SK: "FOLLOWING#carol", GSI1PK: "FOLLOW#alice", GSI1SK: "FOLLOWER#carol", State: storagemodels.RelationshipAccepted},
			}
		case *[]storagemodels.Reputation:
			pk, _ := state.whereString("PK")
			if pk != "" {
				if reps, ok := state.reputationsByPK[pk]; ok {
					*d = reps
					return
				}
			}
			*d = []storagemodels.Reputation{}
		case *[]storagemodels.ModerationEvent:
			if len(state.eventsByID) == 0 {
				*d = nil
				return
			}
			*d = []storagemodels.ModerationEvent{state.eventsByID["evt1"], state.eventsByID["evt2"], state.eventsByID["evt3"]}
		case *[]storagemodels.ModerationDecision:
			objectID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "DECISION#") {
				objectID = strings.TrimPrefix(pk, "DECISION#")
			}
			if objectID != "" {
				if decision, ok := state.moderationDecisionsByObject[objectID]; ok {
					*d = []storagemodels.ModerationDecision{decision}
					return
				}
			}
			*d = []storagemodels.ModerationDecision{}
		case *[]storagemodels.Filter:
			// New access pattern: FilterRepository.GetFilter queries GSI1 by filter ID.
			if gsi1PK, ok := state.whereString("gsi1PK"); ok && strings.HasPrefix(gsi1PK, "FILTER#") {
				filterID := strings.TrimPrefix(gsi1PK, "FILTER#")
				if filter, ok := state.filtersByID[filterID]; ok {
					*d = []storagemodels.Filter{filter}
					return
				}
				*d = []storagemodels.Filter{}
				return
			}

			if len(state.filtersByID) == 0 {
				*d = []storagemodels.Filter{
					{
						ID:           "filter-1",
						Username:     "alice",
						Title:        "Default Filter",
						Context:      []string{"home"},
						FilterAction: "warn",
						Severity:     "medium",
						MatchMode:    "keyword",
						CreatedAt:    time.Now().Add(-2 * time.Hour),
						UpdatedAt:    time.Now().Add(-1 * time.Hour),
					},
				}
				return
			}
			items := make([]storagemodels.Filter, 0, len(state.filtersByID))
			for _, filter := range state.filtersByID {
				items = append(items, filter)
			}
			*d = items
		case *[]*storagemodels.Filter:
			// Pointer-slice variant used by some repositories (e.g. FilterRepository.GetFilter).
			if gsi1PK, ok := state.whereString("gsi1PK"); ok && strings.HasPrefix(gsi1PK, "FILTER#") {
				filterID := strings.TrimPrefix(gsi1PK, "FILTER#")
				if filter, ok := state.filtersByID[filterID]; ok {
					filterCopy := filter
					*d = []*storagemodels.Filter{&filterCopy}
					return
				}
				*d = []*storagemodels.Filter{}
				return
			}

			*d = []*storagemodels.Filter{}
		case *[]storagemodels.FilterKeyword:
			if len(state.filterKeywords) == 0 {
				keywordID := "kw-1"
				if sk, ok := state.whereString("SK"); ok && strings.HasPrefix(sk, "KEYWORD#") {
					keywordID = strings.TrimPrefix(sk, "KEYWORD#")
				}
				*d = []storagemodels.FilterKeyword{
					{
						ID:        keywordID,
						FilterID:  "filter-1",
						Keyword:   "spam",
						WholeWord: true,
						CreatedAt: time.Now().Add(-1 * time.Hour),
					},
				}
				return
			}

			if pk, ok := state.whereString("PK"); ok && strings.TrimSpace(pk) != "" {
				filterID := strings.TrimPrefix(pk, "FILTER#")
				if keywords, ok := state.filterKeywords[filterID]; ok {
					*d = keywords
					return
				}
				*d = []storagemodels.FilterKeyword{}
				return
			}

			if sk, ok := state.whereString("SK"); ok && strings.HasPrefix(sk, "KEYWORD#") {
				keywordID := strings.TrimPrefix(sk, "KEYWORD#")
				matches := make([]storagemodels.FilterKeyword, 0, 1)
				for _, keywords := range state.filterKeywords {
					for _, keyword := range keywords {
						if keyword.ID == keywordID {
							matches = append(matches, keyword)
						}
					}
				}
				*d = matches
				return
			}

			*d = []storagemodels.FilterKeyword{}
		case *[]storagemodels.FilterStatus:
			if len(state.filterStatuses) == 0 {
				statusID := "status-1"
				if sk, ok := state.whereString("SK"); ok && strings.HasPrefix(sk, "STATUS#") {
					statusID = strings.TrimPrefix(sk, "STATUS#")
				}
				*d = []storagemodels.FilterStatus{
					{
						ID:        "fs-1",
						FilterID:  "filter-1",
						StatusID:  statusID,
						CreatedAt: time.Now().Add(-1 * time.Hour),
					},
				}
				return
			}

			if pk, ok := state.whereString("PK"); ok && strings.TrimSpace(pk) != "" {
				filterID := strings.TrimPrefix(pk, "FILTER#")
				if statuses, ok := state.filterStatuses[filterID]; ok {
					*d = statuses
					return
				}
				*d = []storagemodels.FilterStatus{}
				return
			}

			if sk, ok := state.whereString("SK"); ok && strings.HasPrefix(sk, "STATUS#") {
				statusID := strings.TrimPrefix(sk, "STATUS#")
				matches := make([]storagemodels.FilterStatus, 0, 1)
				for _, statuses := range state.filterStatuses {
					for _, status := range statuses {
						if status.StatusID == statusID {
							matches = append(matches, status)
						}
					}
				}
				*d = matches
				return
			}

			*d = []storagemodels.FilterStatus{}
		case *[]storagemodels.Import:
			if len(state.importsByUser) == 0 {
				*d = []storagemodels.Import{
					{
						ID:        "import-1",
						Username:  "alice",
						Type:      "followers",
						Mode:      "merge",
						Status:    "pending",
						CreatedAt: time.Now().Add(-1 * time.Hour),
						UpdatedAt: time.Now().Add(-30 * time.Minute),
					},
				}
				return
			}
			username, _ := state.whereString("gsi1PK")
			username = strings.TrimPrefix(username, "USER#")
			if items, ok := state.importsByUser[username]; ok {
				*d = items
				return
			}
			*d = []storagemodels.Import{}
		case *[]*storagemodels.Import:
			if len(state.importsByUser) == 0 {
				imp := storagemodels.Import{
					ID:        "import-1",
					Username:  "alice",
					Type:      "followers",
					Mode:      "merge",
					Status:    "pending",
					CreatedAt: time.Now().Add(-1 * time.Hour),
					UpdatedAt: time.Now().Add(-30 * time.Minute),
				}
				*d = []*storagemodels.Import{&imp}
				return
			}
			username, _ := state.whereString("gsi1PK")
			username = strings.TrimPrefix(username, "USER#")
			if items, ok := state.importsByUser[username]; ok {
				result := make([]*storagemodels.Import, 0, len(items))
				for _, item := range items {
					imp := item
					result = append(result, &imp)
				}
				*d = result
				return
			}
			*d = []*storagemodels.Import{}
		case *[]storagemodels.WebAuthnCredential:
			username, _ := state.whereString("PK")
			username = strings.TrimPrefix(username, "USER#")
			if creds, ok := state.webAuthnCredentialsByUser[username]; ok {
				items := make([]storagemodels.WebAuthnCredential, len(creds))
				for i := range creds {
					items[i] = round10CanonicalizeWebAuthnCredential(creds[i])
				}
				*d = items
				return
			}
			*d = []storagemodels.WebAuthnCredential{
				{
					ID:         "Y3JlZA==",
					UserID:     "alice",
					PublicKey:  []byte{0x01, 0x02},
					Name:       "Test Key",
					CreatedAt:  time.Now().Add(-2 * time.Hour),
					LastUsedAt: time.Now().Add(-1 * time.Hour),
				},
			}
			for i := range *d {
				(*d)[i] = round10CanonicalizeWebAuthnCredential((*d)[i])
			}
		case *[]storagemodels.PushSubscription:
			username, _ := state.whereString("PK")
			username = strings.TrimPrefix(username, "PUSH#")
			if subs, ok := state.pushSubscriptionsByUser[username]; ok {
				*d = subs
				return
			}
			*d = []storagemodels.PushSubscription{
				{
					ID:       "sub-1",
					Username: "alice",
					Endpoint: "https://push.example.com",
					P256dh:   "p256",
					Auth:     "auth",
					Alerts: storagemodels.PushSubscriptionAlerts{
						Follow: true,
					},
					CreatedAt: time.Now().Add(-1 * time.Hour),
					UpdatedAt: time.Now().Add(-30 * time.Minute),
				},
			}
		case *[]*storagemodels.Vouch:
			pk, _ := state.whereString("PK")
			sk, _ := state.whereString("SK")
			if strings.HasPrefix(pk, "VOUCH#") && sk == storagemodels.SKMetadata {
				vouchID := strings.TrimPrefix(pk, "VOUCH#")
				if vouch, ok := state.vouchModelsByID[vouchID]; ok && vouch != nil {
					*d = []*storagemodels.Vouch{vouch}
					return
				}
				for _, model := range state.vouchModels {
					if model != nil && model.PK == pk {
						*d = []*storagemodels.Vouch{model}
						return
					}
				}
				*d = []*storagemodels.Vouch{}
				return
			}
			if gsi1pk, ok := state.whereString("gsi1PK"); ok && gsi1pk != "" {
				items := make([]*storagemodels.Vouch, 0)
				for _, model := range state.vouchModels {
					if model != nil && model.GSI1PK == gsi1pk {
						items = append(items, model)
					}
				}
				*d = items
				return
			}
			if gsi2pk, ok := state.whereString("gsi2PK"); ok && gsi2pk != "" {
				items := make([]*storagemodels.Vouch, 0)
				for _, model := range state.vouchModels {
					if model != nil && model.GSI2PK == gsi2pk {
						items = append(items, model)
					}
				}
				*d = items
				return
			}
			if state.vouchModels != nil {
				*d = state.vouchModels
				return
			}
			*d = []*storagemodels.Vouch{}
		case *[]*storagemodels.PushSubscription:
			username, _ := state.whereString("PK")
			username = strings.TrimPrefix(username, "PUSH#")
			if subs, ok := state.pushSubscriptionsByUser[username]; ok {
				out := make([]*storagemodels.PushSubscription, len(subs))
				for i := range subs {
					sub := subs[i]
					out[i] = &sub
				}
				*d = out
				return
			}
			*d = []*storagemodels.PushSubscription{
				{
					ID:       "sub-1",
					Username: "alice",
					Endpoint: "https://push.example.com",
					P256dh:   "p256",
					Auth:     "auth",
					Alerts: storagemodels.PushSubscriptionAlerts{
						Follow: true,
					},
					CreatedAt: time.Now().Add(-1 * time.Hour),
					UpdatedAt: time.Now().Add(-30 * time.Minute),
				},
			}
		case *[]*storagemodels.DynamoDBCostRecord:
			if state.costRecords != nil {
				*d = state.costRecords
				return
			}
			*d = []*storagemodels.DynamoDBCostRecord{
				{
					ID:                  "cost-1",
					OperationType:       "GetItem",
					Timestamp:           time.Now().Add(-5 * time.Minute),
					TotalCostMicroCents: 1000,
				},
			}
		case *[]*storagemodels.DynamoDBCostAggregation:
			if state.costAggregations != nil {
				*d = state.costAggregations
				return
			}
			*d = []*storagemodels.DynamoDBCostAggregation{
				{
					Period:              "day",
					OperationType:       "GetItem",
					WindowStart:         time.Now().Add(-24 * time.Hour),
					TotalCostMicroCents: 1200,
					TotalOperations:     10,
				},
				{
					Period:              "day",
					OperationType:       "GetItem",
					WindowStart:         time.Now().Add(-12 * time.Hour),
					TotalCostMicroCents: 1500,
					TotalOperations:     12,
				},
			}
		case *[]*storagemodels.MetricRecord:
			metricType := ""
			if pk, ok := state.whereString("gsi2PK"); ok && strings.HasPrefix(pk, "METRIC_TYPE#") {
				metricType = strings.TrimPrefix(pk, "METRIC_TYPE#")
			}

			var records []storagemodels.MetricRecord
			if state.metricRecords != nil {
				records = state.metricRecords
			} else {
				records = []storagemodels.MetricRecord{
					{
						MetricType: "api_endpoint",
						Count:      5,
						Sum:        250,
						P50:        40,
					},
				}
			}

			out := make([]*storagemodels.MetricRecord, 0, len(records))
			for i := range records {
				rec := records[i]
				if metricType != "" && rec.MetricType != metricType {
					continue
				}
				out = append(out, &rec)
			}
			*d = out
		case *[]storagemodels.MetricRecord:
			if state.metricRecords != nil {
				*d = state.metricRecords
				return
			}
			*d = []storagemodels.MetricRecord{
				{
					MetricType: "api_endpoint",
					Count:      5,
					Sum:        250,
					P50:        40,
				},
			}
		case *[]storagemodels.InstanceHistory:
			if len(state.instanceHistories) > 0 {
				*d = state.instanceHistories
				return
			}
			*d = []storagemodels.InstanceHistory{
				{
					Date:         time.Now().Add(-24 * time.Hour).Format("2006-01-02"),
					StorageBytes: 2048,
					Delta:        100,
					RecordedAt:   time.Now().Add(-24 * time.Hour),
					NewUsers:     2,
				},
				{
					Date:         time.Now().Add(-48 * time.Hour).Format("2006-01-02"),
					StorageBytes: 1024,
					Delta:        50,
					RecordedAt:   time.Now().Add(-48 * time.Hour),
					NewUsers:     1,
				},
			}
		case *[]storagemodels.Poll:
			gsi1pk, _ := state.whereString("gsi1PK")
			if strings.HasPrefix(gsi1pk, "STATUS#") {
				statusID := strings.TrimPrefix(gsi1pk, "STATUS#")
				polls := make([]storagemodels.Poll, 0, len(state.pollsByID))
				for _, poll := range state.pollsByID {
					if poll.StatusID == statusID {
						polls = append(polls, poll)
					}
				}
				*d = polls
				return
			}
			polls := make([]storagemodels.Poll, 0, len(state.pollsByID))
			for _, poll := range state.pollsByID {
				polls = append(polls, poll)
			}
			*d = polls
		case *[]*storagemodels.Poll:
			gsi1pk, _ := state.whereString("gsi1PK")
			if strings.HasPrefix(gsi1pk, "STATUS#") {
				statusID := strings.TrimPrefix(gsi1pk, "STATUS#")
				polls := make([]*storagemodels.Poll, 0, len(state.pollsByID))
				for _, poll := range state.pollsByID {
					if poll.StatusID == statusID {
						poll := poll
						polls = append(polls, &poll)
					}
				}
				*d = polls
				return
			}
			polls := make([]*storagemodels.Poll, 0, len(state.pollsByID))
			for _, poll := range state.pollsByID {
				poll := poll
				polls = append(polls, &poll)
			}
			*d = polls
		case *[]storagemodels.QuoteRelationship:
			*d = state.quoteRelationships
		case *[]storagemodels.InstanceDomainBlock:
			*d = state.domainBlocks
		case *[]storagemodels.DomainAllow:
			*d = state.domainAllows
		case *[]storagemodels.EmailDomainBlock:
			*d = state.emailDomainBlocks
		case *[]*storagemodels.DomainAllow:
			items := make([]*storagemodels.DomainAllow, 0, len(state.domainAllows))
			for i := range state.domainAllows {
				allow := state.domainAllows[i]
				items = append(items, &allow)
			}
			*d = items
		case *[]*storagemodels.EmailDomainBlock:
			items := make([]*storagemodels.EmailDomainBlock, 0, len(state.emailDomainBlocks))
			for i := range state.emailDomainBlocks {
				block := state.emailDomainBlocks[i]
				items = append(items, &block)
			}
			*d = items
		case *[]storagemodels.ModerationReview:
			*d = state.moderationReviews
		case *[]storagemodels.Export:
			*d = state.exportList
		case *[]*storagemodels.Export:
			username, _ := state.whereString("gsi1PK")
			username = strings.TrimPrefix(username, "USER#")

			if len(state.exportList) == 0 {
				exp := storagemodels.Export{
					ID:        "export-1",
					Username:  username,
					Type:      "archive",
					Format:    "activitypub",
					Status:    "pending",
					CreatedAt: time.Now().Add(-1 * time.Hour),
				}
				*d = []*storagemodels.Export{&exp}
				return
			}

			result := make([]*storagemodels.Export, 0, len(state.exportList))
			for _, item := range state.exportList {
				if username != "" && item.Username != "" && item.Username != username {
					continue
				}
				exp := item
				result = append(result, &exp)
			}
			*d = result
		case *[]storagemodels.CommunityNote:
			gsi3pk, _ := state.whereString("gsi3PK")
			if gsi3pk != "" {
				if notes, ok := state.communityNotesByGSI3PK[gsi3pk]; ok {
					*d = round10ApplyCommunityNoteAuthorQuery(state, notes)
					return
				}
			}
			*d = []storagemodels.CommunityNote{}
		case *[]storagemodels.TrustRelationship:
			*d = state.trustRelationships
		case *[]storagemodels.InstanceRule:
			*d = state.instanceRules
		case *[]*storagemodels.Announcement:
			if len(state.announcementByID) == 0 {
				*d = nil
				return
			}
			items := make([]*storagemodels.Announcement, 0, len(state.announcementByID))
			for _, announcement := range state.announcementByID {
				ann := announcement
				items = append(items, &ann)
			}
			*d = items
		case *[]*storagemodels.AnnouncementDismissal:
			username := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "USER#") {
				username = strings.TrimPrefix(pk, "USER#")
			}
			if dismissals, ok := state.announcementDismissalsByID[username]; ok {
				items := make([]*storagemodels.AnnouncementDismissal, 0, len(dismissals))
				for i := range dismissals {
					dismissal := dismissals[i]
					items = append(items, &dismissal)
				}
				*d = items
				return
			}
			*d = []*storagemodels.AnnouncementDismissal{}
		case *[]*storagemodels.AnnouncementReaction:
			announcementID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "ANNOUNCEMENT_REACTION#") {
				announcementID = strings.TrimPrefix(pk, "ANNOUNCEMENT_REACTION#")
			}
			if reactions, ok := state.announcementReactionsByID[announcementID]; ok {
				items := make([]*storagemodels.AnnouncementReaction, 0, len(reactions))
				for i := range reactions {
					reaction := reactions[i]
					items = append(items, &reaction)
				}
				*d = items
				return
			}
			*d = []*storagemodels.AnnouncementReaction{}
		case *[]*storagemodels.FeaturedTag:
			username := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "USER#") {
				username = strings.TrimPrefix(pk, "USER#")
			}
			if tags, ok := state.featuredTagsByUser[username]; ok {
				items := make([]*storagemodels.FeaturedTag, 0, len(tags))
				for i := range tags {
					tag := tags[i]
					items = append(items, &tag)
				}
				*d = items
				return
			}
			*d = []*storagemodels.FeaturedTag{}
		case *[]*storagemodels.HashtagFollow:
			username := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "user#") {
				username = strings.TrimPrefix(pk, "user#")
			}
			if follows, ok := state.hashtagFollowsByUser[username]; ok {
				items := make([]*storagemodels.HashtagFollow, 0, len(follows))
				for i := range follows {
					follow := follows[i]
					items = append(items, &follow)
				}
				*d = items
				return
			}
			*d = []*storagemodels.HashtagFollow{}
		case *[]storagemodels.WalletCredential:
			username := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "USER#") {
				username = strings.TrimPrefix(pk, "USER#")
			}
			if wallets, ok := state.walletCredentialsByUser[username]; ok {
				*d = wallets
				return
			}
			*d = []storagemodels.WalletCredential{}
		case *[]storagemodels.AgentAccessLease:
			items := make([]storagemodels.AgentAccessLease, 0)
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "AGENT_ACCESS_LEASE#") {
				for key, lease := range state.agentAccessLeasesByKey {
					if strings.HasPrefix(key, pk+"#") {
						items = append(items, lease)
					}
				}
			} else {
				for _, lease := range state.agentAccessLeasesByKey {
					items = append(items, lease)
				}
			}
			*d = items
		case *[]storagemodels.RefreshToken:
			items := make([]storagemodels.RefreshToken, 0)
			if state.refreshTokensByToken == nil {
				*d = items
				return
			}
			gsi1pk, hasGSI1 := state.whereString("gsi1PK")
			gsi2pk, hasGSI2 := state.whereString("gsi2PK")
			gsi3pk, hasGSI3 := state.whereString("gsi3PK")
			for _, token := range state.refreshTokensByToken {
				switch {
				case hasGSI1 && token.GSI1PK == gsi1pk:
					items = append(items, token)
				case hasGSI2 && token.GSI2PK == gsi2pk:
					items = append(items, token)
				case hasGSI3 && token.GSI3PK == gsi3pk:
					items = append(items, token)
				}
			}
			*d = items
		case *[]storagemodels.WalletIndex:
			address := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "WALLET#") {
				parts := strings.Split(pk, "#")
				if len(parts) >= 3 {
					address = parts[2]
				}
			}
			if wallet, ok := state.walletCredentialsByAddress[address]; ok {
				*d = []storagemodels.WalletIndex{{
					Username:   wallet.Username,
					WalletType: wallet.Type,
					Address:    wallet.Address,
				}}
				return
			}
			*d = []storagemodels.WalletIndex{}
		case *[]storagemodels.Trustee:
			username := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "USER#") {
				username = strings.TrimPrefix(pk, "USER#")
			}
			if trustees, ok := state.trusteesByUser[username]; ok {
				*d = trustees
				return
			}
			*d = []storagemodels.Trustee{}
		case *[]storagemodels.RecoveryCode:
			username := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "USER#") {
				username = strings.TrimPrefix(pk, "USER#")
			}
			if codes, ok := state.recoveryCodesByUser[username]; ok {
				*d = codes
				return
			}
			*d = []storagemodels.RecoveryCode{}
		case *[]storagemodels.Device:
			deviceID := ""
			// New access pattern: AccountRepository.GetDevice queries gsi3PK = DEVICEID#<id>.
			if gsi3pk, ok := state.whereString("gsi3PK"); ok && strings.HasPrefix(gsi3pk, "DEVICEID#") {
				deviceID = strings.TrimPrefix(gsi3pk, "DEVICEID#")
			} else if id, ok := state.whereString("DeviceID"); ok {
				deviceID = id
			}
			if deviceID != "" {
				if device, ok := state.devicesByID[deviceID]; ok {
					*d = []storagemodels.Device{device}
					return
				}
			}
			*d = []storagemodels.Device{}
		case *[]*storagemodels.HashtagTrend:
			items := make([]*storagemodels.HashtagTrend, 0, len(state.trendingHashtags))
			for i := range state.trendingHashtags {
				items = append(items, &state.trendingHashtags[i])
			}
			*d = items
		case *[]*storagemodels.StatusTrend:
			items := make([]*storagemodels.StatusTrend, 0, len(state.trendingStatuses))
			for i := range state.trendingStatuses {
				items = append(items, &state.trendingStatuses[i])
			}
			*d = items
		case *[]storagemodels.HashtagTrend:
			*d = state.trendingHashtags
		case *[]storagemodels.StatusTrend:
			*d = state.trendingStatuses
		case *[]storagemodels.LinkTrend:
			*d = state.trendingLinks
		default:
			rv := reflect.ValueOf(dest)
			if rv.Kind() == reflect.Ptr && rv.Elem().Kind() == reflect.Slice {
				rv.Elem().Set(reflect.MakeSlice(rv.Elem().Type(), 0, 0))
			}
		}
	}).Maybe()

	if state.scanErrorOnce != nil {
		mockQuery.On("Scan", mock.Anything).Return(state.scanErrorOnce).Once()
	}

	mockQuery.On("Scan", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0)
		switch d := dest.(type) {
		case *[]storagemodels.FederationInstance:
			*d = state.federationInstances
		case *[]storagemodels.Status:
			*d = []storagemodels.Status{state.statusByID["s1"]}
		case *[]*storagemodels.Vouch:
			pk, _ := state.whereString("PK")
			sk, _ := state.whereString("SK")
			if strings.HasPrefix(pk, "VOUCH#") && sk == storagemodels.SKMetadata {
				vouchID := strings.TrimPrefix(pk, "VOUCH#")
				if vouch, ok := state.vouchModelsByID[vouchID]; ok && vouch != nil {
					*d = []*storagemodels.Vouch{vouch}
					return
				}
				for _, model := range state.vouchModels {
					if model != nil && model.PK == pk {
						*d = []*storagemodels.Vouch{model}
						return
					}
				}
				*d = []*storagemodels.Vouch{}
				return
			}

			if gsi1pk, ok := state.whereString("gsi1PK"); ok && gsi1pk != "" {
				items := make([]*storagemodels.Vouch, 0)
				for _, model := range state.vouchModels {
					if model != nil && model.GSI1PK == gsi1pk {
						items = append(items, model)
					}
				}
				*d = items
				return
			}

			if gsi2pk, ok := state.whereString("gsi2PK"); ok && gsi2pk != "" {
				items := make([]*storagemodels.Vouch, 0)
				for _, model := range state.vouchModels {
					if model != nil && model.GSI2PK == gsi2pk {
						items = append(items, model)
					}
				}
				*d = items
				return
			}

			if state.vouchModels != nil {
				*d = state.vouchModels
				return
			}
			*d = []*storagemodels.Vouch{}
		case *[]*storagemodels.Media:
			username := ""
			if gsi1pk, ok := state.whereString("gsi1PK"); ok && strings.HasPrefix(gsi1pk, "USER_MEDIA#") {
				username = strings.TrimPrefix(gsi1pk, "USER_MEDIA#")
			}
			if items, ok := state.userMediaByUser[username]; ok {
				*d = items
				return
			}
			*d = []*storagemodels.Media{{MediaID: "media-123", GSI1SK: "cursor-media-1"}}
		case *[]*storagemodels.TrustRelationship:
			relationships := make([]*storagemodels.TrustRelationship, 0, len(state.trustRelationships))
			for i := range state.trustRelationships {
				rel := state.trustRelationships[i]
				relationships = append(relationships, &rel)
			}
			*d = relationships
		default:
			rv := reflect.ValueOf(dest)
			if rv.Kind() == reflect.Ptr && rv.Elem().Kind() == reflect.Slice {
				rv.Elem().Set(reflect.MakeSlice(rv.Elem().Type(), 0, 0))
			}
		}
	}).Maybe()

	return &round10DynamoHarness{
		db:     &round10TransactionalDB{inner: mockDB, state: state},
		query:  mockQuery,
		update: mockUpdate,
		state:  state,
	}
}

func round10TestLogger(t *testing.T) *zap.Logger {
	t.Helper()
	return zaptest.NewLogger(t)
}

func round10TestConfig() *config.Config {
	return &config.Config{
		Domain:          "example.com",
		JWTSecret:       round11StrongJWTSecret,
		DynamoTableName: "test-table",
		Stage:           "production",
	}
}
