package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTranscodingJob_BeforeCreate_Defaults_Keys_AndValidation(t *testing.T) {
	tj := &TranscodingJob{
		JobID:     "job1",
		MediaID:   "m1",
		UserID:    "u1",
		Username:  "alice",
		JobType:   "video",
		StartedAt: time.Unix(1700000000, 0).UTC(),
	}

	before := time.Now()
	err := tj.BeforeCreate()
	after := time.Now()
	assert.NoError(t, err)

	assert.Equal(t, "TRANSCODING_JOB#job1", tj.PK)
	assert.Equal(t, "JOB_METRICS", tj.SK)
	assert.Equal(t, "USER_TRANSCODING#u1", tj.GSI1PK)
	assert.Contains(t, tj.GSI1SK, "#job1")
	assert.Equal(t, "MEDIA_TRANSCODING#m1", tj.GSI2PK)
	assert.Contains(t, tj.GSI2SK, "#job1")

	assert.Equal(t, "processing", tj.Status)
	assert.NotNil(t, tj.OutputVariants)
	assert.NotNil(t, tj.OutputSizes)
	assert.NotNil(t, tj.CostBreakdown)
	assert.NotNil(t, tj.S3Keys)
	assert.NotNil(t, tj.QualityLevels)
	assert.NotNil(t, tj.ExpiresAt)

	ttl := time.Unix(*tj.ExpiresAt, 0)
	assert.True(t, ttl.After(before.Add(365*24*time.Hour-5*time.Second)))
	assert.True(t, ttl.Before(after.Add(365*24*time.Hour+5*time.Second)))
}

func TestTranscodingJob_UpdateKeys_AndValidateErrors(t *testing.T) {
	tj := &TranscodingJob{}
	assert.Error(t, tj.UpdateKeys())

	tj.JobID = "job1"
	tj.MediaID = "m1"
	tj.UserID = "u1"
	tj.JobType = "nope"
	tj.Status = "processing"
	assert.Error(t, tj.Validate())

	tj.JobType = "video"
	tj.Status = "nope"
	assert.Error(t, tj.Validate())

	tj.Status = "processing"
	tj.InputSize = -1
	assert.Error(t, tj.Validate())

	tj.InputSize = 0
	tj.TotalCostMicros = -1
	assert.Error(t, tj.Validate())

	tj.TotalCostMicros = 0
	assert.NoError(t, tj.UpdateKeys())
	assert.Equal(t, "TRANSCODING_JOB#job1", tj.PK)
}

func TestTranscodingJob_BeforeUpdate_EfficiencyAndVariance(t *testing.T) {
	tj := &TranscodingJob{
		JobID:               "job1",
		MediaID:             "m1",
		UserID:              "u1",
		JobType:             "video",
		Status:              "processing",
		StartedAt:           time.Unix(1700000000, 0).UTC(),
		InputSize:           10 * 1024 * 1024, // 10MB
		TotalOutputSize:     5 * 1024 * 1024,
		ProcessingTimeMs:    1000,
		TotalCostMicros:     200,
		EstimatedCostMicros: 100,
	}

	err := tj.BeforeUpdate()
	assert.NoError(t, err)
	assert.InDelta(t, 0.5, tj.CompressionRatio, 0.00001)
	assert.InDelta(t, 20.0, tj.CostPerMB, 0.00001)
	assert.InDelta(t, 10.0, tj.ProcessingSpeedMBps, 0.00001)
	assert.Equal(t, int64(100), tj.CostVariance)
}

func TestTranscodingJob_StatusHelpers_Outputs_AndCosts(t *testing.T) {
	start := time.Now().Add(-2 * time.Second)
	tj := &TranscodingJob{
		JobID:     "job1",
		MediaID:   "m1",
		UserID:    "u1",
		JobType:   "video",
		Status:    "processing",
		StartedAt: start,
	}

	tj.AddOutputVariant("720p", "hls", 100, "k1")
	tj.AddOutputVariant("720p", "hls", 100, "k1") // dup key should not duplicate s3 key
	assert.Equal(t, int64(200), tj.TotalOutputSize)
	assert.Equal(t, 1, len(tj.S3Keys))

	tj.AddCost("mediaconvert", 10)
	tj.AddCost("s3_storage", 20)
	tj.AddCost("s3_request", 30)
	tj.AddCost("lambda_processing", 40)
	tj.AddCost("rekognition", 50)
	assert.Equal(t, int64(150), tj.TotalCostMicros)

	services := tj.GetServiceCostBreakdown()
	assert.Equal(t, int64(10), services["mediaconvert"])
	assert.Equal(t, int64(20), services["s3_storage"])
	assert.Equal(t, int64(30), services["s3_requests"])
	assert.Equal(t, int64(40), services["lambda"])
	assert.Equal(t, int64(50), services["rekognition"])

	assert.False(t, tj.IsCompleted())
	tj.SetCompleted()
	assert.True(t, tj.IsCompleted())
	assert.NotNil(t, tj.CompletedAt)
	assert.NotZero(t, tj.ProcessingTimeMs)
	assert.NotZero(t, tj.Duration())

	tj2 := &TranscodingJob{StartedAt: start}
	tj2.SetFailed("boom")
	assert.True(t, tj2.IsCompleted())
	assert.Equal(t, "boom", tj2.ErrorMessage)

	eff := tj.GetCostEfficiency()
	assert.NotEmpty(t, eff)

	breakdown := tj.GetQualityBreakdown()
	assert.Equal(t, "hls", breakdown["720p"]["format"])
}
