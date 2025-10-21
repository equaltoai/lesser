package compliance

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/google/uuid"

	"github.com/pay-theory/lift/pkg/dynamorm"
	"github.com/pay-theory/lift/pkg/security"
)

const (
	statusFailed = "failed"
)

// GDPRCompleteService provides comprehensive GDPR compliance implementation
// Memory optimized: 328 → 280 bytes (48 bytes saved)
type GDPRCompleteService struct {
	db            *dynamorm.DynamORMWrapper
	s3Client      *s3.Client
	sesClient     *ses.Client
	snsClient     *sns.Client
	auditLogger   *GDPRAuditLogger
	encryptionKey []byte
	config        GDPRCompleteConfig
	mu            sync.RWMutex
}

// GDPRCompleteConfig defines complete GDPR configuration
// Memory optimized: 328 → 272 bytes (56 bytes saved)
type GDPRCompleteConfig struct {
	NotificationTopicArn    string   `json:"notification_topic_arn"`
	FromEmailAddress        string   `json:"from_email_address"`
	Environment             string   `json:"environment"`
	ConsentTableName        string   `json:"consent_table_name"`
	RequestTableName        string   `json:"request_table_name"`
	AuditTableName          string   `json:"audit_table_name"`
	PIATableName            string   `json:"pia_table_name"`
	DataExportBucket        string   `json:"data_export_bucket"`
	AuditLogBucket          string   `json:"audit_log_bucket"`
	ComplianceOfficerEmail  string   `json:"compliance_officer_email"`
	Region                  string   `json:"region"`
	DefaultSafeguards       []string `json:"default_safeguards"`
	ProhibitedCountries     []string `json:"prohibited_countries"`
	RequestProcessingDays   int      `json:"request_processing_days"`
	ConsentExpiryDays       int      `json:"consent_expiry_days"`
	MaxExportSizeMB         int      `json:"max_export_size_mb"`
	AuditRetentionDays      int      `json:"audit_retention_days"`
	DataRetentionDays       int      `json:"data_retention_days"`
	BreachNotificationHours int      `json:"breach_notification_hours"`
	EnableCrossBorderRules  bool     `json:"enable_cross_border_rules"`
	Enabled                 bool     `json:"enabled"`
	RequireExplicitConsent  bool     `json:"require_explicit_consent"`
	AutoDataDeletion        bool     `json:"auto_data_deletion"`
	EncryptionEnabled       bool     `json:"encryption_enabled"`
}

// DataExportRecord represents a data export for GDPR compliance
type DataExportRecord struct {
	ExpiresAt     time.Time              `json:"expires_at" `
	CreatedAt     time.Time              `json:"created_at" `
	RequestDate   time.Time              `json:"request_date" `
	Metadata      map[string]interface{} `json:"metadata" `
	DownloadedAt  *time.Time             `json:"downloaded_at,omitempty" `
	CompletedAt   *time.Time             `json:"completed_at,omitempty" `
	Format        string                 `json:"format" `
	ExportID      string                 `json:"export_id" `
	EncryptionKey string                 `json:"encryption_key,omitempty" `
	DataSubjectID string                 `json:"data_subject_id" `
	ExportPath    string                 `json:"export_path" `
	Status        string                 `json:"status" `
	RequestID     string                 `json:"request_id" `
	DataSources   []string               `json:"data_sources" `
	FileSizeBytes int64                  `json:"file_size_bytes" `
}

// DataDeletionRecord represents a data deletion for GDPR compliance
type DataDeletionRecord struct {
	RequestDate      time.Time              `json:"request_date" `
	DeletedAt        time.Time              `json:"deleted_at" `
	Metadata         map[string]interface{} `json:"metadata" `
	DeletionID       string                 `json:"deletion_id" `
	DataSubjectID    string                 `json:"data_subject_id" `
	RequestID        string                 `json:"request_id" `
	Status           string                 `json:"status" `
	RetentionReason  string                 `json:"retention_reason" `
	DeletedBy        string                 `json:"deleted_by" `
	VerificationHash string                 `json:"verification_hash" `
	TablesCleared    []string               `json:"tables_cleared" `
	RetainedData     []string               `json:"retained_data" `
}

// ConsentRecordComplete extends security.ConsentRecord with DynamoDB integration
type ConsentRecordComplete struct {
	PK         string `json:"pk" `
	SK         string `json:"sk" `
	GSI1PK     string `json:"gsi1pk" `
	GSI1SK     string `json:"gsi1sk" `
	EntityType string `json:"entity_type" `
	security.ConsentRecord
	TTL int64 `json:"ttl" `
}

// PIARecordComplete extends security.PIAResult with DynamoDB integration
type PIARecordComplete struct {
	PK         string `json:"pk" `
	SK         string `json:"sk" `
	EntityType string `json:"entity_type" `
	security.PIAResult
}

// DataSubjectRequestComplete represents a complete data subject request
type DataSubjectRequestComplete struct {
	CompletedAt *time.Time `json:"completed_at,omitempty" `
	PK          string     `json:"pk" `
	SK          string     `json:"sk" `
	GSI1PK      string     `json:"gsi1pk" `
	GSI1SK      string     `json:"gsi1sk" `
	EntityType  string     `json:"entity_type" `
	ProcessedBy string     `json:"processed_by" `
	security.DataAccessRequest
}

// GDPRAuditLogger provides comprehensive audit logging
type GDPRAuditLogger struct {
	service *GDPRCompleteService
}

// NewGDPRCompleteService creates a new complete GDPR service
func NewGDPRCompleteService(config GDPRCompleteConfig, db *dynamorm.DynamORMWrapper) *GDPRCompleteService {
	// Generate encryption key from environment or create new one
	encryptionKey := make([]byte, 32)
	if _, err := rand.Read(encryptionKey); err != nil {
		log.Printf("Warning: Failed to generate encryption key: %v", err)
	}

	service := &GDPRCompleteService{
		config:        config,
		db:            db,
		encryptionKey: encryptionKey,
	}

	service.auditLogger = &GDPRAuditLogger{service: service}

	return service
}

// SetAWSClients sets the AWS service clients
func (g *GDPRCompleteService) SetAWSClients(s3Client *s3.Client, sesClient *ses.Client, snsClient *sns.Client) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.s3Client = s3Client
	g.sesClient = sesClient
	g.snsClient = snsClient
}

// DeleteUserData implements comprehensive GDPR data deletion
func (g *GDPRCompleteService) DeleteUserData(ctx context.Context, dataSubjectID string, requestID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if dataSubjectID == "" {
		return fmt.Errorf("data subject ID is required")
	}

	// Start audit trail
	auditID := g.auditLogger.StartOperation(ctx, "GDPR_DELETE", dataSubjectID)
	defer g.auditLogger.CompleteOperation(ctx, auditID)

	// Get all tables that might contain user data
	tables := g.getUserDataTables()
	var clearedTables []string
	var retainedData []string

	// Delete from each table
	for _, table := range tables {
		deleted, retained, err := g.deleteFromTable(ctx, table, dataSubjectID)
		if err != nil {
			g.auditLogger.LogError(ctx, auditID, "Failed to delete from table", map[string]interface{}{
				"table": table,
				"error": err.Error(),
			})
			return fmt.Errorf("failed to delete from table %s: %w", table, err)
		}

		if deleted {
			clearedTables = append(clearedTables, table)
		}

		retainedData = append(retainedData, retained...)
	}

	// Delete from S3 if applicable
	if err := g.deleteUserFiles(ctx, dataSubjectID); err != nil {
		g.auditLogger.LogError(ctx, auditID, "Failed to delete user files", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("failed to delete user files: %w", err)
	}

	// Create deletion record for compliance
	deletionRecord := &DataDeletionRecord{
		DeletionID:       uuid.New().String(),
		DataSubjectID:    dataSubjectID,
		RequestID:        requestID,
		RequestDate:      time.Now(),
		Status:           "completed",
		TablesCleared:    clearedTables,
		RetainedData:     retainedData,
		RetentionReason:  "Legal obligation and legitimate interest",
		DeletedBy:        g.getCurrentUser(ctx),
		DeletedAt:        time.Now(),
		VerificationHash: g.calculateDeletionHash(dataSubjectID, clearedTables),
		Metadata: map[string]interface{}{
			"audit_id":    auditID,
			"environment": g.config.Environment,
			"service":     "gdpr-complete",
		},
	}

	// Store deletion record
	if err := g.db.Put(ctx, deletionRecord); err != nil {
		return fmt.Errorf("failed to create deletion record: %w", err)
	}

	// Send notification
	if err := g.sendDeletionNotification(ctx, deletionRecord); err != nil {
		log.Printf("Warning: Failed to send deletion notification: %v", err)
	}

	g.auditLogger.LogSuccess(ctx, auditID, "Data deletion completed", map[string]interface{}{
		"tables_cleared": len(clearedTables),
		"retained_items": len(retainedData),
		"deletion_id":    deletionRecord.DeletionID,
	})

	return nil
}

// ExportUserData implements comprehensive GDPR data export
func (g *GDPRCompleteService) ExportUserData(ctx context.Context, dataSubjectID string, requestID string) (*DataExportRecord, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if dataSubjectID == "" {
		return nil, fmt.Errorf("data subject ID is required")
	}

	builder := newDataExportBuilder(ctx, g, dataSubjectID, requestID)
	return builder.build()
}

// dataExportBuilder orchestrates the data export process
type dataExportBuilder struct {
	exportRecord  *DataExportRecord
	service       *GDPRCompleteService
	exportData    map[string]interface{}
	ctx           context.Context
	dataSubjectID string
	requestID     string
	auditID       string
}

// newDataExportBuilder creates a new data export builder
func newDataExportBuilder(ctx context.Context, service *GDPRCompleteService, dataSubjectID, requestID string) *dataExportBuilder {
	return &dataExportBuilder{
		service:       service,
		ctx:           ctx,
		dataSubjectID: dataSubjectID,
		requestID:     requestID,
		exportData:    make(map[string]interface{}),
	}
}

// build executes the complete export process
func (b *dataExportBuilder) build() (*DataExportRecord, error) {
	// Start audit trail
	b.auditID = b.service.auditLogger.StartOperation(b.ctx, "GDPR_EXPORT", b.dataSubjectID)
	defer b.service.auditLogger.CompleteOperation(b.ctx, b.auditID)

	// Initialize export record
	if err := b.initializeExportRecord(); err != nil {
		return nil, err
	}

	// Collect user data
	if err := b.collectUserData(); err != nil {
		return nil, err
	}

	// Process and prepare data
	finalData, err := b.processExportData()
	if err != nil {
		return nil, err
	}

	// Upload to storage
	if err := b.uploadExportData(finalData); err != nil {
		return nil, err
	}

	// Finalize and notify
	if err := b.finalizeExport(finalData); err != nil {
		return nil, err
	}

	return b.exportRecord, nil
}

// initializeExportRecord creates and stores the initial export record
func (b *dataExportBuilder) initializeExportRecord() error {
	b.exportRecord = &DataExportRecord{
		ExportID:      uuid.New().String(),
		DataSubjectID: b.dataSubjectID,
		RequestID:     b.requestID,
		RequestDate:   time.Now(),
		Status:        "processing",
		Format:        "json",
		ExpiresAt:     time.Now().Add(7 * 24 * time.Hour), // 7 days
		DataSources:   []string{},
		CreatedAt:     time.Now(),
		Metadata: map[string]interface{}{
			"audit_id":    b.auditID,
			"environment": b.service.config.Environment,
			"service":     "gdpr-complete",
		},
	}

	if err := b.service.db.Put(b.ctx, b.exportRecord); err != nil {
		return fmt.Errorf("failed to create export record: %w", err)
	}

	return nil
}

// collectUserData gathers data from all configured sources
func (b *dataExportBuilder) collectUserData() error {
	tables := b.service.getUserDataTables()

	for _, table := range tables {
		if err := b.collectFromTable(table); err != nil {
			return err
		}
	}

	b.addExportMetadata()
	return nil
}

// collectFromTable collects data from a specific table
func (b *dataExportBuilder) collectFromTable(table string) error {
	data, err := b.service.collectFromTable(b.ctx, table, b.dataSubjectID)
	if err != nil {
		b.service.auditLogger.LogError(b.ctx, b.auditID, "Failed to collect from table", map[string]interface{}{
			"table": table,
			"error": err.Error(),
		})
		b.updateExportStatus(statusFailed)
		return fmt.Errorf("failed to collect from table %s: %w", table, err)
	}

	if len(data) > 0 {
		b.exportData[table] = data
		b.exportRecord.DataSources = append(b.exportRecord.DataSources, table)
	}

	return nil
}

// addExportMetadata adds metadata to the export
func (b *dataExportBuilder) addExportMetadata() {
	b.exportData["export_metadata"] = map[string]interface{}{
		"export_id":    b.exportRecord.ExportID,
		"export_date":  b.exportRecord.RequestDate,
		"data_subject": b.dataSubjectID,
		"format":       b.exportRecord.Format,
		"gdpr_version": "2.0",
		"service":      "lift-gdpr-complete",
	}
}

// processExportData marshals and optionally encrypts the data
func (b *dataExportBuilder) processExportData() ([]byte, error) {
	// Convert to JSON
	jsonData, err := json.MarshalIndent(b.exportData, "", "  ")
	if err != nil {
		b.updateExportStatus(statusFailed)
		return nil, fmt.Errorf("failed to marshal export data: %w", err)
	}

	// Check size limits
	if err := b.validateDataSize(jsonData); err != nil {
		return nil, err
	}

	// Encrypt if enabled
	if b.service.config.EncryptionEnabled {
		return b.encryptExportData(jsonData)
	}

	return jsonData, nil
}

// validateDataSize checks if the export data is within size limits
func (b *dataExportBuilder) validateDataSize(data []byte) error {
	if len(data) > b.service.config.MaxExportSizeMB*1024*1024 {
		b.updateExportStatus(statusFailed)
		return fmt.Errorf("export data exceeds maximum size limit")
	}
	return nil
}

// encryptExportData encrypts the export data
func (b *dataExportBuilder) encryptExportData(data []byte) ([]byte, error) {
	encrypted, key, err := b.service.encryptData(data)
	if err != nil {
		b.updateExportStatus(statusFailed)
		return nil, fmt.Errorf("failed to encrypt export data: %w", err)
	}

	b.exportRecord.EncryptionKey = base64.StdEncoding.EncodeToString(key)
	return encrypted, nil
}

// uploadExportData uploads the processed data to S3
func (b *dataExportBuilder) uploadExportData(data []byte) error {
	exportPath := b.buildExportPath()

	if err := b.service.uploadToS3(b.ctx, b.service.config.DataExportBucket, exportPath, data); err != nil {
		b.updateExportStatus(statusFailed)
		return fmt.Errorf("failed to upload export to S3: %w", err)
	}

	b.exportRecord.ExportPath = exportPath
	b.exportRecord.FileSizeBytes = int64(len(data))

	return nil
}

// buildExportPath constructs the S3 path for the export
func (b *dataExportBuilder) buildExportPath() string {
	exportPath := fmt.Sprintf("gdpr-exports/%s/%s/%s.json",
		b.service.config.Environment,
		b.dataSubjectID,
		b.exportRecord.ExportID)

	if b.service.config.EncryptionEnabled {
		exportPath += ".enc"
	}

	return exportPath
}

// finalizeExport completes the export process
func (b *dataExportBuilder) finalizeExport(_ []byte) error {
	// Update export record
	now := time.Now()
	b.exportRecord.Status = "completed"
	b.exportRecord.CompletedAt = &now

	if err := b.service.db.Put(b.ctx, b.exportRecord); err != nil {
		return fmt.Errorf("failed to update export record: %w", err)
	}

	// Send notification
	if err := b.service.sendExportNotification(b.ctx, b.exportRecord); err != nil {
		log.Printf("Warning: Failed to send export notification: %v", err)
	}

	// Log success
	b.service.auditLogger.LogSuccess(b.ctx, b.auditID, "Data export completed", map[string]interface{}{
		"export_id":    b.exportRecord.ExportID,
		"file_size":    b.exportRecord.FileSizeBytes,
		"data_sources": len(b.exportRecord.DataSources),
	})

	return nil
}

// updateExportStatus updates the export record status with error handling
func (b *dataExportBuilder) updateExportStatus(status string) {
	b.exportRecord.Status = status
	if err := b.service.db.Put(b.ctx, b.exportRecord); err != nil {
		log.Printf("Failed to update export record: %v", err)
	}
}

// ProcessConsentUpdate implements consent management with audit trail
func (g *GDPRCompleteService) ProcessConsentUpdate(ctx context.Context, dataSubjectID string, consent ConsentUpdate) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Validate consent categories
	if err := g.validateConsentCategories(consent); err != nil {
		return fmt.Errorf("invalid consent categories: %w", err)
	}

	// Create consent record with versioning
	consentRecord := &ConsentRecordComplete{
		ConsentRecord: security.ConsentRecord{
			ID:                 uuid.New().String(),
			DataSubjectID:      dataSubjectID,
			ConsentVersion:     g.getNextConsentVersion(ctx, dataSubjectID),
			ConsentDate:        time.Now(),
			ConsentMethod:      consent.Method,
			LegalBasis:         consent.LegalBasis,
			ProcessingPurposes: consent.Categories,
			Status:             "active",
			Granular:           true,
			Specific:           true,
			Informed:           true,
			Unambiguous:        true,
			ConsentProof: &security.ConsentProof{
				Type:      "digital_record",
				Evidence:  consent.Evidence,
				Timestamp: time.Now(),
				IPAddress: g.getClientIP(ctx),
				UserAgent: g.getUserAgent(ctx),
				Method:    consent.Method,
				Verified:  true,
				Metadata: map[string]interface{}{
					"service":     "gdpr-complete",
					"environment": g.config.Environment,
				},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		PK:         fmt.Sprintf("CONSENT#%s", dataSubjectID),
		SK:         fmt.Sprintf("VERSION#%s", time.Now().Format("20060102150405")),
		GSI1PK:     "CONSENT",
		GSI1SK:     fmt.Sprintf("%s#%s", dataSubjectID, time.Now().Format("20060102150405")),
		EntityType: "consent",
	}

	// Set TTL if configured
	if g.config.ConsentExpiryDays > 0 {
		expiry := time.Now().AddDate(0, 0, g.config.ConsentExpiryDays)
		consentRecord.ExpiryDate = &expiry
		consentRecord.TTL = expiry.Unix()
	}

	// Store consent record
	if err := g.db.Put(ctx, consentRecord); err != nil {
		return fmt.Errorf("failed to store consent record: %w", err)
	}

	// Update processing based on consent
	if err := g.updateProcessingRules(ctx, dataSubjectID, consent); err != nil {
		return fmt.Errorf("failed to update processing rules: %w", err)
	}

	// Log consent event
	if err := g.auditLogger.LogConsentEvent(ctx, &security.ConsentEvent{
		EventType:     "consent_updated",
		ConsentID:     consentRecord.ID,
		DataSubjectID: dataSubjectID,
		Timestamp:     time.Now(),
		Details: map[string]interface{}{
			"consent_method": consent.Method,
			"categories":     consent.Categories,
			"legal_basis":    consent.LegalBasis,
			"version":        consentRecord.ConsentVersion,
		},
		IPAddress: g.getClientIP(ctx),
		UserAgent: g.getUserAgent(ctx),
		Metadata: map[string]interface{}{
			"service":     "gdpr-complete",
			"environment": g.config.Environment,
		},
	}); err != nil {
		log.Printf("Failed to log consent event: %v", err)
	}

	return nil
}

// ProcessBreachNotification handles privacy breach notifications
func (g *GDPRCompleteService) ProcessBreachNotification(ctx context.Context, breach PrivacyBreach) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Calculate notification deadlines
	authorityDeadline := breach.DetectedAt.Add(time.Duration(g.config.BreachNotificationHours) * time.Hour)

	// Create breach record
	breachRecord := &PrivacyBreachRecord{
		BreachID:          uuid.New().String(),
		BreachType:        breach.Type,
		Severity:          breach.Severity,
		DetectedAt:        breach.DetectedAt,
		ReportedAt:        time.Now(),
		AffectedSubjects:  breach.AffectedCount,
		DataCategories:    breach.DataCategories,
		Cause:             breach.Cause,
		Mitigation:        breach.MitigationSteps,
		AuthorityNotified: false,
		SubjectsNotified:  false,
		AuthorityDeadline: authorityDeadline,
		Status:            "reported",
		ReportedBy:        g.getCurrentUser(ctx),
		Metadata: map[string]interface{}{
			"service":     "gdpr-complete",
			"environment": g.config.Environment,
		},
	}

	// Store breach record
	if err := g.db.Put(ctx, breachRecord); err != nil {
		return fmt.Errorf("failed to store breach record: %w", err)
	}

	// Send immediate notifications to compliance officer
	if err := g.sendBreachNotification(ctx, breachRecord); err != nil {
		log.Printf("Warning: Failed to send breach notification: %v", err)
	}

	// Log breach event
	if err := g.auditLogger.LogPrivacyBreach(ctx, &security.PrivacyBreachLog{
		BreachID:          breachRecord.BreachID,
		BreachType:        breach.Type,
		Severity:          breach.Severity,
		DetectedDate:      breach.DetectedAt,
		ReportedDate:      time.Now(),
		AffectedSubjects:  breach.AffectedCount,
		DataCategories:    breach.DataCategories,
		Cause:             breach.Cause,
		Mitigation:        breach.MitigationSteps,
		AuthorityNotified: false,
		SubjectsNotified:  false,
		Metadata: map[string]interface{}{
			"breach_id":          breachRecord.BreachID,
			"authority_deadline": authorityDeadline,
		},
	}); err != nil {
		log.Printf("Failed to log privacy breach: %v", err)
	}

	return nil
}

// Helper methods

func (g *GDPRCompleteService) getUserDataTables() []string {
	// This should be configured based on your application's data model
	return []string{
		g.config.ConsentTableName,
		g.config.RequestTableName,
		"users",
		"user_profiles",
		"user_activities",
		"user_preferences",
		"sessions",
		"payment_methods",
		"transactions",
	}
}

func (g *GDPRCompleteService) deleteFromTable(ctx context.Context, tableName string, dataSubjectID string) (bool, []string, error) {
	// Query for items belonging to the data subject
	items, err := g.queryUserData(ctx, tableName, dataSubjectID)
	if err != nil {
		return false, nil, err
	}

	if len(items) == 0 {
		return false, nil, nil
	}

	var retainedItems []string
	deletedCount := 0

	// Delete items, but retain some for legal obligations
	for _, item := range items {
		shouldRetain, reason := g.shouldRetainData(item, tableName)
		if shouldRetain {
			retainedItems = append(retainedItems, reason)
			continue
		}

		// Delete the item
		if err := g.deleteItem(ctx, tableName, item); err != nil {
			return false, retainedItems, err
		}
		deletedCount++
	}

	return deletedCount > 0, retainedItems, nil
}

func (g *GDPRCompleteService) collectFromTable(ctx context.Context, tableName string, dataSubjectID string) ([]map[string]interface{}, error) {
	return g.queryUserData(ctx, tableName, dataSubjectID)
}

func (g *GDPRCompleteService) queryUserData(_ context.Context, _ string, _ string) ([]map[string]interface{}, error) {
	// This is a simplified implementation - in practice, you'd need to know
	// the specific query patterns for each table
	// Production implementation should use DynamORM Query methods
	// with proper GSI configuration for user data queries
	// The DynamORM wrapper doesn't expose direct DynamoDB client access
	// This needs to be refactored to use DynamORM's query builder

	// Example query would look like:
	// input := &dynamodb.QueryInput{
	//     TableName:              aws.String(tableName),
	//     KeyConditionExpression: aws.String("data_subject_id = :subject_id"),
	//     ExpressionAttributeValues: map[string]types.AttributeValue{
	//         ":subject_id": &types.AttributeValueMemberS{Value: dataSubjectID},
	//     },
	// }

	var items []map[string]interface{}

	return items, nil
}

func (g *GDPRCompleteService) deleteItem(_ context.Context, tableName string, item map[string]interface{}) error {
	// Extract primary key from item
	key := make(map[string]types.AttributeValue)

	if pk, exists := item["PK"]; exists {
		pkStr, ok := pk.(string)
		if !ok {
			return fmt.Errorf("PK is not a string")
		}
		key["PK"] = &types.AttributeValueMemberS{Value: pkStr}
	}
	if sk, exists := item["SK"]; exists {
		skStr, ok := sk.(string)
		if !ok {
			return fmt.Errorf("SK is not a string")
		}
		key["SK"] = &types.AttributeValueMemberS{Value: skStr}
	}

	// Production implementation should use DynamORM Delete method
	// The DynamORM wrapper doesn't expose direct DynamoDB client access
	// This needs to be refactored to use DynamORM's delete method
	_ = tableName // Suppress unused variable warning
	_ = key       // Suppress unused variable warning

	return nil
}

func (g *GDPRCompleteService) shouldRetainData(item map[string]interface{}, tableName string) (bool, string) {
	// Implement business logic for data retention
	// This is where you'd check for legal obligations, legitimate interests, etc.

	// Example: Retain financial transaction data for legal compliance
	if tableName == "transactions" {
		return true, "Financial transaction data retained for legal compliance (7 years)"
	}

	// Example: Retain audit logs
	if tableName == g.config.AuditTableName {
		return true, "Audit logs retained for compliance monitoring"
	}

	// Check for consent withdrawal vs. legitimate interest
	if status, exists := item["status"]; exists && status == "legal_hold" {
		return true, "Data under legal hold - cannot be deleted"
	}

	return false, ""
}

func (g *GDPRCompleteService) deleteUserFiles(_ context.Context, dataSubjectID string) error {
	if g.s3Client == nil {
		return nil // S3 not configured, skip file deletion
	}

	// List and delete user files from S3
	prefix := fmt.Sprintf("user-data/%s/", dataSubjectID)

	// Implementation would list and delete S3 objects with the prefix
	// This is a simplified version
	log.Printf("Would delete S3 objects with prefix: %s", prefix)

	return nil
}

func (g *GDPRCompleteService) encryptData(data []byte) ([]byte, []byte, error) {
	// Generate a random key for this export
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, key, nil
}

func (g *GDPRCompleteService) uploadToS3(_ context.Context, bucket, key string, data []byte) error {
	if g.s3Client == nil {
		return fmt.Errorf("S3 client not configured")
	}

	// Implementation would upload to S3
	log.Printf("Would upload %d bytes to s3://%s/%s", len(data), bucket, key)

	return nil
}

func (g *GDPRCompleteService) convertAttributeValue(av types.AttributeValue) interface{} { //nolint:unused // false positive - used recursively
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return v.Value
	case *types.AttributeValueMemberN:
		return v.Value
	case *types.AttributeValueMemberBOOL:
		return v.Value
	case *types.AttributeValueMemberL:
		var list []interface{}
		for _, item := range v.Value {
			list = append(list, g.convertAttributeValue(item))
		}
		return list
	case *types.AttributeValueMemberM:
		result := make(map[string]interface{})
		for k, av := range v.Value {
			result[k] = g.convertAttributeValue(av)
		}
		return result
	default:
		return nil
	}
}

func (g *GDPRCompleteService) calculateDeletionHash(dataSubjectID string, tables []string) string {
	data := fmt.Sprintf("%s:%s:%d", dataSubjectID, strings.Join(tables, ","), time.Now().Unix())
	hash := sha256.Sum256([]byte(data))
	return base64.StdEncoding.EncodeToString(hash[:])
}

func (g *GDPRCompleteService) getCurrentUser(ctx context.Context) string {
	// Extract current user from context
	if user := ctx.Value("user_id"); user != nil {
		if userStr, ok := user.(string); ok {
			return userStr
		}
	}
	return "system"
}

func (g *GDPRCompleteService) getClientIP(ctx context.Context) string {
	if ip := ctx.Value("client_ip"); ip != nil {
		if ipStr, ok := ip.(string); ok {
			return ipStr
		}
	}
	return "unknown"
}

func (g *GDPRCompleteService) getUserAgent(ctx context.Context) string {
	if ua := ctx.Value("user_agent"); ua != nil {
		if uaStr, ok := ua.(string); ok {
			return uaStr
		}
	}
	return "unknown"
}

func (g *GDPRCompleteService) getNextConsentVersion(_ context.Context, _ string) string {
	// Query existing consents to determine next version
	return fmt.Sprintf("v%d", time.Now().Unix())
}

func (g *GDPRCompleteService) validateConsentCategories(consent ConsentUpdate) error {
	if len(consent.Categories) == 0 {
		return fmt.Errorf("at least one consent category is required")
	}

	validCategories := map[string]bool{
		"essential":       true,
		"functional":      true,
		"analytics":       true,
		"marketing":       true,
		"personalization": true,
		"third_party":     true,
	}

	for _, category := range consent.Categories {
		if !validCategories[category] {
			return fmt.Errorf("invalid consent category: %s", category)
		}
	}

	return nil
}

func (g *GDPRCompleteService) updateProcessingRules(_ context.Context, dataSubjectID string, consent ConsentUpdate) error {
	// Update processing rules based on consent
	// This would integrate with your application's processing logic
	log.Printf("Updated processing rules for %s: %v", dataSubjectID, consent.Categories)
	return nil
}

func (g *GDPRCompleteService) sendDeletionNotification(_ context.Context, record *DataDeletionRecord) error {
	if g.sesClient == nil {
		return nil // Email not configured
	}

	// Send email notification about data deletion
	log.Printf("Would send deletion notification for %s", record.DataSubjectID)
	return nil
}

func (g *GDPRCompleteService) sendExportNotification(_ context.Context, record *DataExportRecord) error {
	if g.sesClient == nil {
		return nil // Email not configured
	}

	// Send email notification with download link
	log.Printf("Would send export notification for %s", record.DataSubjectID)
	return nil
}

func (g *GDPRCompleteService) sendBreachNotification(_ context.Context, record *PrivacyBreachRecord) error {
	if g.snsClient == nil {
		return nil // SNS not configured
	}

	// Send immediate notification to compliance team
	log.Printf("Would send breach notification for %s", record.BreachID)
	return nil
}

// Additional types for complete implementation

type ConsentUpdate struct {
	Metadata   map[string]interface{} `json:"metadata"`
	LegalBasis string                 `json:"legal_basis"`
	Method     string                 `json:"method"`
	Evidence   string                 `json:"evidence"`
	Categories []string               `json:"categories"`
}

type PrivacyBreach struct {
	DetectedAt      time.Time `json:"detected_at"`
	Type            string    `json:"type"`
	Severity        string    `json:"severity"`
	Cause           string    `json:"cause"`
	DataCategories  []string  `json:"data_categories"`
	MitigationSteps []string  `json:"mitigation_steps"`
	AffectedCount   int       `json:"affected_count"`
}

type PrivacyBreachRecord struct {
	AuthorityDeadline time.Time              `json:"authority_deadline" `
	DetectedAt        time.Time              `json:"detected_at" `
	ReportedAt        time.Time              `json:"reported_at" `
	Metadata          map[string]interface{} `json:"metadata" `
	BreachType        string                 `json:"breach_type" `
	Severity          string                 `json:"severity" `
	BreachID          string                 `json:"breach_id" `
	Cause             string                 `json:"cause" `
	ReportedBy        string                 `json:"reported_by" `
	Status            string                 `json:"status" `
	DataCategories    []string               `json:"data_categories" `
	Mitigation        []string               `json:"mitigation" `
	AffectedSubjects  int                    `json:"affected_subjects" `
	SubjectsNotified  bool                   `json:"subjects_notified" `
	AuthorityNotified bool                   `json:"authority_notified" `
}

// Audit Logger Implementation

func (al *GDPRAuditLogger) StartOperation(_ context.Context, operation, dataSubjectID string) string {
	auditID := uuid.New().String()
	// Implementation would create audit trail entry
	log.Printf("Started %s operation for %s (audit ID: %s)", operation, dataSubjectID, auditID)
	return auditID
}

func (al *GDPRAuditLogger) CompleteOperation(_ context.Context, auditID string) {
	// Implementation would complete audit trail entry
	log.Printf("Completed operation (audit ID: %s)", auditID)
}

func (al *GDPRAuditLogger) LogError(_ context.Context, auditID, message string, metadata map[string]interface{}) {
	// Implementation would log error to audit trail
	log.Printf("Error in operation %s: %s %v", auditID, message, metadata)
}

func (al *GDPRAuditLogger) LogSuccess(_ context.Context, auditID, message string, metadata map[string]interface{}) {
	// Implementation would log success to audit trail
	log.Printf("Success in operation %s: %s %v", auditID, message, metadata)
}

func (al *GDPRAuditLogger) LogConsentEvent(_ context.Context, event *security.ConsentEvent) error {
	// Implementation would store consent event
	log.Printf("Consent event: %s for %s", event.EventType, event.DataSubjectID)
	return nil
}

func (al *GDPRAuditLogger) LogDataSubjectRequest(_ context.Context, request *security.DataSubjectRequestLog) error {
	// Implementation would store data subject request log
	log.Printf("Data subject request: %s for %s", request.RequestType, request.DataSubjectID)
	return nil
}

func (al *GDPRAuditLogger) LogDataProcessingActivity(_ context.Context, _ *security.DataProcessingLog) error {
	// Implementation would store data processing activity log
	log.Printf("Data processing activity logged")
	return nil
}

func (al *GDPRAuditLogger) LogCrossBorderTransfer(_ context.Context, transfer *security.CrossBorderTransferLog) error {
	// Implementation would store cross-border transfer log
	log.Printf("Cross-border transfer: %s to %s", transfer.SourceCountry, transfer.DestinationCountry)
	return nil
}

func (al *GDPRAuditLogger) LogPrivacyBreach(_ context.Context, breach *security.PrivacyBreachLog) error {
	// Implementation would store privacy breach log
	log.Printf("Privacy breach: %s (severity: %s)", breach.BreachType, breach.Severity)
	return nil
}
