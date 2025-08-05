package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"go.uber.org/zap"
)

func main() {
	var (
		setRules       = flag.String("set-rules", "", "Set instance rules (comma-separated)")
		setDescription = flag.String("set-description", "", "Set extended description (HTML)")
		generateVAPID  = flag.Bool("generate-vapid", false, "Generate new VAPID keys for push notifications")
		showConfig     = flag.Bool("show", false, "Show current configuration")
	)
	flag.Parse()

	// Initialize logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Initialize DynamORM
	db, err := dynamorm.GetClient(context.Background())
	if err != nil {
		log.Fatalf("Failed to initialize DynamORM: %v", err)
	}

	// Get table name from environment
	tableName := os.Getenv("DYNAMODB_TABLE")
	if tableName == "" {
		tableName = "lesser-main"
	}

	// Create repository factory
	repos, err := factory.NewRepositoryFactory(db, tableName, logger)
	if err != nil {
		log.Fatalf("Failed to create repository factory: %v", err)
	}

	ctx := context.Background()

	// Show current configuration
	if *showConfig {
		rules, err := repos.Instance().GetInstanceRules(ctx)
		if err != nil {
			log.Printf("Failed to get rules: %v", err)
		} else {
			fmt.Println("Current Rules:")
			if len(rules) == 0 {
				fmt.Println("  (no rules set)")
			}
			for _, rule := range rules {
				fmt.Printf("  %s. %s\n", rule.ID, rule.Text)
			}
		}

		desc, updatedAt, err := repos.Instance().GetExtendedDescription(ctx)
		if err != nil {
			log.Printf("Failed to get extended description: %v", err)
		} else {
			fmt.Printf("\nExtended Description (updated %s):\n%s\n", updatedAt.Format("2006-01-02"), desc)
		}

		// Show VAPID public key if it exists
		vapidKeys, err := repos.PushSubscription().GetVAPIDKeys(ctx)
		if err != nil {
			fmt.Println("\nVAPID Keys: Not configured")
		} else {
			fmt.Printf("\nVAPID Public Key: %s\n", vapidKeys.PublicKey)
			fmt.Printf("VAPID Subject: %s\n", vapidKeys.Subject)
			fmt.Printf("Created: %s\n", vapidKeys.CreatedAt.Format("2006-01-02 15:04:05"))
		}
		return
	}

	// Generate VAPID keys
	if *generateVAPID {
		fmt.Println("Generating VAPID keys for web push notifications...")

		// Generate P-256 ECDSA key pair
		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			log.Fatalf("Failed to generate VAPID private key: %v", err)
		}

		// Encode public key to uncompressed format (65 bytes: 0x04 + X + Y)
		publicKeyBytes := elliptic.Marshal(privateKey.Curve, privateKey.X, privateKey.Y)
		publicKeyBase64 := base64.RawURLEncoding.EncodeToString(publicKeyBytes)

		// Encode private key (32 bytes)
		privateKeyBytes := privateKey.D.Bytes()
		// Pad to 32 bytes if necessary
		if len(privateKeyBytes) < 32 {
			padding := make([]byte, 32-len(privateKeyBytes))
			privateKeyBytes = append(padding, privateKeyBytes...)
		}
		privateKeyBase64 := base64.RawURLEncoding.EncodeToString(privateKeyBytes)

		// Get domain from environment or prompt
		domain := os.Getenv("DOMAIN")
		if domain == "" {
			fmt.Print("Enter your instance domain (e.g., example.com): ")
			if _, err := fmt.Scanln(&domain); err != nil {
				log.Fatalf("Failed to read domain input: %v", err)
			}
		}

		// Create VAPID keys object
		vapidKeys := &storage.VAPIDKeys{
			PublicKey:  publicKeyBase64,
			PrivateKey: privateKeyBase64,
			Subject:    fmt.Sprintf("mailto:admin@%s", domain),
		}

		// Save to storage
		if err := repos.PushSubscription().SetVAPIDKeys(ctx, vapidKeys); err != nil {
			log.Fatalf("Failed to save VAPID keys: %v", err)
		}

		fmt.Println("✓ VAPID keys generated successfully!")
		fmt.Printf("Public Key: %s\n", publicKeyBase64)
		fmt.Println("\nNOTE: The private key has been securely stored in DynamoDB.")
		fmt.Println("Use the public key in your Mastodon client configuration.")
	}

	// Set rules
	if *setRules != "" {
		ruleTexts := strings.Split(*setRules, ",")
		rules := make([]storage.InstanceRule, len(ruleTexts))
		for i, text := range ruleTexts {
			rules[i] = storage.InstanceRule{
				ID:   fmt.Sprintf("%d", i+1),
				Text: strings.TrimSpace(text),
			}
		}

		if err := repos.Instance().SetInstanceRules(ctx, rules); err != nil {
			log.Fatalf("Failed to set rules: %v", err)
		}
		fmt.Printf("✓ Set %d rules\n", len(rules))
	}

	// Set extended description
	if *setDescription != "" {
		if err := repos.Instance().SetExtendedDescription(ctx, *setDescription); err != nil {
			log.Fatalf("Failed to set extended description: %v", err)
		}
		fmt.Println("✓ Set extended description")
	}

	// If no action specified, show usage
	if !*showConfig && *setRules == "" && *setDescription == "" && !*generateVAPID {
		fmt.Println("Usage: configure-instance [options]")
		fmt.Println("\nOptions:")
		flag.PrintDefaults()
		fmt.Println("\nExamples:")
		fmt.Println("  # Show current configuration")
		fmt.Println("  configure-instance -show")
		fmt.Println("")
		fmt.Println("  # Generate VAPID keys for push notifications")
		fmt.Println("  configure-instance -generate-vapid")
		fmt.Println("")
		fmt.Println("  # Set instance rules")
		fmt.Println("  configure-instance -set-rules \"Be respectful,No spam,Follow the law\"")
		fmt.Println("")
		fmt.Println("  # Set extended description")
		fmt.Println("  configure-instance -set-description \"<h1>Welcome!</h1><p>This is a personal instance.</p>\"")
		os.Exit(0)
	}
}
