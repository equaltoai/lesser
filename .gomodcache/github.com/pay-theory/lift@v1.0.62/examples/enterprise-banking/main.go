package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
)

// Banking domain models
type Account struct {
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	ID            string    `json:"id"`
	CustomerID    string    `json:"customerId"`
	AccountNumber string    `json:"accountNumber"`
	AccountType   string    `json:"accountType"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	Balance       float64   `json:"balance"`
}

type Transaction struct {
	ProcessedAt    time.Time      `json:"processedAt"`
	ComplianceData map[string]any `json:"complianceData,omitempty"`
	ID             string         `json:"id"`
	FromAccountID  string         `json:"fromAccountId"`
	ToAccountID    string         `json:"toAccountId"`
	Currency       string         `json:"currency"`
	Type           string         `json:"type"`
	Status         string         `json:"status"`
	Description    string         `json:"description"`
	Reference      string         `json:"reference"`
	Amount         float64        `json:"amount"`
}

type Payment struct {
	ProcessedAt     time.Time `json:"processedAt"`
	ID              string    `json:"id"`
	PayerAccountID  string    `json:"payerAccountId"`
	PayeeAccountID  string    `json:"payeeAccountId"`
	Currency        string    `json:"currency"`
	PaymentMethod   string    `json:"paymentMethod"`
	Status          string    `json:"status"`
	ComplianceFlags []string  `json:"complianceFlags,omitempty"`
	Amount          float64   `json:"amount"`
	FraudScore      float64   `json:"fraudScore"`
}

// Request/Response models
type CreateAccountRequest struct {
	CustomerID  string `json:"customerId" validate:"required"`
	AccountType string `json:"accountType" validate:"required,oneof=checking savings business"`
	Currency    string `json:"currency" validate:"required,len=3"`
}

type CreateTransactionRequest struct {
	FromAccountID string  `json:"fromAccountId" validate:"required"`
	ToAccountID   string  `json:"toAccountId" validate:"required"`
	Currency      string  `json:"currency" validate:"required,len=3"`
	Description   string  `json:"description" validate:"required"`
	Reference     string  `json:"reference"`
	Amount        float64 `json:"amount" validate:"required,gt=0"`
}

type ProcessPaymentRequest struct {
	PayerAccountID string  `json:"payerAccountId" validate:"required"`
	PayeeAccountID string  `json:"payeeAccountId" validate:"required"`
	Currency       string  `json:"currency" validate:"required,len=3"`
	PaymentMethod  string  `json:"paymentMethod" validate:"required"`
	Amount         float64 `json:"amount" validate:"required,gt=0"`
}

type RefundPaymentRequest struct {
	PaymentID string  `json:"paymentId" validate:"required"`
	Reason    string  `json:"reason" validate:"required"`
	Amount    float64 `json:"amount" validate:"required,gt=0"`
}

// Service interfaces (would be implemented with actual business logic)
type AccountService interface {
	// CreateAccount creates a new account with the given request.
	// Parameters:
	//   - ctx: The context for the request
	//   - req: The request to create an account
	// Returns:
	//   - The created account
	//   - An error if the creation fails
	CreateAccount(_ context.Context, req CreateAccountRequest) (*Account, error)

	// GetAccount retrieves an account by its ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - id: The ID of the account
	// Returns:
	//   - The retrieved account
	//   - An error if the retrieval fails
	GetAccount(_ context.Context, id string) (*Account, error)

	// GetBalance retrieves the balance of an account by its ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - accountID: The ID of the account
	// Returns:
	//   - The balance of the account
	//   - An error if the retrieval fails
	GetBalance(_ context.Context, accountID string) (float64, error)

	// UpdateBalance updates the balance of an account by its ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - accountID: The ID of the account
	//   - amount: The amount to update the balance by
	// Returns:
	//   - An error if the update fails
	UpdateBalance(_ context.Context, accountID string, amount float64) error
}

type TransactionService interface {
	// CreateTransaction creates a new transaction with the given request.
	// Parameters:
	//   - ctx: The context for the request
	//   - req: The request to create a transaction
	// Returns:
	//   - The created transaction
	//   - An error if the creation fails
	CreateTransaction(_ context.Context, req CreateTransactionRequest) (*Transaction, error)

	// GetTransaction retrieves a transaction by its ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - id: The ID of the transaction
	// Returns:
	//   - The retrieved transaction
	//   - An error if the retrieval fails
	GetTransaction(_ context.Context, id string) (*Transaction, error)

	// GetAccountTransactions retrieves all transactions for an account by its ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - accountID: The ID of the account
	// Returns:
	//   - A list of transactions for the account
	//   - An error if the retrieval fails
	GetAccountTransactions(_ context.Context, accountID string) ([]Transaction, error)
}

type PaymentService interface {
	// ProcessPayment processes a payment with the given request.
	// Parameters:
	//   - ctx: The context for the request
	//   - req: The request to process a payment
	// Returns:
	//   - The processed payment
	//   - An error if the processing fails
	ProcessPayment(_ context.Context, req ProcessPaymentRequest) (*Payment, error)

	// GetPayment retrieves a payment by its ID.
	// Parameters:
	//   - ctx: The context for the request
	//   - id: The ID of the payment
	// Returns:
	//   - The retrieved payment
	//   - An error if the retrieval fails
	GetPayment(_ context.Context, id string) (*Payment, error)

	// RefundPayment refunds a payment with the given request.
	// Parameters:
	//   - ctx: The context for the request
	//   - req: The request to refund a payment
	// Returns:
	//   - The refunded payment
	//   - An error if the refund fails
	RefundPayment(_ context.Context, req RefundPaymentRequest) (*Payment, error)
}

type ComplianceService interface {
	// ValidateTransaction validates a transaction.
	// Parameters:
	//   - ctx: The context for the request
	//   - transaction: The transaction to validate
	// Returns:
	//   - An error if the validation fails
	ValidateTransaction(_ context.Context, transaction *Transaction) error

	// AuditPayment audits a payment.
	// Parameters:
	//   - ctx: The context for the request
	//   - payment: The payment to audit
	// Returns:
	//   - An error if the audit fails
	AuditPayment(_ context.Context, payment *Payment) error

	// GenerateReport generates a report of the specified type.
	// Parameters:
	//   - ctx: The context for the request
	//   - reportType: The type of report to generate
	// Returns:
	//   - The generated report
	//   - An error if the report generation fails
	GenerateReport(_ context.Context, reportType string) (any, error)
}

type FraudDetectionService interface {
	// AnalyzePayment analyzes a payment for fraud.
	// Parameters:
	//   - ctx: The context for the request
	//   - payment: The payment to analyze
	// Returns:
	//   - The fraud score
	//   - An error if the analysis fails
	AnalyzePayment(_ context.Context, payment *Payment) (float64, error)

	// CheckRisk checks the risk of a transaction.
	// Parameters:
	//   - ctx: The context for the request
	//   - accountID: The ID of the account
	//   - amount: The amount of the transaction
	// Returns:
	//   - A boolean indicating if the transaction is risky
	//   - An error if the risk check fails
	CheckRisk(_ context.Context, accountID string, amount float64) (bool, error)
}

// Mock implementations for demonstration
type mockAccountService struct{}
type mockTransactionService struct{}
type mockPaymentService struct{}
type mockComplianceService struct{}
type mockFraudDetectionService struct{}

func (m *mockAccountService) CreateAccount(_ context.Context, req CreateAccountRequest) (*Account, error) {
	return &Account{
		ID:            generateID(),
		CustomerID:    req.CustomerID,
		AccountNumber: generateAccountNumber(),
		AccountType:   req.AccountType,
		Balance:       0.0,
		Currency:      req.Currency,
		Status:        "active",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}, nil
}

func (m *mockAccountService) GetAccount(_ context.Context, id string) (*Account, error) {
	return &Account{
		ID:            id,
		CustomerID:    "customer_123",
		AccountNumber: "ACC-" + id,
		AccountType:   "checking",
		Balance:       1000.00,
		Currency:      "USD",
		Status:        "active",
		CreatedAt:     time.Now().Add(-24 * time.Hour),
		UpdatedAt:     time.Now(),
	}, nil
}

func (m *mockAccountService) GetBalance(_ context.Context, _ string) (float64, error) {
	return 1000.00, nil
}

func (m *mockAccountService) UpdateBalance(_ context.Context, _ string, _ float64) error {
	return nil
}

func (m *mockTransactionService) CreateTransaction(_ context.Context, req CreateTransactionRequest) (*Transaction, error) {
	return &Transaction{
		ID:            generateID(),
		FromAccountID: req.FromAccountID,
		ToAccountID:   req.ToAccountID,
		Amount:        req.Amount,
		Currency:      req.Currency,
		Type:          "transfer",
		Status:        "completed",
		Description:   req.Description,
		Reference:     req.Reference,
		ProcessedAt:   time.Now(),
		ComplianceData: map[string]any{
			"aml_checked":  true,
			"kyc_verified": true,
		},
	}, nil
}

func (m *mockTransactionService) GetTransaction(_ context.Context, id string) (*Transaction, error) {
	return &Transaction{
		ID:            id,
		FromAccountID: "acc_123",
		ToAccountID:   "acc_456",
		Amount:        100.00,
		Currency:      "USD",
		Type:          "transfer",
		Status:        "completed",
		Description:   "Test transaction",
		ProcessedAt:   time.Now(),
	}, nil
}

func (m *mockTransactionService) GetAccountTransactions(_ context.Context, accountID string) ([]Transaction, error) {
	return []Transaction{
		{
			ID:            "txn_1",
			FromAccountID: accountID,
			ToAccountID:   "acc_456",
			Amount:        50.00,
			Currency:      "USD",
			Type:          "transfer",
			Status:        "completed",
			Description:   "Payment to vendor",
			ProcessedAt:   time.Now().Add(-2 * time.Hour),
		},
		{
			ID:            "txn_2",
			FromAccountID: "acc_789",
			ToAccountID:   accountID,
			Amount:        200.00,
			Currency:      "USD",
			Type:          "deposit",
			Status:        "completed",
			Description:   "Salary deposit",
			ProcessedAt:   time.Now().Add(-24 * time.Hour),
		},
	}, nil
}

func (m *mockPaymentService) ProcessPayment(_ context.Context, req ProcessPaymentRequest) (*Payment, error) {
	return &Payment{
		ID:              generateID(),
		PayerAccountID:  req.PayerAccountID,
		PayeeAccountID:  req.PayeeAccountID,
		Amount:          req.Amount,
		Currency:        req.Currency,
		PaymentMethod:   req.PaymentMethod,
		Status:          "completed",
		ProcessedAt:     time.Now(),
		FraudScore:      0.1, // Low fraud risk
		ComplianceFlags: []string{},
	}, nil
}

func (m *mockPaymentService) GetPayment(_ context.Context, id string) (*Payment, error) {
	return &Payment{
		ID:             id,
		PayerAccountID: "acc_123",
		PayeeAccountID: "acc_456",
		Amount:         100.00,
		Currency:       "USD",
		PaymentMethod:  "card",
		Status:         "completed",
		ProcessedAt:    time.Now(),
		FraudScore:     0.1,
	}, nil
}

func (m *mockPaymentService) RefundPayment(_ context.Context, req RefundPaymentRequest) (*Payment, error) {
	return &Payment{
		ID:             generateID(),
		PayerAccountID: "system",
		PayeeAccountID: "acc_123",
		Amount:         req.Amount,
		Currency:       "USD",
		PaymentMethod:  "refund",
		Status:         "completed",
		ProcessedAt:    time.Now(),
		FraudScore:     0.0,
	}, nil
}

func (m *mockComplianceService) ValidateTransaction(_ context.Context, transaction *Transaction) error {
	// Simulate compliance validation
	if transaction.Amount > 10000 {
		return fmt.Errorf("transaction amount exceeds daily limit")
	}
	return nil
}

func (m *mockComplianceService) AuditPayment(_ context.Context, payment *Payment) error {
	// Simulate audit logging
	log.Printf("AUDIT: Payment %s processed for amount %f %s", payment.ID, payment.Amount, payment.Currency)
	return nil
}

func (m *mockComplianceService) GenerateReport(_ context.Context, reportType string) (any, error) {
	return map[string]any{
		"reportType":  reportType,
		"generatedAt": time.Now(),
		"data": map[string]any{
			"totalTransactions":   1000,
			"totalAmount":         50000.00,
			"flaggedTransactions": 5,
		},
	}, nil
}

func (m *mockFraudDetectionService) AnalyzePayment(_ context.Context, payment *Payment) (float64, error) {
	// Simple fraud scoring logic
	score := 0.0

	if payment.Amount > 5000 {
		score += 0.3
	}
	if payment.PaymentMethod == "card" {
		score += 0.1
	}

	return score, nil
}

func (m *mockFraudDetectionService) CheckRisk(_ context.Context, _ string, amount float64) (bool, error) {
	// Simple risk check
	return amount > 10000, nil
}

// Utility functions
func generateID() string {
	return fmt.Sprintf("id_%d", time.Now().UnixNano())
}

func generateAccountNumber() string {
	return fmt.Sprintf("ACC-%d", time.Now().UnixNano()%1000000)
}

// Handler functions
func createAccount(ctx *lift.Context) error {
	var req CreateAccountRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid request", 400)
	}

	// Get services from context (would be injected via DI)
	accountService := &mockAccountService{}

	// Create account
	account, err := accountService.CreateAccount(ctx.Request.Context(), req)
	if err != nil {
		return ctx.SystemError("Failed to create account", err)
	}

	// Compliance logging
	log.Printf("COMPLIANCE: Account created - ID: %s, Customer: %s, Type: %s",
		account.ID, account.CustomerID, account.AccountType)

	return ctx.Created(account)
}

func getAccount(ctx *lift.Context) error {
	accountID := ctx.PathParam("id")
	if accountID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Account ID is required", 400)
	}

	accountService := &mockAccountService{}

	account, err := accountService.GetAccount(ctx.Request.Context(), accountID)
	if err != nil {
		return ctx.NotFound("Account not found", err)
	}

	return ctx.OK(account)
}

func getBalance(ctx *lift.Context) error {
	accountID := ctx.PathParam("id")
	if accountID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Account ID is required", 400)
	}

	accountService := &mockAccountService{}

	balance, err := accountService.GetBalance(ctx.Request.Context(), accountID)
	if err != nil {
		return ctx.SystemError("Failed to get balance", err)
	}

	return ctx.OK(map[string]any{
		"accountId": accountID,
		"balance":   balance,
		"currency":  "USD",
		"timestamp": time.Now(),
	})
}

func createTransaction(ctx *lift.Context) error {
	accountID := ctx.PathParam("id")
	if accountID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Account ID is required", 400)
	}

	var req CreateTransactionRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid request", 400)
	}

	// Validate account ID matches request
	if req.FromAccountID != accountID {
		return lift.NewLiftError("BAD_REQUEST", "Account ID mismatch", 400)
	}

	transactionService := &mockTransactionService{}
	complianceService := &mockComplianceService{}

	// Create transaction
	transaction, err := transactionService.CreateTransaction(ctx.Request.Context(), req)
	if err != nil {
		return ctx.SystemError("Failed to create transaction", err)
	}

	// Compliance validation
	if err := complianceService.ValidateTransaction(ctx.Request.Context(), transaction); err != nil {
		return ctx.Forbidden("Transaction failed compliance check", err)
	}

	// Compliance audit
	log.Printf("AUDIT: Transaction created - ID: %s, Amount: %f %s, From: %s, To: %s",
		transaction.ID, transaction.Amount, transaction.Currency,
		transaction.FromAccountID, transaction.ToAccountID)

	return ctx.Created(transaction)
}

func processPayment(ctx *lift.Context) error {
	var req ProcessPaymentRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid request", 400)
	}

	paymentService := &mockPaymentService{}
	fraudService := &mockFraudDetectionService{}
	complianceService := &mockComplianceService{}

	// Fraud detection
	isHighRisk, err := fraudService.CheckRisk(ctx.Request.Context(), req.PayerAccountID, req.Amount)
	if err != nil {
		return ctx.SystemError("Fraud check failed", err)
	}

	if isHighRisk {
		return ctx.Forbidden("Payment blocked due to high fraud risk", nil)
	}

	// Process payment
	payment, err := paymentService.ProcessPayment(ctx.Request.Context(), req)
	if err != nil {
		return ctx.SystemError("Payment processing failed", err)
	}

	// Analyze fraud score
	fraudScore, err := fraudService.AnalyzePayment(ctx.Request.Context(), payment)
	if err != nil {
		log.Printf("WARNING: Fraud analysis failed for payment %s: %v", payment.ID, err)
	} else {
		payment.FraudScore = fraudScore
	}

	// Compliance audit
	if err := complianceService.AuditPayment(ctx.Request.Context(), payment); err != nil {
		log.Printf("WARNING: Compliance audit failed for payment %s: %v", payment.ID, err)
	}

	return ctx.Created(payment)
}

func getPayment(ctx *lift.Context) error {
	paymentID := ctx.PathParam("id")
	if paymentID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Payment ID is required", 400)
	}

	paymentService := &mockPaymentService{}

	payment, err := paymentService.GetPayment(ctx.Request.Context(), paymentID)
	if err != nil {
		return ctx.NotFound("Payment not found", err)
	}

	return ctx.OK(payment)
}

func refundPayment(ctx *lift.Context) error {
	paymentID := ctx.PathParam("id")
	if paymentID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Payment ID is required", 400)
	}

	var req RefundPaymentRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return lift.NewLiftError("BAD_REQUEST", "Invalid request", 400)
	}

	// Validate payment ID matches request
	if req.PaymentID != paymentID {
		return lift.NewLiftError("BAD_REQUEST", "Payment ID mismatch", 400)
	}

	paymentService := &mockPaymentService{}

	// Process refund
	refund, err := paymentService.RefundPayment(ctx.Request.Context(), req)
	if err != nil {
		return ctx.SystemError("Refund processing failed", err)
	}

	// Compliance audit
	log.Printf("AUDIT: Refund processed - ID: %s, Original Payment: %s, Amount: %f, Reason: %s",
		refund.ID, paymentID, refund.Amount, req.Reason)

	return ctx.Created(refund)
}

func getAuditTrail(ctx *lift.Context) error {
	// Query parameters for filtering
	startDate := ctx.QueryParam("start_date")
	endDate := ctx.QueryParam("end_date")
	accountID := ctx.QueryParam("account_id")

	// Mock audit trail data
	auditTrail := map[string]any{
		"filters": map[string]string{
			"start_date": startDate,
			"end_date":   endDate,
			"account_id": accountID,
		},
		"events": []map[string]any{
			{
				"timestamp":  time.Now().Add(-2 * time.Hour),
				"event_type": "account_created",
				"account_id": "acc_123",
				"user_id":    "user_456",
				"details":    "New checking account created",
			},
			{
				"timestamp":      time.Now().Add(-1 * time.Hour),
				"event_type":     "transaction_processed",
				"account_id":     "acc_123",
				"transaction_id": "txn_789",
				"amount":         100.00,
				"details":        "Transfer to acc_456",
			},
		},
		"total_events": 2,
		"generated_at": time.Now(),
	}

	return ctx.OK(auditTrail)
}

func generateComplianceReport(ctx *lift.Context) error {
	reportType := ctx.PathParam("type")
	if reportType == "" {
		return lift.NewLiftError("BAD_REQUEST", "Report type is required", 400)
	}

	complianceService := &mockComplianceService{}

	report, err := complianceService.GenerateReport(ctx.Request.Context(), reportType)
	if err != nil {
		return ctx.SystemError("Failed to generate report", err)
	}

	return ctx.OK(report)
}

// Health check handler
func healthCheck(ctx *lift.Context) error {
	health := map[string]any{
		"status":    "healthy",
		"timestamp": time.Now(),
		"version":   "1.0.0",
		"services": map[string]string{
			"database":        "healthy",
			"payment_gateway": "healthy",
			"fraud_detection": "healthy",
			"compliance":      "healthy",
		},
		"metrics": map[string]any{
			"uptime_seconds":      3600,
			"total_transactions":  1000,
			"successful_payments": 995,
			"failed_payments":     5,
		},
	}

	return ctx.OK(health)
}

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
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			origin := ctx.Header("Origin")

			if isOriginAllowed(origin, config.AllowOrigins) {
				applyCORSHeaders(ctx, config, origin)
			}

			if ctx.Request.Method == "OPTIONS" {
				ctx.Response.StatusCode = 204
				return nil
			}

			return next.Handle(ctx)
		})
	}
}

// helper: check if origin is allowed
func isOriginAllowed(origin string, allow []string) bool {
	for _, allowedOrigin := range allow {
		if allowedOrigin == "*" || allowedOrigin == origin {
			return true
		}
	}
	return false
}

// helper: apply CORS headers
func applyCORSHeaders(ctx *lift.Context, config CORSConfig, origin string) {
	ctx.Response.Header("Access-Control-Allow-Origin", origin)
	if len(config.AllowMethods) > 0 {
		ctx.Response.Header("Access-Control-Allow-Methods", strings.Join(config.AllowMethods, ", "))
	}
	if len(config.AllowHeaders) > 0 {
		ctx.Response.Header("Access-Control-Allow-Headers", strings.Join(config.AllowHeaders, ", "))
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
type Config struct {
	Metrics bool
	Tracing bool
	Logging bool
}

// Enhanced observability middleware
func EnhancedObservability(config Config) lift.Middleware {
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

// Group represents a route group
type Group struct {
	app    *lift.App
	prefix string
}

// GET adds a GET route to the group
func (g *Group) GET(path string, handler func(*lift.Context) error) error {
	return g.app.GET(g.prefix+path, handler)
}

// POST adds a POST route to the group
func (g *Group) POST(path string, handler func(*lift.Context) error) error {
	return g.app.POST(g.prefix+path, handler)
}

// Group creates a new route group
func (g *Group) Group(prefix string) *Group {
	return &Group{
		prefix: g.prefix + prefix,
		app:    g.app,
	}
}

// NewGroup creates a new route group
func NewGroup(app *lift.App, prefix string) *Group {
	return &Group{
		prefix: prefix,
		app:    app,
	}
}

func main() {
	// Create Lift application
	app := lift.New()

	// Enterprise middleware stack
	app.Use(Logger())
	app.Use(Recovery())
	app.Use(CORS(CORSConfig{
		AllowOrigins: []string{"https://banking.example.com"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE"},
		AllowHeaders: []string{"Authorization", "Content-Type"},
	}))

	// Enhanced observability
	app.Use(EnhancedObservability(Config{
		Metrics: true,
		Tracing: true,
		Logging: true,
	}))

	// Security middleware (would include JWT, rate limiting, etc.)
	app.Use(RateLimit(RateLimitConfig{
		Limit: 100,
		Burst: 20,
	}))

	// API versioning
	api := NewGroup(app, "/api/v1")

	// Health check endpoint
	if err := api.GET("/health", healthCheck); err != nil {
		log.Fatalf("Failed to register health endpoint: %v", err)
	}

	// Account management endpoints
	accounts := api.Group("/accounts")
	if err := accounts.POST("", createAccount); err != nil {
		log.Fatalf("Failed to register POST /accounts: %v", err)
	}
	if err := accounts.GET("/:id", getAccount); err != nil {
		log.Fatalf("Failed to register GET /accounts/:id: %v", err)
	}
	if err := accounts.GET("/:id/balance", getBalance); err != nil {
		log.Fatalf("Failed to register GET /accounts/:id/balance: %v", err)
	}
	if err := accounts.POST("/:id/transactions", createTransaction); err != nil {
		log.Fatalf("Failed to register POST /accounts/:id/transactions: %v", err)
	}

	// Payment processing endpoints
	payments := api.Group("/payments")
	if err := payments.POST("", processPayment); err != nil {
		log.Fatalf("Failed to register POST /payments: %v", err)
	}
	if err := payments.GET("/:id", getPayment); err != nil {
		log.Fatalf("Failed to register GET /payments/:id: %v", err)
	}
	if err := payments.POST("/:id/refund", refundPayment); err != nil {
		log.Fatalf("Failed to register POST /payments/:id/refund: %v", err)
	}

	// Compliance endpoints
	compliance := api.Group("/compliance")
	if err := compliance.GET("/audit-trail", getAuditTrail); err != nil {
		log.Fatalf("Failed to register GET /compliance/audit-trail: %v", err)
	}
	if err := compliance.GET("/reports/:type", generateComplianceReport); err != nil {
		log.Fatalf("Failed to register GET /compliance/reports/:type: %v", err)
	}

	// Start the application
	log.Println("Starting Enterprise Banking API on port 8080...")
	log.Println("Available endpoints:")
	log.Println("  GET  /api/v1/health")
	log.Println("  POST /api/v1/accounts")
	log.Println("  GET  /api/v1/accounts/:id")
	log.Println("  GET  /api/v1/accounts/:id/balance")
	log.Println("  POST /api/v1/accounts/:id/transactions")
	log.Println("  POST /api/v1/payments")
	log.Println("  GET  /api/v1/payments/:id")
	log.Println("  POST /api/v1/payments/:id/refund")
	log.Println("  GET  /api/v1/compliance/audit-trail")
	log.Println("  GET  /api/v1/compliance/reports/:type")

	if err := app.Start(); err != nil {
		log.Fatalf("Failed to start application: %v", err)
	}
}
