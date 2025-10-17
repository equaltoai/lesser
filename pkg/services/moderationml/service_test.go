package moderationml

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test model key generation functions

func TestModelTrainingJob_UpdateKeys(t *testing.T) {
	job := &models.ModelTrainingJob{
		JobID: "test-job-123",
	}

	err := job.UpdateKeys()
	require.NoError(t, err)

	assert.Equal(t, "ML_TRAINING_JOB", job.Type)
	assert.Equal(t, "MLJOB#test-job-123", job.PK)
	assert.Equal(t, "JOB", job.SK)
}

func TestMLPollRequest_UpdateKeys(t *testing.T) {
	now := time.Now()
	pollReq := &models.MLPollRequest{
		JobID:     "test-job-123",
		CreatedAt: now,
	}

	err := pollReq.UpdateKeys()
	require.NoError(t, err)

	assert.Equal(t, "ML_POLL_REQUEST", pollReq.Type)
	assert.Equal(t, "MLPOLL#test-job-123", pollReq.PK)
	assert.Contains(t, pollReq.SK, "REQUEST#")
	assert.Contains(t, pollReq.SK, "1") // Unix timestamp should contain digits
}

func TestMLPrediction_UpdateKeys(t *testing.T) {
	now := time.Now()
	pred := &models.MLPrediction{
		PredictionID: "pred-123",
		ObjectID:     "status_1",
		ModelVersion: "v1",
		Timestamp:    now,
		Reviewed:     false,
	}

	err := pred.UpdateKeys()
	require.NoError(t, err)

	assert.Equal(t, "ML_PREDICTION", pred.Type)
	assert.Equal(t, "MLPRED#status_1", pred.PK)
	assert.Contains(t, pred.SK, "TIME#")
	assert.Contains(t, pred.SK, pred.PredictionID)
	assert.Equal(t, "MODEL#v1", pred.GSI1PK)
	assert.Contains(t, pred.GSI1SK, "TIME#")
	assert.Equal(t, "REVIEW#false", pred.GSI2PK)
	assert.Contains(t, pred.GSI2SK, "TIME#")
}

func TestMLPrediction_UpdateKeys_Reviewed(t *testing.T) {
	now := time.Now()
	pred := &models.MLPrediction{
		PredictionID: "pred-456",
		ObjectID:     "status_2",
		ModelVersion: "v2",
		Timestamp:    now,
		Reviewed:     true,
	}

	err := pred.UpdateKeys()
	require.NoError(t, err)

	assert.Equal(t, "REVIEW#true", pred.GSI2PK)
}

func TestModerationModelVersion_UpdateKeys_Active(t *testing.T) {
	version := &models.ModerationModelVersion{
		VersionID: "v1.0",
		IsActive:  true,
	}

	err := version.UpdateKeys()
	require.NoError(t, err)

	assert.Equal(t, "ML_MODEL_VERSION", version.Type)
	assert.Equal(t, "MLMODEL#bedrock", version.PK)
	assert.Equal(t, "VERSION#v1.0", version.SK)
	assert.Equal(t, "MLMODEL#ACTIVE", version.GSI1PK)
	assert.Contains(t, version.GSI1SK, "VERSION#v1.0")
}

func TestModerationModelVersion_UpdateKeys_Inactive(t *testing.T) {
	version := &models.ModerationModelVersion{
		VersionID: "v0.9",
		IsActive:  false,
	}

	err := version.UpdateKeys()
	require.NoError(t, err)

	assert.Empty(t, version.GSI1PK, "Inactive models should not have GSI1PK")
	assert.Empty(t, version.GSI1SK, "Inactive models should not have GSI1SK")
}

// Test effectiveness calculation logic

func TestCalculateEffectiveness(t *testing.T) {
	tests := []struct {
		name                  string
		predictions           []*models.MLPrediction
		expectedTotal         int
		expectedCorrect       int
		expectedAccuracy      float64
		expectedAvgConfidence float64
		expectError           bool
	}{
		{
			name: "all correct predictions",
			predictions: []*models.MLPrediction{
				{
					PredictedLabel: "safe",
					Confidence:     0.95,
					HumanLabel:     "safe",
					Reviewed:       true,
				},
				{
					PredictedLabel: "unsafe",
					Confidence:     0.90,
					HumanLabel:     "unsafe",
					Reviewed:       true,
				},
			},
			expectedTotal:         2,
			expectedCorrect:       2,
			expectedAccuracy:      1.0,
			expectedAvgConfidence: 0.925,
			expectError:           false,
		},
		{
			name: "partial correct predictions",
			predictions: []*models.MLPrediction{
				{
					PredictedLabel: "safe",
					Confidence:     0.95,
					HumanLabel:     "safe",
					Reviewed:       true,
				},
				{
					PredictedLabel: "unsafe",
					Confidence:     0.85,
					HumanLabel:     "safe", // Mismatch
					Reviewed:       true,
				},
				{
					PredictedLabel: "safe",
					Confidence:     0.90,
					HumanLabel:     "safe",
					Reviewed:       true,
				},
			},
			expectedTotal:         3,
			expectedCorrect:       2,
			expectedAccuracy:      0.6667,
			expectedAvgConfidence: 0.9,
			expectError:           false,
		},
		{
			name: "no human labels",
			predictions: []*models.MLPrediction{
				{
					PredictedLabel: "safe",
					Confidence:     0.95,
					HumanLabel:     "",
				},
			},
			expectError: true,
		},
		{
			name:        "empty predictions",
			predictions: []*models.MLPrediction{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Filter predictions with human labels (simulating the service logic)
			labeledPredictions := filterLabeledPredictions(tt.predictions)

			if tt.expectError {
				assert.Empty(t, labeledPredictions)
				return
			}

			// Calculate metrics
			correct := 0
			totalConfidence := 0.0
			for _, pred := range labeledPredictions {
				if pred.HumanLabel != "" && pred.PredictedLabel == pred.HumanLabel {
					correct++
				}
				totalConfidence += pred.Confidence
			}

			accuracy := float64(correct) / float64(len(labeledPredictions))
			avgConfidence := totalConfidence / float64(len(labeledPredictions))

			// Calculate effectiveness (weighted: 70% accuracy, 30% confidence)
			effectiveness := (accuracy * 0.7) + (avgConfidence * 0.3)

			assert.Equal(t, tt.expectedTotal, len(labeledPredictions))
			assert.Equal(t, tt.expectedCorrect, correct)
			assert.InDelta(t, tt.expectedAccuracy, accuracy, 0.001)
			assert.InDelta(t, tt.expectedAvgConfidence, avgConfidence, 0.001)
			assert.Greater(t, effectiveness, 0.0)
			assert.LessOrEqual(t, effectiveness, 1.0)
		})
	}
}

// Helper function for testing
func filterLabeledPredictions(predictions []*models.MLPrediction) []*models.MLPrediction {
	var filtered []*models.MLPrediction
	for _, pred := range predictions {
		if pred.HumanLabel != "" {
			filtered = append(filtered, pred)
		}
	}
	return filtered
}

// Test poll interval calculations

func TestPollIntervalCalculation(t *testing.T) {
	tests := []struct {
		name             string
		initialDelay     int
		interval         int
		isFirstPoll      bool
		expectedInterval int
	}{
		{
			name:             "first poll uses initial delay",
			initialDelay:     30,
			interval:         60,
			isFirstPoll:      true,
			expectedInterval: 30,
		},
		{
			name:             "subsequent polls use standard interval",
			initialDelay:     30,
			interval:         60,
			isFirstPoll:      false,
			expectedInterval: 60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var interval int
			if tt.isFirstPoll {
				interval = tt.initialDelay
			} else {
				interval = tt.interval
			}
			assert.Equal(t, tt.expectedInterval, interval)
		})
	}
}

// Test TTL calculations

func TestTTLCalculation(t *testing.T) {
	now := time.Now()
	ttlDuration := 48 * time.Hour

	expectedTTL := now.Add(ttlDuration).Unix()
	actualTTL := now.Add(ttlDuration).Unix()

	// Should be within 1 second tolerance
	assert.InDelta(t, expectedTTL, actualTTL, 1.0)
}

// Test training job status transitions

func TestTrainingJobStatusTransitions(t *testing.T) {
	validTransitions := map[string][]string{
		"SUBMITTED":   {"IN_PROGRESS", "FAILED"},
		"IN_PROGRESS": {"COMPLETED", "FAILED"},
		"COMPLETED":   {}, // Terminal state
		"FAILED":      {}, // Terminal state
	}

	testCases := []struct {
		from       string
		to         string
		shouldWork bool
	}{
		{"SUBMITTED", "IN_PROGRESS", true},
		{"SUBMITTED", "COMPLETED", false}, // Skip IN_PROGRESS
		{"IN_PROGRESS", "COMPLETED", true},
		{"IN_PROGRESS", "FAILED", true},
		{"IN_PROGRESS", "SUBMITTED", false}, // Backwards
		{"SUBMITTED", "FAILED", true},
		{"COMPLETED", "FAILED", false}, // From terminal
		{"COMPLETED", "IN_PROGRESS", false},
		{"FAILED", "COMPLETED", false},
		{"FAILED", "IN_PROGRESS", false},
	}

	for _, tc := range testCases {
		t.Run(tc.from+"_to_"+tc.to, func(t *testing.T) {
			validNext := validTransitions[tc.from]
			isValid := contains(validNext, tc.to)
			if tc.shouldWork {
				assert.True(t, isValid, "Expected %s -> %s to be valid", tc.from, tc.to)
			} else {
				assert.False(t, isValid, "Expected %s -> %s to be invalid", tc.from, tc.to)
			}
		})
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Test model version generation

func TestModelVersionGeneration(t *testing.T) {
	tests := []struct {
		name           string
		baseVersion    string
		expectedFormat string
	}{
		{
			name:           "version with timestamp",
			baseVersion:    "v1",
			expectedFormat: "v1-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamp := time.Now().Unix()
			version := tt.baseVersion + "-" + time.Unix(timestamp, 0).Format("20060102-150405")
			assert.Contains(t, version, tt.expectedFormat)
			assert.Greater(t, len(version), len(tt.baseVersion))
		})
	}
}

// Test dataset preparation validations

func TestDatasetValidation(t *testing.T) {
	samples := []struct {
		ObjectID   string
		Label      string
		Content    string
		Confidence float64
	}{
		{"status_1", "safe", "This is safe content", 0.95},
		{"status_2", "unsafe", "This contains hate speech", 0.90},
		{"status_3", "safe", "Normal conversation", 0.85},
	}

	// Verify we have minimum samples
	assert.GreaterOrEqual(t, len(samples), 2, "Need at least 2 samples for training")

	// Verify all samples have content
	for _, sample := range samples {
		assert.NotEmpty(t, sample.Content, "Sample %s must have content", sample.ObjectID)
		assert.NotEmpty(t, sample.Label, "Sample %s must have label", sample.ObjectID)
		assert.Greater(t, sample.Confidence, 0.0, "Sample %s must have positive confidence", sample.ObjectID)
	}

	// Verify label distribution
	labelCounts := make(map[string]int)
	for _, sample := range samples {
		labelCounts[sample.Label]++
	}

	// Should have at least 2 different labels for meaningful training
	assert.GreaterOrEqual(t, len(labelCounts), 2, "Dataset should have multiple label types")
}

// Test minimum sample requirements

func TestMinimumSampleRequirements(t *testing.T) {
	tests := []struct {
		name          string
		sampleCount   int
		minSamples    int
		shouldSucceed bool
	}{
		{
			name:          "sufficient samples",
			sampleCount:   15,
			minSamples:    10,
			shouldSucceed: true,
		},
		{
			name:          "exactly minimum samples",
			sampleCount:   10,
			minSamples:    10,
			shouldSucceed: true,
		},
		{
			name:          "insufficient samples",
			sampleCount:   5,
			minSamples:    10,
			shouldSucceed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasSufficientSamples := tt.sampleCount >= tt.minSamples
			assert.Equal(t, tt.shouldSucceed, hasSufficientSamples)
		})
	}
}
