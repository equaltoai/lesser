//go:build example
// +build example

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"go.uber.org/zap"
)

// Example showing how to use the notification cost tracking system
func main() {
	// Initialize configuration
	cfg := config.Get()
	
	// Initialize logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()
	
	// Initialize DynamoDB client
	db, err := dynamorm.NewLambdaOptimizedClient(context.Background(), cfg.Region)
	if err != nil {
		log.Fatalf("Failed to initialize DynamoDB client: %v", err)
	}
	
	// Initialize repositories
	notificationCostRepo := repositories.NewNotificationCostRepository(db, cfg.DynamoTableName, logger)
	
	ctx := context.Background()
	
	// Example 1: Create a notification cost tracking record
	fmt.Println("=== Example 1: Creating Notification Cost Tracking ===")
	
	costTracking := models.NewNotificationCostTrackingBuilder().
		WithNotification("notif_123", "user_456", "alice", "mention").
		WithDelivery("email", "primary", true, 0).
		WithCosts(
			models.CalculateEmailCost(1),      // Email cost
			0,                                 // Push cost
			0,                                 // SMS cost
			0,                                 // WebSocket cost
			models.CalculateLambdaCost(1, 0.1), // Lambda cost (1 invocation, 0.1 GB-seconds)
			models.CalculateDynamoDBCost(1, 1), // DynamoDB cost (1 RCU, 1 WCU)
		).
		WithPerformance(150, 300, 200, 1024).
		WithContext("req_789", "notification-processor", "notification-processor", "stream_123").
		WithProperty("priority", "high").
		WithTag("environment", "production").
		Build()
	
	if err := notificationCostRepo.CreateCostTracking(ctx, costTracking); err != nil {
		log.Printf("Failed to create cost tracking: %v", err)
	} else {
		fmt.Printf("Created cost tracking record: %s\n", costTracking.ID)
		fmt.Printf("Total cost: $%.6f\n", costTracking.TotalCostDollars)
	}
	
	// Example 2: Create multiple cost tracking records for demonstration
	fmt.Println("\n=== Example 2: Creating Multiple Cost Records ===")
	
	users := []string{"alice", "bob", "charlie"}
	channels := []string{"email", "push", "websocket"}
	notificationTypes := []string{"mention", "follow", "favourite", "reblog"}
	
	for i := 0; i < 10; i++ {
		user := users[i%len(users)]
		channel := channels[i%len(channels)]
		notifType := notificationTypes[i%len(notificationTypes)]
		success := i%4 != 0 // 75% success rate
		
		var emailCost, pushCost, websocketCost int64
		switch channel {
		case "email":
			emailCost = models.CalculateEmailCost(1)
		case "push":
			pushCost = models.CalculatePushCost(1)
		case "websocket":
			websocketCost = models.CalculateWebSocketCost(1)
		}
		
		lambdaCost := models.CalculateLambdaCost(1, 0.05)
		dynamodbCost := models.CalculateDynamoDBCost(0.5, 0.5)
		
		record := models.NewNotificationCostTrackingBuilder().
			WithNotification(fmt.Sprintf("notif_%d", i), fmt.Sprintf("user_%s", user), user, notifType).
			WithDelivery(channel, channel, success, 0).
			WithCosts(emailCost, pushCost, 0, websocketCost, lambdaCost, dynamodbCost).
			WithPerformance(100+int64(i*10), 200+int64(i*20), 200, 512).
			WithContext(fmt.Sprintf("req_%d", i), "notification-processor", "notification-processor", fmt.Sprintf("stream_%d", i)).
			WithTimestamp(time.Now().Add(-time.Duration(i)*time.Hour)).
			Build()
		
		if !success {
			record.SetError("Delivery failed: timeout")
		}
		
		if err := notificationCostRepo.CreateCostTracking(ctx, record); err != nil {
			log.Printf("Failed to create cost tracking %d: %v", i, err)
		} else {
			fmt.Printf("Created record %d: %s, user=%s, channel=%s, success=%t, cost=$%.6f\n", 
				i, record.ID, user, channel, success, record.TotalCostDollars)
		}
	}
	
	// Example 3: Query cost tracking by user
	fmt.Println("\n=== Example 3: Querying Costs by User ===")
	
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)
	
	userCosts, err := notificationCostRepo.GetCostTrackingByUser(ctx, "alice", startTime, endTime, 10)
	if err != nil {
		log.Printf("Failed to get user costs: %v", err)
	} else {
		fmt.Printf("Found %d cost records for alice in the last 24 hours:\n", len(userCosts))
		for _, cost := range userCosts {
			fmt.Printf("  - %s: %s via %s, cost=$%.6f, success=%t\n",
				cost.Timestamp.Format("15:04:05"), cost.NotificationType, cost.DeliveryMethod, 
				cost.TotalCostDollars, cost.Success)
		}
	}
	
	// Example 4: Query costs by delivery method
	fmt.Println("\n=== Example 4: Querying Costs by Delivery Method ===")
	
	emailCosts, err := notificationCostRepo.GetCostTrackingByMethod(ctx, "email", startTime, endTime, 10)
	if err != nil {
		log.Printf("Failed to get email costs: %v", err)
	} else {
		fmt.Printf("Found %d email delivery records in the last 24 hours:\n", len(emailCosts))
		totalEmailCost := int64(0)
		successCount := 0
		for _, cost := range emailCosts {
			totalEmailCost += cost.TotalCostMicroCents
			if cost.Success {
				successCount++
			}
		}
		
		if len(emailCosts) > 0 {
			fmt.Printf("  Total email cost: $%.6f\n", float64(totalEmailCost)/1_000_000.0)
			fmt.Printf("  Success rate: %.1f%%\n", float64(successCount)/float64(len(emailCosts))*100)
		}
	}
	
	// Example 5: Get cost summary
	fmt.Println("\n=== Example 5: Cost Summary ===")
	
	summary, err := notificationCostRepo.GetNotificationCostSummary(ctx, startTime, endTime)
	if err != nil {
		log.Printf("Failed to get cost summary: %v", err)
	} else {
		fmt.Printf("Notification Cost Summary for last 24 hours:\n")
		fmt.Printf("  Total notifications: %d\n", summary.TotalNotifications)
		fmt.Printf("  Successful deliveries: %d\n", summary.SuccessfulDeliveries)
		fmt.Printf("  Failed deliveries: %d\n", summary.FailedDeliveries)
		fmt.Printf("  Success rate: %.1f%%\n", summary.SuccessRate)
		fmt.Printf("  Total cost: $%.6f\n", summary.TotalCostDollars)
		fmt.Printf("  Average cost per notification: $%.8f\n", summary.AverageCostPerNotification)
		
		fmt.Println("\n  Breakdown by delivery method:")
		for method, stats := range summary.DeliveryMethodBreakdown {
			fmt.Printf("    %s: %d notifications, $%.6f total, %.1f%% success\n",
				method, stats.Count, stats.TotalCostDollars, stats.SuccessRate)
		}
		
		fmt.Println("\n  Breakdown by notification type:")
		for notifType, stats := range summary.NotificationTypeBreakdown {
			fmt.Printf("    %s: %d notifications, $%.6f total, %.1f%% success\n",
				notifType, stats.Count, stats.TotalCostDollars, stats.SuccessRate)
		}
	}
	
	// Example 6: Create and manage notification budgets
	fmt.Println("\n=== Example 6: Notification Budgets ===")
	
	// Create a daily budget for alice
	aliceBudget := &models.NotificationBudget{
		Username:              "alice",
		Period:                "daily",
		LimitMicroCents:       100000, // $0.10 per day
		SpentMicroCents:       0,
		PeriodStart:           time.Now().Truncate(24 * time.Hour),
		PeriodEnd:             time.Now().Truncate(24 * time.Hour).Add(24 * time.Hour),
		MaxNotificationsPerPeriod: 100,
		Enabled:               true,
		SendWarningAt:         80.0, // Warning at 80%
		BlockDeliveryAt:       95.0, // Block at 95%
		AllowedDeliveryMethods: []string{"email", "push", "websocket"},
	}
	
	if err := notificationCostRepo.CreateBudget(ctx, aliceBudget); err != nil {
		log.Printf("Failed to create budget: %v", err)
	} else {
		fmt.Printf("Created daily budget for alice: $%.2f limit\n", aliceBudget.LimitDollars)
	}
	
	// Get user budgets
	budgets, err := notificationCostRepo.GetUserBudgets(ctx, "alice")
	if err != nil {
		log.Printf("Failed to get user budgets: %v", err)
	} else {
		fmt.Printf("Alice has %d budget(s):\n", len(budgets))
		for _, budget := range budgets {
			fmt.Printf("  %s: $%.2f limit, $%.4f spent, %s\n",
				budget.Period, budget.LimitDollars, budget.SpentDollars,
				map[bool]string{true: "enabled", false: "disabled"}[budget.Enabled])
		}
	}
	
	// Example 7: High cost notifications
	fmt.Println("\n=== Example 7: High Cost Notifications ===")
	
	// Find notifications that cost more than $0.0001 (10 micro-cents)
	highCostNotifications, err := notificationCostRepo.GetHighCostNotifications(ctx, 10, startTime, endTime, 5)
	if err != nil {
		log.Printf("Failed to get high cost notifications: %v", err)
	} else {
		fmt.Printf("Found %d high-cost notifications (>$0.0001):\n", len(highCostNotifications))
		for _, notif := range highCostNotifications {
			fmt.Printf("  %s: %s via %s, cost=$%.6f, user=%s\n",
				notif.Timestamp.Format("15:04:05"), notif.NotificationType, 
				notif.DeliveryMethod, notif.TotalCostDollars, notif.Username)
		}
	}
	
	// Example 8: User spending summary
	fmt.Println("\n=== Example 8: User Spending Summary ===")
	
	spendingSummary, err := notificationCostRepo.GetUserSpending(ctx, "alice", startTime, endTime)
	if err != nil {
		log.Printf("Failed to get user spending: %v", err)
	} else {
		fmt.Printf("Alice's spending summary for last 24 hours:\n")
		fmt.Printf("  Total notifications: %d\n", spendingSummary.TotalNotifications)
		fmt.Printf("  Total cost: $%.6f\n", spendingSummary.TotalCostDollars)
		fmt.Printf("  Average cost per notification: $%.8f\n", spendingSummary.AverageCostPerNotification)
		fmt.Printf("  Success rate: %.1f%%\n", spendingSummary.SuccessRate)
		
		fmt.Println("\n  Spending by delivery method:")
		for method, spending := range spendingSummary.DeliveryMethodBreakdown {
			fmt.Printf("    %s: %d notifications, $%.6f total\n",
				method, spending.Count, spending.TotalCostDollars)
		}
	}
	
	fmt.Println("\n=== Cost Tracking Examples Complete ===")
}