package quotes

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// TestValidateCreateQuoteRequest tests quote request validation
func TestValidateCreateQuoteRequest(t *testing.T) {
	qs := &QuoteService{logger: zap.NewNop()}

	tests := []struct {
		name        string
		req         *CreateQuoteRequest
		shouldError bool
		errorMsg    string
	}{
		{
			name: "valid request",
			req: &CreateQuoteRequest{
				QuoterUsername: "user1",
				TargetStatusID: "status123",
				Content:        "Great post!",
				Visibility:     "public",
			},
			shouldError: false,
		},
		{
			name: "missing quoter username",
			req: &CreateQuoteRequest{
				QuoterUsername: "",
				TargetStatusID: "status123",
			},
			shouldError: true,
		},
		{
			name: "missing target status ID",
			req: &CreateQuoteRequest{
				QuoterUsername: "user1",
				TargetStatusID: "",
			},
			shouldError: true,
		},
		{
			name: "content too long",
			req: &CreateQuoteRequest{
				QuoterUsername: "user1",
				TargetStatusID: "status123",
				Content:        string(make([]byte, 501)), // 501 characters
			},
			shouldError: true,
		},
		{
			name: "empty content is valid",
			req: &CreateQuoteRequest{
				QuoterUsername: "user1",
				TargetStatusID: "status123",
				Content:        "",
			},
			shouldError: false,
		},
		{
			name: "max valid content length",
			req: &CreateQuoteRequest{
				QuoterUsername: "user1",
				TargetStatusID: "status123",
				Content:        string(make([]byte, 500)), // Exactly 500 characters
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := qs.validateCreateQuoteRequest(tt.req)

			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestIsStatusQuotable tests quote eligibility logic
func TestIsStatusQuotable(t *testing.T) {
	qs := &QuoteService{logger: zap.NewNop()}

	tests := []struct {
		name       string
		visibility string
		expected   bool
	}{
		{
			name:       "public status is quotable",
			visibility: "public",
			expected:   true,
		},
		{
			name:       "unlisted status is quotable",
			visibility: "unlisted",
			expected:   true,
		},
		{
			name:       "private status not quotable",
			visibility: "private",
			expected:   false,
		},
		{
			name:       "direct status not quotable",
			visibility: "direct",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &models.Status{
				Visibility: tt.visibility,
			}

			result := qs.isStatusQuotable(status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGenerateStatusID tests ID generation
func TestGenerateStatusID(t *testing.T) {
	id1 := generateStatusID()
	id2 := generateStatusID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2, "Generated IDs should be unique")
	assert.Contains(t, id1, "quote_")
	assert.Contains(t, id2, "quote_")
}
