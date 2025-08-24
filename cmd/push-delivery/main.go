// Package main implements the push-delivery Lambda function for delivering web push notifications.
package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// Push notification delivery status constants
const (
	PushStatusPending             = "pending"
	PushStatusDelivered           = "delivered"
	PushStatusFailed              = "failed"
	PushStatusSubscriptionInvalid = "subscription_invalid"
)

// PushDeliveryProcessor handles push notification delivery via SQS
type PushDeliveryProcessor struct {
	repos       core.RepositoryStorage
	logger      *zap.Logger
	cfg         *config.Config
	rateLimiter *RateLimiter
	httpClient  *http.Client
}

// PushMessage represents a message from the SQS queue
type PushMessage struct {
	Username         string `json:"username"`
	NotificationType string `json:"notification_type"`
	Title            string `json:"title"`
	Body             string `json:"body"`
	Icon             string `json:"icon,omitempty"`
	NotificationID   string `json:"notification_id"`
	AccessToken      string `json:"access_token"`
}

// WebPushPayload represents the payload sent to the push service
type WebPushPayload struct {
	Title            string `json:"title"`
	Body             string `json:"body"`
	Icon             string `json:"icon,omitempty"`
	Badge            string `json:"badge,omitempty"`
	PreferredLocale  string `json:"preferred_locale"`
	NotificationType string `json:"notification_type"`
	NotificationID   string `json:"notification_id"`
	AccessToken      string `json:"access_token"`
}

// RateLimiter provides simple rate limiting per user
type RateLimiter struct {
	limits map[string]*userLimit
	mu     sync.RWMutex
}

type userLimit struct {
	count     int
	resetTime time.Time
}

// Allow checks if a user is within rate limits (100 notifications per hour)
func (rl *RateLimiter) Allow(userID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	limit, exists := rl.limits[userID]

	if !exists || now.After(limit.resetTime) {
		rl.limits[userID] = &userLimit{
			count:     1,
			resetTime: now.Add(time.Hour),
		}
		return true
	}

	if limit.count >= 100 {
		return false
	}

	limit.count++
	return true
}

// NewPushDeliveryProcessor creates a new push delivery processor
func NewPushDeliveryProcessor() (*PushDeliveryProcessor, error) {
	// Use standardized Lambda initialization
	lambdaCtx := common.MustInitializeLambda(common.LambdaConfig{
		ServiceName:        "push-delivery",
		LambdaType:         common.LambdaTypeProcessor,
		Version:            "1.0.0",
		EnableMetrics:      true,
		EnableHealthCheck:  false,
		EnableTracing:      true,
		EnableCostTracking: true,
	})

	// Initialize storage independently to avoid import cycles
	db, err := dynamorm.GetClient(context.Background())
	if err != nil {
		return nil, ErrDynamoDBInit(err)
	}

	// Initialize repository factory
	tableName := lambdaCtx.Config.DynamoTableName
	if err := common.ValidateRequiredParam("tableName", tableName); err != nil {
		tableName = "lesser-main"
	}
	repos, err := factory.NewRepositoryFactory(db, tableName, lambdaCtx.Logger)
	if err != nil {
		return nil, ErrRepositoryFactory(err)
	}

	// Set storage in lambdaCtx for reference
	lambdaCtx.DynamoDB = db
	lambdaCtx.Repos = repos

	return &PushDeliveryProcessor{
		repos:  repos,
		logger: lambdaCtx.Logger,
		cfg:    lambdaCtx.Config,
		rateLimiter: &RateLimiter{
			limits: make(map[string]*userLimit),
		},
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// HandleSQSBatch processes a batch of SQS messages concurrently
func (pdp *PushDeliveryProcessor) HandleSQSBatch(ctx *lift.Context, event events.SQSEvent) error {
	requestID, _ := ctx.Get("requestID").(string)
	if err := common.ValidateRequiredParam("requestID", requestID); err != nil {
		requestID = fmt.Sprintf("push-%d", time.Now().UnixNano())
		ctx.Set("requestID", requestID)
	}

	pdp.logger.Info("processing push notification batch",
		zap.String("request_id", requestID),
		zap.Int("message_count", len(event.Records)),
	)

	// Process messages concurrently with error collection
	results := make(chan error, len(event.Records))
	failures := make([]error, 0)
	var failureMutex sync.Mutex

	for _, record := range event.Records {
		go func(msg events.SQSMessage) {
			err := pdp.processMessage(ctx, msg)
			results <- err

			if err != nil {
				failureMutex.Lock()
				failures = append(failures, err)
				failureMutex.Unlock()
			}
		}(record)
	}

	// Collect results
	for i := 0; i < len(event.Records); i++ {
		<-results
	}

	// Return batch item failures for SQS to retry specific messages
	if len(failures) > 0 {
		pdp.logger.Error("batch processing had failures",
			zap.String("request_id", requestID),
			zap.Int("failure_count", len(failures)),
			zap.Int("total_count", len(event.Records)),
		)

		// Return a custom batch error for SQS partial failure handling
		return lift.NewLiftError("PARTIAL_FAILURE", "partial batch failure", 500).
			WithDetail("failed_count", len(failures)).
			WithDetail("total_count", len(event.Records))
	}

	pdp.logger.Info("batch processing completed successfully",
		zap.String("request_id", requestID),
		zap.Int("processed_count", len(event.Records)),
	)

	return nil
}

// processMessage processes a single SQS message
func (pdp *PushDeliveryProcessor) processMessage(ctx *lift.Context, msg events.SQSMessage) error {
	start := time.Now()

	// Parse notification
	var notification PushMessage
	if err := json.Unmarshal([]byte(msg.Body), &notification); err != nil {
		pdp.logger.Error("invalid message format",
			zap.String("message_id", msg.MessageId),
			zap.Error(err),
		)
		return ErrInvalidMessageFormat(err)
	}

	pdp.logger.Info("processing push notification",
		zap.String("message_id", msg.MessageId),
		zap.String("username", notification.Username),
		zap.String("type", notification.NotificationType),
	)

	// Check rate limits
	if !pdp.rateLimiter.Allow(notification.Username) {
		pdp.logger.Warn("rate limit exceeded",
			zap.String("username", notification.Username),
		)
		// Don't return error - just drop the notification
		return nil
	}

	// Get user's push subscriptions
	subscriptions, err := pdp.repos.PushSubscription().GetUserPushSubscriptions(ctx.Context, notification.Username)
	if err != nil {
		pdp.logger.Error("failed to get push subscriptions",
			zap.String("username", notification.Username),
			zap.Error(err),
		)
		return ErrGetPushSubscriptions(err)
	}

	if err := common.ValidateSliceNotEmpty("subscriptions", subscriptions); err != nil {
		pdp.logger.Debug("no push subscriptions for user",
			zap.String("username", notification.Username),
		)
		return nil
	}

	// Get VAPID keys
	vapidKeys, err := pdp.repos.PushSubscription().GetVAPIDKeys(ctx.Context)
	if err != nil {
		pdp.logger.Error("failed to get VAPID keys", zap.Error(err))
		return ErrGetVAPIDKeys(err)
	}

	// Send to each subscription
	deliveryResults := make([]DeliveryResult, 0, len(subscriptions))
	for _, sub := range subscriptions {
		// Check if this notification type is enabled
		if !shouldSendNotification(sub.Alerts, notification.NotificationType) {
			pdp.logger.Debug("notification type not enabled for subscription",
				zap.String("subscription_id", sub.ID),
				zap.String("type", notification.NotificationType),
			)
			continue
		}

		// Send the notification
		result := pdp.sendWebPush(ctx.Context, sub, &notification, vapidKeys)
		deliveryResults = append(deliveryResults, result)

		// Track delivery status
		if err := pdp.trackDelivery(ctx.Context, notification, result); err != nil {
			pdp.logger.Warn("failed to track delivery status",
				zap.String("subscription_id", sub.ID),
				zap.Error(err),
			)
		}
	}

	// Record metrics
	duration := time.Since(start)
	status := "success"
	if hasDeliveryFailures(deliveryResults) {
		status = "partial_failure"
	}

	pdp.recordMetrics(ctx, notification.NotificationType, duration, nil, status)

	pdp.logger.Info("push notification processing completed",
		zap.String("message_id", msg.MessageId),
		zap.String("username", notification.Username),
		zap.Int("subscription_count", len(subscriptions)),
		zap.Duration("duration", duration),
	)

	return nil
}

// DeliveryResult represents the result of a push notification delivery
type DeliveryResult struct {
	SubscriptionID string
	Status         string
	Error          error
	StatusCode     int
}

// sendWebPush sends a web push notification to a specific subscription
func (pdp *PushDeliveryProcessor) sendWebPush(ctx context.Context, subscription *storage.PushSubscription, msg *PushMessage, vapidKeys *storage.VAPIDKeys) DeliveryResult {
	result := DeliveryResult{
		SubscriptionID: subscription.ID,
		Status:         PushStatusPending,
	}

	// Create the payload
	payload := WebPushPayload{
		Title:            msg.Title,
		Body:             msg.Body,
		Icon:             msg.Icon,
		Badge:            pdp.cfg.BaseURL() + "/badge.png",
		PreferredLocale:  "en",
		NotificationType: msg.NotificationType,
		NotificationID:   msg.NotificationID,
		AccessToken:      msg.AccessToken,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		result.Status = PushStatusFailed
		result.Error = ErrMarshalPayload(err)
		return result
	}

	// Encrypt the payload
	encryptedPayload, salt, serverPublicKey, err := pdp.encryptPayload(payloadBytes, subscription.P256dh, subscription.Auth)
	if err != nil {
		result.Status = PushStatusFailed
		result.Error = ErrEncryptPayload(err)
		return result
	}

	// Create VAPID JWT
	vapidJWT, err := pdp.createVAPIDJWT(subscription.Endpoint, vapidKeys.Subject, vapidKeys.PrivateKey)
	if err != nil {
		result.Status = PushStatusFailed
		result.Error = ErrCreateVAPIDJWT(err)
		return result
	}

	// Send the request
	req, err := http.NewRequestWithContext(ctx, "POST", subscription.Endpoint, strings.NewReader(encryptedPayload))
	if err != nil {
		result.Status = PushStatusFailed
		result.Error = ErrCreateRequest(err)
		return result
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", "86400") // 24 hours
	req.Header.Set("Urgency", "normal")
	req.Header.Set("Authorization", fmt.Sprintf("vapid t=%s, k=%s", vapidJWT, vapidKeys.PublicKey))
	req.Header.Set("Crypto-Key", fmt.Sprintf("dh=%s", serverPublicKey))
	req.Header.Set("Encryption", fmt.Sprintf("salt=%s", salt))

	resp, err := pdp.httpClient.Do(req)
	if err != nil {
		result.Status = PushStatusFailed
		result.Error = ErrSendRequest(err)
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	result.StatusCode = resp.StatusCode

	if resp.StatusCode >= 400 {
		// Handle different error codes
		switch resp.StatusCode {
		case 404, 410:
			// Subscription is no longer valid, delete it
			pdp.logger.Info("deleting invalid subscription",
				zap.String("subscription_id", subscription.ID),
				zap.Int("status_code", resp.StatusCode),
			)
			if err := pdp.repos.PushSubscription().DeletePushSubscription(ctx, subscription.Username, subscription.ID); err != nil {
				pdp.logger.Error("failed to delete invalid subscription",
					zap.String("subscription_id", subscription.ID),
					zap.Error(err),
				)
			}
			result.Status = PushStatusSubscriptionInvalid
		default:
			pdp.logger.Warn("push service returned error",
				zap.String("subscription_id", subscription.ID),
				zap.Int("status_code", resp.StatusCode),
			)
			result.Status = PushStatusFailed
		}
		result.Error = ErrPushServiceError()
		// Log the status code for debugging
		pdp.logger.Error("push service error",
			zap.String("subscription_id", subscription.ID),
			zap.Int("status_code", resp.StatusCode),
		)
		return result
	}

	result.Status = PushStatusDelivered
	pdp.logger.Info("successfully sent push notification",
		zap.String("subscription_id", subscription.ID),
		zap.String("type", msg.NotificationType),
	)

	return result
}

// trackDelivery tracks the delivery status of a push notification
func (pdp *PushDeliveryProcessor) trackDelivery(ctx context.Context, notification PushMessage, result DeliveryResult) error {
	now := time.Now()

	// Record the push notification delivery attempt using RecordActivity
	activityType := fmt.Sprintf("push_delivery_%s_%s", notification.NotificationType, result.Status)
	if err := pdp.repos.Activity().RecordActivity(ctx, activityType, notification.Username, now); err != nil {
		pdp.logger.Error("failed to record push delivery activity",
			zap.String("notification_id", notification.NotificationID),
			zap.String("username", notification.Username),
			zap.Error(err),
		)
		// Don't return error - delivery tracking failure shouldn't fail the main operation
	}

	pdp.logger.Info("push delivery status recorded",
		zap.String("notification_id", notification.NotificationID),
		zap.String("subscription_id", result.SubscriptionID),
		zap.String("username", notification.Username),
		zap.String("status", result.Status),
		zap.Int("status_code", result.StatusCode),
		zap.String("notification_type", notification.NotificationType),
	)

	return nil
}

// recordMetrics records performance and delivery metrics
func (pdp *PushDeliveryProcessor) recordMetrics(_ *lift.Context, notificationType string, duration time.Duration, err error, status string) {
	// Record push notification metrics to CloudWatch via structured logging
	// The log aggregation system will parse these and send to CloudWatch
	pdp.logger.Info("push_delivery_metrics",
		zap.String("metric_type", "push_notification"),
		zap.String("notification_type", notificationType),
		zap.Duration("duration", duration),
		zap.String("status", status),
		zap.Bool("has_error", err != nil),
		zap.Int64("duration_ms", duration.Milliseconds()),
		zap.String("component", "push-delivery"),
	)

	// Also log as a structured metric event for easy parsing by log processors
	pdp.logger.Info("METRIC",
		zap.String("name", "push_delivery_duration"),
		zap.Float64("value", float64(duration.Milliseconds())),
		zap.String("unit", "milliseconds"),
		zap.String("notification_type", notificationType),
		zap.String("status", status),
	)

	pdp.logger.Info("METRIC",
		zap.String("name", "push_delivery_count"),
		zap.Float64("value", 1),
		zap.String("unit", "count"),
		zap.String("notification_type", notificationType),
		zap.String("status", status),
	)
}

// Helper functions from the original implementation

func shouldSendNotification(alerts storage.PushSubscriptionAlerts, notificationType string) bool {
	switch notificationType {
	case "follow":
		return alerts.Follow
	case "favourite":
		return alerts.Favourite
	case "reblog":
		return alerts.Reblog
	case "mention":
		return alerts.Mention
	case "poll":
		return alerts.Poll
	case "follow_request":
		return alerts.FollowRequest
	case "status":
		return alerts.Status
	case "update":
		return alerts.Update
	case "admin.sign_up":
		return alerts.AdminSignUp
	case "admin.report":
		return alerts.AdminReport
	default:
		return false
	}
}

func hasDeliveryFailures(results []DeliveryResult) bool {
	for _, result := range results {
		if result.Status == PushStatusFailed {
			return true
		}
	}
	return false
}

// Encryption and VAPID functions (copied from original implementation)

func (pdp *PushDeliveryProcessor) createVAPIDJWT(endpoint, subject, privateKeyBase64 string) (string, error) {
	// Parse the endpoint to get the audience
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", ErrParseEndpoint(err)
	}
	audience := fmt.Sprintf("%s://%s", u.Scheme, u.Host)

	// Create the JWT header
	header := map[string]any{
		"typ": "JWT",
		"alg": "ES256",
	}

	// Create the JWT claims
	claims := map[string]any{
		"aud": audience,
		"exp": time.Now().Add(12 * time.Hour).Unix(),
		"sub": subject,
	}

	// Encode header and claims
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	headerEncoded := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsEncoded := base64.RawURLEncoding.EncodeToString(claimsJSON)

	message := headerEncoded + "." + claimsEncoded

	// Decode the private key
	privateKeyBytes, err := base64.RawURLEncoding.DecodeString(privateKeyBase64)
	if err != nil {
		return "", ErrDecodePrivateKey(err)
	}

	// Create ECDSA private key
	privateKey := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: elliptic.P256(),
		},
		D: new(big.Int).SetBytes(privateKeyBytes),
	}

	// Calculate the public key
	privateKey.X, privateKey.Y = privateKey.ScalarBaseMult(privateKeyBytes)

	// Sign the message
	hash := sha256.Sum256([]byte(message))
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash[:])
	if err != nil {
		return "", ErrSign(err)
	}

	// Encode signature in correct format (r || s)
	signature := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()

	// Pad r and s to 32 bytes each
	copy(signature[32-len(rBytes):32], rBytes)
	copy(signature[64-len(sBytes):64], sBytes)

	signatureEncoded := base64.RawURLEncoding.EncodeToString(signature)

	return message + "." + signatureEncoded, nil
}

func (pdp *PushDeliveryProcessor) encryptPayload(payload []byte, p256dhBase64, authBase64 string) (string, string, string, error) {
	// Decode the client's public key and auth secret
	clientPublicKeyBytes, err := base64.RawURLEncoding.DecodeString(p256dhBase64)
	if err != nil {
		return "", "", "", ErrDecodeP256dh(err)
	}

	authSecret, err := base64.RawURLEncoding.DecodeString(authBase64)
	if err != nil {
		return "", "", "", ErrDecodeAuth(err)
	}

	// Generate server key pair
	serverPrivateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", "", ErrGenerateServerKey(err)
	}

	// Convert server private key to ECDH and get public key bytes
	serverECDHKey, err := serverPrivateKey.ECDH()
	if err != nil {
		return "", "", "", ErrConvertToECDH(err)
	}
	serverPublicKeyBytes := serverECDHKey.PublicKey().Bytes()

	// Convert client public key bytes to ECDH public key
	clientECDHPublicKey, err := ecdh.P256().NewPublicKey(clientPublicKeyBytes)
	if err != nil {
		return "", "", "", ErrParseClientPublicKey(err)
	}
	// Perform ECDH key exchange
	sharedSecret, err := serverECDHKey.ECDH(clientECDHPublicKey)
	if err != nil {
		return "", "", "", ErrPerformECDH(err)
	}

	// Generate salt
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", "", "", ErrGenerateSalt(err)
	}

	// Derive keys using HKDF
	prk := hkdf(authSecret, sharedSecret, []byte("Content-Encoding: auth\x00"))
	cek := hkdf(salt, prk, buildInfo("aesgcm", clientPublicKeyBytes, serverPublicKeyBytes))[:16]

	// Generate a random nonce for each encryption
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", "", "", ErrGenerateNonce(err)
	}

	// Encrypt the payload
	block, err := aes.NewCipher(cek)
	if err != nil {
		return "", "", "", ErrCreateCipher(err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", "", ErrCreateGCM(err)
	}

	// Add padding
	paddedPayload := append([]byte{0, 0}, payload...)

	// Encrypt
	ciphertext := gcm.Seal(nil, nonce, paddedPayload, nil)

	// Encode results
	saltBase64 := base64.RawURLEncoding.EncodeToString(salt)
	serverPublicKeyBase64 := base64.RawURLEncoding.EncodeToString(serverPublicKeyBytes)
	encryptedPayloadBase64 := base64.RawURLEncoding.EncodeToString(ciphertext)

	return encryptedPayloadBase64, saltBase64, serverPublicKeyBase64, nil
}

func hkdf(salt, ikm, info []byte) []byte {
	h := sha256.New
	prk := hkdfExtract(h, salt, ikm)
	return hkdfExpand(h, prk, append(info, byte(1)))
}

func hkdfExtract(h func() hash.Hash, salt, ikm []byte) []byte {
	mac := hmac.New(h, salt)
	mac.Write(ikm)
	return mac.Sum(nil)
}

func hkdfExpand(h func() hash.Hash, prk, info []byte) []byte {
	mac := hmac.New(h, prk)
	mac.Write(info)
	return mac.Sum(nil)
}

func buildInfo(typ string, clientPublicKey, serverPublicKey []byte) []byte {
	info := []byte("Content-Encoding: " + typ + "\x00P-256\x00")
	info = append(info, 0, 65) // client public key length
	info = append(info, clientPublicKey...)
	info = append(info, 0, 65) // server public key length
	info = append(info, serverPublicKey...)
	return info
}

func main() {
	processor, err := NewPushDeliveryProcessor()
	if err != nil {
		panic(ErrProcessorInitialization(err))
	}

	app := lift.New()

	// Add request ID middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			requestID := fmt.Sprintf("push-%d", time.Now().UnixNano())
			ctx.Set("requestID", requestID)
			return next.Handle(ctx)
		})
	})

	// Add error handling middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			err := next.Handle(ctx)
			if err != nil {
				processor.logger.Error("handler error",
					zap.String("request_id", ctx.Get("requestID").(string)),
					zap.Error(err),
				)
			}
			return err
		})
	})

	// Set SQS handler for push notification delivery
	_ = app.SQS("push-delivery", func(ctx *lift.Context) error {
		// Extract SQS event from Lift context - proper implementation
		if ctx.Request.RawEvent == nil {
			return lift.NewLiftError("MISSING_EVENT", "no SQS event in request", 400)
		}

		// Parse the raw event as SQS event
		var event events.SQSEvent
		if sqsEvent, ok := ctx.Request.RawEvent.(events.SQSEvent); ok {
			event = sqsEvent
		} else {
			// Try to parse from interface if it's a map
			eventBytes, err := json.Marshal(ctx.Request.RawEvent)
			if err != nil {
				return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to marshal raw event", 500).WithCause(err)
			}

			if err := json.Unmarshal(eventBytes, &event); err != nil {
				return lift.NewLiftError("EVENT_PARSE_ERROR", "failed to parse SQS event", 500).WithCause(err)
			}
		}

		return processor.HandleSQSBatch(ctx, event)
	})

	lambda.Start(app.HandleRequest)
}
