package dynamorm

import (
	"context"
	"log"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
)

// LambdaInit is a helper function to initialize DynamORM in Lambda functions
// It creates a Lambda-optimized client and pre-registers the provided models
// This should be called in the init() function of Lambda handlers
func LambdaInit(models ...interface{}) (core.DB, error) {
	log.Println("Initializing Lambda-optimized DynamORM client...")
	startTime := time.Now()

	// Get the Lambda-optimized client
	db, err := GetLambdaClient(context.Background())
	if err != nil {
		log.Printf("Failed to initialize DynamORM: %v", err)
		return nil, err
	}

	// Pre-register models to reduce cold start time
	if len(models) > 0 {
		if err := db.PreRegisterModels(models...); err != nil {
			log.Printf("Failed to pre-register models: %v", err)
			return db, err
		}
	}

	log.Printf("DynamORM Lambda initialization completed in %v", time.Since(startTime))
	return db, nil
}

// Example Lambda initialization pattern:
/*
package main

import (
	"context"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aron23/lesser/pkg/storage/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/core"
)

// Global variables for connection reuse across Lambda invocations
var (
	db core.DB
)

// Define your models
type User struct {
	dynamorm.StandardModel
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// Initialize DynamORM in the init function
func init() {
	var err error
	// Initialize with models to pre-register
	db, err = dynamorm.LambdaInit(&User{})
	if err != nil {
		panic(err)
	}
}

// Lambda handler function
func handler(ctx context.Context, event map[string]interface{}) (map[string]interface{}, error) {
	// Use the pre-initialized db
	// ...
	return map[string]interface{}{
		"statusCode": 200,
		"body":       "Success",
	}, nil
}

func main() {
	lambda.Start(handler)
}
*/
