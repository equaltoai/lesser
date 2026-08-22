package federation

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	appConfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/httpclient"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// DeliveryService handles sending activities to remote instances
type DeliveryService struct {
	store      FederationStorage
	httpClient interface {
		Do(req *http.Request) (*http.Response, error)
	}
	logger    *zap.Logger
	sqsClient interface {
		SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	}
	queueURL string
	cfg      *appConfig.Config
}

// NewDeliveryService creates a new delivery service
func NewDeliveryService(store FederationStorage, cfg *appConfig.Config) *DeliveryService {
	logger := common.Logger()

	svc := &DeliveryService{
		store:      store,
		httpClient: httpclient.NewSecureClient(httpclient.WithLogger(logger)),
		logger:     logger,
		cfg:        cfg,
	}

	// Initialize SQS client if queue URL is configured
	queueURL := cfg.FederationQueueURL
	if queueURL != "" {
		awsCfg, err := config.LoadDefaultConfig(context.Background())
		if err == nil {
			svc.sqsClient = sqs.NewFromConfig(awsCfg)
			svc.queueURL = queueURL
			logger.Info("SQS queue configured for federation delivery",
				zap.String("queue_url", queueURL))
		} else {
			logger.Warn("Failed to initialize SQS client", zap.Error(err))
		}
	} else {
		logger.Info("No SQS queue configured, using synchronous delivery")
	}

	return svc
}

// DeliverActivityWithPrivacy delivers an activity to a remote inbox with privacy controls
func (d *DeliveryService) DeliverActivityWithPrivacy(ctx context.Context, activity *activitypub.Activity, targetInbox string, signingActor *activitypub.Actor, targetActorID string) error {
	// Create addressing validator
	addressingValidator := activitypub.NewAddressingValidator()

	// Check if this is a direct message and should be delivered to this recipient
	if addressingValidator.IsDirectMessage(activity) {
		// For direct messages, only deliver to explicit recipients
		if !d.isExplicitRecipient(activity, targetActorID) {
			d.logger.Debug("skipping delivery - not an explicit recipient",
				zap.String("target_actor", targetActorID),
				zap.String("activity_id", activity.ID))
			return nil
		}
	}

	// Sanitize activity for delivery (remove BCC, and BTo for non-recipients)
	isRecipient := d.isExplicitRecipient(activity, targetActorID)
	sanitizedActivity := addressingValidator.SanitizeForDelivery(activity, !isRecipient)

	return d.DeliverActivity(ctx, sanitizedActivity, targetInbox, signingActor)
}

// isExplicitRecipient checks if an actor is explicitly mentioned in addressing fields
func (d *DeliveryService) isExplicitRecipient(activity *activitypub.Activity, actorID string) bool {
	// Check all addressing fields
	allRecipients := append(append(append(activity.To, activity.CC...), activity.BTo...), activity.BCC...)

	for _, recipient := range allRecipients {
		if recipient == actorID {
			return true
		}
		// Check if it's a followers collection for this actor
		if strings.Contains(recipient, "/followers") && strings.Contains(recipient, actorID) {
			return true
		}
	}

	return false
}

// DeliverActivity delivers an activity to a remote inbox
func (d *DeliveryService) DeliverActivity(ctx context.Context, activity *activitypub.Activity, targetInbox string, signingActor *activitypub.Actor) error {
	log := common.WithContext(ctx).With(
		zap.String("activity_id", activity.ID),
		zap.String("activity_type", activity.Type),
		zap.String("target_inbox", targetInbox),
		zap.String("signing_actor", signingActor.ID),
	)

	log.Info("delivering activity to remote inbox")

	// Track start time for response time measurement
	startTime := time.Now()

	// Extract domain from target inbox URL
	domain := extractDomainFromURL(targetInbox)

	// Serialize the activity
	body, err := json.Marshal(activity)
	if err != nil {
		log.Error("failed to marshal activity", zap.Error(err))
		return errors.Join(ErrActivityMarshalFailed, err)
	}

	// Record federation activity for cost tracking
	federationActivity := &storage.FederationActivity{
		Domain:       domain,
		Type:         "egress",
		ActivityType: activity.Type,
		ByteSize:     int64(len(body)),
		Timestamp:    startTime,
	}

	traceTarget := targetInbox
	if objectID, ok := activity.Object.(string); ok && strings.TrimSpace(objectID) != "" {
		traceTarget = objectID
	}
	trace := NewFollowTraceMetadata(activity, signingActor.PreferredUsername, traceTarget)
	if trace != nil {
		ctx = WithFollowTrace(ctx, trace)
	}
	activityCtx := context.WithoutCancel(ctx)

	// Get the actor's private key from storage
	privateKeyPEM, err := d.store.GetActorPrivateKey(ctx, signingActor.PreferredUsername)
	if err != nil {
		log.Error("failed to get private key", zap.Error(err))
		federationActivity.Success = false
		federationActivity.ErrorMessage = err.Error()
		federationActivity.ResponseTime = time.Since(startTime).Milliseconds()
		go func() { _ = d.store.RecordFederationActivity(activityCtx, federationActivity) }()
		return errors.Join(ErrPrivateKeyRetrievalFailed, err)
	}

	// Parse the private key
	privateKey, err := ParsePrivateKeyPEM([]byte(privateKeyPEM))
	if err != nil {
		log.Error("failed to parse private key", zap.Error(err))
		federationActivity.Success = false
		federationActivity.ErrorMessage = err.Error()
		federationActivity.ResponseTime = time.Since(startTime).Milliseconds()
		go func() { _ = d.store.RecordFederationActivity(activityCtx, federationActivity) }()
		return errors.Join(ErrPrivateKeyParseFailed, err)
	}

	signingKeyID := ""
	if signingActor.PublicKey != nil {
		signingKeyID = signingActor.PublicKey.ID
	}

	req, err := BuildSignedActivityPubRequest(ctx, http.MethodPost, targetInbox, "Lesser/1.0", body, privateKey, signingKeyID)
	if err != nil {
		log.Error("failed to sign request",
			zap.Error(err))
		federationActivity.Success = false
		federationActivity.ErrorMessage = err.Error()
		federationActivity.ResponseTime = time.Since(startTime).Milliseconds()
		go func() { _ = d.store.RecordFederationActivity(activityCtx, federationActivity) }()
		if req == nil {
			return errors.Join(ErrRequestCreationFailed, err)
		}
		return errors.Join(ErrRequestSigningFailed, err)
	}

	if trace != nil {
		fields := append(FollowTraceFields(trace, "sender.delivery"),
			zap.String("signing_actor_id", signingActor.ID),
			zap.String("signing_actor_public_key_id", signingKeyID),
			zap.String("target_inbox", targetInbox),
		)
		fields = append(fields, BuildSignatureTraceFields(req)...)
		d.logger.Info("federation follow trace", fields...)
	}

	// Send the request
	resp, err := d.httpClient.Do(req)
	if err != nil {
		log.Error("failed to send request", zap.Error(err))
		federationActivity.Success = false
		federationActivity.ErrorMessage = err.Error()
		federationActivity.ResponseTime = time.Since(startTime).Milliseconds()
		go func() { _ = d.store.RecordFederationActivity(activityCtx, federationActivity) }()
		return errors.Join(ErrHTTPRequestFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBodyBytes, respBodyTruncated, readErr := common.ReadUntrustedHTTPResponseBody(resp.Body, common.MaxUntrustedHTTPResponseBodyBytes)
	if readErr != nil {
		log.Warn("failed to read response body", zap.Error(readErr))
	}
	respBody := common.FormatUntrustedHTTPBodySnippet(respBodyBytes, respBodyTruncated)

	// Record response time
	federationActivity.ResponseTime = time.Since(startTime).Milliseconds()

	// Check the response
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Info("activity delivered successfully",
			zap.Int("status_code", resp.StatusCode),
			zap.Bool("response_truncated", respBodyTruncated),
			zap.String("response", respBody))

		federationActivity.Success = true
		go func() { _ = d.store.RecordFederationActivity(activityCtx, federationActivity) }()
		return nil
	}

	log.Warn("activity delivery failed",
		zap.Int("status_code", resp.StatusCode),
		zap.Bool("response_truncated", respBodyTruncated),
		zap.String("response", respBody))

	// Record failure
	federationActivity.Success = false
	federationActivity.ErrorMessage = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, respBody)
	go func() { _ = d.store.RecordFederationActivity(activityCtx, federationActivity) }()

	// Return error for non-2xx status codes
	log.Error("delivery failed with non-2xx status",
		zap.Int("status_code", resp.StatusCode),
		zap.Bool("response_truncated", respBodyTruncated),
		zap.String("response_body", respBody))
	return ErrDeliveryHTTPStatusFailed
}

type resolvedDeliveryTarget struct {
	actor   *activitypub.Actor
	isLocal bool
}

func orderedUniqueRecipients(activity *activitypub.Activity) []string {
	if activity == nil {
		return nil
	}

	seen := make(map[string]bool)
	var recipients []string

	appendRecipients := func(values []string) {
		for _, value := range values {
			recipient := strings.TrimSpace(value)
			if recipient == "" || recipient == activitypub.PublicAddress || seen[recipient] {
				continue
			}
			seen[recipient] = true
			recipients = append(recipients, recipient)
		}
	}

	appendRecipients(activity.To)
	appendRecipients(activity.CC)
	appendRecipients(activity.BTo)
	appendRecipients(activity.BCC)

	return recipients
}

func followersCollectionForActor(actor *activitypub.Actor) string {
	if actor == nil {
		return ""
	}
	if followers := strings.TrimSpace(actor.Followers); followers != "" {
		return followers
	}
	if actorID := strings.TrimSpace(actor.ID); actorID != "" {
		return strings.TrimRight(actorID, "/") + "/followers"
	}
	return ""
}

func isFollowersCollectionForActor(actor *activitypub.Actor, recipient string) bool {
	followersCollection := followersCollectionForActor(actor)
	return followersCollection != "" && strings.EqualFold(strings.TrimSpace(recipient), followersCollection)
}

func (d *DeliveryService) resolveDeliverableTarget(ctx context.Context, identifier string) (*resolvedDeliveryTarget, error) {
	identifier = strings.TrimSpace(identifier)
	if err := common.ValidateRequiredParam("identifier", identifier); err != nil {
		return nil, err
	}

	localDomain := ""
	if d.cfg != nil {
		localDomain = normalizeActorDomain(d.cfg.Domain)
	}

	if isActorURL(identifier) {
		parsed, err := url.Parse(identifier)
		if err != nil {
			return nil, err
		}

		host := normalizeActorDomain(parsed.Hostname())
		if localDomain != "" && host == localDomain {
			username := usernameFromActorPath(parsed.Path)
			if username == "" {
				return nil, common.ActorNotFoundError{Username: identifier}
			}

			actor, err := d.store.GetActor(ctx, username)
			if err != nil {
				return nil, err
			}
			return &resolvedDeliveryTarget{actor: actor, isLocal: true}, nil
		}

		actor, err := d.fetchRemoteActor(ctx, identifier)
		if err != nil {
			return nil, err
		}
		return &resolvedDeliveryTarget{actor: actor}, nil
	}

	username, domain, err := parseHandle(identifier)
	if err != nil {
		return nil, err
	}

	if domain == "" || (localDomain != "" && domain == localDomain) {
		actor, err := d.store.GetActor(ctx, username)
		if err != nil {
			return nil, err
		}
		return &resolvedDeliveryTarget{actor: actor, isLocal: true}, nil
	}

	handle := exactHandle(username, domain)
	actor, err := d.resolveRemoteHandleTarget(ctx, handle)
	if err != nil {
		return nil, err
	}

	return &resolvedDeliveryTarget{actor: actor}, nil
}

func (d *DeliveryService) resolveRemoteHandleTarget(ctx context.Context, handle string) (*activitypub.Actor, error) {
	if cached, err := d.store.GetCachedRemoteActor(ctx, handle); err == nil && cached != nil {
		if err := activitypub.ValidateResolvedActor(cached); err == nil && cachedRemoteActorMatchesHandle(cached, handle) {
			return cached, nil
		}
	}

	username, domain, err := parseHandle(handle)
	if err != nil {
		return nil, err
	}

	actorURL, err := webFingerLookupWithClient(ctx, d.httpClient, d.logger, username, domain)
	if err != nil {
		return nil, errors.Join(ErrWebFingerLookupFailed, err)
	}

	actor, err := d.fetchRemoteActor(ctx, actorURL)
	if err != nil {
		return nil, err
	}

	if err := d.store.CacheRemoteActor(ctx, handle, actor, 24*time.Hour); err != nil {
		d.logger.Warn("failed to cache remote actor resolved by handle",
			zap.String("handle", handle),
			zap.String("actor_id", strings.TrimSpace(actor.ID)),
			zap.Error(err))
	}

	return actor, nil
}

func (d *DeliveryService) resolveFollowerTargets(ctx context.Context, actor *activitypub.Actor, log *zap.Logger) (map[string]*activitypub.Actor, error) {
	followerUsernames, _, err := d.store.GetFollowers(ctx, actor.PreferredUsername, 1000, "")
	if err != nil {
		if log != nil {
			log.Error("failed to get followers", zap.Error(err))
		}
		return nil, errors.Join(ErrGetFollowersFailed, err)
	}

	if log != nil {
		log.Info("found followers", zap.Int("count", len(followerUsernames)))
	}

	targets := make(map[string]*activitypub.Actor)
	for _, followerUsername := range followerUsernames {
		target, err := d.resolveDeliverableTarget(ctx, followerUsername)
		if err != nil {
			if log != nil {
				log.Warn("failed to get follower actor",
					zap.String("username", followerUsername),
					zap.Error(err))
			}
			continue
		}

		follower := target.actor
		if follower == nil || strings.TrimSpace(follower.ID) == "" {
			continue
		}

		// Skip local followers (they already have the activity via fan-out)
		if target.isLocal || isLocalActor(follower.ID, actor.ID) {
			continue
		}

		targets[follower.ID] = follower
	}

	return targets, nil
}

func recipientResolutionPolicy(activity *activitypub.Activity) (expandFollowers bool, skipFollowersCollections bool) {
	addressingValidator := activitypub.NewAddressingValidator()
	return addressingValidator.IsPrivateMessage(activity),
		addressingValidator.IsPublicMessage(activity) || addressingValidator.IsUnlistedMessage(activity)
}

func (d *DeliveryService) addResolvedRecipientTarget(
	ctx context.Context,
	recipient string,
	actor *activitypub.Actor,
	expandFollowers bool,
	skipFollowersCollections bool,
	targets map[string]*activitypub.Actor,
	log *zap.Logger,
) error {
	if strings.Contains(recipient, "/followers") {
		if !isFollowersCollectionForActor(actor, recipient) || skipFollowersCollections || !expandFollowers {
			return nil
		}

		followerTargets, err := d.resolveFollowerTargets(ctx, actor, log)
		if err != nil {
			return err
		}
		for actorID, follower := range followerTargets {
			targets[actorID] = follower
		}
		return nil
	}

	target, err := d.resolveDeliverableTarget(ctx, recipient)
	if err != nil {
		if log != nil {
			log.Warn("failed to resolve recipient actor",
				zap.String("recipient", recipient),
				zap.Error(err))
		}
		return nil
	}
	if target.isLocal || target.actor == nil || strings.TrimSpace(target.actor.ID) == "" {
		return nil
	}

	targets[target.actor.ID] = target.actor
	return nil
}

func (d *DeliveryService) resolveRecipientTargets(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor, log *zap.Logger) (map[string]*activitypub.Actor, error) {
	shouldExpandFollowers, skipFollowersCollections := recipientResolutionPolicy(activity)

	targets := make(map[string]*activitypub.Actor)
	for _, recipient := range orderedUniqueRecipients(activity) {
		if err := d.addResolvedRecipientTarget(ctx, recipient, actor, shouldExpandFollowers, skipFollowersCollections, targets, log); err != nil {
			return nil, err
		}
	}

	return targets, nil
}

type resolvedRecipientGroups struct {
	actorsByID        map[string]*activitypub.Actor
	individualTargets map[string]*activitypub.Actor
	sharedInboxGroups map[string][]string
}

func groupResolvedRecipientTargets(targets map[string]*activitypub.Actor) resolvedRecipientGroups {
	groups := resolvedRecipientGroups{
		actorsByID:        make(map[string]*activitypub.Actor, len(targets)),
		individualTargets: make(map[string]*activitypub.Actor),
		sharedInboxGroups: make(map[string][]string),
	}

	for actorID, recipient := range targets {
		if recipient == nil {
			continue
		}
		groups.actorsByID[actorID] = recipient

		if recipient.Endpoints != nil && strings.TrimSpace(recipient.Endpoints.SharedInbox) != "" {
			sharedInbox := strings.TrimSpace(recipient.Endpoints.SharedInbox)
			groups.sharedInboxGroups[sharedInbox] = append(groups.sharedInboxGroups[sharedInbox], actorID)
			continue
		}

		groups.individualTargets[actorID] = recipient
	}

	return groups
}

func (d *DeliveryService) deliverSharedInboxGroups(
	ctx context.Context,
	activity *activitypub.Activity,
	actor *activitypub.Actor,
	groups resolvedRecipientGroups,
	log *zap.Logger,
) []error {
	var deliveryErrors []error

	for sharedInbox, recipientIDs := range groups.sharedInboxGroups {
		if len(recipientIDs) < 2 {
			recipient := groups.actorsByID[recipientIDs[0]]
			if recipient != nil {
				groups.individualTargets[recipientIDs[0]] = recipient
			}
			continue
		}

		if err := d.deliverToSharedInbox(ctx, activity, sharedInbox, actor, recipientIDs); err != nil {
			if log != nil {
				log.Warn("shared inbox delivery failed, falling back to individual inboxes",
					zap.String("shared_inbox", sharedInbox),
					zap.Error(err))
			}
			for _, recipientID := range recipientIDs {
				if recipient := groups.actorsByID[recipientID]; recipient != nil {
					groups.individualTargets[recipientID] = recipient
				}
			}
		}
	}

	return deliveryErrors
}

func (d *DeliveryService) deliverResolvedIndividualTargets(
	ctx context.Context,
	activity *activitypub.Activity,
	actor *activitypub.Actor,
	individualTargets map[string]*activitypub.Actor,
	log *zap.Logger,
) []error {
	var deliveryErrors []error

	for recipientID, recipientActor := range individualTargets {
		inboxURL := strings.TrimSpace(recipientActor.Inbox)
		if inboxURL == "" {
			continue
		}

		if err := d.DeliverActivityWithPrivacy(ctx, activity, inboxURL, actor, recipientID); err != nil {
			if log != nil {
				log.Error("failed to deliver to individual inbox",
					zap.String("recipient", recipientID),
					zap.String("inbox", inboxURL),
					zap.Error(err))
			}
			deliveryErrors = append(deliveryErrors, err)
		}
	}

	return deliveryErrors
}

func (d *DeliveryService) deliverResolvedRecipientTargets(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor, targets map[string]*activitypub.Actor, log *zap.Logger) error {
	if len(targets) == 0 {
		return nil
	}

	groups := groupResolvedRecipientTargets(targets)
	deliveryErrors := d.deliverSharedInboxGroups(ctx, activity, actor, groups, log)
	deliveryErrors = append(deliveryErrors, d.deliverResolvedIndividualTargets(ctx, activity, actor, groups.individualTargets, log)...)

	if err := common.ValidateSliceNotEmpty("delivery_errors", deliveryErrors); err == nil {
		if log != nil {
			log.Error("failed to deliver to multiple recipients",
				zap.Int("failed_recipient_count", len(deliveryErrors)))
		}
		return ErrDeliveryToDomainsFailed
	}

	return nil
}

func (d *DeliveryService) deliverResolvedFollowerTargets(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor, targets map[string]*activitypub.Actor, log *zap.Logger) error {
	if len(targets) == 0 {
		return nil
	}

	// Deliver to each unique inbox.
	var deliveryErrors []error
	inboxMap := make(map[string][]string)
	for actorID, follower := range targets {
		inboxURL := strings.TrimSpace(follower.Inbox)
		if follower.Endpoints != nil && strings.TrimSpace(follower.Endpoints.SharedInbox) != "" {
			inboxURL = strings.TrimSpace(follower.Endpoints.SharedInbox)
		}
		if inboxURL == "" {
			continue
		}
		inboxMap[inboxURL] = append(inboxMap[inboxURL], actorID)
	}

	for inbox, followerIDs := range inboxMap {
		if log != nil {
			log.Info("delivering to inbox",
				zap.String("inbox", inbox),
				zap.Int("follower_count", len(followerIDs)))
		}

		// For shared inboxes, use the first follower as the target actor ID for privacy checks.
		targetActorID := followerIDs[0]
		if err := d.DeliverActivityWithPrivacy(ctx, activity, inbox, actor, targetActorID); err != nil {
			if log != nil {
				log.Error("failed to deliver to inbox",
					zap.String("inbox", inbox),
					zap.Error(err))
			}
			deliveryErrors = append(deliveryErrors, err)
			// Continue delivering to other inboxes.
		}
	}

	if err := common.ValidateSliceNotEmpty("delivery_errors", deliveryErrors); err == nil {
		if log != nil {
			log.Error("failed to deliver to multiple inboxes",
				zap.Int("failed_inbox_count", len(deliveryErrors)))
		}
		return ErrDeliveryToInboxesFailed
	}

	return nil
}

// DeliverToFollowers delivers an activity to all followers of an actor
func (d *DeliveryService) DeliverToFollowers(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	log := common.WithContext(ctx).With(
		zap.String("activity_id", activity.ID),
		zap.String("actor", actor.ID),
	)

	log.Info("delivering activity to followers")

	targets, err := d.resolveFollowerTargets(ctx, actor, log)
	if err != nil {
		return err
	}

	return d.deliverResolvedFollowerTargets(ctx, activity, actor, targets, log)
}

// DeliverToFollowersAndRecipients delivers public/unlisted activities to
// followers and explicit recipients while suppressing duplicate deliveries to
// actors that appear in both target sets.
func (d *DeliveryService) DeliverToFollowersAndRecipients(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	log := common.WithContext(ctx).With(
		zap.String("activity_id", activity.ID),
		zap.String("actor", actor.ID),
	)

	log.Info("delivering activity to followers and explicit recipients")

	followerTargets, err := d.resolveFollowerTargets(ctx, actor, log)
	if err != nil {
		log.Error("failed to resolve followers for activity delivery",
			zap.String("activity_id", activity.ID),
			zap.String("activity_type", activity.Type),
			zap.Error(err))
		followerTargets = map[string]*activitypub.Actor{}
	}

	if err == nil {
		if err := d.deliverResolvedFollowerTargets(ctx, activity, actor, followerTargets, log); err != nil {
			log.Error("failed to deliver activity to followers",
				zap.String("activity_id", activity.ID),
				zap.String("activity_type", activity.Type),
				zap.Error(err))
		}
	}

	recipientTargets, err := d.resolveRecipientTargets(ctx, activity, actor, log)
	if err != nil {
		return err
	}

	duplicateRecipientCount := 0
	for actorID := range followerTargets {
		if _, exists := recipientTargets[actorID]; exists {
			delete(recipientTargets, actorID)
			duplicateRecipientCount++
		}
	}
	if duplicateRecipientCount > 0 {
		log.Info("suppressed duplicate explicit deliveries already covered by follower fanout",
			zap.Int("duplicate_recipient_count", duplicateRecipientCount))
	}

	log.Info("delivering to remaining explicit recipients with privacy controls",
		zap.Int("resolved_recipients", len(recipientTargets)))

	return d.deliverResolvedRecipientTargets(ctx, activity, actor, recipientTargets, log)
}

// DeliverToRecipients delivers an activity to specific recipients (to, cc, bto, bcc)
func (d *DeliveryService) DeliverToRecipients(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	return d.DeliverToRecipientsWithPrivacy(ctx, activity, actor)
}

// DeliverToRecipientsWithPrivacy delivers an activity to specific recipients with privacy controls
func (d *DeliveryService) DeliverToRecipientsWithPrivacy(ctx context.Context, activity *activitypub.Activity, actor *activitypub.Actor) error {
	log := common.WithContext(ctx).With(
		zap.String("activity_id", activity.ID),
		zap.String("actor", actor.ID),
	)

	resolvedTargets, err := d.resolveRecipientTargets(ctx, activity, actor, log)
	if err != nil {
		return err
	}

	log.Info("delivering to recipients with privacy controls",
		zap.Int("resolved_recipients", len(resolvedTargets)))

	return d.deliverResolvedRecipientTargets(ctx, activity, actor, resolvedTargets, log)
}

// deliverToSharedInbox delivers to a shared inbox with proper privacy controls
func (d *DeliveryService) deliverToSharedInbox(ctx context.Context, activity *activitypub.Activity, sharedInbox string, actor *activitypub.Actor, recipients []string) error {
	log := common.WithContext(ctx).With(
		zap.String("shared_inbox", sharedInbox),
		zap.Int("recipient_count", len(recipients)),
	)

	// For shared inbox delivery, we sanitize the activity to remove BCC
	// but keep BTo for recipients who should see it
	addressingValidator := activitypub.NewAddressingValidator()
	sanitizedActivity := addressingValidator.SanitizeForDelivery(activity, false)

	log.Info("delivering to shared inbox")
	return d.DeliverActivity(ctx, sanitizedActivity, sharedInbox, actor)
}

// fetchRemoteActor fetches a remote actor by their ID
func (d *DeliveryService) fetchRemoteActor(ctx context.Context, actorID string) (*activitypub.Actor, error) {
	// Check cache first
	cached, err := d.store.GetCachedRemoteActor(ctx, actorID)
	if err == nil && cached != nil {
		return cached, nil
	}

	// Fetch from remote using secure client
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, actorID, nil)
	if err != nil {
		return nil, errors.Join(ErrRequestCreationFailed, err)
	}

	req.Header.Set("Accept", ActivityPubAcceptType)
	req.Header.Set("User-Agent", UserAgent)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, errors.Join(ErrFetchRemoteActorFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, bodyTruncated, readErr := common.ReadUntrustedHTTPResponseBody(resp.Body, common.MaxUntrustedHTTPResponseBodyBytes)
		if readErr != nil {
			d.logger.Error("failed to read actor fetch response body", zap.String("actor_id", actorID), zap.Error(readErr))
		}
		body := common.FormatUntrustedHTTPBodySnippet(bodyBytes, bodyTruncated)
		d.logger.Error("failed to fetch actor with non-2xx status",
			zap.String("actor_id", actorID),
			zap.Int("status_code", resp.StatusCode),
			zap.Bool("response_truncated", bodyTruncated),
			zap.String("response_body", body))
		return nil, ErrFetchActorHTTPStatusFailed
	}

	var actor activitypub.Actor
	if err := common.ParseHTTPResponse(resp.Body, &actor); err != nil {
		return nil, errors.Join(ErrActorDecodeFailed, err)
	}

	// Validate domain consistency: the fetched actor document's declared ID
	// must belong to the same domain as the URL used to fetch it.
	if err := common.ValidateActorDomainConsistency(actorID, actor.ID); err != nil {
		d.logger.Error("actor domain mismatch in delivery fetch — possible spoofing",
			zap.String("fetch_url", actorID),
			zap.String("declared_id", actor.ID),
			zap.Error(err))
		return nil, errors.Join(ErrActorDomainMismatch, err)
	}

	// Cache the actor (ignore errors)
	// Extract handle from actor ID
	handle := extractHandleFromActorID(actor.ID, actor.PreferredUsername)
	if handle != "" {
		if err := d.store.CacheRemoteActor(ctx, handle, &actor, 24*time.Hour); err != nil {
			d.logger.Warn("failed to cache remote actor",
				zap.String("actor_id", actor.ID),
				zap.Error(err))
		}
	}

	return &actor, nil
}

// isLocalActor checks if two actor IDs share the same domain using
// URL hostname comparison rather than substring matching.
func isLocalActor(actorID, localActorID string) bool {
	localDomain := common.ExtractDomainFromActorID(localActorID)
	if localDomain == "" {
		return false
	}
	return common.IsLocalActorID(actorID, localDomain)
}

// QueueDelivery queues an activity for async delivery with proper retry handling
func (d *DeliveryService) QueueDelivery(ctx context.Context, activity *activitypub.Activity, targetInbox string, signingActor *activitypub.Actor) error {
	// Check delivery mode configuration
	deliveryMode := d.cfg.FederationDeliveryMode
	if err := common.ValidateRequiredParam("delivery_mode", deliveryMode); err != nil {
		deliveryMode = "sync" // Default to sync for backwards compatibility
	}

	// Check if SQS is available for async delivery
	sqsClient := d.getSQSClient()
	queueURL := d.getQueueURL()

	// Force sync delivery if explicitly configured or SQS not available
	if deliveryMode == "sync" || sqsClient == nil || common.ValidateRequiredParam("queue_url", queueURL) != nil {
		d.logger.Debug("using synchronous delivery",
			zap.String("reason", fmt.Sprintf("mode=%s, sqs_available=%t, queue_url_set=%t",
				deliveryMode, sqsClient != nil, queueURL != "")),
			zap.String("activity_id", activity.ID),
			zap.String("target_inbox", targetInbox))
		return d.DeliverActivity(ctx, activity, targetInbox, signingActor)
	}

	// Create delivery message for async processing
	deliveryID := fmt.Sprintf("delivery_%s_%d", generateDeliveryID(), time.Now().UnixNano())

	// Get retry configuration from environment
	maxRetries := getEnvInt("FEDERATION_MAX_RETRIES", 5)

	message := map[string]any{
		"delivery_id":      deliveryID,
		"activity":         activity,
		"target_inbox":     targetInbox,
		"signing_actor_id": signingActor.PreferredUsername,
		"retry_count":      0,
		"max_retries":      maxRetries,
		"created_at":       time.Now(),
	}

	messageJSON, err := json.Marshal(message)
	if err != nil {
		d.logger.Error("failed to marshal delivery message", zap.Error(err))
		// Fall back to sync delivery on serialization error
		d.logger.Warn("falling back to synchronous delivery due to message marshal error")
		return d.DeliverActivity(ctx, activity, targetInbox, signingActor)
	}

	// Calculate priority based on activity type and target domain health
	priority := d.calculateDeliveryPriority(ctx, activity, targetInbox)

	// Send to SQS queue with attributes for processing optimization
	_, err = sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(queueURL),
		MessageBody: aws.String(string(messageJSON)),
		MessageAttributes: map[string]types.MessageAttributeValue{
			"activity_type": {
				DataType:    aws.String("String"),
				StringValue: aws.String(activity.Type),
			},
			"target_domain": {
				DataType:    aws.String("String"),
				StringValue: aws.String(extractDomainFromURL(targetInbox)),
			},
			"priority": {
				DataType:    aws.String("Number"),
				StringValue: aws.String(fmt.Sprintf("%d", priority)),
			},
			"delivery_id": {
				DataType:    aws.String("String"),
				StringValue: aws.String(deliveryID),
			},
		},
	})
	if err != nil {
		d.logger.Error("failed to send to SQS, falling back to synchronous delivery",
			zap.String("delivery_id", deliveryID),
			zap.String("activity_id", activity.ID),
			zap.Error(err))

		// Record the SQS failure for monitoring
		d.recordSQSFailure(ctx, deliveryID, err)

		// Fall back to synchronous delivery
		return d.DeliverActivity(ctx, activity, targetInbox, signingActor)
	}

	d.logger.Info("queued activity for async delivery",
		zap.String("delivery_id", deliveryID),
		zap.String("activity_id", activity.ID),
		zap.String("activity_type", activity.Type),
		zap.String("target_inbox", targetInbox),
		zap.String("target_domain", extractDomainFromURL(targetInbox)),
		zap.Int("priority", priority),
		zap.Int("max_retries", maxRetries),
		zap.String("queue_url", queueURL))

	// Record delivery queued metrics
	d.recordDeliveryMetrics(ctx, "queued", deliveryID, activity.Type, extractDomainFromURL(targetInbox))

	return nil
}

// getSQSClient returns the SQS client if configured
func (d *DeliveryService) getSQSClient() interface {
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
} {
	// This would be initialized in NewDeliveryService if SQS is configured
	// For now, return nil to indicate SQS is not configured
	return d.sqsClient
}

// getQueueURL returns the configured queue URL
func (d *DeliveryService) getQueueURL() string {
	// This would come from configuration
	return d.queueURL
}

// generateDeliveryID generates a unique delivery ID
func generateDeliveryID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to less random source on error
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", b)
}

// extractHandleFromActorID extracts a handle (e.g., @user@domain) from an actor ID
func extractHandleFromActorID(actorID, preferredUsername string) string {
	// Extract domain from actor ID
	// Format: https://domain.com/users/username
	domain := common.ExtractDomainFromActorID(actorID)
	if err := common.ValidateMultipleRequiredParams(map[string]string{
		"domain":             domain,
		"preferred_username": preferredUsername,
	}); err != nil {
		return ""
	}
	return fmt.Sprintf("@%s@%s", preferredUsername, domain)
}

// extractDomainFromURL extracts the domain from a URL
func extractDomainFromURL(urlStr string) string {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "unknown"
	}
	return parsedURL.Host
}

// DeliverDirectMessage delivers a direct message to specific recipients
func (d *DeliveryService) DeliverDirectMessage(ctx context.Context, activity *activitypub.Activity, signingActor *activitypub.Actor) error {
	log := common.WithContext(ctx).With(
		zap.String("activity_id", activity.ID),
		zap.String("actor", signingActor.ID),
	)

	log.Info("delivering direct message")

	// Get all recipients from addressing fields
	addressingValidator := activitypub.NewAddressingValidator()
	targets := addressingValidator.DetermineDeliveryRecipients(activity)

	if len(targets.DirectRecipients) == 0 {
		log.Info("no remote recipients for direct message")
		return nil
	}

	// Group recipients by inbox (prefer shared inbox)
	inboxMap := make(map[string][]string) // inbox URL -> recipient IDs

	for recipientID := range targets.DirectRecipients {
		target, err := d.resolveDeliverableTarget(ctx, recipientID)
		if err != nil {
			log.Warn("failed to get recipient actor",
				zap.String("recipient_id", recipientID),
				zap.Error(err))
			continue
		}
		if target.isLocal {
			continue
		}
		recipient := target.actor

		// Determine inbox URL (prefer shared inbox for efficiency)
		inboxURL := recipient.Inbox
		if recipient.Endpoints != nil && recipient.Endpoints.SharedInbox != "" {
			inboxURL = recipient.Endpoints.SharedInbox
		}

		inboxMap[inboxURL] = append(inboxMap[inboxURL], recipientID)
	}

	// Deliver to each unique inbox with privacy controls
	var deliveryErrors []error
	for inbox, recipientIDs := range inboxMap {
		log.Info("delivering direct message to inbox",
			zap.String("inbox", inbox),
			zap.Int("recipient_count", len(recipientIDs)))

		// For shared inboxes, deliver once but ensure privacy
		targetActorID := recipientIDs[0] // Use first recipient for privacy checks
		if err := d.DeliverActivityWithPrivacy(ctx, activity, inbox, signingActor, targetActorID); err != nil {
			log.Error("failed to deliver direct message to inbox",
				zap.String("inbox", inbox),
				zap.Error(err))
			deliveryErrors = append(deliveryErrors, err)
			// Continue delivering to other inboxes
		}
	}

	if err := common.ValidateSliceNotEmpty("delivery_errors", deliveryErrors); err == nil {
		log.Error("failed to deliver direct message to multiple inboxes",
			zap.Int("failed_inbox_count", len(deliveryErrors)))
		return ErrDeliveryDirectMessageToInboxesFailed
	}

	return nil
}

// calculateDeliveryPriority calculates delivery priority based on activity type and target health
func (d *DeliveryService) calculateDeliveryPriority(ctx context.Context, activity *activitypub.Activity, targetInbox string) int {
	priority := 5 // Default priority

	// Higher priority for interactive activities
	switch activity.Type {
	case "Create":
		priority = 7 // Posts are important
	case "Like", "Announce":
		priority = 6 // Interactions are moderately important
	case "Follow", "Accept":
		priority = 8 // Social interactions are high priority
	case "Undo":
		priority = 7 // Undos should be processed quickly
	case "Delete":
		priority = 9 // Deletions are urgent
	case "Update":
		priority = 6 // Updates are moderately important
	default:
		// Keep default priority of 5
	}

	// Adjust priority based on target domain health (if available)
	targetDomain := extractDomainFromURL(targetInbox)
	if d.store != nil {
		// Try to get instance health to adjust priority
		if instanceStats, err := d.getInstanceHealth(ctx, targetDomain); err == nil {
			// Lower priority for unhealthy instances to avoid overwhelming them
			if instanceStats.ErrorRate > 0.3 {
				priority = maxInt(1, priority-2)
			} else if instanceStats.ErrorRate > 0.1 {
				priority = maxInt(1, priority-1)
			}
		}
	}

	return priority
}

// recordSQSFailure records SQS failures for monitoring
func (d *DeliveryService) recordSQSFailure(_ context.Context, deliveryID string, err error) {
	d.logger.Error("SQS delivery queue failure",
		zap.String("delivery_id", deliveryID),
		zap.String("error", err.Error()),
		zap.String("fallback", "synchronous_delivery"))

	// Could also record metrics here for monitoring SQS health
}

// getInstanceHealth retrieves health stats for a domain (helper for priority calculation)
func (d *DeliveryService) getInstanceHealth(_ context.Context, _ string) (*instanceHealth, error) {
	// This would integrate with existing federation repository methods
	// For now, return a basic healthy status
	return &instanceHealth{
		ErrorRate: 0.1,
		Available: true,
	}, nil
}

// instanceHealth represents basic health metrics for priority calculation
type instanceHealth struct {
	ErrorRate float64
	Available bool
}

// getEnvInt gets an integer environment variable with a default value
func getEnvInt(key string, defaultValue int) int {
	return common.GetEnvInt(key, defaultValue)
}

// max returns the maximum of two integers
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// recordDeliveryMetrics records delivery metrics for monitoring
func (d *DeliveryService) recordDeliveryMetrics(_ context.Context, event, deliveryID, activityType, targetDomain string) {
	d.logger.Info("federation_delivery_metric",
		zap.String("metric_type", "delivery_event"),
		zap.String("event", event), // queued, delivered, failed, retrying
		zap.String("delivery_id", deliveryID),
		zap.String("activity_type", activityType),
		zap.String("target_domain", targetDomain),
		zap.Time("timestamp", time.Now()))

	// Could integrate with CloudWatch EMF or other metrics systems
	// This structured logging format can be parsed by log analysis tools
}
