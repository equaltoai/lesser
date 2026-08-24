package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	apptheory "github.com/theory-cloud/apptheory/v4/runtime"
)

func TestParseRequestWithFallback_MoreErrorBranches(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}

	var out payload

	// Nil ctx branch.
	err := ParseRequestWithFallback(nil, &out)
	assert.Error(t, err)

	// Invalid JSON branch.
	ctx := &apptheory.Context{
		Request: apptheory.Request{
			Body: []byte("{not-json"),
		},
	}
	err = ParseRequestWithFallback(ctx, &out)
	assert.Error(t, err)
}
