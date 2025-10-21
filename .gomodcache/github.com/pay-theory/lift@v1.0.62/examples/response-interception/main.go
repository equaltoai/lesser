package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/middleware"
)

// Example: Custom middleware that uses response interception
func ResponseLoggingMiddleware() lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Enable response buffering to capture response data
			ctx.EnableResponseBuffering()

			// Record start time
			start := time.Now()

			// Execute the handler
			err := next.Handle(ctx)

			// After handler execution, access the buffered response
			if buffer := ctx.GetResponseBuffer(); buffer != nil {
				// Get all captured data
				body, statusCode, headers, capturedData := buffer.Get()

				// Log the response details
				log.Printf("Response intercepted - Status: %d, Duration: %v",
					statusCode, time.Since(start))

				// Log response body (be careful with sensitive data!)
				if jsonData, marshalErr := json.Marshal(capturedData); marshalErr == nil {
					log.Printf("Response body: %s", string(jsonData))
				}

				// Example: Add custom headers based on response
				if statusCode >= 200 && statusCode < 300 {
					ctx.Response.Header("X-Response-Time", fmt.Sprintf("%dms", time.Since(start).Milliseconds()))
				}

				_ = body    // body contains the raw bytes if needed
				_ = headers // headers contains response headers
			}

			return err
		})
	}
}

// Example: Response caching middleware
func ResponseCachingMiddleware(cache map[string]interface{}) lift.Middleware {
	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Create cache key from request
			cacheKey := fmt.Sprintf("%s:%s", ctx.Request.Method, ctx.Request.Path)

			// Check cache first
			if cached, exists := cache[cacheKey]; exists {
				log.Printf("Cache hit for %s", cacheKey)
				ctx.Response.Header("X-Cache", "HIT")
				return ctx.JSON(cached)
			}

			// Enable response buffering to capture response for caching
			ctx.EnableResponseBuffering()

			// Execute handler
			err := next.Handle(ctx)
			if err != nil {
				return err
			}

			// Cache successful responses
			if buffer := ctx.GetResponseBuffer(); buffer != nil {
				_, statusCode, _, capturedData := buffer.Get()

				// Only cache successful responses
				if statusCode >= 200 && statusCode < 300 && capturedData != nil {
					cache[cacheKey] = capturedData
					log.Printf("Cached response for %s", cacheKey)
				}
			}

			ctx.Response.Header("X-Cache", "MISS")
			return nil
		})
	}
}

// Example handler
type Product struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
}

func main() {
	// Create app
	app := lift.New()

	// Create a simple in-memory cache
	cache := make(map[string]interface{})

	// Apply middleware in order
	app.Use(ResponseLoggingMiddleware())
	app.Use(ResponseCachingMiddleware(cache))

	// Add idempotency middleware
	store := middleware.NewMemoryIdempotencyStore()
	idempotencyMiddleware := middleware.Idempotency(middleware.IdempotencyOptions{
		Store:      store,
		HeaderName: "Idempotency-Key",
		TTL:        24 * time.Hour,
	})
	app.Use(lift.Middleware(idempotencyMiddleware))

	// Product catalog
	products := []Product{
		{ID: "1", Name: "Laptop", Price: 999.99},
		{ID: "2", Name: "Mouse", Price: 29.99},
		{ID: "3", Name: "Keyboard", Price: 79.99},
	}

	// GET /products - List all products (will be cached)
	if err := app.GET("/products", func(ctx *lift.Context) error {
		log.Println("Handler: Fetching all products")
		return ctx.JSON(products)
	}); err != nil {
		log.Fatalf("Failed to register GET /products: %v", err)
	}

	// GET /products/:id - Get single product
	if err := app.GET("/products/:id", func(ctx *lift.Context) error {
		productID := ctx.Param("id")
		log.Printf("Handler: Fetching product %s", productID)

		for _, p := range products {
			if p.ID == productID {
				return ctx.JSON(p)
			}
		}

		return ctx.NotFound("Product not found", nil)
	}); err != nil {
		log.Fatalf("Failed to register GET /products/:id: %v", err)
	}

	// POST /products - Create product (idempotent)
	if err := app.POST("/products", func(ctx *lift.Context) error {
		var newProduct Product
		if err := ctx.ParseRequest(&newProduct); err != nil {
			return err
		}

		// Generate ID
		newProduct.ID = fmt.Sprintf("%d", len(products)+1)
		products = append(products, newProduct)

		log.Printf("Handler: Created product %s", newProduct.ID)
		return ctx.Status(201).JSON(newProduct)
	}); err != nil {
		log.Fatalf("Failed to register POST /products: %v", err)
	}

	// Start the app
	if err := app.Start(); err != nil {
		log.Fatal(err)
	}

	// Simulate some requests
	log.Println("\n=== Starting request simulation ===")

	// Test 1: Make the same GET request twice (should hit cache)
	for i := 0; i < 2; i++ {
		log.Printf("\n--- Request %d: GET /products ---", i+1)
		req := &lift.Request{
			Method:  "GET",
			Path:    "/products",
			Headers: map[string]string{},
		}
		ctx := lift.NewContext(context.Background(), req)

		if err := app.HandleTestRequest(ctx); err != nil {
			log.Printf("Error: %v", err)
		}
	}

	// Test 2: Make idempotent POST requests
	for i := 0; i < 2; i++ {
		log.Printf("\n--- Request %d: POST /products (idempotent) ---", i+1)
		req := &lift.Request{
			Method: "POST",
			Path:   "/products",
			Headers: map[string]string{
				"Idempotency-Key": "create-product-123",
				"Content-Type":    "application/json",
			},
			Body: []byte(`{"name": "Monitor", "price": 299.99}`),
		}
		ctx := lift.NewContext(context.Background(), req)

		if err := app.HandleTestRequest(ctx); err != nil {
			log.Printf("Error: %v", err)
		}

		// Check if it was a replay
		if ctx.Response.Headers["X-Idempotent-Replay"] == "true" {
			log.Println("This was an idempotent replay!")
		}
	}

	// Test 3: Different product request
	log.Printf("\n--- Request: GET /products/2 ---")
	req := &lift.Request{
		Method:  "GET",
		Path:    "/products/2",
		Headers: map[string]string{},
	}
	ctx := lift.NewContext(context.Background(), req)

	if err := app.HandleTestRequest(ctx); err != nil {
		log.Printf("Error: %v", err)
	}
}
