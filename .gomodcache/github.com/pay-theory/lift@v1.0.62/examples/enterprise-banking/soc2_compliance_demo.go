package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/testing/enterprise"
)

// SOC2ComplianceDemo demonstrates SOC 2 Type II compliance validation
func SOC2ComplianceDemo() {
	fmt.Println("🏦 Enterprise Banking - SOC 2 Type II Compliance Demo")
	fmt.Println(strings.Repeat("=", 60))

	// Create banking application
	app := createComplianceTestApp()

	// Create SOC 2 Type II compliance framework
	auditPeriod := 365 * 24 * time.Hour // 1 year audit period
	compliance := enterprise.NewSOC2TypeIICompliance(auditPeriod)

	fmt.Printf("📋 Starting SOC 2 Type II compliance validation...\n")
	fmt.Printf("   Audit Period: %v\n", auditPeriod)
	fmt.Printf("   Framework: SOC 2 Type II\n\n")

	// Run compliance validation
	ctx := context.Background()
	startTime := time.Now()

	report, err := compliance.ValidateCompliance(ctx, app)
	if err != nil {
		log.Fatalf("❌ Compliance validation failed: %v", err)
	}

	duration := time.Since(startTime)

	// Display results
	displaySOC2ComplianceReport(report, duration)

	// Generate detailed compliance summary
	generateSOC2ComplianceSummary(report)

	// Demonstrate continuous monitoring
	demonstrateSOC2ContinuousMonitoring(compliance, app)

	fmt.Println("\n✅ SOC 2 Type II compliance validation completed successfully!")
}

// createComplianceTestApp creates a sample banking application for compliance testing
func createComplianceTestApp() *lift.App {
	app := lift.New()

	// Banking API routes for compliance testing
	api := app.Group("/api/v1")

	// Account management endpoints
	accounts := api.Group("/accounts")
	if err := accounts.POST("", complianceCreateAccount); err != nil {
		log.Fatalf("Failed to register POST /accounts: %v", err)
	}
	if err := accounts.GET("/:id", complianceGetAccount); err != nil {
		log.Fatalf("Failed to register GET /accounts/:id: %v", err)
	}
	if err := accounts.GET("/:id/balance", complianceGetBalance); err != nil {
		log.Fatalf("Failed to register GET /accounts/:id/balance: %v", err)
	}

	// Payment processing endpoints
	payments := api.Group("/payments")
	if err := payments.POST("", complianceProcessPayment); err != nil {
		log.Fatalf("Failed to register POST /payments: %v", err)
	}
	if err := payments.GET("/:id", complianceGetPayment); err != nil {
		log.Fatalf("Failed to register GET /payments/:id: %v", err)
	}

	// Compliance endpoints
	compliance := api.Group("/compliance")
	if err := compliance.GET("/audit-trail", complianceGetAuditTrail); err != nil {
		log.Fatalf("Failed to register GET /compliance/audit-trail: %v", err)
	}
	if err := compliance.GET("/reports/:type", complianceGenerateReport); err != nil {
		log.Fatalf("Failed to register GET /compliance/reports/:type: %v", err)
	}

	return app
}

// Sample handlers for banking operations with compliance features
func complianceCreateAccount(ctx *lift.Context) error {
	// Simulate account creation with compliance logging
	account := map[string]any{
		"id":           "acc_" + fmt.Sprintf("%d", time.Now().Unix()),
		"customer_id":  "cust_123",
		"account_type": "checking",
		"balance":      0.0,
		"currency":     "USD",
		"status":       "active",
		"created_at":   time.Now(),
		"compliance": map[string]any{
			"kyc_verified":    true,
			"aml_checked":     true,
			"risk_assessment": "low",
			"audit_logged":    true,
		},
	}

	return ctx.JSON(account)
}

func complianceGetAccount(ctx *lift.Context) error {
	accountID := ctx.Param("id")

	// Simulate account retrieval with access logging
	account := map[string]any{
		"id":           accountID,
		"customer_id":  "cust_123",
		"account_type": "checking",
		"balance":      1500.00,
		"currency":     "USD",
		"status":       "active",
		"access_log": map[string]any{
			"accessed_at": time.Now(),
			"accessed_by": "user_456",
			"ip_address":  "192.168.1.100",
			"user_agent":  "BankingApp/1.0",
		},
	}

	return ctx.JSON(account)
}

func complianceGetBalance(ctx *lift.Context) error {
	accountID := ctx.Param("id")

	// Simulate balance inquiry with audit trail
	balance := map[string]any{
		"account_id": accountID,
		"balance":    1500.00,
		"currency":   "USD",
		"as_of":      time.Now(),
		"audit_trail": map[string]any{
			"operation":  "balance_inquiry",
			"timestamp":  time.Now(),
			"user_id":    "user_456",
			"session_id": "sess_789",
			"compliance": "logged",
		},
	}

	return ctx.JSON(balance)
}

func complianceProcessPayment(ctx *lift.Context) error {
	// Simulate payment processing with comprehensive compliance
	payment := map[string]any{
		"id":               "pay_" + fmt.Sprintf("%d", time.Now().Unix()),
		"payer_account_id": "acc_123",
		"payee_account_id": "acc_456",
		"amount":           100.00,
		"currency":         "USD",
		"status":           "completed",
		"processed_at":     time.Now(),
		"compliance": map[string]any{
			"fraud_score":       0.1,
			"aml_checked":       true,
			"sanctions_checked": true,
			"risk_assessment":   "low",
			"audit_logged":      true,
			"encryption_used":   true,
			"pci_compliant":     true,
		},
	}

	return ctx.JSON(payment)
}

func complianceGetPayment(ctx *lift.Context) error {
	paymentID := ctx.Param("id")

	payment := map[string]any{
		"id":               paymentID,
		"payer_account_id": "acc_123",
		"payee_account_id": "acc_456",
		"amount":           100.00,
		"currency":         "USD",
		"status":           "completed",
		"processed_at":     time.Now().Add(-1 * time.Hour),
	}

	return ctx.JSON(payment)
}

func complianceGetAuditTrail(ctx *lift.Context) error {
	// Simulate audit trail retrieval
	auditTrail := []map[string]any{
		{
			"id":          "audit_001",
			"timestamp":   time.Now().Add(-2 * time.Hour),
			"operation":   "account_creation",
			"user_id":     "user_123",
			"resource_id": "acc_789",
			"result":      "success",
		},
		{
			"id":          "audit_002",
			"timestamp":   time.Now().Add(-1 * time.Hour),
			"operation":   "payment_processing",
			"user_id":     "user_456",
			"resource_id": "pay_101",
			"result":      "success",
		},
	}

	return ctx.JSON(map[string]any{
		"audit_trail": auditTrail,
		"total_count": len(auditTrail),
		"compliance":  "SOC2_compliant",
	})
}

func complianceGenerateReport(ctx *lift.Context) error {
	reportType := ctx.Param("type")

	report := map[string]any{
		"report_type":       reportType,
		"generated_at":      time.Now(),
		"period":            "last_30_days",
		"compliance_status": "compliant",
		"controls_tested":   15,
		"controls_passed":   15,
		"controls_failed":   0,
	}

	return ctx.JSON(report)
}

// displaySOC2ComplianceReport displays the compliance validation results
func displaySOC2ComplianceReport(report *enterprise.ComplianceReport, duration time.Duration) {
	fmt.Printf("📊 Compliance Validation Results\n")
	fmt.Printf("   Framework: %s\n", report.Framework)
	fmt.Printf("   Duration: %v\n", duration)
	fmt.Printf("   Audit Period: %v\n", report.AuditPeriod)
	fmt.Printf("   Overall Status: %s\n", report.OverallStatus)
	fmt.Printf("   Controls Tested: %d\n\n", len(report.Controls))

	// Display control results
	fmt.Printf("🔍 Control Test Results:\n")
	for controlID, result := range report.Controls {
		statusIcon := "✅"
		if result.Status != "passing" {
			statusIcon = "❌"
		}

		category := "unknown"
		if cat, ok := result.Category.(string); ok {
			category = cat
		}
		fmt.Printf("   %s %s (%s)\n", statusIcon, controlID, category)
		fmt.Printf("      Status: %s\n", result.Status)
		fmt.Printf("      Duration: %v\n", result.Duration)
		fmt.Printf("      Tests: %d\n", len(result.TestResults))
		fmt.Printf("      Evidence: %d items\n\n", len(result.Evidence))
	}
}

// generateSOC2ComplianceSummary generates a detailed compliance summary
func generateSOC2ComplianceSummary(report *enterprise.ComplianceReport) {
	fmt.Printf("📈 Compliance Summary\n")

	// Calculate statistics
	totalControls := len(report.Controls)
	passingControls := 0
	failingControls := 0
	totalTests := 0
	passingTests := 0

	categoryStats := make(map[string]int)

	for _, result := range report.Controls {
		if result.Status == "passing" {
			passingControls++
		} else {
			failingControls++
		}

		if cat, ok := result.Category.(string); ok {
			categoryStats[cat]++
		}
		totalTests += len(result.TestResults)

		for _, testResult := range result.TestResults {
			if testResult.Status == "passed" {
				passingTests++
			}
		}
	}

	complianceScore := float64(passingControls) / float64(totalControls) * 100

	fmt.Printf("   Total Controls: %d\n", totalControls)
	fmt.Printf("   Passing Controls: %d\n", passingControls)
	fmt.Printf("   Failing Controls: %d\n", failingControls)
	fmt.Printf("   Compliance Score: %.1f%%\n", complianceScore)
	fmt.Printf("   Total Tests: %d\n", totalTests)
	fmt.Printf("   Passing Tests: %d\n\n", passingTests)

	fmt.Printf("📋 Controls by Category:\n")
	for category, count := range categoryStats {
		fmt.Printf("   %s: %d controls\n", category, count)
	}
	fmt.Println()

	// Generate JSON report for external systems
	jsonReport, err := json.MarshalIndent(report, "", "  ")
	if err == nil {
		fmt.Printf("💾 JSON Report Generated (%d bytes)\n", len(jsonReport))
		fmt.Printf("   Available for export to compliance management systems\n\n")
	}
}

// demonstrateSOC2ContinuousMonitoring shows continuous compliance monitoring
func demonstrateSOC2ContinuousMonitoring(_ *enterprise.SOC2TypeIICompliance, _ *lift.App) {
	fmt.Printf("🔄 Continuous Compliance Monitoring\n")
	fmt.Printf("   Monitoring Framework: SOC 2 Type II\n")
	fmt.Printf("   Monitoring Frequency: Real-time\n")
	fmt.Printf("   Alert Thresholds: Configured\n")
	fmt.Printf("   Evidence Collection: Automated\n\n")

	// Simulate monitoring metrics
	metrics := map[string]any{
		"active_monitors":    5,
		"compliance_score":   99.2,
		"last_assessment":    time.Now().Add(-1 * time.Hour),
		"next_assessment":    time.Now().Add(23 * time.Hour),
		"alerts_generated":   0,
		"evidence_collected": 150,
		"retention_policy":   "7_years",
	}

	fmt.Printf("📊 Current Monitoring Status:\n")
	for key, value := range metrics {
		fmt.Printf("   %s: %v\n", key, value)
	}
	fmt.Println()

	// Demonstrate alert configuration
	fmt.Printf("🚨 Alert Configuration:\n")
	alerts := []map[string]any{
		{
			"type":      "compliance_violation",
			"threshold": "any_failure",
			"channels":  []string{"email", "slack", "webhook"},
			"enabled":   true,
		},
		{
			"type":      "evidence_retention",
			"threshold": "30_days_before_expiry",
			"channels":  []string{"email"},
			"enabled":   true,
		},
		{
			"type":      "control_failure",
			"threshold": "2_consecutive_failures",
			"channels":  []string{"email", "pagerduty"},
			"enabled":   true,
		},
	}

	for _, alert := range alerts {
		status := "✅ Enabled"
		if enabled, ok := alert["enabled"].(bool); ok && !enabled {
			status = "❌ Disabled"
		}
		fmt.Printf("   %s %s (threshold: %s)\n", status, alert["type"], alert["threshold"])
	}
	fmt.Println()

	// Demonstrate compliance metrics over time
	fmt.Printf("📈 Compliance Trends (Last 30 Days):\n")
	trends := []map[string]any{
		{"date": "2025-06-01", "score": 98.5, "controls_passed": 14, "controls_failed": 1},
		{"date": "2025-06-08", "score": 99.1, "controls_passed": 15, "controls_failed": 0},
		{"date": "2025-06-15", "score": 99.3, "controls_passed": 15, "controls_failed": 0},
		{"date": "2025-06-22", "score": 99.2, "controls_passed": 15, "controls_failed": 0},
	}

	for _, trend := range trends {
		fmt.Printf("   %s: %.1f%% (passed: %v, failed: %v)\n",
			trend["date"], trend["score"], trend["controls_passed"], trend["controls_failed"])
	}
	fmt.Println()

	// Demonstrate evidence retention status
	fmt.Printf("📁 Evidence Retention Status:\n")
	retentionStatus := map[string]any{
		"total_evidence_items":   1250,
		"items_expiring_30_days": 15,
		"items_expiring_90_days": 45,
		"retention_compliance":   "100%",
		"storage_encrypted":      true,
		"backup_verified":        true,
	}

	for key, value := range retentionStatus {
		fmt.Printf("   %s: %v\n", key, value)
	}
	fmt.Println()
}
