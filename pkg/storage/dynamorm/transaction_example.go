package dynamorm

import (
	"context"
	"fmt"
	"log"
)

// This file contains examples of how to use the transaction support in DynamORM

// ExampleUser is a simple user model for examples
type ExampleUser struct {
	StandardModel
	Name  string `json:"name"`
	Email string `json:"email"`
}

// ExamplePost is a simple post model for examples
type ExamplePost struct {
	StandardModel
	UserID  string `json:"user_id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// ExampleCreateUserWithPosts demonstrates how to use transactions to create a user and their posts atomically
func ExampleCreateUserWithPosts(ctx context.Context) error {
	// Get the DynamoDB client
	client, err := GetClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to get DynamoDB client: %w", err)
	}

	// Create repositories
	userRepo := NewGenericRepository(client, "users", "user")
	postRepo := NewGenericRepository(client, "posts", "post")

	// Create a new transaction
	tx := NewTransaction(client)

	// Execute the transaction
	err = tx.Execute(ctx, func(tx *Transaction) error {
		// Create user with transaction
		user := &ExampleUser{
			StandardModel: StandardModel{
				PK: "user#123",
				SK: "user#123",
			},
			Name:  "John Doe",
			Email: "john@example.com",
		}

		// Use the transaction-aware repository
		txUserRepo := userRepo.WithTransaction(tx)
		if err := txUserRepo.Create(ctx, user); err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}

		// Create posts with transaction
		posts := []*ExamplePost{
			{
				StandardModel: StandardModel{
					PK: "user#123",
					SK: "post#1",
				},
				UserID:  "123",
				Title:   "First Post",
				Content: "Hello, world!",
			},
			{
				StandardModel: StandardModel{
					PK: "user#123",
					SK: "post#2",
				},
				UserID:  "123",
				Title:   "Second Post",
				Content: "Transaction example",
			},
		}

		// Use the transaction-aware repository
		txPostRepo := postRepo.WithTransaction(tx)
		for _, post := range posts {
			if err := txPostRepo.Create(ctx, post); err != nil {
				return fmt.Errorf("failed to create post: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}

	log.Println("User and posts created successfully")
	return nil
}

// ExampleTransferBalance demonstrates how to use transactions for a balance transfer between accounts
func ExampleTransferBalance(ctx context.Context, fromAccountID, toAccountID string, amount float64) error {
	// Get the DynamoDB client
	client, err := GetClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to get DynamoDB client: %w", err)
	}

	// Create account repository
	accountRepo := NewGenericRepository(client, "accounts", "account")

	// Create a new transaction
	tx := NewTransaction(client)

	// Execute the transaction
	err = tx.Execute(ctx, func(tx *Transaction) error {
		// Get the transaction-aware repository
		txAccountRepo := accountRepo.WithTransaction(tx)

		// Get the source account
		var fromAccount struct {
			StandardModel
			Balance float64 `json:"balance"`
		}
		if err := accountRepo.Get(ctx, fromAccountID, &fromAccount); err != nil {
			return fmt.Errorf("failed to get source account: %w", err)
		}

		// Get the destination account
		var toAccount struct {
			StandardModel
			Balance float64 `json:"balance"`
		}
		if err := accountRepo.Get(ctx, toAccountID, &toAccount); err != nil {
			return fmt.Errorf("failed to get destination account: %w", err)
		}

		// Check if source account has sufficient balance
		if fromAccount.Balance < amount {
			return fmt.Errorf("insufficient balance: %.2f < %.2f", fromAccount.Balance, amount)
		}

		// Update balances
		fromAccount.Balance -= amount
		toAccount.Balance += amount

		// Update source account
		if err := txAccountRepo.Update(ctx, &fromAccount); err != nil {
			return fmt.Errorf("failed to update source account: %w", err)
		}

		// Update destination account
		if err := txAccountRepo.Update(ctx, &toAccount); err != nil {
			return fmt.Errorf("failed to update destination account: %w", err)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("transaction failed: %w", err)
	}

	log.Printf("Successfully transferred %.2f from account %s to account %s", amount, fromAccountID, toAccountID)
	return nil
}
