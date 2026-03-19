package handlers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

type round10Where struct {
	field string
	op    string
	value any
}

type round10QueryState struct {
	wheres []round10Where
	model  any

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
	oauthClientsByID              map[string]storagemodels.OAuthClient
	authorizationCodesByCode      map[string]storagemodels.AuthorizationCode
	refreshTokensByToken          map[string]storagemodels.RefreshToken
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
	agentMemoryEventsByAgent  map[string][]storagemodels.AgentMemoryEvent
	remoteActorsByPK          map[string]storagemodels.RemoteActor
	auditLogsByUser           map[string][]*storagemodels.AuthAuditLog

	forceVapidNotFound bool

	notFoundPKs    map[string]bool
	notFoundPKSK   map[string]bool
	notFoundGSI3PK map[string]bool

	allErrorOnce     error
	allErrorByType   map[string]error
	scanErrorOnce    error
	firstErrorOnce   error
	updateErrorOnce  error
	createErrorOnce  error
	deleteErrorOnce  error
	executeErrorOnce error

	firstErrorPK     map[string]error
	firstErrorGSI3PK map[string]error
}

func (s *round10QueryState) reset() {
	s.wheres = nil
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
			hdr["content-type"] = []string{"application/json; charset=utf-8"}
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
	db     *mocks.MockDB
	query  *mocks.MockQuery
	update *mocks.MockUpdateBuilder
	state  *round10QueryState
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
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Cursor", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("ConsistentRead").Return(mockQuery).Maybe()
	mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("IfNotExists").Return(mockQuery).Maybe()
	mockQuery.On("IfExists").Return(mockQuery).Maybe()

	// UpdateBuilder support
	mockQuery.On("UpdateBuilder").Return(mockUpdate).Maybe()
	mockUpdate.On("Set", mock.Anything, mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("Add", mock.Anything, mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("SetIfNotExists", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("ConditionVersion", mock.Anything).Return(mockUpdate).Maybe()
	if state.executeErrorOnce != nil {
		mockUpdate.On("Execute").Return(state.executeErrorOnce).Once()
	}
	mockUpdate.On("Execute").Return(nil).Maybe()

	// Mutations
	if state.createErrorOnce != nil {
		mockQuery.On("Create").Return(state.createErrorOnce).Once()
	}
	mockQuery.On("Create").Return(nil).Run(func(_ mock.Arguments) {
		switch m := state.model.(type) {
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
			if m == nil {
				return
			}
			if state.refreshTokensByToken == nil {
				state.refreshTokensByToken = map[string]storagemodels.RefreshToken{}
			}
			state.refreshTokensByToken[m.Token] = *m
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
		}
	}).Maybe()

	if state.deleteErrorOnce != nil {
		mockQuery.On("Delete").Return(state.deleteErrorOnce).Once()
	}
	mockQuery.On("Delete").Return(nil).Run(func(_ mock.Arguments) {
		if state.refreshTokensByToken == nil {
			return
		}
		if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "REFRESHTOKEN#") {
			delete(state.refreshTokensByToken, strings.TrimPrefix(pk, "REFRESHTOKEN#"))
		}
	}).Maybe()

	if state.updateErrorOnce != nil {
		mockQuery.On("Update", mock.Anything).Return(state.updateErrorOnce).Once()
	}
	mockQuery.On("Update", mock.Anything).Return(nil).Run(func(_ mock.Arguments) {
		switch m := state.model.(type) {
		case *storagemodels.RefreshToken:
			if m == nil {
				return
			}
			if state.refreshTokensByToken == nil {
				state.refreshTokensByToken = map[string]storagemodels.RefreshToken{}
			}
			state.refreshTokensByToken[m.Token] = *m
		}
	}).Maybe()
	mockQuery.On("Count").Return(int64(2), nil).Maybe()

	// Query executions
	if state.forceVapidNotFound {
		mockQuery.On("First", mock.AnythingOfType("*models.VAPIDKeyRecord")).Return(dynamormerrors.ErrItemNotFound).Maybe()
	}

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

	for gsi3pk, err := range state.firstErrorGSI3PK {
		gsi3pk := gsi3pk
		err := err
		mockQuery.On("First", mock.MatchedBy(func(_ any) bool {
			value, ok := state.whereString("gsi3PK")
			return ok && value == gsi3pk
		})).Return(err).Maybe()
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

	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0)
		switch d := dest.(type) {
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
			}
			if user, ok := state.usersByUsername[username]; ok {
				*d = user
				return
			}
			*d = storagemodels.User{Username: username, Role: "user", Approved: true, Version: 1}
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
		case *storagemodels.WebAuthnCredential:
			credentialID := ""
			if pk, ok := state.whereString("PK"); ok && strings.HasPrefix(pk, "WEBAUTHN_CREDENTIAL#") {
				credentialID = strings.TrimPrefix(pk, "WEBAUTHN_CREDENTIAL#")
			}
			if cred, ok := state.webAuthnCredentialByID[credentialID]; ok {
				*d = cred
				return
			}
			*d = storagemodels.WebAuthnCredential{
				ID:         "Y3JlZA==",
				UserID:     "alice",
				PublicKey:  []byte{0x01, 0x02},
				Name:       "Test Key",
				CreatedAt:  time.Now().Add(-2 * time.Hour),
				LastUsedAt: time.Now().Add(-1 * time.Hour),
			}
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
			username, _ := state.whereString("gsi1PK")
			username = strings.TrimPrefix(username, "USER#")
			if creds, ok := state.webAuthnCredentialsByUser[username]; ok {
				*d = creds
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
					*d = notes
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
		db:     mockDB,
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
