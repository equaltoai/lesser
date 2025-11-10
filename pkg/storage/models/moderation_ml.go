package models

import (
	"fmt"
	"time"
)

// ModerationSample represents a labeled training sample for ML moderation
type ModerationSample struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - Samples by object
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "MLSAMPLE#{object_id}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "VERSION#{version}#{sample_id}"

	// GSI1 - Reviewer queries
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK" json:"gsi1_pk,omitempty"` // Format: "REVIEWER#{reviewer_id}"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK" json:"gsi1_sk,omitempty"` // Format: "TIME#{RFC3339}"

	// GSI2 - Label/Severity queries
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsI2PK" json:"gsi2_pk,omitempty"` // Format: "LABEL#{label}"
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsI2SK" json:"gsi2_sk,omitempty"` // Format: "CONFIDENCE#{confidence}#{RFC3339}"

	// GSI3 - Sample ID lookups
	GSI3PK string `dynamorm:"index:gsi3,pk,attr:gsI3PK" json:"gsi3_pk,omitempty"` // Format: "SAMPLEID#{sample_id}"
	GSI3SK string `dynamorm:"index:gsi3,sk,attr:gsI3SK" json:"gsi3_sk,omitempty"` // Format: "SAMPLEID#{sample_id}"

	// Type marker
	Type string `dynamorm:"attr:type" json:"type"` // "ML_SAMPLE"

	// Sample fields
	ID         string                 `dynamorm:"attr:id" json:"id"`                       // Unique sample ID
	ObjectID   string                 `dynamorm:"attr:objectID" json:"object_id"`          // Object being labeled
	ObjectType string                 `dynamorm:"attr:objectType" json:"object_type"`      // status, account, media
	Label      string                 `dynamorm:"attr:label" json:"label"`                 // The moderation label (spam, hate_speech, etc.)
	ReviewerID string                 `dynamorm:"attr:reviewerID" json:"reviewer_id"`      // Who labeled this sample
	Timestamp  time.Time              `dynamorm:"attr:timestamp" json:"timestamp"`         // When sample was created
	Confidence float64                `dynamorm:"attr:confidence" json:"confidence"`       // Reviewer confidence (0.0-1.0)
	Metadata   map[string]interface{} `dynamorm:"attr:metadata" json:"metadata,omitempty"` // Additional context

	// DynamoDB TTL (samples can expire after use)
	TTL       int64     `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
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
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - Model versions
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "MLMODEL#bedrock"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "VERSION#{version_id}"

	// GSI1 - Active model queries
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK" json:"gsi1_pk,omitempty"` // Format: "MLMODEL#ACTIVE"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK" json:"gsi1_sk,omitempty"` // Format: "ACCURACY#{accuracy}#VERSION#{version_id}"

	// GSI2 - Training job queries
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsI2PK" json:"gsi2_pk,omitempty"` // Format: "TRAININGJOB#{job_id}"
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsI2SK" json:"gsi2_sk,omitempty"` // Format: "TRAININGJOB#{job_id}"

	// Type marker
	Type string `dynamorm:"attr:type" json:"type"` // "ML_MODEL_VERSION"

	// Model version fields
	VersionID      string                 `dynamorm:"attr:versionID" json:"version_id"`           // Model version identifier
	DatasetHash    string                 `dynamorm:"attr:datasetHash" json:"dataset_hash"`       // Hash of training dataset
	Accuracy       float64                `dynamorm:"attr:accuracy" json:"accuracy"`              // Model accuracy
	Precision      float64                `dynamorm:"attr:precision" json:"precision"`            // Model precision
	Recall         float64                `dynamorm:"attr:recall" json:"recall"`                  // Model recall
	F1Score        float64                `dynamorm:"attr:f1Score" json:"f1_score"`               // F1 score
	SamplesUsed    int                    `dynamorm:"attr:samplesUsed" json:"samples_used"`       // Number of training samples
	TrainingJobID  string                 `dynamorm:"attr:trainingJobID" json:"training_job_id"`  // Bedrock training job ID
	TrainingStatus string                 `dynamorm:"attr:trainingStatus" json:"training_status"` // pending, in_progress, completed, failed
	TrainingTime   int                    `dynamorm:"attr:trainingTime" json:"training_time"`     // Training duration in seconds
	IsActive       bool                   `dynamorm:"attr:isActive" json:"is_active"`             // Whether this is the active model
	ModelARN       string                 `dynamorm:"attr:modelARN" json:"model_arn"`             // Bedrock model ARN
	Metadata       map[string]interface{} `dynamorm:"attr:metadata" json:"metadata,omitempty"`    // Training config, hyperparams, etc.

	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
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
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - Metrics by pattern/model
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "MLMETRICS#{pattern_id}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "PERIOD#{period}#{start_time}"

	// GSI1 - Timeframe queries
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK" json:"gsi1_pk,omitempty"` // Format: "METRICS#{period}"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK" json:"gsi1_sk,omitempty"` // Format: "F1SCORE#{f1}#{pattern_id}"

	// Type marker
	Type string `dynamorm:"attr:type" json:"type"` // "ML_METRICS"

	// Metric fields
	PatternID      string    `dynamorm:"attr:patternID" json:"pattern_id"`           // Pattern or model ID
	Period         string    `dynamorm:"attr:period" json:"period"`                  // hourly, daily, weekly, monthly
	StartTime      time.Time `dynamorm:"attr:startTime" json:"start_time"`           // Start of measurement period
	EndTime        time.Time `dynamorm:"attr:endTime" json:"end_time"`               // End of measurement period
	TruePositives  int       `dynamorm:"attr:truePositives" json:"true_positives"`   // Correctly flagged content
	FalsePositives int       `dynamorm:"attr:falsePositives" json:"false_positives"` // Incorrectly flagged content
	TrueNegatives  int       `dynamorm:"attr:trueNegatives" json:"true_negatives"`   // Correctly passed content
	FalseNegatives int       `dynamorm:"attr:falseNegatives" json:"false_negatives"` // Missed problematic content
	Precision      float64   `dynamorm:"attr:precision" json:"precision"`            // TP / (TP + FP)
	Recall         float64   `dynamorm:"attr:recall" json:"recall"`                  // TP / (TP + FN)
	F1Score        float64   `dynamorm:"attr:f1Score" json:"f1_score"`               // 2 * (precision * recall) / (precision + recall)
	TotalReviewed  int       `dynamorm:"attr:totalReviewed" json:"total_reviewed"`   // Total items reviewed

	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
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
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - Training jobs
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "MLJOB#{job_id}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "JOB"

	// GSI1 - Status queries
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK" json:"gsi1_pk,omitempty"` // Format: "MLJOB#{status}"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK" json:"gsi1_sk,omitempty"` // Format: "TIME#{RFC3339}"

	// GSI2 - Tenant queries
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsI2PK" json:"gsi2_pk,omitempty"` // Format: "TENANT#{tenant_id}"
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsI2SK" json:"gsi2_sk,omitempty"` // Format: "TIME#{RFC3339}"

	// Type marker
	Type string `dynamorm:"attr:type" json:"type"` // "ML_TRAINING_JOB"

	// Job fields
	JobID          string                 `dynamorm:"attr:jobID" json:"job_id"`                   // Bedrock job ARN/ID
	JobName        string                 `dynamorm:"attr:jobName" json:"job_name"`               // Human-readable job name
	Status         string                 `dynamorm:"attr:status" json:"status"`                  // SUBMITTED, IN_PROGRESS, COMPLETED, FAILED, STOPPED
	TenantID       string                 `dynamorm:"attr:tenantID" json:"tenant_id"`             // Tenant that initiated training
	InitiatedBy    string                 `dynamorm:"attr:initiatedBy" json:"initiated_by"`       // User who started the training
	DatasetS3Key   string                 `dynamorm:"attr:datasetS3Key" json:"dataset_s3_key"`    // S3 key of training dataset
	DatasetSamples int                    `dynamorm:"attr:datasetSamples" json:"dataset_samples"` // Number of samples in dataset
	BaseModelID    string                 `dynamorm:"attr:baseModelID" json:"base_model_id"`      // Base Bedrock model
	ModelARN       string                 `dynamorm:"attr:modelARN" json:"model_arn"`             // Output model ARN (when completed)
	ErrorMessage   string                 `dynamorm:"attr:errorMessage" json:"error_message"`     // Error details (when failed)
	StartedAt      time.Time              `dynamorm:"attr:startedAt" json:"started_at"`           // When job was submitted
	CompletedAt    time.Time              `dynamorm:"attr:completedAt" json:"completed_at"`       // When job finished
	Metrics        TrainingMetrics        `dynamorm:"attr:metrics" json:"metrics"`                // Training metrics (when completed)
	Metadata       map[string]interface{} `dynamorm:"attr:metadata" json:"metadata,omitempty"`    // Additional context

	// DynamoDB TTL (jobs can expire after 90 days)
	TTL       int64     `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// TrainingMetrics holds training job metrics
type TrainingMetrics struct {
	Accuracy     float64 `json:"accuracy"`
	Precision    float64 `json:"precision"`
	Recall       float64 `json:"recall"`
	F1Score      float64 `json:"f1_score"`
	TrainingTime int     `json:"training_time"` // seconds
}

// TableName returns the DynamoDB table backing TrainingMetrics.
func (TrainingMetrics) TableName() string {
	return MainTableName
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
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - Poll requests
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "MLPOLL#{job_id}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "REQUEST#{timestamp}"

	// GSI1 - Status queries (for finding pending polls)
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK" json:"gsi1_pk,omitempty"` // Format: "MLPOLL#PENDING"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK" json:"gsi1_sk,omitempty"` // Format: "TIME#{RFC3339}"

	// Type marker
	Type string `dynamorm:"attr:type" json:"type"` // "ML_POLL_REQUEST"

	// Poll fields
	JobID         string    `dynamorm:"attr:jobID" json:"job_id"`                  // Bedrock job ARN
	JobName       string    `dynamorm:"attr:jobName" json:"job_name"`              // Human-readable job name
	Attempt       int       `dynamorm:"attr:attempt" json:"attempt"`               // Poll attempt number
	MaxAttempts   int       `dynamorm:"attr:maxAttempts" json:"max_attempts"`      // Maximum poll attempts
	NextPollAfter time.Time `dynamorm:"attr:nextPollAfter" json:"next_poll_after"` // When to poll next
	Status        string    `dynamorm:"attr:status" json:"status"`                 // PENDING, PROCESSING, COMPLETED, FAILED
	CreatedAt     time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt     time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`

	// DynamoDB TTL (poll requests expire after completion or timeout)
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
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
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key - Predictions by object
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // Format: "MLPRED#{object_id}"
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // Format: "TIME#{RFC3339}#{prediction_id}"

	// GSI1 - Model version queries
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsI1PK" json:"gsi1_pk,omitempty"` // Format: "MODEL#{model_version}"
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsI1SK" json:"gsi1_sk,omitempty"` // Format: "TIME#{RFC3339}"

	// GSI2 - Human label queries (for validation)
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsI2PK" json:"gsi2_pk,omitempty"` // Format: "REVIEW#{reviewed}"
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsI2SK" json:"gsi2_sk,omitempty"` // Format: "TIME#{RFC3339}"

	// Type marker
	Type string `dynamorm:"attr:type" json:"type"` // "ML_PREDICTION"

	// Prediction fields
	PredictionID   string                 `dynamorm:"attr:predictionID" json:"prediction_id"`
	ObjectID       string                 `dynamorm:"attr:objectID" json:"object_id"`
	ObjectType     string                 `dynamorm:"attr:objectType" json:"object_type"`
	ModelVersion   string                 `dynamorm:"attr:modelVersion" json:"model_version"`
	PredictedLabel string                 `dynamorm:"attr:predictedLabel" json:"predicted_label"`
	Confidence     float64                `dynamorm:"attr:confidence" json:"confidence"`
	HumanLabel     string                 `dynamorm:"attr:humanLabel" json:"human_label"` // Set when human reviews
	Reviewed       bool                   `dynamorm:"attr:reviewed" json:"reviewed"`      // Whether human has reviewed
	ReviewedBy     string                 `dynamorm:"attr:reviewedBy" json:"reviewed_by"` // Who reviewed
	ReviewedAt     time.Time              `dynamorm:"attr:reviewedAt" json:"reviewed_at"` // When reviewed
	Timestamp      time.Time              `dynamorm:"attr:timestamp" json:"timestamp"`
	Metadata       map[string]interface{} `dynamorm:"attr:metadata" json:"metadata,omitempty"`

	// DynamoDB TTL (predictions can expire after 90 days)
	TTL       int64     `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
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
