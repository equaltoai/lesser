package models

import (
	"fmt"
	"time"
)

// ModerationSample represents a labeled training sample for ML moderation
type ModerationSample struct {
	// Primary key - Samples by object
	PK string `dynamorm:"pk" json:"pk"` // Format: "MLSAMPLE#{object_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "VERSION#{version}#{sample_id}"

	// GSI1 - Reviewer queries
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1_pk,omitempty"` // Format: "REVIEWER#{reviewer_id}"
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1_sk,omitempty"` // Format: "TIME#{RFC3339}"

	// GSI2 - Label/Severity queries
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"gsi2_pk,omitempty"` // Format: "LABEL#{label}"
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"gsi2_sk,omitempty"` // Format: "CONFIDENCE#{confidence}#{RFC3339}"

	// GSI3 - Sample ID lookups
	GSI3PK string `dynamorm:"index:gsi3,pk" json:"gsi3_pk,omitempty"` // Format: "SAMPLEID#{sample_id}"
	GSI3SK string `dynamorm:"index:gsi3,sk" json:"gsi3_sk,omitempty"` // Format: "SAMPLEID#{sample_id}"

	// Type marker
	Type string `json:"type"` // "ML_SAMPLE"

	// Sample fields
	ID         string                 `json:"id"`                 // Unique sample ID
	ObjectID   string                 `json:"object_id"`          // Object being labeled
	ObjectType string                 `json:"object_type"`        // status, account, media
	Label      string                 `json:"label"`              // The moderation label (spam, hate_speech, etc.)
	ReviewerID string                 `json:"reviewer_id"`        // Who labeled this sample
	Timestamp  time.Time              `json:"timestamp"`          // When sample was created
	Confidence float64                `json:"confidence"`         // Reviewer confidence (0.0-1.0)
	Metadata   map[string]interface{} `json:"metadata,omitempty"` // Additional context

	// DynamoDB TTL (samples can expire after use)
	TTL       int64     `json:"ttl,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName returns the DynamoDB table name
func (ModerationSample) TableName() string {
	return MainTableName
}

// GetPK returns the primary partition key
func (m *ModerationSample) GetPK() string {
	return m.PK
}

// GetSK returns the primary sort key
func (m *ModerationSample) GetSK() string {
	return m.SK
}

// UpdateKeys updates the GSI keys based on current field values
func (m *ModerationSample) UpdateKeys() error {
	// Primary key - samples by object
	m.PK = fmt.Sprintf("MLSAMPLE#%s", m.ObjectID)
	m.SK = fmt.Sprintf("VERSION#v1#%s", m.ID)

	// GSI1 - Reviewer queries
	m.GSI1PK = fmt.Sprintf("REVIEWER#%s", m.ReviewerID)
	m.GSI1SK = fmt.Sprintf("TIME#%s", m.Timestamp.Format(time.RFC3339))

	// GSI2 - Label queries
	m.GSI2PK = fmt.Sprintf("LABEL#%s", m.Label)
	m.GSI2SK = fmt.Sprintf("CONFIDENCE#%.2f#%s", m.Confidence, m.Timestamp.Format(time.RFC3339))

	// GSI3 - Sample ID lookup
	m.GSI3PK = fmt.Sprintf("SAMPLEID#%s", m.ID)
	m.GSI3SK = fmt.Sprintf("SAMPLEID#%s", m.ID)

	m.Type = "ML_SAMPLE"
	return nil
}

// ModerationModelVersion represents metadata about a trained ML model version
type ModerationModelVersion struct {
	// Primary key - Model versions
	PK string `dynamorm:"pk" json:"pk"` // Format: "MLMODEL#bedrock"
	SK string `dynamorm:"sk" json:"sk"` // Format: "VERSION#{version_id}"

	// GSI1 - Active model queries
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1_pk,omitempty"` // Format: "MLMODEL#ACTIVE"
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1_sk,omitempty"` // Format: "ACCURACY#{accuracy}#VERSION#{version_id}"

	// GSI2 - Training job queries
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"gsi2_pk,omitempty"` // Format: "TRAININGJOB#{job_id}"
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"gsi2_sk,omitempty"` // Format: "TRAININGJOB#{job_id}"

	// Type marker
	Type string `json:"type"` // "ML_MODEL_VERSION"

	// Model version fields
	VersionID      string                 `json:"version_id"`         // Model version identifier
	DatasetHash    string                 `json:"dataset_hash"`       // Hash of training dataset
	Accuracy       float64                `json:"accuracy"`           // Model accuracy
	Precision      float64                `json:"precision"`          // Model precision
	Recall         float64                `json:"recall"`             // Model recall
	F1Score        float64                `json:"f1_score"`           // F1 score
	SamplesUsed    int                    `json:"samples_used"`       // Number of training samples
	TrainingJobID  string                 `json:"training_job_id"`    // Bedrock training job ID
	TrainingStatus string                 `json:"training_status"`    // pending, in_progress, completed, failed
	TrainingTime   int                    `json:"training_time"`      // Training duration in seconds
	IsActive       bool                   `json:"is_active"`          // Whether this is the active model
	ModelARN       string                 `json:"model_arn"`          // Bedrock model ARN
	Metadata       map[string]interface{} `json:"metadata,omitempty"` // Training config, hyperparams, etc.

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName returns the DynamoDB table name
func (ModerationModelVersion) TableName() string {
	return MainTableName
}

// GetPK returns the primary partition key
func (m *ModerationModelVersion) GetPK() string {
	return m.PK
}

// GetSK returns the primary sort key
func (m *ModerationModelVersion) GetSK() string {
	return m.SK
}

// UpdateKeys updates the GSI keys based on current field values
func (m *ModerationModelVersion) UpdateKeys() error {
	// Primary key - model versions
	m.PK = "MLMODEL#bedrock"
	m.SK = fmt.Sprintf("VERSION#%s", m.VersionID)

	// GSI1 - Active model queries
	if m.IsActive {
		m.GSI1PK = "MLMODEL#ACTIVE"
		m.GSI1SK = fmt.Sprintf("ACCURACY#%.4f#VERSION#%s", m.Accuracy, m.VersionID)
	} else {
		m.GSI1PK = ""
		m.GSI1SK = ""
	}

	// GSI2 - Training job queries
	if m.TrainingJobID != "" {
		m.GSI2PK = fmt.Sprintf("TRAININGJOB#%s", m.TrainingJobID)
		m.GSI2SK = fmt.Sprintf("TRAININGJOB#%s", m.TrainingJobID)
	}

	m.Type = "ML_MODEL_VERSION"
	return nil
}

// ModerationEffectivenessMetric represents effectiveness metrics for a moderation pattern or model
type ModerationEffectivenessMetric struct {
	// Primary key - Metrics by pattern/model
	PK string `dynamorm:"pk" json:"pk"` // Format: "MLMETRICS#{pattern_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "PERIOD#{period}#{start_time}"

	// GSI1 - Timeframe queries
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1_pk,omitempty"` // Format: "METRICS#{period}"
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1_sk,omitempty"` // Format: "F1SCORE#{f1}#{pattern_id}"

	// Type marker
	Type string `json:"type"` // "ML_METRICS"

	// Metric fields
	PatternID      string    `json:"pattern_id"`      // Pattern or model ID
	Period         string    `json:"period"`          // hourly, daily, weekly, monthly
	StartTime      time.Time `json:"start_time"`      // Start of measurement period
	EndTime        time.Time `json:"end_time"`        // End of measurement period
	TruePositives  int       `json:"true_positives"`  // Correctly flagged content
	FalsePositives int       `json:"false_positives"` // Incorrectly flagged content
	TrueNegatives  int       `json:"true_negatives"`  // Correctly passed content
	FalseNegatives int       `json:"false_negatives"` // Missed problematic content
	Precision      float64   `json:"precision"`       // TP / (TP + FP)
	Recall         float64   `json:"recall"`          // TP / (TP + FN)
	F1Score        float64   `json:"f1_score"`        // 2 * (precision * recall) / (precision + recall)
	TotalReviewed  int       `json:"total_reviewed"`  // Total items reviewed

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName returns the DynamoDB table name
func (ModerationEffectivenessMetric) TableName() string {
	return MainTableName
}

// GetPK returns the primary partition key
func (m *ModerationEffectivenessMetric) GetPK() string {
	return m.PK
}

// GetSK returns the primary sort key
func (m *ModerationEffectivenessMetric) GetSK() string {
	return m.SK
}

// UpdateKeys updates the GSI keys based on current field values
func (m *ModerationEffectivenessMetric) UpdateKeys() error {
	// Primary key - metrics by pattern
	m.PK = fmt.Sprintf("MLMETRICS#%s", m.PatternID)
	m.SK = fmt.Sprintf("PERIOD#%s#%s", m.Period, m.StartTime.Format(time.RFC3339))

	// GSI1 - Timeframe queries (for aggregation)
	m.GSI1PK = fmt.Sprintf("METRICS#%s", m.Period)
	m.GSI1SK = fmt.Sprintf("F1SCORE#%.4f#%s", m.F1Score, m.PatternID)

	m.Type = "ML_METRICS"
	return nil
}

// CalculateMetrics computes precision, recall, and F1 score from counts
func (m *ModerationEffectivenessMetric) CalculateMetrics() {
	// Precision = TP / (TP + FP)
	if m.TruePositives+m.FalsePositives > 0 {
		m.Precision = float64(m.TruePositives) / float64(m.TruePositives+m.FalsePositives)
	} else {
		m.Precision = 0.0
	}

	// Recall = TP / (TP + FN)
	if m.TruePositives+m.FalseNegatives > 0 {
		m.Recall = float64(m.TruePositives) / float64(m.TruePositives+m.FalseNegatives)
	} else {
		m.Recall = 0.0
	}

	// F1 Score = 2 * (precision * recall) / (precision + recall)
	if m.Precision+m.Recall > 0 {
		m.F1Score = 2 * (m.Precision * m.Recall) / (m.Precision + m.Recall)
	} else {
		m.F1Score = 0.0
	}

	m.TotalReviewed = m.TruePositives + m.FalsePositives + m.TrueNegatives + m.FalseNegatives
}

// ModelTrainingJob tracks asynchronous ML model training jobs
type ModelTrainingJob struct {
	// Primary key - Training jobs
	PK string `dynamorm:"pk" json:"pk"` // Format: "MLJOB#{job_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "JOB"

	// GSI1 - Status queries
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1_pk,omitempty"` // Format: "MLJOB#{status}"
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1_sk,omitempty"` // Format: "TIME#{RFC3339}"

	// GSI2 - Tenant queries
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"gsi2_pk,omitempty"` // Format: "TENANT#{tenant_id}"
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"gsi2_sk,omitempty"` // Format: "TIME#{RFC3339}"

	// Type marker
	Type string `json:"type"` // "ML_TRAINING_JOB"

	// Job fields
	JobID          string                 `json:"job_id"`             // Bedrock job ARN/ID
	JobName        string                 `json:"job_name"`           // Human-readable job name
	Status         string                 `json:"status"`             // SUBMITTED, IN_PROGRESS, COMPLETED, FAILED, STOPPED
	TenantID       string                 `json:"tenant_id"`          // Tenant that initiated training
	InitiatedBy    string                 `json:"initiated_by"`       // User who started the training
	DatasetS3Key   string                 `json:"dataset_s3_key"`     // S3 key of training dataset
	DatasetSamples int                    `json:"dataset_samples"`    // Number of samples in dataset
	BaseModelID    string                 `json:"base_model_id"`      // Base Bedrock model
	ModelARN       string                 `json:"model_arn"`          // Output model ARN (when completed)
	ErrorMessage   string                 `json:"error_message"`      // Error details (when failed)
	StartedAt      time.Time              `json:"started_at"`         // When job was submitted
	CompletedAt    time.Time              `json:"completed_at"`       // When job finished
	Metrics        TrainingMetrics        `json:"metrics"`            // Training metrics (when completed)
	Metadata       map[string]interface{} `json:"metadata,omitempty"` // Additional context

	// DynamoDB TTL (jobs can expire after 90 days)
	TTL       int64     `json:"ttl,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TrainingMetrics holds training job metrics
type TrainingMetrics struct {
	Accuracy     float64 `json:"accuracy"`
	Precision    float64 `json:"precision"`
	Recall       float64 `json:"recall"`
	F1Score      float64 `json:"f1_score"`
	TrainingTime int     `json:"training_time"` // seconds
}

// TableName returns the DynamoDB table name
func (ModelTrainingJob) TableName() string {
	return MainTableName
}

// GetPK returns the primary partition key
func (m *ModelTrainingJob) GetPK() string {
	return m.PK
}

// GetSK returns the primary sort key
func (m *ModelTrainingJob) GetSK() string {
	return m.SK
}

// UpdateKeys updates the GSI keys based on current field values
func (m *ModelTrainingJob) UpdateKeys() error {
	// Primary key - training jobs
	m.PK = fmt.Sprintf("MLJOB#%s", m.JobID)
	m.SK = "JOB"

	// GSI1 - Status queries (for polling pending/in-progress jobs)
	m.GSI1PK = fmt.Sprintf("MLJOB#%s", m.Status)
	m.GSI1SK = fmt.Sprintf("TIME#%s", m.StartedAt.Format(time.RFC3339))

	// GSI2 - Tenant queries
	if m.TenantID != "" {
		m.GSI2PK = fmt.Sprintf("TENANT#%s", m.TenantID)
		m.GSI2SK = fmt.Sprintf("TIME#%s", m.StartedAt.Format(time.RFC3339))
	}

	m.Type = "ML_TRAINING_JOB"
	return nil
}

// MLPollRequest tracks pending status checks for training jobs
type MLPollRequest struct {
	// Primary key - Poll requests
	PK string `dynamorm:"pk" json:"pk"` // Format: "MLPOLL#{job_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "REQUEST#{timestamp}"

	// GSI1 - Status queries (for finding pending polls)
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1_pk,omitempty"` // Format: "MLPOLL#PENDING"
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1_sk,omitempty"` // Format: "TIME#{RFC3339}"

	// Type marker
	Type string `json:"type"` // "ML_POLL_REQUEST"

	// Poll fields
	JobID         string    `json:"job_id"`          // Bedrock job ARN
	JobName       string    `json:"job_name"`        // Human-readable job name
	Attempt       int       `json:"attempt"`         // Poll attempt number
	MaxAttempts   int       `json:"max_attempts"`    // Maximum poll attempts
	NextPollAfter time.Time `json:"next_poll_after"` // When to poll next
	Status        string    `json:"status"`          // PENDING, PROCESSING, COMPLETED, FAILED
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// DynamoDB TTL (poll requests expire after completion or timeout)
	TTL int64 `json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (MLPollRequest) TableName() string {
	return MainTableName
}

// GetPK returns the primary partition key
func (m *MLPollRequest) GetPK() string {
	return m.PK
}

// GetSK returns the primary sort key
func (m *MLPollRequest) GetSK() string {
	return m.SK
}

// UpdateKeys updates the GSI keys based on current field values
func (m *MLPollRequest) UpdateKeys() error {
	// Primary key - poll requests by job
	m.PK = fmt.Sprintf("MLPOLL#%s", m.JobID)
	m.SK = fmt.Sprintf("REQUEST#%d", m.CreatedAt.UnixNano())

	// GSI1 - Status queries (for finding pending polls)
	if m.Status == "PENDING" {
		m.GSI1PK = "MLPOLL#PENDING"
		m.GSI1SK = fmt.Sprintf("TIME#%s", m.NextPollAfter.Format(time.RFC3339))
	} else {
		m.GSI1PK = ""
		m.GSI1SK = ""
	}

	m.Type = "ML_POLL_REQUEST"
	return nil
}

// MLPrediction tracks ML model inference predictions for effectiveness metrics
type MLPrediction struct {
	// Primary key - Predictions by object
	PK string `dynamorm:"pk" json:"pk"` // Format: "MLPRED#{object_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "TIME#{RFC3339}#{prediction_id}"

	// GSI1 - Model version queries
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"gsi1_pk,omitempty"` // Format: "MODEL#{model_version}"
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"gsi1_sk,omitempty"` // Format: "TIME#{RFC3339}"

	// GSI2 - Human label queries (for validation)
	GSI2PK string `dynamorm:"index:gsi2,pk" json:"gsi2_pk,omitempty"` // Format: "REVIEW#{reviewed}"
	GSI2SK string `dynamorm:"index:gsi2,sk" json:"gsi2_sk,omitempty"` // Format: "TIME#{RFC3339}"

	// Type marker
	Type string `json:"type"` // "ML_PREDICTION"

	// Prediction fields
	PredictionID   string                 `json:"prediction_id"`
	ObjectID       string                 `json:"object_id"`
	ObjectType     string                 `json:"object_type"`
	ModelVersion   string                 `json:"model_version"`
	PredictedLabel string                 `json:"predicted_label"`
	Confidence     float64                `json:"confidence"`
	HumanLabel     string                 `json:"human_label"` // Set when human reviews
	Reviewed       bool                   `json:"reviewed"`    // Whether human has reviewed
	ReviewedBy     string                 `json:"reviewed_by"` // Who reviewed
	ReviewedAt     time.Time              `json:"reviewed_at"` // When reviewed
	Timestamp      time.Time              `json:"timestamp"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`

	// DynamoDB TTL (predictions can expire after 90 days)
	TTL       int64     `json:"ttl,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName returns the DynamoDB table name
func (MLPrediction) TableName() string {
	return MainTableName
}

// GetPK returns the primary partition key
func (m *MLPrediction) GetPK() string {
	return m.PK
}

// GetSK returns the primary sort key
func (m *MLPrediction) GetSK() string {
	return m.SK
}

// UpdateKeys updates the GSI keys based on current field values
func (m *MLPrediction) UpdateKeys() error {
	// Primary key - predictions by object
	m.PK = fmt.Sprintf("MLPRED#%s", m.ObjectID)
	m.SK = fmt.Sprintf("TIME#%s#%s", m.Timestamp.Format(time.RFC3339), m.PredictionID)

	// GSI1 - Model version queries
	m.GSI1PK = fmt.Sprintf("MODEL#%s", m.ModelVersion)
	m.GSI1SK = fmt.Sprintf("TIME#%s", m.Timestamp.Format(time.RFC3339))

	// GSI2 - Review status queries
	if m.Reviewed {
		m.GSI2PK = "REVIEW#true"
	} else {
		m.GSI2PK = "REVIEW#false"
	}
	m.GSI2SK = fmt.Sprintf("TIME#%s", m.Timestamp.Format(time.RFC3339))

	m.Type = "ML_PREDICTION"
	return nil
}
