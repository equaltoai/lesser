package common

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnauthorizedResponse(t *testing.T) {
	err := errors.New("invalid token")
	resp := Unauthorized(err)

	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Contains(t, resp.Body, "UNAUTHORIZED")
}

func TestForbiddenResponse(t *testing.T) {
	err := errors.New("access denied")
	resp := Forbidden(err)

	require.NotNil(t, resp)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Contains(t, resp.Body, "FORBIDDEN")
}

func TestMethodNotAllowedResponse(t *testing.T) {
	err := errors.New("POST not supported")
	resp := MethodNotAllowed(err)

	require.NotNil(t, resp)
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	assert.Contains(t, resp.Body, "METHOD_NOT_ALLOWED")
}

func TestConflictResponse(t *testing.T) {
	err := errors.New("resource already exists")
	resp := Conflict(err)

	require.NotNil(t, resp)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Contains(t, resp.Body, "CONFLICT")
}

func TestUnprocessableEntityResponse(t *testing.T) {
	err := errors.New("validation failed")
	resp := UnprocessableEntity(err)

	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	assert.Contains(t, resp.Body, "VALIDATION_ERROR")
}

func TestTooManyRequestsResponse(t *testing.T) {
	err := errors.New("rate limit exceeded")
	resp := TooManyRequests(err)

	require.NotNil(t, resp)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.Contains(t, resp.Body, "RATE_LIMITED")
}

func TestOKResponse(t *testing.T) {
	body := map[string]string{"message": "success"}
	resp := OK(body)

	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Body, "success")
}

func TestCreatedResponse(t *testing.T) {
	body := map[string]string{"id": "123"}
	resp := Created(body)

	require.NotNil(t, resp)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Contains(t, resp.Body, "123")
}

func TestAcceptedResponse(t *testing.T) {
	body := map[string]string{"status": "pending"}
	resp := Accepted(body)

	require.NotNil(t, resp)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	assert.Contains(t, resp.Body, "pending")
}

func TestNoContentResponse(t *testing.T) {
	resp := NoContent()

	require.NotNil(t, resp)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, resp.Body)
}

func TestActivityPubResponseCoverage(t *testing.T) {
	body := map[string]interface{}{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type":     "Note",
	}
	resp := ActivityPubResponse(http.StatusOK, body)

	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/activity+json", resp.Headers["Content-Type"])
}
