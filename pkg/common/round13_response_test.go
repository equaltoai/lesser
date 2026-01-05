//go:build !production
// +build !production

package common

import (
	"errors"
	"testing"

	appErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestResponseHeaders_JSONAndActivityPub(t *testing.T) {
	h := Headers()
	assert.Equal(t, "application/json", h["Content-Type"])
	assert.NotEmpty(t, h["Access-Control-Allow-Origin"])

	ah := ActivityPubHeaders()
	assert.Equal(t, "application/activity+json", ah["Content-Type"])
}

func TestJSONResponse_AndErrorResponses(t *testing.T) {
	resp := JSONResponse(200, map[string]string{"ok": "true"})
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, resp.Body, "true")
	assert.Equal(t, "application/json", resp.Headers["Content-Type"])

	// Marshal failure should fall back to 500.
	resp = JSONResponse(200, make(chan int))
	assert.Equal(t, 500, resp.StatusCode)

	resp = ActivityPubResponse(200, map[string]string{"ok": "true"})
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "application/activity+json", resp.Headers["Content-Type"])

	errResp := BadRequest(errors.New("bad"))
	assert.Equal(t, 400, errResp.StatusCode)

	errResp = NotFound(errors.New("missing"))
	assert.Equal(t, 404, errResp.StatusCode)
}

func TestErrorFromType_MapsCommonErrorCategories(t *testing.T) {
	assert.Equal(t, 404, ErrorFromType(appErrors.NotFound("thing")).StatusCode)
	assert.Equal(t, 400, ErrorFromType(appErrors.ValidationFailed("field", "bad")).StatusCode)
	assert.Equal(t, 401, ErrorFromType(appErrors.Unauthorized("nope")).StatusCode)
	assert.Equal(t, 403, ErrorFromType(appErrors.Forbidden("nope")).StatusCode)
	assert.Equal(t, 409, ErrorFromType(appErrors.NewAppError(appErrors.CodeConflict, appErrors.CategoryBusiness, "conflict")).StatusCode)
	assert.Equal(t, 503, ErrorFromType(appErrors.NewFederationInternalError(appErrors.CodeExternalServiceUnavailable, "fed", errors.New("x"))).StatusCode)
	assert.Equal(t, 500, ErrorFromType(errors.New("unknown")).StatusCode)
}
