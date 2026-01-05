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
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/factory"
	dynamormCore "github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// configFlags holds command-line configuration flags
type configFlags struct {
	setRules       string
	setDescription string
	generateVAPID  bool
	showConfig     bool
}

type instanceRepository interface {
	GetInstanceRules(ctx context.Context) ([]storage.InstanceRule, error)
	SetInstanceRules(ctx context.Context, rules []storage.InstanceRule) error
	GetExtendedDescription(ctx context.Context) (string, time.Time, error)
	SetExtendedDescription(ctx context.Context, description string) error
}

type pushSubscriptionRepository interface {
	GetVAPIDKeys(ctx context.Context) (*storage.VAPIDKeys, error)
	SetVAPIDKeys(ctx context.Context, keys *storage.VAPIDKeys) error
}

type repositoriesProvider interface {
	Instance() instanceRepository
	PushSubscription() pushSubscriptionRepository
}

type repositoryFactoryAdapter struct {
	repos *factory.RepositoryFactory
}

func (a repositoryFactoryAdapter) Instance() instanceRepository {
	return a.repos.Instance()
}

func (a repositoryFactoryAdapter) PushSubscription() pushSubscriptionRepository {
	return a.repos.PushSubscription()
}

// appContext holds application-wide context and dependencies
type appContext struct {
	ctx    context.Context
	logger *zap.Logger
	repos  repositoriesProvider
}

var (
	mustInitializeLambdaFn = common.MustInitializeLambda
	getDynamormClientFn    = dynamorm.GetClient
	newRepositoryFactoryFn = func(db dynamormCore.DB, tableName string, logger *zap.Logger) (repositoriesProvider, error) {
		repos, err := factory.NewRepositoryFactory(db, tableName, logger)
		if err != nil {
			return nil, err
		}
		return repositoryFactoryAdapter{repos: repos}, nil
	}
	getConfigFn = config.Get
	scanlnFn    = fmt.Scanln
)

func main() {
	if err := runConfigureInstance(context.Background(), os.Args[1:]); err != nil {
		log.Printf("configure-instance failed: %v", err)
		os.Exit(1)
	}
}

func runConfigureInstance(ctx context.Context, args []string) error {
	flags, flagSet, err := parseFlags(args)
	if err != nil {
		showUsage(flagSet)
		return err
	}

	lambdaCtx := mustInitializeLambdaFn(common.LambdaConfig{
		ServiceName: "configure-instance",
		LambdaType:  common.LambdaTypeBasic,
		Version:     "1.0.0",
	})

	// Initialize storage independently to avoid import cycles
	db, err := getDynamormClientFn(ctx)
	if err != nil {
		return err
	}

	repos, err := newRepositoryFactoryFn(db, lambdaCtx.Config.DynamoTableName, lambdaCtx.Logger)
	if err != nil {
		return err
	}

	appCtx := &appContext{
		ctx:    ctx,
		logger: lambdaCtx.Logger,
		repos:  repos,
	}

	// Handle different operation modes
	if flags.showConfig {
		showCurrentConfiguration(appCtx)
		return nil
	}

	if flags.generateVAPID {
		if err := generateVAPIDKeys(appCtx); err != nil {
			return err
		}
	}

	if flags.setRules != "" {
		if err := setInstanceRules(appCtx, flags.setRules); err != nil {
			return err
		}
	}

	if flags.setDescription != "" {
		if err := setExtendedDescription(appCtx, flags.setDescription); err != nil {
			return err
		}
	}

	// If no action specified, show usage
	if !hasAnyAction(flags) {
		showUsage(flagSet)
	}

	return nil
}

// parseFlags parses command-line flags
func parseFlags(args []string) (configFlags, *flag.FlagSet, error) {
	flags := configFlags{}
	fs := flag.NewFlagSet("configure-instance", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	fs.StringVar(&flags.setRules, "set-rules", "", "Set instance rules (comma-separated)")
	fs.StringVar(&flags.setDescription, "set-description", "", "Set extended description (HTML)")
	fs.BoolVar(&flags.generateVAPID, "generate-vapid", false, "Generate new VAPID keys for push notifications")
	fs.BoolVar(&flags.showConfig, "show", false, "Show current configuration")

	err := fs.Parse(args)
	return flags, fs, err
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
		appCtx.logger.Error("failed to get rules", zap.Error(err))
		return
	}

	fmt.Println("Current Rules:")
	if err := common.ValidateSliceNotEmpty("rules", rules); err != nil {
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
		appCtx.logger.Error("failed to get extended description", zap.Error(err))
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
func generateVAPIDKeys(appCtx *appContext) error {
	fmt.Println("Generating VAPID keys for web push notifications...")

	privateKey, err := generateECDSAKey()
	if err != nil {
		return err
	}

	publicKeyBase64, privateKeyBase64, err := encodeKeys(privateKey)
	if err != nil {
		return err
	}

	domain, err := getDomain()
	if err != nil {
		return err
	}

	vapidKeys := &storage.VAPIDKeys{
		PublicKey:  publicKeyBase64,
		PrivateKey: privateKeyBase64,
		Subject:    fmt.Sprintf("mailto:admin@%s", domain),
	}

	if err := saveVAPIDKeys(appCtx, vapidKeys); err != nil {
		return err
	}
	displayVAPIDSuccess(publicKeyBase64)
	return nil
}

// generateECDSAKey generates a P-256 ECDSA key pair
func generateECDSAKey() (*ecdsa.PrivateKey, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	return privateKey, nil
}

// encodeKeys encodes the public and private keys to base64
func encodeKeys(privateKey *ecdsa.PrivateKey) (string, string, error) {
	// Convert ECDSA key to ECDH and get public key bytes
	ecdhKey, err := privateKey.ECDH()
	if err != nil {
		return "", "", err
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

	return publicKeyBase64, privateKeyBase64, nil
}

// getDomain gets the domain from centralized config or prompts the user
func getDomain() (string, error) {
	cfg := getConfigFn()
	domain := cfg.Domain
	if err := common.ValidateRequiredParam("domain", domain); err != nil {
		fmt.Print("Enter your instance domain (e.g., example.com): ")
		if _, err := scanlnFn(&domain); err != nil {
			return "", err
		}
	}
	return domain, nil
}

// saveVAPIDKeys saves the VAPID keys to storage
func saveVAPIDKeys(appCtx *appContext, vapidKeys *storage.VAPIDKeys) error {
	if err := appCtx.repos.PushSubscription().SetVAPIDKeys(appCtx.ctx, vapidKeys); err != nil {
		return err
	}
	return nil
}

// displayVAPIDSuccess shows success message after VAPID key generation
func displayVAPIDSuccess(publicKeyBase64 string) {
	fmt.Println("✓ VAPID keys generated successfully!")
	fmt.Printf("Public Key: %s\n", publicKeyBase64)
	fmt.Println("\nNOTE: The private key has been securely stored in DynamoDB.")
	fmt.Println("Use the public key in your Mastodon client configuration.")
}

// setInstanceRules sets the instance rules from comma-separated text
func setInstanceRules(appCtx *appContext, rulesText string) error {
	rules := parseRules(rulesText)

	if err := appCtx.repos.Instance().SetInstanceRules(appCtx.ctx, rules); err != nil {
		return err
	}
	fmt.Printf("✓ Set %d rules\n", len(rules))
	return nil
}

// parseRules parses comma-separated rules text into InstanceRule slice
func parseRules(rulesText string) []storage.InstanceRule {
	ruleTexts := strings.Split(rulesText, ",")
	rules := make([]storage.InstanceRule, len(ruleTexts))
	for i, text := range ruleTexts {
		rules[i] = storage.InstanceRule{
			ID:   fmt.Sprintf("%d", i+1),
			Text: common.SanitizeInput(text),
		}
	}
	return rules
}

// setExtendedDescription sets the extended description for the instance
func setExtendedDescription(appCtx *appContext, description string) error {
	if err := appCtx.repos.Instance().SetExtendedDescription(appCtx.ctx, description); err != nil {
		return err
	}
	fmt.Println("✓ Set extended description")
	return nil
}

// hasAnyAction checks if any action flag was provided
func hasAnyAction(flags configFlags) bool {
	return flags.showConfig || flags.setRules != "" || flags.setDescription != "" || flags.generateVAPID
}

// showUsage displays usage information
func showUsage(flagSet *flag.FlagSet) {
	fmt.Println("Usage: configure-instance [options]")
	fmt.Println("\nOptions:")
	if flagSet != nil {
		flagSet.PrintDefaults()
	}
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
}
