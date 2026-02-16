package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/runtime"
)

func TestActorHeaderValue_Round22(t *testing.T) {
	require.Equal(t, "", actorHeaderValue(nil, "x-test"))

	ctx := &apptheory.Context{Request: apptheory.Request{
		Headers: map[string][]string{"x-test": {"a", "b"}},
	}}
	require.Equal(t, "", actorHeaderValue(ctx, "missing"))
	require.Equal(t, "a", actorHeaderValue(ctx, "x-test"))
	require.Equal(t, "a", actorHeaderValue(ctx, " X-Test "))
}

func TestActorRequestID_Round22(t *testing.T) {
	require.Equal(t, "rid", actorRequestID(&apptheory.Context{RequestID: "rid"}, "ignored"))

	id := actorRequestID(&apptheory.Context{}, "")
	require.True(t, strings.HasPrefix(id, "actor-"))

	id = actorRequestID(&apptheory.Context{}, "custom")
	require.True(t, strings.HasPrefix(id, "custom-"))
}

func TestActorContextRequestID_Round22(t *testing.T) {
	require.Equal(t, "", actorContextRequestID(nil))

	ctx := &apptheory.Context{RequestID: "rid"}
	require.Equal(t, "rid", actorContextRequestID(ctx))

	ctx = &apptheory.Context{RequestID: "rid"}
	ctx.Set("requestID", " abc ")
	require.Equal(t, "abc", actorContextRequestID(ctx))

	ctx = &apptheory.Context{}
	ctx.Set("requestID", "   ")
	require.Equal(t, "", actorContextRequestID(ctx))
}

func TestActorJSONError_Round22(t *testing.T) {
	resp := actorJSONError(http.StatusBadRequest, " nope ")
	require.Equal(t, http.StatusBadRequest, resp.Status)

	var body map[string]string
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "nope", body["error"])

	resp = actorJSONError(http.StatusInternalServerError, "")
	require.Equal(t, http.StatusInternalServerError, resp.Status)

	body = map[string]string{}
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "internal server error", body["error"])
}

func TestActorActivityPubSecurityHeaders_Round22(t *testing.T) {
	t.Run("returns nil response unchanged", func(t *testing.T) {
		mw := actorActivityPubSecurityHeaders()
		wantErr := errors.New("boom")
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return nil, wantErr
		})(&apptheory.Context{})
		require.Nil(t, resp)
		require.ErrorIs(t, err, wantErr)
	})

	t.Run("adds headers and CSP for html", func(t *testing.T) {
		mw := actorActivityPubSecurityHeaders()
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return &apptheory.Response{
				Status:  http.StatusOK,
				Headers: map[string][]string{"content-type": {"text/html; charset=utf-8"}},
				Body:    []byte("ok"),
			}, nil
		})(&apptheory.Context{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "nosniff", resp.Headers["x-content-type-options"][0])
		require.Equal(t, "SAMEORIGIN", resp.Headers["x-frame-options"][0])
		require.Equal(t, "noindex, nofollow", resp.Headers["x-robots-tag"][0])
		require.NotEmpty(t, resp.Headers["content-security-policy"])
	})

	t.Run("does not add CSP for non-html", func(t *testing.T) {
		mw := actorActivityPubSecurityHeaders()
		resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
			return &apptheory.Response{
				Status:  http.StatusOK,
				Headers: map[string][]string{"content-type": {"application/activity+json"}},
				Body:    []byte("ok"),
			}, nil
		})(&apptheory.Context{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		_, ok := resp.Headers["content-security-policy"]
		require.False(t, ok)
	})
}

func TestActorPanicRecovery_Round22(t *testing.T) {
	mw := actorPanicRecovery(nil)
	resp, err := mw(func(*apptheory.Context) (*apptheory.Response, error) {
		panic("boom")
	})(&apptheory.Context{RequestID: "rid"})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusInternalServerError, resp.Status)

	var body map[string]string
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "internal server error", body["error"])
}
