package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
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
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/storage"
	storageDB "github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/zap"
)

var (
	logger *zap.Logger
	store  storage.Storage
	cfg    *config.Config
)

func init() {
	var err error
	logger, err = zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	cfg = config.Get()

	store, err = storageDB.New()
	if err != nil {
		logger.Fatal("Failed to initialize storage", zap.Error(err))
	}
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

func main() {
	lambda.Start(handleSQSEvent)
}

func handleSQSEvent(ctx context.Context, sqsEvent events.SQSEvent) error {
	for _, record := range sqsEvent.Records {
		if err := processMessage(ctx, record.Body); err != nil {
			logger.Error("failed to process message",
				zap.String("message_id", record.MessageId),
				zap.Error(err))
			// Continue processing other messages
		}
	}
	return nil
}

func processMessage(ctx context.Context, messageBody string) error {
	// Parse the message
	var msg PushMessage
	if err := common.ParseRequestBody([]byte(messageBody), &msg); err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	logger.Info("processing push notification",
		zap.String("username", msg.Username),
		zap.String("type", msg.NotificationType))

	// Get user's push subscriptions
	subscriptions, err := store.GetUserPushSubscriptions(ctx, msg.Username)
	if err != nil {
		return fmt.Errorf("failed to get push subscriptions: %w", err)
	}

	if len(subscriptions) == 0 {
		logger.Debug("no push subscriptions for user", zap.String("username", msg.Username))
		return nil
	}

	// Get VAPID keys
	vapidKeys, err := store.GetVAPIDKeys(ctx)
	if err != nil {
		return fmt.Errorf("failed to get VAPID keys: %w", err)
	}

	// Send to each subscription
	for _, sub := range subscriptions {
		// Check if this notification type is enabled
		if !shouldSendNotification(sub.Alerts, msg.NotificationType) {
			logger.Debug("notification type not enabled for subscription",
				zap.String("subscription_id", sub.ID),
				zap.String("type", msg.NotificationType))
			continue
		}

		// Send the notification
		if err := sendWebPush(ctx, sub, &msg, vapidKeys); err != nil {
			logger.Error("failed to send web push",
				zap.String("subscription_id", sub.ID),
				zap.Error(err))
			// Continue with other subscriptions
		}
	}

	return nil
}

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

func sendWebPush(ctx context.Context, subscription *storage.PushSubscription, msg *PushMessage, vapidKeys *storage.VAPIDKeys) error {
	// Create the payload
	payload := WebPushPayload{
		Title:            msg.Title,
		Body:             msg.Body,
		Icon:             msg.Icon,
		Badge:            cfg.BaseURL() + "/badge.png",
		PreferredLocale:  "en",
		NotificationType: msg.NotificationType,
		NotificationID:   msg.NotificationID,
		AccessToken:      msg.AccessToken,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Encrypt the payload
	encryptedPayload, salt, serverPublicKey, err := encryptPayload(payloadBytes, subscription.P256dh, subscription.Auth)
	if err != nil {
		return fmt.Errorf("failed to encrypt payload: %w", err)
	}

	// Create VAPID JWT
	vapidJWT, err := createVAPIDJWT(subscription.Endpoint, vapidKeys.Subject, vapidKeys.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to create VAPID JWT: %w", err)
	}

	// Send the request
	req, err := http.NewRequestWithContext(ctx, "POST", subscription.Endpoint, strings.NewReader(encryptedPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", "86400") // 24 hours
	req.Header.Set("Urgency", "normal")
	req.Header.Set("Authorization", fmt.Sprintf("vapid t=%s, k=%s", vapidJWT, vapidKeys.PublicKey))
	req.Header.Set("Crypto-Key", fmt.Sprintf("dh=%s", serverPublicKey))
	req.Header.Set("Encryption", fmt.Sprintf("salt=%s", salt))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("failed to close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode >= 400 {
		// Handle different error codes
		switch resp.StatusCode {
		case 404, 410:
			// Subscription is no longer valid, delete it
			logger.Info("deleting invalid subscription",
				zap.String("subscription_id", subscription.ID),
				zap.Int("status_code", resp.StatusCode))
			if err := store.DeletePushSubscription(ctx, subscription.Username, subscription.ID); err != nil {
				logger.Error("failed to delete invalid subscription",
					zap.String("subscription_id", subscription.ID),
					zap.Error(err))
			}
		default:
			logger.Warn("push service returned error",
				zap.String("subscription_id", subscription.ID),
				zap.Int("status_code", resp.StatusCode))
		}
		return fmt.Errorf("push service returned status %d", resp.StatusCode)
	}

	logger.Info("successfully sent push notification",
		zap.String("subscription_id", subscription.ID),
		zap.String("type", msg.NotificationType))

	return nil
}

func createVAPIDJWT(endpoint, subject, privateKeyBase64 string) (string, error) {
	// Parse the endpoint to get the audience
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("failed to parse endpoint: %w", err)
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
		return "", fmt.Errorf("failed to decode private key: %w", err)
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
		return "", fmt.Errorf("failed to sign: %w", err)
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

func encryptPayload(payload []byte, p256dhBase64, authBase64 string) (string, string, string, error) {
	// Decode the client's public key and auth secret
	clientPublicKeyBytes, err := base64.RawURLEncoding.DecodeString(p256dhBase64)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to decode p256dh: %w", err)
	}

	authSecret, err := base64.RawURLEncoding.DecodeString(authBase64)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to decode auth: %w", err)
	}

	// Generate server key pair
	serverPrivateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate server key: %w", err)
	}

	// Marshal server public key
	serverPublicKeyBytes := elliptic.Marshal(serverPrivateKey.Curve, serverPrivateKey.X, serverPrivateKey.Y)

	// Unmarshal client public key
	x, y := elliptic.Unmarshal(elliptic.P256(), clientPublicKeyBytes)
	if x == nil {
		return "", "", "", fmt.Errorf("failed to unmarshal client public key")
	}
	clientPublicKey := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}

	// Perform ECDH
	sharedX, _ := serverPrivateKey.ScalarMult(clientPublicKey.X, clientPublicKey.Y, serverPrivateKey.D.Bytes())
	sharedSecret := sharedX.Bytes()

	// Generate salt
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", "", "", fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive keys using HKDF
	prk := hkdf(authSecret, sharedSecret, []byte("Content-Encoding: auth\x00"))
	cek := hkdf(salt, prk, buildInfo("aesgcm", clientPublicKeyBytes, serverPublicKeyBytes))[:16]

	// Generate a random nonce for each encryption
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", "", "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the payload
	block, err := aes.NewCipher(cek)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create GCM: %w", err)
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
