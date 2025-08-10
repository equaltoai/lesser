// Package main implements the configure-instance Lambda function for managing instance configuration.
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

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	"go.uber.org/zap"
)

// configFlags holds command-line configuration flags
type configFlags struct {
	setRules       *string
	setDescription *string
	generateVAPID  *bool
	showConfig     *bool
}

// appContext holds application-wide context and dependencies
type appContext struct {
	ctx    context.Context
	logger *zap.Logger
	repos  *factory.RepositoryFactory
}

func main() {
	flags := parseFlags()

	logger := initializeLogger()
	defer syncLogger(logger)

	repos := initializeRepositories(logger)

	appCtx := &appContext{
		ctx:    context.Background(),
		logger: logger,
		repos:  repos,
	}

	// Handle different operation modes
	if *flags.showConfig {
		showCurrentConfiguration(appCtx)
		return
	}

	if *flags.generateVAPID {
		generateVAPIDKeys(appCtx)
	}

	if *flags.setRules != "" {
		setInstanceRules(appCtx, *flags.setRules)
	}

	if *flags.setDescription != "" {
		setExtendedDescription(appCtx, *flags.setDescription)
	}

	// If no action specified, show usage
	if !hasAnyAction(flags) {
		showUsage()
	}
}

// parseFlags parses command-line flags
func parseFlags() *configFlags {
	flags := &configFlags{
		setRules:       flag.String("set-rules", "", "Set instance rules (comma-separated)"),
		setDescription: flag.String("set-description", "", "Set extended description (HTML)"),
		generateVAPID:  flag.Bool("generate-vapid", false, "Generate new VAPID keys for push notifications"),
		showConfig:     flag.Bool("show", false, "Show current configuration"),
	}
	flag.Parse()
	return flags
}

// initializeLogger creates and returns a zap logger
func initializeLogger() *zap.Logger {
	logger, _ := zap.NewProduction()
	return logger
}

// syncLogger ensures the logger is properly synced
func syncLogger(logger *zap.Logger) {
	if err := logger.Sync(); err != nil {
		log.Printf("Failed to sync logger: %v", err)
	}
}

// initializeRepositories sets up DynamORM and repository factory
func initializeRepositories(logger *zap.Logger) *factory.RepositoryFactory {
	// Get configuration for AWS region
	cfg := config.Get()

	// Load AWS config
	awsConfig, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.Region),
	)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	db, err := dynamorm.GetClient(context.Background())
	if err != nil {
		log.Fatalf("Failed to initialize DynamORM: %v", err)
	}

	tableName := getTableName()

	repos, err := factory.NewRepositoryFactory(db, tableName, awsConfig, logger)
	if err != nil {
		log.Fatalf("Failed to create repository factory: %v", err)
	}

	return repos
}

// getTableName returns the DynamoDB table name from environment or default
func getTableName() string {
	tableName := os.Getenv("DYNAMODB_TABLE")
	if tableName == "" {
		tableName = "lesser-main"
	}
	return tableName
}

// showCurrentConfiguration displays the current instance configuration
func showCurrentConfiguration(appCtx *appContext) {
	showInstanceRules(appCtx)
	showExtendedDescription(appCtx)
	showVAPIDConfiguration(appCtx)
}

// showInstanceRules displays the current instance rules
func showInstanceRules(appCtx *appContext) {
	rules, err := appCtx.repos.Instance().GetInstanceRules(appCtx.ctx)
	if err != nil {
		log.Printf("Failed to get rules: %v", err)
		return
	}

	fmt.Println("Current Rules:")
	if len(rules) == 0 {
		fmt.Println("  (no rules set)")
	}
	for _, rule := range rules {
		fmt.Printf("  %s. %s\n", rule.ID, rule.Text)
	}
}

// showExtendedDescription displays the extended description
func showExtendedDescription(appCtx *appContext) {
	desc, updatedAt, err := appCtx.repos.Instance().GetExtendedDescription(appCtx.ctx)
	if err != nil {
		log.Printf("Failed to get extended description: %v", err)
		return
	}
	fmt.Printf("\nExtended Description (updated %s):\n%s\n", updatedAt.Format(common.DateFormat), desc)
}

// showVAPIDConfiguration displays VAPID key configuration
func showVAPIDConfiguration(appCtx *appContext) {
	vapidKeys, err := appCtx.repos.PushSubscription().GetVAPIDKeys(appCtx.ctx)
	if err != nil {
		fmt.Println("\nVAPID Keys: Not configured")
		return
	}

	fmt.Printf("\nVAPID Public Key: %s\n", vapidKeys.PublicKey)
	fmt.Printf("VAPID Subject: %s\n", vapidKeys.Subject)
	fmt.Printf("Created: %s\n", vapidKeys.CreatedAt.Format("2006-01-02 15:04:05"))
}

// generateVAPIDKeys generates and saves VAPID keys for push notifications
func generateVAPIDKeys(appCtx *appContext) {
	fmt.Println("Generating VAPID keys for web push notifications...")

	privateKey := generateECDSAKey()
	publicKeyBase64, privateKeyBase64 := encodeKeys(privateKey)
	domain := getDomain()

	vapidKeys := &storage.VAPIDKeys{
		PublicKey:  publicKeyBase64,
		PrivateKey: privateKeyBase64,
		Subject:    fmt.Sprintf("mailto:admin@%s", domain),
	}

	saveVAPIDKeys(appCtx, vapidKeys)
	displayVAPIDSuccess(publicKeyBase64)
}

// generateECDSAKey generates a P-256 ECDSA key pair
func generateECDSAKey() *ecdsa.PrivateKey {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("Failed to generate VAPID private key: %v", err)
	}
	return privateKey
}

// encodeKeys encodes the public and private keys to base64
func encodeKeys(privateKey *ecdsa.PrivateKey) (string, string) {
	// Convert ECDSA key to ECDH and get public key bytes
	ecdhKey, err := privateKey.ECDH()
	if err != nil {
		log.Fatalf("Failed to convert to ECDH key: %v", err)
	}
	publicKeyBytes := ecdhKey.PublicKey().Bytes()
	publicKeyBase64 := base64.RawURLEncoding.EncodeToString(publicKeyBytes)

	// Encode private key (32 bytes)
	privateKeyBytes := privateKey.D.Bytes()
	// Pad to 32 bytes if necessary
	if len(privateKeyBytes) < 32 {
		padding := make([]byte, 32-len(privateKeyBytes))
		privateKeyBytes = append(padding, privateKeyBytes...)
	}
	privateKeyBase64 := base64.RawURLEncoding.EncodeToString(privateKeyBytes)

	return publicKeyBase64, privateKeyBase64
}

// getDomain gets the domain from environment or prompts the user
func getDomain() string {
	domain := os.Getenv("DOMAIN")
	if domain == "" {
		fmt.Print("Enter your instance domain (e.g., example.com): ")
		if _, err := fmt.Scanln(&domain); err != nil {
			log.Fatalf("Failed to read domain input: %v", err)
		}
	}
	return domain
}

// saveVAPIDKeys saves the VAPID keys to storage
func saveVAPIDKeys(appCtx *appContext, vapidKeys *storage.VAPIDKeys) {
	if err := appCtx.repos.PushSubscription().SetVAPIDKeys(appCtx.ctx, vapidKeys); err != nil {
		log.Fatalf("Failed to save VAPID keys: %v", err)
	}
}

// displayVAPIDSuccess shows success message after VAPID key generation
func displayVAPIDSuccess(publicKeyBase64 string) {
	fmt.Println("✓ VAPID keys generated successfully!")
	fmt.Printf("Public Key: %s\n", publicKeyBase64)
	fmt.Println("\nNOTE: The private key has been securely stored in DynamoDB.")
	fmt.Println("Use the public key in your Mastodon client configuration.")
}

// setInstanceRules sets the instance rules from comma-separated text
func setInstanceRules(appCtx *appContext, rulesText string) {
	rules := parseRules(rulesText)

	if err := appCtx.repos.Instance().SetInstanceRules(appCtx.ctx, rules); err != nil {
		log.Fatalf("Failed to set rules: %v", err)
	}
	fmt.Printf("✓ Set %d rules\n", len(rules))
}

// parseRules parses comma-separated rules text into InstanceRule slice
func parseRules(rulesText string) []storage.InstanceRule {
	ruleTexts := strings.Split(rulesText, ",")
	rules := make([]storage.InstanceRule, len(ruleTexts))
	for i, text := range ruleTexts {
		rules[i] = storage.InstanceRule{
			ID:   fmt.Sprintf("%d", i+1),
			Text: strings.TrimSpace(text),
		}
	}
	return rules
}

// setExtendedDescription sets the extended description for the instance
func setExtendedDescription(appCtx *appContext, description string) {
	if err := appCtx.repos.Instance().SetExtendedDescription(appCtx.ctx, description); err != nil {
		log.Fatalf("Failed to set extended description: %v", err)
	}
	fmt.Println("✓ Set extended description")
}

// hasAnyAction checks if any action flag was provided
func hasAnyAction(flags *configFlags) bool {
	return *flags.showConfig || *flags.setRules != "" || *flags.setDescription != "" || *flags.generateVAPID
}

// showUsage displays usage information
func showUsage() {
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
