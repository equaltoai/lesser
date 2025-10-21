package lift

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestResponseBinary_Base64EncodingAndHeaders(t *testing.T) {
	r := NewResponse()
	data := []byte{0x00, 0x01, 0x02, 0xFF}

	if err := r.Binary(data); err != nil {
		t.Fatalf("Binary() error = %v", err)
	}

	if !r.IsBase64Encoded {
		t.Fatalf("expected IsBase64Encoded = true")
	}

	// Marshal to Lambda response format
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("MarshalJSON error = %v", err)
	}

	var out struct {
		Headers         map[string]string `json:"headers"`
		Body            string            `json:"body"`
		StatusCode      int               `json:"statusCode"`
		IsBase64Encoded bool              `json:"isBase64Encoded"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal Lambda response error = %v", err)
	}

	if out.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", out.StatusCode)
	}
	if ct := out.Headers["Content-Type"]; ct != "application/octet-stream" {
		t.Fatalf("expected Content-Type application/octet-stream, got %s", ct)
	}
	if !out.IsBase64Encoded {
		t.Fatalf("expected isBase64Encoded true in marshaled output")
	}
	expected := base64.StdEncoding.EncodeToString(data)
	if out.Body != expected {
		t.Fatalf("expected base64 body %q, got %q", expected, out.Body)
	}
}
