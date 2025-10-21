package tests

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pay-theory/dynamorm/examples/payment"
	"github.com/pay-theory/dynamorm/examples/payment/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebhookSender tests the webhook notification system
func TestWebhookSender(t *testing.T) {
	// Setup test database
	db, err := initTestDB(t)
	if err != nil {
		t.Skip("Skipping test - DynamoDB connection not available")
		return
	}
	require.NotNil(t, db)

	// Create a test merchant with webhook URL
	merchant := &payment.Merchant{
		ID:            "test-merchant-123",
		Name:          "Test Merchant",
		Email:         "test@merchant.com",
		Status:        "active",
		WebhookURL:    "", // Will be set to test server URL
		WebhookSecret: "test-secret-key",
		Features:      []string{"webhooks"},
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Version:       1,
	}

	// Track received webhooks
	var receivedWebhooks []utils.WebhookPayload
	var mu sync.Mutex

	// Create test webhook server
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.NotEmpty(t, r.Header.Get("X-Webhook-ID"))
		assert.NotEmpty(t, r.Header.Get("X-Webhook-Timestamp"))
		assert.NotEmpty(t, r.Header.Get("X-Webhook-Signature"))

		// Parse webhook payload
		var payload utils.WebhookPayload
		err := json.NewDecoder(r.Body).Decode(&payload)
		require.NoError(t, err)

		// Store received webhook
		mu.Lock()
		receivedWebhooks = append(receivedWebhooks, payload)
		mu.Unlock()

		// Return success
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "received"})
	}))
	defer webhookServer.Close()

	// Update merchant with test server URL
	merchant.WebhookURL = webhookServer.URL
	err = db.Model(merchant).Create()
	require.NoError(t, err)

	// Initialize webhook sender
	sender := utils.NewWebhookSender(db, 2)
	defer sender.Stop()

	// Test payment data
	testPayment := &payment.Payment{
		ID:            "pay-123",
		MerchantID:    merchant.ID,
		Amount:        1000,
		Currency:      "USD",
		Status:        payment.PaymentStatusSucceeded,
		PaymentMethod: "card",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Version:       1,
	}

	// Send webhook
	job := &utils.WebhookJob{
		MerchantID: merchant.ID,
		EventType:  "payment.succeeded",
		PaymentID:  testPayment.ID,
		Data:       testPayment,
	}

	err = sender.Send(job)
	require.NoError(t, err)

	// Wait for webhook delivery
	time.Sleep(2 * time.Second)

	// Verify webhook was received
	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, receivedWebhooks, 1)
	assert.Equal(t, "payment.succeeded", receivedWebhooks[0].EventType)
	assert.NotEmpty(t, receivedWebhooks[0].ID)
	assert.NotNil(t, receivedWebhooks[0].Data)

	// Verify webhook record in database
	var webhookRecord payment.Webhook
	err = db.Model(&payment.Webhook{}).
		Where("MerchantID", "=", merchant.ID).
		Where("EventType", "=", "payment.succeeded").
		First(&webhookRecord)

	require.NoError(t, err)
	assert.Equal(t, payment.WebhookStatusDelivered, webhookRecord.Status)
	assert.Equal(t, 200, webhookRecord.ResponseCode)
	assert.Equal(t, 1, webhookRecord.Attempts)
}

// TestWebhookRetry tests the webhook retry mechanism
func TestWebhookRetry(t *testing.T) {
	// Setup test database
	db, err := initTestDB(t)
	if err != nil {
		t.Skip("Skipping test - DynamoDB connection not available")
		return
	}
	require.NotNil(t, db)

	// Create test merchant
	merchant := &payment.Merchant{
		ID:            "test-merchant-retry",
		Name:          "Test Merchant Retry",
		Email:         "retry@merchant.com",
		Status:        "active",
		WebhookSecret: "test-secret",
		Features:      []string{"webhooks"},
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Version:       1,
	}

	// Track attempt count
	attemptCount := 0
	var mu sync.Mutex

	// Create failing webhook server (returns 500 for first 2 attempts)
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attemptCount++
		currentAttempt := attemptCount
		mu.Unlock()

		if currentAttempt <= 2 {
			// Fail first 2 attempts
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "temporary failure"})
		} else {
			// Succeed on 3rd attempt
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		}
	}))
	defer webhookServer.Close()

	merchant.WebhookURL = webhookServer.URL
	err = db.Model(merchant).Create()
	require.NoError(t, err)

	// Initialize webhook sender
	sender := utils.NewWebhookSender(db, 1)
	defer sender.Stop()

	// Send webhook
	job := &utils.WebhookJob{
		MerchantID: merchant.ID,
		EventType:  "payment.failed",
		PaymentID:  "pay-456",
		Data:       map[string]string{"reason": "insufficient_funds"},
	}

	err = sender.Send(job)
	require.NoError(t, err)

	// Wait for retries
	time.Sleep(10 * time.Second)

	// Verify webhook was eventually delivered
	var webhookRecord payment.Webhook
	err = db.Model(&payment.Webhook{}).
		Where("MerchantID", "=", merchant.ID).
		Where("EventType", "=", "payment.failed").
		First(&webhookRecord)

	require.NoError(t, err)
	assert.Equal(t, payment.WebhookStatusDelivered, webhookRecord.Status)
	assert.Equal(t, 200, webhookRecord.ResponseCode)
	assert.Equal(t, 3, webhookRecord.Attempts)
}

// TestJWTValidation tests the JWT validator
func TestJWTValidation(t *testing.T) {
	// Initialize JWT validator
	validator := utils.NewSimpleJWTValidator(
		"test-secret-key",
		"test-issuer",
		"payment-api",
	)

	// Test cases
	tests := []struct {
		name      string
		token     string
		wantError bool
		errorMsg  string
	}{
		{
			name:      "Valid token",
			token:     createTestToken("merchant-123", "test@example.com", time.Now().Add(time.Hour)),
			wantError: false,
		},
		{
			name:      "Expired token",
			token:     createTestToken("merchant-123", "test@example.com", time.Now().Add(-time.Hour)),
			wantError: true,
			errorMsg:  "token expired",
		},
		{
			name:      "Invalid format",
			token:     "invalid.token.format",
			wantError: true,
			errorMsg:  "failed to",
		},
		{
			name:      "Missing merchant ID",
			token:     createTestTokenWithoutMerchant("test@example.com", time.Now().Add(time.Hour)),
			wantError: true,
			errorMsg:  "missing merchant_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := validator.ValidateToken(tt.token)

			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, claims)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, claims)
				if claims != nil {
					assert.Equal(t, "merchant-123", claims.MerchantID)
					assert.Equal(t, "test@example.com", claims.Email)
				}
			}
		})
	}
}

// TestExtractTokenFromHeader tests token extraction from Authorization header
func TestExtractTokenFromHeader(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		wantToken string
		wantError bool
	}{
		{
			name:      "Valid Bearer token",
			header:    "Bearer eyJhbGciOiJIUzI1NiJ9.eyJtZXJjaGFudF9pZCI6IjEyMyJ9.abc",
			wantToken: "eyJhbGciOiJIUzI1NiJ9.eyJtZXJjaGFudF9pZCI6IjEyMyJ9.abc",
			wantError: false,
		},
		{
			name:      "Empty header",
			header:    "",
			wantError: true,
		},
		{
			name:      "Missing Bearer prefix",
			header:    "eyJhbGciOiJIUzI1NiJ9.eyJtZXJjaGFudF9pZCI6IjEyMyJ9.abc",
			wantError: true,
		},
		{
			name:      "Empty token after Bearer",
			header:    "Bearer ",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := utils.ExtractTokenFromHeader(tt.header)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantToken, token)
			}
		})
	}
}

// Helper functions


// createTestToken creates a test JWT token with the simple validator format
func createTestToken(merchantID string, email string, expiry time.Time) string {
	header := map[string]interface{}{
		"alg": "HS256",
		"typ": "JWT",
	}
	claims := map[string]interface{}{
		"merchant_id": merchantID,
		"email":       email,
		"exp":        expiry.Unix(),
		"iss":        "test-issuer",
		"aud":        []string{"payment-api"},
	}
	return createJWT(header, claims, "test-secret-key")
}

func createTestTokenWithoutMerchant(email string, expiry time.Time) string {
	header := map[string]interface{}{
		"alg": "HS256",
		"typ": "JWT",
	}
	claims := map[string]interface{}{
		"email": email,
		"exp":   expiry.Unix(),
		"iss":   "test-issuer",
		"aud":   []string{"payment-api"},
	}
	return createJWT(header, claims, "test-secret-key")
}

// createJWT creates a JWT token with the given header, claims, and secret
func createJWT(header, claims map[string]interface{}, secret string) string {
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)
	
	headerEncoded := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsEncoded := base64.RawURLEncoding.EncodeToString(claimsJSON)
	
	message := headerEncoded + "." + claimsEncoded
	
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	signature := base64.RawURLEncoding.EncodeToString(h.Sum(nil))
	
	return message + "." + signature
}

