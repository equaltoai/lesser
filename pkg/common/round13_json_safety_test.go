package common

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeUnmarshalJSON_RejectsDepthAndKeyAndArrayAndStringLimits(t *testing.T) {
	t.Run("depth", func(t *testing.T) {
		// Create nesting that exceeds MaxJSONDepth (validateJSON errors when depth > MaxJSONDepth).
		payload := strings.Repeat("[", MaxJSONDepth+2) + strings.Repeat("]", MaxJSONDepth+2)
		var out any
		err := SafeUnmarshalJSON([]byte(payload), &out)
		assert.Error(t, err)
	})

	t.Run("keys", func(t *testing.T) {
		obj := make(map[string]any, MaxJSONKeys+1)
		for i := 0; i < MaxJSONKeys+1; i++ {
			obj["k"+strconv.Itoa(i)] = i
		}
		b, _ := json.Marshal(obj)
		var out any
		err := SafeUnmarshalJSON(b, &out)
		assert.Error(t, err)
	})

	t.Run("array_len", func(t *testing.T) {
		var sb strings.Builder
		sb.WriteByte('[')
		for i := 0; i < MaxJSONArrayLength+1; i++ {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteByte('0')
		}
		sb.WriteByte(']')
		var out any
		err := SafeUnmarshalJSON([]byte(sb.String()), &out)
		assert.Error(t, err)
	})

	t.Run("string_len", func(t *testing.T) {
		long := strings.Repeat("a", MaxJSONStringLength+1)
		b, _ := json.Marshal(long)
		var out string
		err := SafeUnmarshalJSON(b, &out)
		assert.Error(t, err)
	})
}

func TestSafeUnmarshalJSON_SizeLimit(t *testing.T) {
	var out any
	err := SafeUnmarshalJSON(bytes.Repeat([]byte("a"), MaxJSONSize+1), &out)
	assert.Error(t, err)
}

func TestParseRequestBody_AndParseActivityPubObject(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	var out payload
	assert.NoError(t, ParseRequestBody([]byte(`{"name":"alice"}`), &out))
	assert.Equal(t, "alice", out.Name)

	// Repetition-based JSON bomb detection.
	bomb := []byte(`"` + strings.Repeat("abcdefghij", 200) + `"`)
	err := ParseRequestBody(bomb, &out)
	assert.Error(t, err)

	// Nesting-based detection (does not require valid JSON).
	err = DetectJSONBomb([]byte(strings.Repeat("[", 101)))
	assert.Error(t, err)

	// ActivityPub parser allows unknown fields, but should still validate structure.
	out = payload{}
	assert.NoError(t, ParseActivityPubObject([]byte(`{"name":"bob","extra":1}`), &out))
	assert.Equal(t, "bob", out.Name)
}

func TestParseHTTPResponse_AndParseFormValues(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	var out payload
	r := strings.NewReader(`{"name":"ok"}`)
	assert.NoError(t, ParseHTTPResponse(r, &out))
	assert.Equal(t, "ok", out.Name)

	_, err := ParseFormValues("")
	assert.Error(t, err)

	values, err := ParseFormValues("a=1&b=two")
	assert.NoError(t, err)
	assert.Equal(t, "1", values.Get("a"))
	assert.Equal(t, "two", values.Get("b"))
}
