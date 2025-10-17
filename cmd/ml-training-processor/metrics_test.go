package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// TestParseBedrockMetricsJSON tests JSON parsing from S3 files
func TestParseBedrockMetricsJSON_DirectFields(t *testing.T) {
	jsonContent := `{
		"accuracy": 0.95,
		"precision": 0.92,
		"recall": 0.88,
		"f1_score": 0.90
	}`

	result, err := parseBedrockMetricsJSON(jsonContent)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0.95, result.Accuracy)
	assert.Equal(t, 0.92, result.Precision)
	assert.Equal(t, 0.88, result.Recall)
	assert.Equal(t, 0.90, result.F1Score)
}

func TestParseBedrockMetricsJSON_ValidationMetricsNested(t *testing.T) {
	jsonContent := `{
		"validation_metrics": {
			"accuracy": 0.93,
			"precision": 0.91,
			"recall": 0.89,
			"f1_score": 0.87
		}
	}`

	result, err := parseBedrockMetricsJSON(jsonContent)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0.93, result.Accuracy)
	assert.Equal(t, 0.91, result.Precision)
	assert.Equal(t, 0.89, result.Recall)
	assert.Equal(t, 0.87, result.F1Score)
}

func TestParseBedrockMetricsJSON_EvaluationNested(t *testing.T) {
	jsonContent := `{
		"evaluation": {
			"accuracy": 0.94,
			"precision": 0.90,
			"recall": 0.86,
			"f1": 0.88
		}
	}`

	result, err := parseBedrockMetricsJSON(jsonContent)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0.94, result.Accuracy)
	assert.Equal(t, 0.90, result.Precision)
	assert.Equal(t, 0.86, result.Recall)
	assert.Equal(t, 0.88, result.F1Score)
}

func TestParseBedrockMetricsJSON_F1AlternativeName(t *testing.T) {
	jsonContent := `{
		"accuracy": 0.96,
		"precision": 0.93,
		"recall": 0.91,
		"f1": 0.92
	}`

	result, err := parseBedrockMetricsJSON(jsonContent)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0.96, result.Accuracy)
	assert.Equal(t, 0.93, result.Precision)
	assert.Equal(t, 0.91, result.Recall)
	assert.Equal(t, 0.92, result.F1Score)
}

func TestParseBedrockMetricsJSON_InvalidJSON(t *testing.T) {
	jsonContent := `{invalid json}`

	result, err := parseBedrockMetricsJSON(jsonContent)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestParseBedrockMetricsJSON_NoValidMetrics(t *testing.T) {
	jsonContent := `{"other_field": "value", "another_field": 123}`

	_, err := parseBedrockMetricsJSON(jsonContent)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no valid metrics found")
}

func TestParseBedrockMetricsJSON_PartialMetrics(t *testing.T) {
	// With at least some valid metrics, should succeed
	jsonContent := `{"accuracy": 0.95, "precision": 0.92, "other_field": "value"}`

	metrics, err := parseBedrockMetricsJSON(jsonContent)

	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Equal(t, 0.95, metrics.Accuracy)
	assert.Equal(t, 0.92, metrics.Precision)
	// Recall and F1Score will be 0.0 but no error
}

func TestExtractMetricsFromBedrockOutput_NilInput(t *testing.T) {
	processor := &MLTrainingProcessor{
		logger: zap.NewNop(),
	}

	result := processor.extractMetricsFromBedrockOutput(nil)

	assert.NotNil(t, result)
	assert.Equal(t, 0.0, result.Accuracy)
	assert.Equal(t, 0.0, result.Precision)
	assert.Equal(t, 0.0, result.Recall)
	assert.Equal(t, 0.0, result.F1Score)
}

func TestExtractVersionFromARN(t *testing.T) {
	tests := []struct {
		name     string
		arn      string
		expected string
	}{
		{
			name:     "valid ARN with version",
			arn:      "arn:aws:bedrock:us-east-1:123456789012:custom-model/anthropic.claude-v2:1:12k/abcd1234",
			expected: "abcd1234",
		},
		{
			name:     "simple ARN",
			arn:      "arn:aws:bedrock:us-east-1:123456789012:model/version123",
			expected: "version123",
		},
		{
			name:     "single part",
			arn:      "version456",
			expected: "version456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			versionID := extractVersionFromARN(tt.arn)
			assert.Equal(t, tt.expected, versionID)
		})
	}
}
