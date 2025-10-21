package main

import (
	"log"

	"github.com/pay-theory/lift/pkg/lift"
)

func main() {
	// Create Lift application
	app := lift.New()

	// Register routes with error handling
	if err := app.GET("/health", func(ctx *lift.Context) error {
		return ctx.JSON(map[string]string{"status": "healthy"})
	}); err != nil {
		log.Fatalf("Failed to register GET /health: %v", err)
	}

	// Register a route with an invalid handler to demonstrate error handling
	if err := app.POST("/invalid", "this is not a valid handler"); err != nil {
		log.Printf("Expected error: %v", err)
	}

	// Register event handlers with error handling
	if err := app.SQS("my-queue", func(_ *lift.Context) error {
		log.Println("Processing SQS message")
		return nil
	}); err != nil {
		log.Fatalf("Failed to register SQS handler: %v", err)
	}

	// Register EventBridge handler
	if err := app.EventBridge("custom.event", func(_ *lift.Context) error {
		log.Println("Processing EventBridge event")
		return nil
	}); err != nil {
		log.Fatalf("Failed to register EventBridge handler: %v", err)
	}

	// Start the application
	if err := app.Start(); err != nil {
		log.Fatalf("Failed to start application: %v", err)
	}

	log.Println("Application started successfully")
}
