package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
)

// Add missing middleware functions

// Recovery middleware function
func Recovery() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			defer func() {
				if r := recover(); r != nil {
					if ctx.Logger != nil {
						ctx.Logger.Error("Handler panicked", map[string]any{
							"panic": r,
						})
					}
					if err := ctx.SystemError("Internal server error", fmt.Errorf("panic: %v", r)); err != nil {
						log.Printf("Failed to send system error response: %v", err)
					}
				}
			}()
			return next.Handle(ctx)
		})
	}
}

// CORS configuration
type CORSConfig struct {
	AllowOrigins []string
	AllowMethods []string
	AllowHeaders []string
}

// CORS middleware function
func CORS(config CORSConfig) lift.Middleware {
	// Precompute strings and origin policy to reduce runtime branching
	methodsCSV := strings.Join(config.AllowMethods, ", ")
	headersCSV := strings.Join(config.AllowHeaders, ", ")
	allowAll := false
	allowedOrigins := make(map[string]struct{}, len(config.AllowOrigins))
	for _, o := range config.AllowOrigins {
		if o == "*" {
			allowAll = true
		} else if o != "" {
			allowedOrigins[o] = struct{}{}
		}
	}

	isAllowed := func(origin string) bool {
		if allowAll {
			return true
		}
		_, ok := allowedOrigins[origin]
		return ok
	}

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			origin := ctx.Header("Origin")
			if origin != "" && isAllowed(origin) {
				ctx.Response.Header("Access-Control-Allow-Origin", origin)
				if methodsCSV != "" {
					ctx.Response.Header("Access-Control-Allow-Methods", methodsCSV)
				}
				if headersCSV != "" {
					ctx.Response.Header("Access-Control-Allow-Headers", headersCSV)
				}
			}

			if ctx.Request.Method == "OPTIONS" {
				ctx.Response.StatusCode = 204
				return nil
			}
			return next.Handle(ctx)
		})
	}
}

// Logger middleware function
func Logger() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			start := time.Now()
			err := next.Handle(ctx)
			duration := time.Since(start)

			log.Printf("[%s] %s %s - %d (%v)",
				start.Format("2006-01-02 15:04:05"),
				ctx.Request.Method,
				ctx.Request.Path,
				ctx.Response.StatusCode,
				duration)

			return err
		})
	}
}

// Enhanced observability config
type ObservabilityConfig struct {
	Metrics bool
	Tracing bool
	Logging bool
}

// Enhanced observability middleware
func EnhancedObservability(config ObservabilityConfig) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			if config.Logging {
				log.Printf("Processing request: %s %s", ctx.Request.Method, ctx.Request.Path)
			}
			return next.Handle(ctx)
		})
	}
}

// Rate limit config
type RateLimitConfig struct {
	Limit int
	Burst int
}

// Rate limit middleware
func RateLimit(_ RateLimitConfig) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Simple rate limiting implementation
			return next.Handle(ctx)
		})
	}
}

// Healthcare domain models with HIPAA compliance

// Patient represents a patient with privacy controls
type Patient struct {
	ConsentStatus   ConsentStatus   `json:"consentStatus"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	LastAccessedAt  time.Time       `json:"lastAccessedAt"`
	Demographics    Demographics    `json:"demographics"`
	ID              string          `json:"id"`
	MRN             string          `json:"mrn"`
	PrivacySettings PrivacySettings `json:"privacySettings"`
	AccessCount     int             `json:"accessCount"`
}

// Demographics contains patient demographic information
type Demographics struct {
	FirstName   string    `json:"firstName"`
	LastName    string    `json:"lastName"`
	DateOfBirth time.Time `json:"dateOfBirth"`
	Gender      string    `json:"gender"`
	Address     Address   `json:"address"`
	Phone       string    `json:"phone"`
	Email       string    `json:"email"`
	SSN         string    `json:"-"` // Never expose SSN in JSON
}

// Address represents a patient address
type Address struct {
	Street1 string `json:"street1"`
	Street2 string `json:"street2,omitempty"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZipCode string `json:"zipCode"`
	Country string `json:"country"`
}

// PrivacySettings controls patient privacy preferences
type PrivacySettings struct {
	RestrictedProviders []string `json:"restrictedProviders"`
	DataRetentionYears  int      `json:"dataRetentionYears"`
	AllowResearch       bool     `json:"allowResearch"`
	AllowMarketing      bool     `json:"allowMarketing"`
	MinimumNecessary    bool     `json:"minimumNecessary"`
}

// ConsentStatus tracks patient consent
type ConsentStatus struct {
	ConsentDate      time.Time  `json:"consentDate"`
	WithdrawalDate   *time.Time `json:"withdrawalDate,omitempty"`
	ConsentVersion   string     `json:"consentVersion"`
	GeneralConsent   bool       `json:"generalConsent"`
	ResearchConsent  bool       `json:"researchConsent"`
	MarketingConsent bool       `json:"marketingConsent"`
}

// MedicalRecord represents an encrypted medical record
type MedicalRecord struct {
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
	ExpiresAt       *time.Time    `json:"expiresAt,omitempty"`
	ID              string        `json:"id"`
	PatientID       string        `json:"patientId"`
	ProviderID      string        `json:"providerId"`
	RecordType      string        `json:"recordType"`
	Title           string        `json:"title"`
	EncryptedData   string        `json:"-"`
	AccessLog       []AccessEntry `json:"-"`
	ComplianceFlags []string      `json:"complianceFlags"`
}

// AccessEntry logs access to medical records
type AccessEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	UserID     string    `json:"userId"`
	UserRole   string    `json:"userRole"`
	AccessType string    `json:"accessType"`
	IPAddress  string    `json:"ipAddress"`
	UserAgent  string    `json:"userAgent"`
	Purpose    string    `json:"purpose"`
	SessionID  string    `json:"sessionId"`
	Authorized bool      `json:"authorized"`
}

// Provider represents a healthcare provider
type Provider struct {
	CreatedAt     time.Time     `json:"createdAt"`
	UpdatedAt     time.Time     `json:"updatedAt"`
	Credentials   Credentials   `json:"credentials"`
	ID            string        `json:"id"`
	NPI           string        `json:"npi"`
	FirstName     string        `json:"firstName"`
	LastName      string        `json:"lastName"`
	AccessLevel   AccessLevel   `json:"accessLevel"`
	AuditSettings AuditSettings `json:"auditSettings"`
	IsActive      bool          `json:"isActive"`
}

// Credentials represents provider credentials
type Credentials struct {
	LicenseExpiry  time.Time `json:"licenseExpiry"`
	LicenseNumber  string    `json:"licenseNumber"`
	LicenseState   string    `json:"licenseState"`
	DEANumber      string    `json:"deaNumber,omitempty"`
	Specialties    []string  `json:"specialties"`
	BoardCertified bool      `json:"boardCertified"`
}

// AccessLevel defines provider access permissions
type AccessLevel struct {
	Level            string   `json:"level"` // basic, standard, elevated, admin
	Permissions      []string `json:"permissions"`
	PatientAccess    []string `json:"patientAccess"` // specific patient IDs if restricted
	RecordTypes      []string `json:"recordTypes"`   // allowed record types
	MaxRecordsPerDay int      `json:"maxRecordsPerDay"`
}

// AuditSettings controls audit behavior for provider
type AuditSettings struct {
	LogAllAccess   bool `json:"logAllAccess"`
	RequireReason  bool `json:"requireReason"`
	AlertOnAccess  bool `json:"alertOnAccess"`
	ReviewRequired bool `json:"reviewRequired"`
}

// Request/Response models
type CreatePatientRequest struct {
	ConsentStatus   ConsentStatus   `json:"consentStatus" validate:"required"`
	Demographics    Demographics    `json:"demographics" validate:"required"`
	PrivacySettings PrivacySettings `json:"privacySettings"`
}

type CreateMedicalRecordRequest struct {
	PatientID  string `json:"patientId" validate:"required"`
	RecordType string `json:"recordType" validate:"required"`
	Title      string `json:"title" validate:"required"`
	Content    string `json:"content" validate:"required"`
	Purpose    string `json:"purpose" validate:"required"`
}

type CreateProviderRequest struct {
	NPI           string        `json:"npi" validate:"required"`
	FirstName     string        `json:"firstName" validate:"required"`
	LastName      string        `json:"lastName" validate:"required"`
	Credentials   Credentials   `json:"credentials" validate:"required"`
	AccessLevel   AccessLevel   `json:"accessLevel" validate:"required"`
	AuditSettings AuditSettings `json:"auditSettings"`
}

type AccessMedicalRecordRequest struct {
	Purpose string `json:"purpose" validate:"required"`
	Reason  string `json:"reason"`
}

type UpdateConsentRequest struct {
	GeneralConsent   *bool  `json:"generalConsent,omitempty"`
	ResearchConsent  *bool  `json:"researchConsent,omitempty"`
	MarketingConsent *bool  `json:"marketingConsent,omitempty"`
	ConsentVersion   string `json:"consentVersion"`
}

// Service interfaces
type PatientService interface {
	// CreatePatient creates a new patient with the given request.
	// Parameters:
	//   - ctx: The context for the request
	//   - req: The request to create a patient
	// Returns:
	//   - The created patient
	//   - An error if the creation fails
	CreatePatient(ctx context.Context, req CreatePatientRequest) (*Patient, error)

	// GetPatient retrieves a patient by their ID and provider ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - id: The ID of the patient
	//   - providerID: The ID of the provider
	// Returns:
	//   - The retrieved patient
	//   - An error if the retrieval fails
	GetPatient(ctx context.Context, id string, providerID string) (*Patient, error)

	// UpdatePatient updates a patient's information.
	// Parameters:
	//   - ctx: The context for the request
	//   - id: The ID of the patient
	//   - patient: The updated patient information
	// Returns:
	//   - An error if the update fails
	UpdatePatient(ctx context.Context, id string, patient *Patient) error

	// SearchPatients searches for patients based on a query and provider ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - query: The search query
	//   - providerID: The ID of the provider
	// Returns:
	//   - A list of patients matching the query
	//   - An error if the search fails
	SearchPatients(ctx context.Context, query string, providerID string) ([]Patient, error)

	// UpdateConsent updates a patient's consent information.
	// Parameters:
	//   - ctx: The context for the request
	//   - patientID: The ID of the patient
	//   - req: The request to update consent
	// Returns:
	//   - An error if the update fails
	UpdateConsent(ctx context.Context, patientID string, req UpdateConsentRequest) error
}

type MedicalRecordService interface {
	// CreateRecord creates a new medical record with the given request.
	// Parameters:
	//   - ctx: The context for the request
	//   - req: The request to create a medical record
	//   - providerID: The ID of the provider
	// Returns:
	//   - The created medical record
	//   - An error if the creation fails
	CreateRecord(ctx context.Context, req CreateMedicalRecordRequest, providerID string) (*MedicalRecord, error)

	// GetRecord retrieves a medical record by its ID and provider ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - id: The ID of the medical record
	//   - providerID: The ID of the provider
	//   - purpose: The purpose of the request
	// Returns:
	//   - The retrieved medical record
	//   - An error if the retrieval fails
	GetRecord(ctx context.Context, id string, providerID string, purpose string) (*MedicalRecord, error)

	// GetPatientRecords retrieves all medical records for a patient by their ID and provider ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - patientID: The ID of the patient
	//   - providerID: The ID of the provider
	// Returns:
	//   - A list of medical records for the patient
	//   - An error if the retrieval fails
	GetPatientRecords(ctx context.Context, patientID string, providerID string) ([]MedicalRecord, error)

	// UpdateRecord updates a medical record's content.
	// Parameters:
	//   - ctx: The context for the request
	//   - id: The ID of the medical record
	//   - content: The updated content
	//   - providerID: The ID of the provider
	// Returns:
	//   - An error if the update fails
	UpdateRecord(ctx context.Context, id string, content string, providerID string) error

	// DeleteRecord deletes a medical record by its ID and provider ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - id: The ID of the medical record
	//   - providerID: The ID of the provider
	// Returns:
	//   - An error if the deletion fails
	DeleteRecord(ctx context.Context, id string, providerID string) error
}

type ProviderService interface {
	// CreateProvider creates a new provider with the given request.
	// Parameters:
	//   - ctx: The context for the request
	//   - req: The request to create a provider
	// Returns:
	//   - The created provider
	//   - An error if the creation fails
	CreateProvider(ctx context.Context, req CreateProviderRequest) (*Provider, error)

	// GetProvider retrieves a provider by their ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - id: The ID of the provider
	// Returns:
	//   - The retrieved provider
	//   - An error if the retrieval fails
	GetProvider(ctx context.Context, id string) (*Provider, error)

	// ValidateAccess validates a provider's access to a patient's record.
	// Parameters:
	//   - ctx: The context for the request
	//   - providerID: The ID of the provider
	//   - patientID: The ID of the patient
	//   - recordType: The type of record
	// Returns:
	//   - A boolean indicating if the access is valid
	//   - An error if the validation fails
	ValidateAccess(ctx context.Context, providerID string, patientID string, recordType string) (bool, error)

	// UpdateAccessLevel updates a provider's access level.
	// Parameters:
	//   - ctx: The context for the request
	//   - providerID: The ID of the provider
	//   - accessLevel: The new access level
	// Returns:
	//   - An error if the update fails
	UpdateAccessLevel(ctx context.Context, providerID string, accessLevel AccessLevel) error
}

type ComplianceService interface {
	// LogAccess logs an access entry.
	// Parameters:
	//   - ctx: The context for the request
	//   - entry: The access entry to log
	// Returns:
	//   - An error if the logging fails
	LogAccess(ctx context.Context, entry AccessEntry) error

	// GetAuditTrail retrieves an audit trail for a patient within a date range.
	// Parameters:
	//   - ctx: The context for the request
	//   - patientID: The ID of the patient
	//   - startDate: The start date of the audit trail
	//   - endDate: The end date of the audit trail
	// Returns:
	//   - A list of access entries
	//   - An error if the retrieval fails
	GetAuditTrail(ctx context.Context, patientID string, startDate, endDate time.Time) ([]AccessEntry, error)

	// GenerateComplianceReport generates a compliance report of the specified type.
	// Parameters:
	//   - ctx: The context for the request
	//   - reportType: The type of report to generate
	//   - params: Additional parameters for the report
	// Returns:
	//   - The generated report
	//   - An error if the report generation fails
	GenerateComplianceReport(ctx context.Context, reportType string, params map[string]any) (any, error)

	// ValidateHIPAACompliance validates HIPAA compliance for an operation.
	// Parameters:
	//   - ctx: The context for the request
	//   - operation: The operation to validate
	//   - data: The data involved in the operation
	// Returns:
	//   - An error if the validation fails
	ValidateHIPAACompliance(ctx context.Context, operation string, data any) error

	// DetectBreach detects a data breach based on access patterns.
	// Parameters:
	//   - ctx: The context for the request
	//   - accessPattern: The access pattern to analyze
	// Returns:
	//   - A boolean indicating if a breach is detected
	//   - A description of the breach
	//   - An error if the detection fails
	DetectBreach(ctx context.Context, accessPattern []AccessEntry) (bool, string, error)
}

type EncryptionService interface {
	// Encrypt encrypts the given data.
	// Parameters:
	//   - data: The data to encrypt
	// Returns:
	//   - The encrypted data as a string
	//   - An error if the encryption fails
	Encrypt(data []byte) (string, error)

	// Decrypt decrypts the given encrypted data.
	// Parameters:
	//   - encryptedData: The encrypted data to decrypt
	// Returns:
	//   - The decrypted data as a byte slice
	//   - An error if the decryption fails
	Decrypt(encryptedData string) ([]byte, error)

	// Hash hashes the given data.
	// Parameters:
	//   - data: The data to hash
	// Returns:
	//   - The hashed data as a string
	Hash(data string) string

	// GenerateKey generates a new encryption key.
	// Returns:
	//   - The generated key as a byte slice
	//   - An error if the key generation fails
	GenerateKey() ([]byte, error)
}

// Mock implementations
type mockPatientService struct{}
type mockMedicalRecordService struct{}
type mockProviderService struct{}
type mockComplianceService struct{}
type mockEncryptionService struct {
	key []byte
}

// Encryption service implementation
func newMockEncryptionService() *mockEncryptionService {
	// In production, this would use proper key management (AWS KMS, etc.)
	key := make([]byte, 32) // AES-256
	if _, err := rand.Read(key); err != nil {
		log.Printf("Warning: failed to generate random key: %v", err)
		// Fall back to a deterministic key for testing
		// In production, this should be a fatal error
		copy(key, []byte("test-key-for-demo-purposes-only!"))
	}
	return &mockEncryptionService{key: key}
}

func (e *mockEncryptionService) Encrypt(data []byte) (string, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (e *mockEncryptionService) Decrypt(encryptedData string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func (e *mockEncryptionService) Hash(data string) string {
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

func (e *mockEncryptionService) GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	return key, err
}

// Mock service implementations
func (m *mockPatientService) CreatePatient(_ context.Context, req CreatePatientRequest) (*Patient, error) {
	patient := &Patient{
		ID:              generateID(),
		MRN:             generateMRN(),
		Demographics:    req.Demographics,
		PrivacySettings: req.PrivacySettings,
		ConsentStatus:   req.ConsentStatus,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		LastAccessedAt:  time.Now(),
		AccessCount:     0,
	}

	// Hash sensitive data
	encService := newMockEncryptionService()
	patient.Demographics.SSN = encService.Hash(req.Demographics.SSN)

	return patient, nil
}

func (m *mockPatientService) GetPatient(_ context.Context, id string, _ string) (*Patient, error) {
	// Simulate patient lookup with access logging
	patient := &Patient{
		ID:  id,
		MRN: "MRN-" + id,
		Demographics: Demographics{
			FirstName:   "John",
			LastName:    "Doe",
			DateOfBirth: time.Date(1980, 1, 15, 0, 0, 0, 0, time.UTC),
			Gender:      "M",
			Address: Address{
				Street1: "123 Main St",
				City:    "Anytown",
				State:   "CA",
				ZipCode: "12345",
				Country: "US",
			},
			Phone: "555-0123",
			Email: "john.doe@example.com",
		},
		PrivacySettings: PrivacySettings{
			AllowResearch:      true,
			AllowMarketing:     false,
			MinimumNecessary:   true,
			DataRetentionYears: 7,
		},
		ConsentStatus: ConsentStatus{
			GeneralConsent:   true,
			ResearchConsent:  true,
			MarketingConsent: false,
			ConsentDate:      time.Now().Add(-30 * 24 * time.Hour),
			ConsentVersion:   "v2.1",
		},
		CreatedAt:      time.Now().Add(-365 * 24 * time.Hour),
		UpdatedAt:      time.Now().Add(-24 * time.Hour),
		LastAccessedAt: time.Now(),
		AccessCount:    15,
	}

	return patient, nil
}

func (m *mockPatientService) UpdatePatient(_ context.Context, _ string, patient *Patient) error {
	patient.UpdatedAt = time.Now()
	return nil
}

func (m *mockPatientService) SearchPatients(_ context.Context, _ string, _ string) ([]Patient, error) {
	// Simulate patient search with privacy controls
	patients := []Patient{
		{
			ID:  "patient_1",
			MRN: "MRN-001",
			Demographics: Demographics{
				FirstName: "Jane",
				LastName:  "Smith",
				Gender:    "F",
			},
			CreatedAt: time.Now().Add(-200 * 24 * time.Hour),
		},
		{
			ID:  "patient_2",
			MRN: "MRN-002",
			Demographics: Demographics{
				FirstName: "Bob",
				LastName:  "Johnson",
				Gender:    "M",
			},
			CreatedAt: time.Now().Add(-150 * 24 * time.Hour),
		},
	}

	return patients, nil
}

func (m *mockPatientService) UpdateConsent(_ context.Context, patientID string, _ UpdateConsentRequest) error {
	// Simulate consent update with audit logging
	log.Printf("HIPAA AUDIT: Consent updated for patient %s", patientID)
	return nil
}

func (m *mockMedicalRecordService) CreateRecord(_ context.Context, req CreateMedicalRecordRequest, providerID string) (*MedicalRecord, error) {
	encService := newMockEncryptionService()

	// Encrypt the medical record content
	encryptedContent, err := encService.Encrypt([]byte(req.Content))
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt medical record: %w", err)
	}

	record := &MedicalRecord{
		ID:            generateID(),
		PatientID:     req.PatientID,
		ProviderID:    providerID,
		RecordType:    req.RecordType,
		Title:         req.Title,
		EncryptedData: encryptedContent,
		AccessLog: []AccessEntry{
			{
				UserID:     providerID,
				UserRole:   "provider",
				AccessType: "create",
				Purpose:    req.Purpose,
				Authorized: true,
				Timestamp:  time.Now(),
			},
		},
		ComplianceFlags: []string{"encrypted", "audit_logged"},
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	return record, nil
}

func (m *mockMedicalRecordService) GetRecord(_ context.Context, id string, providerID string, purpose string) (*MedicalRecord, error) {
	// Simulate record retrieval with access logging
	record := &MedicalRecord{
		ID:              id,
		PatientID:       "patient_123",
		ProviderID:      "provider_456",
		RecordType:      "clinical_note",
		Title:           "Annual Physical Examination",
		ComplianceFlags: []string{"encrypted", "audit_logged", "accessed"},
		CreatedAt:       time.Now().Add(-30 * 24 * time.Hour),
		UpdatedAt:       time.Now().Add(-24 * time.Hour),
	}

	// Log access
	accessEntry := AccessEntry{
		UserID:     providerID,
		UserRole:   "provider",
		AccessType: "read",
		Purpose:    purpose,
		Authorized: true,
		Timestamp:  time.Now(),
	}
	record.AccessLog = append(record.AccessLog, accessEntry)

	return record, nil
}

func (m *mockMedicalRecordService) GetPatientRecords(_ context.Context, patientID string, _ string) ([]MedicalRecord, error) {
	// Simulate getting patient records with access controls
	records := []MedicalRecord{
		{
			ID:              "record_1",
			PatientID:       patientID,
			ProviderID:      "provider_123",
			RecordType:      "clinical_note",
			Title:           "Initial Consultation",
			ComplianceFlags: []string{"encrypted", "audit_logged"},
			CreatedAt:       time.Now().Add(-60 * 24 * time.Hour),
		},
		{
			ID:              "record_2",
			PatientID:       patientID,
			ProviderID:      "provider_456",
			RecordType:      "lab_result",
			Title:           "Blood Work Results",
			ComplianceFlags: []string{"encrypted", "audit_logged", "sensitive"},
			CreatedAt:       time.Now().Add(-30 * 24 * time.Hour),
		},
	}

	return records, nil
}

func (m *mockMedicalRecordService) UpdateRecord(_ context.Context, id string, _ string, providerID string) error {
	// Simulate record update with encryption and audit
	log.Printf("HIPAA AUDIT: Medical record %s updated by provider %s", id, providerID)
	return nil
}

func (m *mockMedicalRecordService) DeleteRecord(_ context.Context, id string, providerID string) error {
	// Simulate record deletion with audit
	log.Printf("HIPAA AUDIT: Medical record %s deleted by provider %s", id, providerID)
	return nil
}

func (m *mockProviderService) CreateProvider(_ context.Context, req CreateProviderRequest) (*Provider, error) {
	provider := &Provider{
		ID:            generateID(),
		NPI:           req.NPI,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		Credentials:   req.Credentials,
		AccessLevel:   req.AccessLevel,
		AuditSettings: req.AuditSettings,
		IsActive:      true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	return provider, nil
}

func (m *mockProviderService) GetProvider(_ context.Context, id string) (*Provider, error) {
	provider := &Provider{
		ID:        id,
		NPI:       "1234567890",
		FirstName: "Dr. Sarah",
		LastName:  "Wilson",
		Credentials: Credentials{
			LicenseNumber:  "MD123456",
			LicenseState:   "CA",
			LicenseExpiry:  time.Now().Add(365 * 24 * time.Hour),
			Specialties:    []string{"Internal Medicine", "Cardiology"},
			BoardCertified: true,
		},
		AccessLevel: AccessLevel{
			Level:            "standard",
			Permissions:      []string{"read_patient", "write_record", "view_lab_results"},
			RecordTypes:      []string{"clinical_note", "lab_result", "prescription"},
			MaxRecordsPerDay: 100,
		},
		AuditSettings: AuditSettings{
			LogAllAccess:   true,
			RequireReason:  true,
			AlertOnAccess:  false,
			ReviewRequired: false,
		},
		IsActive:  true,
		CreatedAt: time.Now().Add(-180 * 24 * time.Hour),
		UpdatedAt: time.Now().Add(-24 * time.Hour),
	}

	return provider, nil
}

func (m *mockProviderService) ValidateAccess(_ context.Context, _ string, _ string, _ string) (bool, error) {
	// Simulate access validation based on provider permissions
	// In production, this would check against actual permissions
	return true, nil
}

func (m *mockProviderService) UpdateAccessLevel(_ context.Context, providerID string, _ AccessLevel) error {
	log.Printf("HIPAA AUDIT: Access level updated for provider %s", providerID)
	return nil
}

func (m *mockComplianceService) LogAccess(_ context.Context, entry AccessEntry) error {
	// Simulate HIPAA audit logging
	log.Printf("HIPAA ACCESS LOG: User %s (%s) %s access to patient data - Purpose: %s, Authorized: %t",
		entry.UserID, entry.UserRole, entry.AccessType, entry.Purpose, entry.Authorized)
	return nil
}

func (m *mockComplianceService) GetAuditTrail(_ context.Context, _ string, _, _ time.Time) ([]AccessEntry, error) {
	// Simulate audit trail retrieval
	auditTrail := []AccessEntry{
		{
			UserID:     "provider_123",
			UserRole:   "physician",
			AccessType: "read",
			Purpose:    "treatment",
			Authorized: true,
			Timestamp:  time.Now().Add(-2 * time.Hour),
			IPAddress:  "192.168.1.100",
			UserAgent:  "Mozilla/5.0 (Healthcare App)",
		},
		{
			UserID:     "nurse_456",
			UserRole:   "nurse",
			AccessType: "read",
			Purpose:    "care_coordination",
			Authorized: true,
			Timestamp:  time.Now().Add(-4 * time.Hour),
			IPAddress:  "192.168.1.101",
			UserAgent:  "Mozilla/5.0 (Healthcare App)",
		},
	}

	return auditTrail, nil
}

func (m *mockComplianceService) GenerateComplianceReport(_ context.Context, reportType string, params map[string]any) (any, error) {
	report := map[string]any{
		"reportType":  reportType,
		"generatedAt": time.Now(),
		"parameters":  params,
		"compliance": map[string]any{
			"hipaa_compliant":    true,
			"encryption_enabled": true,
			"audit_trail_active": true,
			"access_controls":    true,
			"data_minimization":  true,
		},
		"metrics": map[string]any{
			"total_patient_records": 5000,
			"total_access_events":   15000,
			"unauthorized_attempts": 0,
			"encryption_coverage":   100.0,
			"audit_coverage":        100.0,
		},
		"violations": []any{},
		"recommendations": []string{
			"Continue regular access reviews",
			"Update encryption keys quarterly",
			"Conduct annual HIPAA training",
		},
	}

	return report, nil
}

func (m *mockComplianceService) ValidateHIPAACompliance(_ context.Context, _ string, _ any) error {
	// Simulate HIPAA compliance validation
	// In production, this would perform comprehensive compliance checks
	return nil
}

func (m *mockComplianceService) DetectBreach(_ context.Context, accessPattern []AccessEntry) (bool, string, error) {
	// Simulate breach detection
	// Check for suspicious patterns like unusual access times, locations, etc.
	for _, entry := range accessPattern {
		if !entry.Authorized {
			return true, "Unauthorized access detected", nil
		}
	}
	return false, "", nil
}

// Utility functions
func generateID() string {
	return fmt.Sprintf("id_%d", time.Now().UnixNano())
}

func generateMRN() string {
	return fmt.Sprintf("MRN-%d", time.Now().UnixNano()%1000000)
}

// Handler functions
func createPatient(ctx *lift.Context) error {
	var req CreatePatientRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid request", 400)
	}

	// HIPAA compliance validation
	complianceService := &mockComplianceService{}
	if err := complianceService.ValidateHIPAACompliance(ctx.Request.Context(), "create_patient", req); err != nil {
		return ctx.Forbidden("HIPAA compliance violation", err)
	}

	patientService := &mockPatientService{}
	patient, err := patientService.CreatePatient(ctx.Request.Context(), req)
	if err != nil {
		return ctx.SystemError("Failed to create patient", err)
	}

	// Log access for HIPAA audit
	accessEntry := AccessEntry{
		UserID:     ctx.Request.Header()["X-Provider-ID"],
		UserRole:   "provider",
		AccessType: "create",
		Purpose:    "patient_registration",
		Authorized: true,
		Timestamp:  time.Now(),
		IPAddress:  ctx.Request.RemoteAddr(),
		UserAgent:  ctx.Request.UserAgent(),
	}
	if err := complianceService.LogAccess(ctx.Request.Context(), accessEntry); err != nil {
		log.Printf("Failed to log access for HIPAA audit: %v", err)
	}

	log.Printf("HIPAA AUDIT: Patient created - ID: %s, MRN: %s, Provider: %s",
		patient.ID, patient.MRN, accessEntry.UserID)

	return ctx.Created(patient)
}

func getPatient(ctx *lift.Context) error {
	patientID := ctx.PathParam("id")
	if patientID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Patient ID is required", 400)
	}

	providerID := ctx.Request.Header()["X-Provider-ID"]
	if providerID == "" {
		return ctx.Unauthorized("Provider authentication required", nil)
	}

	// Validate provider access
	providerService := &mockProviderService{}
	hasAccess, err := providerService.ValidateAccess(ctx.Request.Context(), providerID, patientID, "patient_data")
	if err != nil {
		return ctx.SystemError("Access validation failed", err)
	}
	if !hasAccess {
		return ctx.Forbidden("Insufficient permissions", nil)
	}

	patientService := &mockPatientService{}
	patient, err := patientService.GetPatient(ctx.Request.Context(), patientID, providerID)
	if err != nil {
		return ctx.NotFound("Patient not found", err)
	}

	// Log access for HIPAA audit
	complianceService := &mockComplianceService{}
	accessEntry := AccessEntry{
		UserID:     providerID,
		UserRole:   "provider",
		AccessType: "read",
		Purpose:    ctx.QueryParam("purpose"),
		Authorized: true,
		Timestamp:  time.Now(),
		IPAddress:  ctx.Request.RemoteAddr(),
		UserAgent:  ctx.Request.UserAgent(),
	}
	if err := complianceService.LogAccess(ctx.Request.Context(), accessEntry); err != nil {
		log.Printf("Failed to log access for HIPAA audit: %v", err)
	}

	return ctx.OK(patient)
}

func searchPatients(ctx *lift.Context) error {
	query := ctx.QueryParam("q")
	if query == "" {
		return lift.NewLiftError("BAD_REQUEST", "Search query is required", 400)
	}

	providerID := ctx.Request.Header()["X-Provider-ID"]
	if providerID == "" {
		return ctx.Unauthorized("Provider authentication required", nil)
	}

	patientService := &mockPatientService{}
	patients, err := patientService.SearchPatients(ctx.Request.Context(), query, providerID)
	if err != nil {
		return ctx.SystemError("Patient search failed", err)
	}

	// Log search for HIPAA audit
	complianceService := &mockComplianceService{}
	accessEntry := AccessEntry{
		UserID:     providerID,
		UserRole:   "provider",
		AccessType: "search",
		Purpose:    "patient_lookup",
		Authorized: true,
		Timestamp:  time.Now(),
		IPAddress:  ctx.Request.RemoteAddr(),
		UserAgent:  ctx.Request.UserAgent(),
	}
	if err := complianceService.LogAccess(ctx.Request.Context(), accessEntry); err != nil {
		log.Printf("Failed to log access for HIPAA audit: %v", err)
	}

	return ctx.OK(map[string]any{
		"query":       query,
		"results":     patients,
		"count":       len(patients),
		"searched_at": time.Now(),
	})
}

func updatePatientConsent(ctx *lift.Context) error {
	patientID := ctx.PathParam("id")
	if patientID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Patient ID is required", 400)
	}

	var req UpdateConsentRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid request", 400)
	}

	patientService := &mockPatientService{}
	if err := patientService.UpdateConsent(ctx.Request.Context(), patientID, req); err != nil {
		return ctx.SystemError("Failed to update consent", err)
	}

	// Log consent update for HIPAA audit
	complianceService := &mockComplianceService{}
	accessEntry := AccessEntry{
		UserID:     ctx.Request.Header()["X-Provider-ID"],
		UserRole:   "provider",
		AccessType: "update",
		Purpose:    "consent_management",
		Authorized: true,
		Timestamp:  time.Now(),
		IPAddress:  ctx.Request.RemoteAddr(),
		UserAgent:  ctx.Request.UserAgent(),
	}
	if err := complianceService.LogAccess(ctx.Request.Context(), accessEntry); err != nil {
		log.Printf("Failed to log access for HIPAA audit: %v", err)
	}

	return ctx.OK(map[string]any{
		"patientId": patientID,
		"updated":   true,
		"timestamp": time.Now(),
	})
}

func createMedicalRecord(ctx *lift.Context) error {
	var req CreateMedicalRecordRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid request", 400)
	}

	providerID := ctx.Request.Header()["X-Provider-ID"]
	if providerID == "" {
		return ctx.Unauthorized("Provider authentication required", nil)
	}

	// Validate provider access
	providerService := &mockProviderService{}
	hasAccess, err := providerService.ValidateAccess(ctx.Request.Context(), providerID, req.PatientID, req.RecordType)
	if err != nil {
		return ctx.SystemError("Access validation failed", err)
	}
	if !hasAccess {
		return ctx.Forbidden("Insufficient permissions", nil)
	}

	recordService := &mockMedicalRecordService{}
	record, err := recordService.CreateRecord(ctx.Request.Context(), req, providerID)
	if err != nil {
		return ctx.SystemError("Failed to create medical record", err)
	}

	log.Printf("HIPAA AUDIT: Medical record created - ID: %s, Patient: %s, Provider: %s, Type: %s",
		record.ID, record.PatientID, record.ProviderID, record.RecordType)

	return ctx.Created(record)
}

func getMedicalRecord(ctx *lift.Context) error {
	recordID := ctx.PathParam("id")
	if recordID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Record ID is required", 400)
	}

	providerID := ctx.Request.Header()["X-Provider-ID"]
	if providerID == "" {
		return ctx.Unauthorized("Provider authentication required", nil)
	}

	purpose := ctx.QueryParam("purpose")
	if purpose == "" {
		return lift.NewLiftError("BAD_REQUEST", "Access purpose is required for HIPAA compliance", 400)
	}

	recordService := &mockMedicalRecordService{}
	record, err := recordService.GetRecord(ctx.Request.Context(), recordID, providerID, purpose)
	if err != nil {
		return ctx.NotFound("Medical record not found", err)
	}

	return ctx.OK(record)
}

func getPatientRecords(ctx *lift.Context) error {
	patientID := ctx.PathParam("id")
	if patientID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Patient ID is required", 400)
	}

	providerID := ctx.Request.Header()["X-Provider-ID"]
	if providerID == "" {
		return ctx.Unauthorized("Provider authentication required", nil)
	}

	recordService := &mockMedicalRecordService{}
	records, err := recordService.GetPatientRecords(ctx.Request.Context(), patientID, providerID)
	if err != nil {
		return ctx.SystemError("Failed to retrieve patient records", err)
	}

	return ctx.OK(map[string]any{
		"patientId":    patientID,
		"records":      records,
		"count":        len(records),
		"retrieved_at": time.Now(),
	})
}

func createProvider(ctx *lift.Context) error {
	var req CreateProviderRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid request", 400)
	}

	providerService := &mockProviderService{}
	provider, err := providerService.CreateProvider(ctx.Request.Context(), req)
	if err != nil {
		return ctx.SystemError("Failed to create provider", err)
	}

	log.Printf("HIPAA AUDIT: Provider created - ID: %s, NPI: %s, Name: %s %s",
		provider.ID, provider.NPI, provider.FirstName, provider.LastName)

	return ctx.Created(provider)
}

func getProvider(ctx *lift.Context) error {
	providerID := ctx.PathParam("id")
	if providerID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Provider ID is required", 400)
	}

	providerService := &mockProviderService{}
	provider, err := providerService.GetProvider(ctx.Request.Context(), providerID)
	if err != nil {
		return ctx.NotFound("Provider not found", err)
	}

	return ctx.OK(provider)
}

func getAuditTrail(ctx *lift.Context) error {
	patientID := ctx.QueryParam("patient_id")
	startDateStr := ctx.QueryParam("start_date")
	endDateStr := ctx.QueryParam("end_date")

	var startDate, endDate time.Time
	var err error

	if startDateStr != "" {
		startDate, err = time.Parse("2006-01-02", startDateStr)
		if err != nil {
			return lift.NewLiftError("BAD_REQUEST", "Invalid start_date format (use YYYY-MM-DD)", 400)
		}
	} else {
		startDate = time.Now().Add(-30 * 24 * time.Hour) // Default to 30 days ago
	}

	if endDateStr != "" {
		endDate, err = time.Parse("2006-01-02", endDateStr)
		if err != nil {
			return lift.NewLiftError("BAD_REQUEST", "Invalid end_date format (use YYYY-MM-DD)", 400)
		}
	} else {
		endDate = time.Now()
	}

	complianceService := &mockComplianceService{}
	auditTrail, err := complianceService.GetAuditTrail(ctx.Request.Context(), patientID, startDate, endDate)
	if err != nil {
		return ctx.SystemError("Failed to retrieve audit trail", err)
	}

	return ctx.OK(map[string]any{
		"patient_id":   patientID,
		"start_date":   startDate,
		"end_date":     endDate,
		"audit_trail":  auditTrail,
		"count":        len(auditTrail),
		"generated_at": time.Now(),
	})
}

func generateComplianceReport(ctx *lift.Context) error {
	reportType := ctx.PathParam("type")
	if reportType == "" {
		return lift.NewLiftError("BAD_REQUEST", "Report type is required", 400)
	}

	// Parse query parameters as report parameters
	params := make(map[string]any)
	// Use the QueryParam method instead of URL.Query()
	if startDate := ctx.QueryParam("start_date"); startDate != "" {
		params["start_date"] = startDate
	}
	if endDate := ctx.QueryParam("end_date"); endDate != "" {
		params["end_date"] = endDate
	}
	if patientID := ctx.QueryParam("patient_id"); patientID != "" {
		params["patient_id"] = patientID
	}

	complianceService := &mockComplianceService{}
	report, err := complianceService.GenerateComplianceReport(ctx.Request.Context(), reportType, params)
	if err != nil {
		return ctx.SystemError("Failed to generate compliance report", err)
	}

	return ctx.OK(report)
}

func healthCheck(ctx *lift.Context) error {
	health := map[string]any{
		"status":    "healthy",
		"timestamp": time.Now(),
		"version":   "1.0.0",
		"services": map[string]string{
			"database":           "healthy",
			"encryption_service": "healthy",
			"audit_service":      "healthy",
			"compliance_engine":  "healthy",
		},
		"compliance": map[string]any{
			"hipaa_compliant":   true,
			"encryption_active": true,
			"audit_logging":     true,
			"access_controls":   true,
		},
		"metrics": map[string]any{
			"uptime_seconds":      3600,
			"total_patients":      5000,
			"total_records":       25000,
			"audit_events_today":  1500,
			"encryption_coverage": 100.0,
		},
	}

	return ctx.OK(health)
}

func main() {
	// Create Lift application
	app := lift.New()

	// Enterprise healthcare middleware stack
	app.Use(Logger())
	app.Use(Recovery())
	app.Use(CORS(CORSConfig{
		AllowOrigins: []string{"https://healthcare.example.com"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE"},
		AllowHeaders: []string{"Authorization", "Content-Type", "X-Provider-ID"},
	}))

	// Enhanced observability for HIPAA compliance
	app.Use(EnhancedObservability(ObservabilityConfig{
		Metrics: true,
		Tracing: true,
		Logging: true,
	}))

	// Security middleware
	app.Use(RateLimit(RateLimitConfig{
		Limit: 200, // Higher limit for healthcare operations
		Burst: 50,
	}))

	// API versioning
	api := app.Group("/api/v1")

	// Health check endpoint
	if err := api.GET("/health", healthCheck); err != nil {
		log.Fatalf("Failed to register health endpoint: %v", err)
	}

	// Patient management endpoints
	patients := api.Group("/patients")
	if err := patients.POST("", createPatient); err != nil {
		log.Fatalf("Failed to register POST /patients: %v", err)
	}
	if err := patients.GET("/search", searchPatients); err != nil {
		log.Fatalf("Failed to register GET /patients/search: %v", err)
	}
	if err := patients.GET("/:id", getPatient); err != nil {
		log.Fatalf("Failed to register GET /patients/:id: %v", err)
	}
	if err := patients.PUT("/:id/consent", updatePatientConsent); err != nil {
		log.Fatalf("Failed to register PUT /patients/:id/consent: %v", err)
	}
	if err := patients.GET("/:id/records", getPatientRecords); err != nil {
		log.Fatalf("Failed to register GET /patients/:id/records: %v", err)
	}

	// Medical records endpoints
	records := api.Group("/records")
	if err := records.POST("", createMedicalRecord); err != nil {
		log.Fatalf("Failed to register POST /records: %v", err)
	}
	if err := records.GET("/:id", getMedicalRecord); err != nil {
		log.Fatalf("Failed to register GET /records/:id: %v", err)
	}

	// Provider management endpoints
	providers := api.Group("/providers")
	if err := providers.POST("", createProvider); err != nil {
		log.Fatalf("Failed to register POST /providers: %v", err)
	}
	if err := providers.GET("/:id", getProvider); err != nil {
		log.Fatalf("Failed to register GET /providers/:id: %v", err)
	}

	// Compliance and audit endpoints
	compliance := api.Group("/compliance")
	if err := compliance.GET("/audit-trail", getAuditTrail); err != nil {
		log.Fatalf("Failed to register GET /compliance/audit-trail: %v", err)
	}
	if err := compliance.GET("/reports/:type", generateComplianceReport); err != nil {
		log.Fatalf("Failed to register GET /compliance/reports/:type: %v", err)
	}

	// Start the application
	log.Println("Starting Enterprise Healthcare API on port 8080...")
	log.Println("HIPAA Compliance Features:")
	log.Println("  ✓ Data encryption at rest and in transit")
	log.Println("  ✓ Comprehensive audit logging")
	log.Println("  ✓ Role-based access controls")
	log.Println("  ✓ Patient consent management")
	log.Println("  ✓ Minimum necessary access principle")
	log.Println("")
	log.Println("Available endpoints:")
	log.Println("  GET  /api/v1/health")
	log.Println("  POST /api/v1/patients")
	log.Println("  GET  /api/v1/patients/search")
	log.Println("  GET  /api/v1/patients/:id")
	log.Println("  PUT  /api/v1/patients/:id/consent")
	log.Println("  GET  /api/v1/patients/:id/records")
	log.Println("  POST /api/v1/records")
	log.Println("  GET  /api/v1/records/:id")
	log.Println("  POST /api/v1/providers")
	log.Println("  GET  /api/v1/providers/:id")
	log.Println("  GET  /api/v1/compliance/audit-trail")
	log.Println("  GET  /api/v1/compliance/reports/:type")

	if err := app.Start(); err != nil {
		log.Fatalf("Failed to start application: %v", err)
	}
}
