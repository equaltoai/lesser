package main

import (
	"context"
	"log"

	"github.com/aron23/lesser/internal/testutil/mocks"
	"github.com/aron23/lesser/pkg/storage"
)

func main() {
	// Test that MockStorage satisfies the Storage interface
	var mockStore storage.Storage = &mocks.MockStorage{}

	// Use the storage interface to force compilation
	ctx := context.Background()
	_, err := mockStore.GetActor(ctx, "test")
	if err != nil {
		log.Printf("Error (expected in test): %v", err)
	}

	// If we get here, MockStorage implements all Storage interface methods
	log.Println("MockStorage compilation test successful!")
}
