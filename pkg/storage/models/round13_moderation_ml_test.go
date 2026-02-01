package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestModerationMLModels_UpdateKeys_AndMetrics(t *testing.T) {
	ts := time.Unix(1700000000, 0).UTC()

	sample := &ModerationSample{
		ID:         "s1",
		ObjectID:   "obj1",
		ReviewerID: "rev1",
		Label:      "spam",
		Timestamp:  ts,
		Confidence: 0.75,
	}
	assert.NoError(t, sample.UpdateKeys())
	assert.Equal(t, "MLSAMPLE#obj1", sample.PK)
	assert.Contains(t, sample.SK, "VERSION#v1#")
	assert.Equal(t, "REVIEWER#rev1", sample.GSI1PK)
	assert.Contains(t, sample.GSI2SK, "CONFIDENCE#")
	assert.Equal(t, "SAMPLEID#s1", sample.GSI3PK)
	assert.Equal(t, "ML_SAMPLE", sample.Type)

	mv := &ModerationModelVersion{
		VersionID:     "v1",
		Accuracy:      0.99,
		IsActive:      true,
		TrainingJobID: "job1",
	}
	assert.NoError(t, mv.UpdateKeys())
	assert.Equal(t, "MLMODEL#bedrock", mv.PK)
	assert.Equal(t, "MLMODEL#ACTIVE", mv.GSI1PK)
	assert.Contains(t, mv.GSI1SK, "ACCURACY#")
	assert.Equal(t, "TRAININGJOB#job1", mv.GSI2PK)
	assert.Equal(t, "ML_MODEL_VERSION", mv.Type)

	mv2 := &ModerationModelVersion{VersionID: "v2", IsActive: false}
	assert.NoError(t, mv2.UpdateKeys())
	assert.Equal(t, "", mv2.GSI1PK)
	assert.Equal(t, "", mv2.GSI1SK)

	metric := &ModerationEffectivenessMetric{
		PatternID:      "p1",
		Period:         PeriodDaily,
		StartTime:      ts,
		TruePositives:  10,
		FalsePositives: 2,
		TrueNegatives:  50,
		FalseNegatives: 3,
		F1Score:        0.0,
	}
	metric.CalculateMetrics()
	assert.Greater(t, metric.Precision, 0.0)
	assert.Greater(t, metric.Recall, 0.0)
	assert.Greater(t, metric.F1Score, 0.0)
	assert.Equal(t, 65, metric.TotalReviewed)

	assert.NoError(t, metric.UpdateKeys())
	assert.Equal(t, "MLMETRICS#p1", metric.PK)
	assert.Contains(t, metric.SK, "PERIOD#")
	assert.Equal(t, "METRICS#"+PeriodDaily, metric.GSI1PK)
	assert.Contains(t, metric.GSI1SK, "F1SCORE#")
	assert.Equal(t, "ML_METRICS", metric.Type)

	job := &ModelTrainingJob{
		JobID:     "arn:job1",
		Status:    "IN_PROGRESS",
		TenantID:  "t1",
		StartedAt: ts,
	}
	assert.NoError(t, job.UpdateKeys())
	assert.Equal(t, "MLJOB#arn:job1", job.PK)
	assert.Equal(t, "JOB", job.SK)
	assert.Equal(t, "MLJOB#IN_PROGRESS", job.GSI1PK)
	assert.Equal(t, "TENANT#t1", job.GSI2PK)
	assert.Equal(t, "ML_TRAINING_JOB", job.Type)

	poll := &MLPollRequest{
		JobID:         "job1",
		CreatedAt:     ts,
		NextPollAfter: ts.Add(5 * time.Minute),
		Status:        "PENDING",
	}
	assert.NoError(t, poll.UpdateKeys())
	assert.Equal(t, "MLPOLL#job1", poll.PK)
	assert.Contains(t, poll.SK, "REQUEST#")
	assert.Equal(t, "MLPOLL#PENDING", poll.GSI1PK)
	assert.Equal(t, "ML_POLL_REQUEST", poll.Type)

	// Non-pending clears GSI1.
	poll2 := &MLPollRequest{
		JobID:     "job2",
		CreatedAt: ts,
		Status:    "COMPLETED",
	}
	assert.NoError(t, poll2.UpdateKeys())
	assert.Equal(t, "", poll2.GSI1PK)
	assert.Equal(t, "", poll2.GSI1SK)

	pred := &MLPrediction{
		PredictionID: "p1",
		ObjectID:     "obj1",
		ModelVersion: "v1",
		Reviewed:     true,
		Timestamp:    ts,
	}
	assert.NoError(t, pred.UpdateKeys())
	assert.Equal(t, "MLPRED#obj1", pred.PK)
	assert.Contains(t, pred.SK, "TIME#")
	assert.Equal(t, "MODEL#v1", pred.GSI1PK)
	assert.Equal(t, "REVIEW#true", pred.GSI2PK)
	assert.Equal(t, "ML_PREDICTION", pred.Type)
}
