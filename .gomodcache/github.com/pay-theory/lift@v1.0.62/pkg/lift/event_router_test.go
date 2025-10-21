package lift

import (
	"sync"
	"testing"

	"github.com/pay-theory/lift/pkg/lift/adapters"
)

// TestEventRouterThreadSafety tests concurrent access to EventRouter
func TestEventRouterThreadSafety(t *testing.T) {
	router := NewEventRouter()

	// Number of concurrent goroutines
	numGoroutines := 100
	numOperations := 1000

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3) // 3 operations per goroutine

	// Concurrent writes (AddEventRoute)
	for i := 0; i < numGoroutines; i++ {
		go func(_ int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				pattern := "*"
				if j%2 == 0 {
					pattern = "test-queue"
				}
				router.AddEventRoute(TriggerSQS, pattern, EventHandlerFunc(func(_ *Context) error {
					return nil
				}))
			}
		}(i)
	}

	// Concurrent reads (FindEventHandler)
	for i := 0; i < numGoroutines; i++ {
		go func(_ int) {
			defer wg.Done()
			ctx := &Context{
				Request: &Request{
					Request: &adapters.Request{
						TriggerType: TriggerSQS,
						Records: []any{
							map[string]any{
								"eventSourceARN": "arn:aws:sqs:us-east-1:123456789012:test-queue",
							},
						},
					},
				},
			}
			for j := 0; j < numOperations; j++ {
				if _, err := router.FindEventHandler(ctx); err != nil {
					t.Logf("Warning: FindEventHandler failed: %v", err)
				}
			}
		}(i)
	}

	// Concurrent reads (GetRoutes)
	for i := 0; i < numGoroutines; i++ {
		go func(_ int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				routes := router.GetRoutes()
				// Verify the returned map is a copy by trying to modify it
				routes[TriggerSQS] = nil
			}
		}(i)
	}

	// Wait for all operations to complete
	wg.Wait()

	// Verify routes were added
	routes := router.GetRoutes()
	if len(routes[TriggerSQS]) == 0 {
		t.Fatal("No routes were added")
	}

	// Verify GetRoutes returns a copy
	originalLen := len(routes[TriggerSQS])
	routes[TriggerSQS] = nil
	newRoutes := router.GetRoutes()
	if len(newRoutes[TriggerSQS]) != originalLen {
		t.Fatal("GetRoutes did not return a copy")
	}
}

// TestEventRouterMatchingThreadSafety tests pattern matching under concurrent access
func TestEventRouterMatchingThreadSafety(t *testing.T) {
	router := NewEventRouter()

	// Add multiple routes with different patterns
	patterns := []string{"*", "test-*", "*-queue", "specific-queue"}
	for _, pattern := range patterns {
		router.AddEventRoute(TriggerSQS, pattern, EventHandlerFunc(func(_ *Context) error {
			return nil
		}))
	}

	// Add S3 routes
	s3Patterns := []string{"*", "my-bucket/*", "*/uploads/*", "docs/*/reports"}
	for _, pattern := range s3Patterns {
		router.AddEventRoute(TriggerS3, pattern, EventHandlerFunc(func(_ *Context) error {
			return nil
		}))
	}

	var wg sync.WaitGroup
	numGoroutines := 50

	// Concurrent pattern matching for SQS
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(_ int) {
			defer wg.Done()
			ctx := &Context{
				Request: &Request{
					Request: &adapters.Request{
						TriggerType: TriggerSQS,
						Records: []any{
							map[string]any{
								"eventSourceARN": "arn:aws:sqs:us-east-1:123456789012:test-queue",
							},
						},
					},
				},
			}
			for j := 0; j < 100; j++ {
				handler, err := router.FindEventHandler(ctx)
				if err != nil {
					t.Errorf("Failed to find handler: %v", err)
				}
				if handler == nil {
					t.Error("Handler is nil")
				}
			}
		}(i)
	}

	// Concurrent pattern matching for S3
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(_ int) {
			defer wg.Done()
			ctx := &Context{
				Request: &Request{
					Request: &adapters.Request{
						TriggerType: TriggerS3,
						Records: []any{
							map[string]any{
								"s3": map[string]any{
									"bucket": map[string]any{
										"name": "my-bucket",
									},
									"object": map[string]any{
										"key": "uploads/file.txt",
									},
								},
							},
						},
					},
				},
			}
			for j := 0; j < 100; j++ {
				handler, err := router.FindEventHandler(ctx)
				if err != nil {
					t.Errorf("Failed to find handler: %v", err)
				}
				if handler == nil {
					t.Error("Handler is nil")
				}
			}
		}(i)
	}

	wg.Wait()
}
