package main

import (
	"log"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/pay-theory/lift/pkg/lift"
)

func main() {
	app := lift.New()

	// Add simple logging middleware
	app.Use(func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			log.Printf("Scheduled event received at %s", time.Now().Format(time.RFC3339))
			return next.Handle(ctx)
		})
	})

	// Handle different scheduled events based on the rule name
	// EventBridge sends the rule name in the event details
	if err := app.EventBridge("scheduled-*", func(ctx *lift.Context) error {
		// Get the event details to determine which schedule triggered
		eventMap, ok := ctx.Request.RawEvent.(map[string]interface{})
		if !ok {
			return lift.NewLiftError("INVALID_EVENT", "Failed to parse event", 400)
		}

		// Extract the rule name from resources
		resources, ok := eventMap["resources"].([]interface{})
		if !ok || len(resources) == 0 {
			return lift.NewLiftError("NO_RESOURCES", "No resources in event", 400)
		}

		// The ARN contains the rule name
		arn, ok := resources[0].(string)
		if !ok {
			return lift.NewLiftError("INVALID_ARN", "Resource ARN is not a string", 400)
		}
		log.Printf("Triggered by rule: %s", arn)

		// Route based on the schedule
		switch {
		case contains(arn, "hourly-cleanup"):
			return handleHourlyCleanup(ctx)
		case contains(arn, "daily-report"):
			return handleDailyReport(ctx)
		case contains(arn, "weekly-backup"):
			return handleWeeklyBackup(ctx)
		case contains(arn, "monthly-audit"):
			return handleMonthlyAudit(ctx)
		default:
			log.Printf("Unknown scheduled event: %s", arn)
			return ctx.JSON(map[string]string{
				"status": "unknown_schedule",
				"arn":    arn,
			})
		}
	}); err != nil {
		log.Fatalf("Failed to register EventBridge handler: %v", err)
	}

	// Start Lambda handler
	lambda.Start(app.HandleRequest)
}

func handleHourlyCleanup(ctx *lift.Context) error {
	log.Println("Running hourly cleanup task")

	// Perform cleanup tasks
	// - Remove expired sessions
	// - Clean temporary files
	// - Update metrics

	return ctx.JSON(map[string]interface{}{
		"task":      "hourly_cleanup",
		"status":    "completed",
		"timestamp": time.Now().UTC(),
	})
}

func handleDailyReport(ctx *lift.Context) error {
	log.Println("Running daily report generation")

	// Generate daily reports
	// - Aggregate daily metrics
	// - Send summary emails
	// - Update dashboards

	return ctx.JSON(map[string]interface{}{
		"task":      "daily_report",
		"status":    "completed",
		"timestamp": time.Now().UTC(),
	})
}

func handleWeeklyBackup(ctx *lift.Context) error {
	log.Println("Running weekly backup")

	// Perform backup operations
	// - Backup databases
	// - Archive logs
	// - Verify backup integrity

	return ctx.JSON(map[string]interface{}{
		"task":      "weekly_backup",
		"status":    "completed",
		"timestamp": time.Now().UTC(),
	})
}

func handleMonthlyAudit(ctx *lift.Context) error {
	log.Println("Running monthly audit")

	// Perform audit tasks
	// - Security audit
	// - Compliance checks
	// - Resource utilization review

	return ctx.JSON(map[string]interface{}{
		"task":      "monthly_audit",
		"status":    "completed",
		"timestamp": time.Now().UTC(),
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr ||
		len(s) >= len(substr) && s[:len(substr)] == substr ||
		len(s) > len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
