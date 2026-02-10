package common

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeUnmarshalJSON_SizeAndDecodeErrors(t *testing.T) {
	t.Run("oversized input rejected", func(t *testing.T) {
		payload := make([]byte, MaxJSONSize+1)
		var out map[string]any
		assert.Error(t, SafeUnmarshalJSON(payload, &out))
	})

	t.Run("invalid JSON rejected", func(t *testing.T) {
		var out map[string]any
		assert.Error(t, SafeUnmarshalJSON([]byte("{"), &out))
	})
}

func TestSafeJSONDecoder_validateJSON_Limits(t *testing.T) {
	decoder := &SafeJSONDecoder{}

	t.Run("depth limit enforced", func(t *testing.T) {
		var nested any = map[string]any{"leaf": "x"}
		for i := 0; i < MaxJSONDepth+2; i++ {
			nested = map[string]any{"a": nested}
		}
		assert.Error(t, decoder.validateJSON(nested, 0))
	})

	t.Run("max keys enforced", func(t *testing.T) {
		obj := make(map[string]any, MaxJSONKeys+1)
		for i := 0; i < MaxJSONKeys+1; i++ {
			obj[fmt.Sprintf("k%d", i)] = i
		}
		assert.Error(t, decoder.validateJSON(obj, 0))
	})

	t.Run("max array length enforced", func(t *testing.T) {
		arr := make([]any, MaxJSONArrayLength+1)
		assert.Error(t, decoder.validateJSON(arr, 0))
	})

	t.Run("max string length enforced", func(t *testing.T) {
		long := strings.Repeat("a", MaxJSONStringLength+1)
		assert.Error(t, decoder.validateJSON(long, 0))
	})

	t.Run("unexpected type rejected", func(t *testing.T) {
		assert.Error(t, decoder.validateJSON(struct{}{}, 0))
	})
}

func TestJSONBombDetection_Branches(t *testing.T) {
	t.Run("repetition detected", func(t *testing.T) {
		data := []byte(strings.Repeat("abcdefghij", 200))
		assert.ErrorIs(t, DetectJSONBomb(data), ErrJSONBombRepetitionDetected)
	})

	t.Run("nesting detected", func(t *testing.T) {
		data := []byte(strings.Repeat("[", 101))
		assert.ErrorIs(t, DetectJSONBomb(data), ErrJSONBombNestingDetected)
	})
}

func TestParseRequestBody_AndActivityPubObject(t *testing.T) {
	type payload struct {
		A string `json:"a"`
	}

	t.Run("request body rejects bomb patterns", func(t *testing.T) {
		var out payload
		err := ParseRequestBody([]byte(strings.Repeat("abcdefghij", 200)), &out)
		assert.Error(t, err)
	})

	t.Run("strict parse succeeds for normal JSON", func(t *testing.T) {
		var out payload
		require.NoError(t, ParseRequestBody([]byte(`{"a":"x"}`), &out))
		assert.Equal(t, "x", out.A)
	})

	t.Run("activitypub parse succeeds for normal JSON", func(t *testing.T) {
		var out payload
		require.NoError(t, ParseActivityPubObject([]byte(`{"a":"x"}`), &out))
		assert.Equal(t, "x", out.A)
	})
}

func TestParseHTTPResponse_AndFormValues(t *testing.T) {
	type payload struct {
		A string `json:"a"`
	}
	var out payload
	require.NoError(t, ParseHTTPResponse(bytes.NewReader([]byte(`{"a":"x"}`)), &out))
	assert.Equal(t, "x", out.A)

	_, err := ParseFormValues("")
	assert.Error(t, err)

	values, err := ParseFormValues("a=b&c=d")
	require.NoError(t, err)
	assert.Equal(t, "b", values.Get("a"))
	assert.Equal(t, "d", values.Get("c"))
}
