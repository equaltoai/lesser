package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aron23/lesser/pkg/storage"
	storageDB "github.com/aron23/lesser/pkg/storage/dynamodb"
)

func main() {
	var (
		setRules       = flag.String("set-rules", "", "Set instance rules (comma-separated)")
		setDescription = flag.String("set-description", "", "Set extended description (HTML)")
		showConfig     = flag.Bool("show", false, "Show current configuration")
	)
	flag.Parse()

	// Initialize storage
	store, err := storageDB.New()
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	ctx := context.Background()

	// Show current configuration
	if *showConfig {
		rules, err := store.GetInstanceRules(ctx)
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

		desc, updatedAt, err := store.GetExtendedDescription(ctx)
		if err != nil {
			log.Printf("Failed to get extended description: %v", err)
		} else {
			fmt.Printf("\nExtended Description (updated %s):\n%s\n", updatedAt.Format("2006-01-02"), desc)
		}
		return
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

		if err := store.SetInstanceRules(ctx, rules); err != nil {
			log.Fatalf("Failed to set rules: %v", err)
		}
		fmt.Printf("✓ Set %d rules\n", len(rules))
	}

	// Set extended description
	if *setDescription != "" {
		if err := store.SetExtendedDescription(ctx, *setDescription); err != nil {
			log.Fatalf("Failed to set extended description: %v", err)
		}
		fmt.Println("✓ Set extended description")
	}

	// If no action specified, show usage
	if !*showConfig && *setRules == "" && *setDescription == "" {
		fmt.Println("Usage: configure-instance [options]")
		fmt.Println("\nOptions:")
		flag.PrintDefaults()
		fmt.Println("\nExamples:")
		fmt.Println("  # Show current configuration")
		fmt.Println("  configure-instance -show")
		fmt.Println("")
		fmt.Println("  # Set instance rules")
		fmt.Println("  configure-instance -set-rules \"Be respectful,No spam,Follow the law\"")
		fmt.Println("")
		fmt.Println("  # Set extended description")
		fmt.Println("  configure-instance -set-description \"<h1>Welcome!</h1><p>This is a personal instance.</p>\"")
		os.Exit(0)
	}
}
