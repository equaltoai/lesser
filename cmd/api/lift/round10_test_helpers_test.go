package lift

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/mocks"
	liftframework "github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/mock"
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

	usersByUsername             map[string]storagemodels.User
	actorsByUser                map[string]storagemodels.Actor
	actorList                   []storagemodels.Actor
	statusByID                  map[string]storagemodels.Status
	statusList                  []storagemodels.Status
	objectsByID                 map[string]storagemodels.Object
	objectList                  []storagemodels.Object
	reportsByID                 map[string]storagemodels.Report
	eventsByID                  map[string]storagemodels.ModerationEvent
	userMediaByUser             map[string][]*storagemodels.Media
	sessionsByID                map[string]storagemodels.Session
	filtersByID                 map[string]storagemodels.Filter
	filterKeywords              map[string][]storagemodels.FilterKeyword
	filterStatuses              map[string][]storagemodels.FilterStatus
	importsByID                 map[string]storagemodels.Import
	importsByUser               map[string][]storagemodels.Import
	pushSubscriptionsByUser     map[string][]storagemodels.PushSubscription
	webAuthnCredentialsByUser   map[string][]storagemodels.WebAuthnCredential
	webAuthnCredentialByID      map[string]storagemodels.WebAuthnCredential
	webAuthnChallengesByID      map[string]storagemodels.WebAuthnChallenge
	oauthClientsByID            map[string]storagemodels.OAuthClient
	costRecords                 []*storagemodels.DynamoDBCostRecord
	costAggregations            []*storagemodels.DynamoDBCostAggregation
	metricRecords               []storagemodels.MetricRecord
	instanceHistories           []storagemodels.InstanceHistory
	instanceMetrics             map[string]storagemodels.InstanceMetrics
	quoteRelationships          []storagemodels.QuoteRelationship
	announcesByKey              map[string]storagemodels.Announce
	oauthStates                 map[string]storagemodels.OAuthState
	notificationsByID           map[string]storagemodels.Notification
	domainBlocks                []storagemodels.InstanceDomainBlock
	moderationReviews           []storagemodels.ModerationReview
	moderationDecisionsByObject map[string]storagemodels.ModerationDecision
	exportsByID                 map[string]storagemodels.Export
	exportList                  []storagemodels.Export

	trustRelationships []storagemodels.TrustRelationship
	instanceRules      []storagemodels.InstanceRule

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

	forceVapidNotFound bool

	notFoundPKs    map[string]bool
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

func round10NewLiftContext(method, path string, headers, query map[string]string, body any) (*liftframework.Context, error) {
	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyBytes = b
	}

	req := liftframework.NewRequest(&adapters.Request{
		Method:      method,
		Path:        path,
		Headers:     headers,
		QueryParams: query,
		Body:        bodyBytes,
	})

	return liftframework.NewContext(context.Background(), req), nil
}

func round10NewLiftContextWithBodyBytes(method, path string, headers, query map[string]string, body []byte) *liftframework.Context {
	req := liftframework.NewRequest(&adapters.Request{
		Method:      method,
		Path:        path,
		Headers:     headers,
		QueryParams: query,
		Body:        body,
	})
	return liftframework.NewContext(context.Background(), req)
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
	mockDB.On("Model", mock.Anything).Return(mockQuery).Run(func(_ mock.Arguments) {
		state.reset()
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

	// UpdateBuilder support
	mockQuery.On("UpdateBuilder").Return(mockUpdate).Maybe()
	mockUpdate.On("Set", mock.Anything, mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("Add", mock.Anything, mock.Anything).Return(mockUpdate).Maybe()
	mockUpdate.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdate).Maybe()
	if state.executeErrorOnce != nil {
		mockUpdate.On("Execute").Return(state.executeErrorOnce).Once()
	}
	mockUpdate.On("Execute").Return(nil).Maybe()

	// Mutations
	if state.createErrorOnce != nil {
		mockQuery.On("Create").Return(state.createErrorOnce).Once()
	}
	mockQuery.On("Create").Return(nil).Maybe()

	if state.deleteErrorOnce != nil {
		mockQuery.On("Delete").Return(state.deleteErrorOnce).Once()
	}
	mockQuery.On("Delete").Return(nil).Maybe()

	if state.updateErrorOnce != nil {
		mockQuery.On("Update", mock.Anything).Return(state.updateErrorOnce).Once()
	}
	mockQuery.On("Update", mock.Anything).Return(nil).Maybe()
	mockQuery.On("Count").Return(int64(2), nil).Maybe()

	// Query executions
	if state.forceVapidNotFound {
		mockQuery.On("First", mock.AnythingOfType("*models.VAPIDKeyRecord")).Return(dynamormerrors.ErrItemNotFound).Maybe()
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
		})).Return(storage.ErrNotFound).Maybe()
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

	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		dest := args.Get(0)
		switch d := dest.(type) {
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
			d.Data = *state.vapidKeys
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
			*d = []storagemodels.RelationshipRecord{
				{PK: "FOLLOW#alice", SK: "FOLLOWING#bob", GSI1PK: "FOLLOW#alice", GSI1SK: "FOLLOWER#bob", State: storagemodels.RelationshipAccepted},
				{PK: "FOLLOW#alice", SK: "FOLLOWING#carol", GSI1PK: "FOLLOW#alice", GSI1SK: "FOLLOWER#carol", State: storagemodels.RelationshipAccepted},
			}
		case *[]storagemodels.ModerationEvent:
			if len(state.eventsByID) == 0 {
				*d = nil
				return
			}
			*d = []storagemodels.ModerationEvent{state.eventsByID["evt1"], state.eventsByID["evt2"], state.eventsByID["evt3"]}
		case *[]storagemodels.Filter:
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
		case *[]storagemodels.FilterKeyword:
			if len(state.filterKeywords) == 0 {
				*d = []storagemodels.FilterKeyword{
					{
						ID:        "kw-1",
						FilterID:  "filter-1",
						Keyword:   "spam",
						WholeWord: true,
						CreatedAt: time.Now().Add(-1 * time.Hour),
					},
				}
				return
			}
			filterID, _ := state.whereString("PK")
			filterID = strings.TrimPrefix(filterID, "FILTER#")
			if keywords, ok := state.filterKeywords[filterID]; ok {
				*d = keywords
				return
			}
			*d = []storagemodels.FilterKeyword{}
		case *[]storagemodels.FilterStatus:
			if len(state.filterStatuses) == 0 {
				*d = []storagemodels.FilterStatus{
					{
						ID:        "fs-1",
						FilterID:  "filter-1",
						StatusID:  "status-1",
						CreatedAt: time.Now().Add(-1 * time.Hour),
					},
				}
				return
			}
			filterID, _ := state.whereString("PK")
			filterID = strings.TrimPrefix(filterID, "FILTER#")
			if statuses, ok := state.filterStatuses[filterID]; ok {
				*d = statuses
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
		case *[]*storagemodels.DynamoDBCostRecord:
			if len(state.costRecords) > 0 {
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
			if len(state.costAggregations) > 0 {
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
		case *[]storagemodels.MetricRecord:
			if len(state.metricRecords) > 0 {
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
		case *[]storagemodels.QuoteRelationship:
			*d = state.quoteRelationships
		case *[]storagemodels.InstanceDomainBlock:
			*d = state.domainBlocks
		case *[]storagemodels.ModerationReview:
			*d = state.moderationReviews
		case *[]storagemodels.Export:
			*d = state.exportList
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
			if id, ok := state.whereString("DeviceID"); ok {
				deviceID = id
			}
			if deviceID != "" {
				if device, ok := state.devicesByID[deviceID]; ok {
					*d = []storagemodels.Device{device}
					return
				}
			}
			*d = []storagemodels.Device{}
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
		case *[]storagemodels.Status:
			*d = []storagemodels.Status{state.statusByID["s1"]}
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
