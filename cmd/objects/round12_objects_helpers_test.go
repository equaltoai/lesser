package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/require"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

func TestObjectsStringSliceFromAny_Round12(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		require.Nil(t, objectsStringSliceFromAny(nil))
	})

	t.Run("slice_string", func(t *testing.T) {
		require.Equal(t, []string{"a", "  b "}, objectsStringSliceFromAny([]string{"a", "  b "}))
	})

	t.Run("slice_any", func(t *testing.T) {
		got := objectsStringSliceFromAny([]any{" a ", "", "b", 123, "  "})
		require.Equal(t, []string{"a", "b"}, got)
	})

	t.Run("string", func(t *testing.T) {
		require.Equal(t, []string{"abc"}, objectsStringSliceFromAny(" abc "))
		require.Nil(t, objectsStringSliceFromAny("   "))
	})

	t.Run("default", func(t *testing.T) {
		require.Nil(t, objectsStringSliceFromAny(123))
	})
}

func TestObjectsIsPubliclyAddressed_Round12(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		require.False(t, objectsIsPubliclyAddressed(nil))
	})

	t.Run("activity", func(t *testing.T) {
		act := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Type: "Create",
				To:   []string{activitypub.PublicAddress},
			},
		}
		require.True(t, objectsIsPubliclyAddressed(act))
	})

	t.Run("map_with_any_slices", func(t *testing.T) {
		obj := map[string]any{
			"to": []any{" ", activitypub.PublicAddress},
			"cc": []any{"https://example.com/users/alice"},
		}
		require.True(t, objectsIsPubliclyAddressed(obj))
	})

	t.Run("fallback_json_success", func(t *testing.T) {
		obj := struct {
			To []string `json:"to"`
			CC []string `json:"cc"`
		}{
			To: []string{"https://example.com/users/alice"},
			CC: []string{activitypub.PublicAddress},
		}
		require.True(t, objectsIsPubliclyAddressed(obj))
	})

	t.Run("fallback_json_marshal_error", func(t *testing.T) {
		require.False(t, objectsIsPubliclyAddressed(func() {}))
	})

	t.Run("fallback_json_unmarshal_error", func(t *testing.T) {
		// Marshals to a JSON array, which can't be unmarshaled into the struct in objectsIsPubliclyAddressed.
		require.False(t, objectsIsPubliclyAddressed([]int{1, 2, 3}))
	})
}

func TestObjectsRequestID_Round12(t *testing.T) {
	t.Run("uses_request_id_from_context", func(t *testing.T) {
		ctx := &apptheory.Context{RequestID: "req-1"}
		require.Equal(t, "req-1", objectsRequestID(ctx, "ignored"))
	})

	t.Run("defaults_prefix", func(t *testing.T) {
		got := objectsRequestID(nil, "")
		require.True(t, strings.HasPrefix(got, "objects-"))
		_, err := strconv.ParseInt(strings.TrimPrefix(got, "objects-"), 10, 64)
		require.NoError(t, err)
	})

	t.Run("trims_prefix", func(t *testing.T) {
		got := objectsRequestID(nil, "  obj ")
		require.True(t, strings.HasPrefix(got, "obj-"))
	})
}

func TestObjectsJSONError_DefaultMessage_Round12(t *testing.T) {
	resp := objectsJSONError(http.StatusInternalServerError, "  ")
	require.NotNil(t, resp)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(resp.Body, &payload))
	require.Equal(t, "internal server error", payload["error"])
}

func TestObjectsActivityJSON_MarshalError_Round12(t *testing.T) {
	_, err := objectsActivityJSON(http.StatusOK, make(chan int))
	require.Error(t, err)
}

func TestObjectsPanicRecovery_Round12(t *testing.T) {
	mw := objectsPanicRecovery(nil)
	wrapped := mw(func(*apptheory.Context) (*apptheory.Response, error) {
		panic("boom")
	})

	resp, err := wrapped(nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusInternalServerError, resp.Status)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(resp.Body, &payload))
	require.Equal(t, "internal server error", payload["error"])
}
