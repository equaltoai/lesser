package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	basicType = "basic"
)

// CDKInitCommand creates a new CDK app for a Lift project
type CDKInitCommand struct{}

func (c *CDKInitCommand) Name() string {
	return "cdk-init"
}

func (c *CDKInitCommand) Description() string {
	return "Initialize CDK infrastructure for your Lift app"
}

func (c *CDKInitCommand) Usage() string {
	return "lift cdk-init [stack-type]"
}

func (c *CDKInitCommand) Execute(_ context.Context, args []string) error {
	stackType := basicType
	if len(args) > 0 {
		stackType = args[0]
	}

	// Check if we're in a Lift project
	if _, err := os.Stat("go.mod"); err != nil {
		return fmt.Errorf("not in a Lift project directory (go.mod not found)")
	}

	// Create cdk directory
	cdkDir := "cdk"
	if err := os.MkdirAll(cdkDir, 0750); err != nil {
		return fmt.Errorf("failed to create cdk directory: %w", err)
	}

	// Generate CDK app based on stack type
	switch stackType {
	case basicType:
		return c.generateBasicCDKApp(cdkDir)
	case "microservice":
		return c.generateMicroserviceCDKApp(cdkDir)
	case "saas":
		return c.generateSaaSCDKApp(cdkDir)
	case "event-driven":
		return c.generateEventDrivenCDKApp(cdkDir)
	default:
		return fmt.Errorf("unknown stack type: %s (options: basic, microservice, saas, event-driven)", stackType)
	}
}

func (c *CDKInitCommand) generateBasicCDKApp(cdkDir string) error {
	// Create main.go for CDK app
	mainContent := `package main

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/pay-theory/lift/pkg/cdk/patterns"
)

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	NewLiftStack(app, "LiftStack", &awscdk.StackProps{
		Env: env(),
	})

	app.Synth(nil)
}

func env() *awscdk.Environment {
	return nil // Uses default environment
}

func NewLiftStack(scope constructs.Construct, id string, props *awscdk.StackProps) awscdk.Stack {
	stack := awscdk.NewStack(scope, &id, props)

	// Create Lift application
	patterns.NewLiftApp(stack, jsii.String("App"), &patterns.LiftAppProps{
		AppName:           jsii.String("my-lift-app"),
		CodeAssetPath:     jsii.String("../dist"),
		EnableDatabase:    jsii.Bool(true),
		EnableRateLimiting: jsii.Bool(true),
		Environment: &map[string]*string{
			"LOG_LEVEL": jsii.String("info"),
		},
	})

	return stack
}
`
	if err := os.WriteFile(filepath.Join(cdkDir, "main.go"), []byte(mainContent), 0600); err != nil {
		return fmt.Errorf("failed to create main.go: %w", err)
	}

	// Create cdk.json
	cdkJSON := `{
  "app": "go mod download && go run main.go",
  "watch": {
    "include": ["**"],
    "exclude": ["cdk.out", "go.sum", "go.mod"]
  },
  "context": {
    "@aws-cdk/aws-lambda:recognizeLayerVersion": true,
    "@aws-cdk/core:checkSecretUsage": true,
    "@aws-cdk/core:target-partitions": ["aws", "aws-cn"],
    "@aws-cdk/aws-ec2:uniqueImdsv2TemplateName": true,
    "@aws-cdk/aws-iam:minimizePolicies": true,
    "@aws-cdk/core:validateSnapshotRemovalPolicy": true
  }
}
`
	if err := os.WriteFile(filepath.Join(cdkDir, "cdk.json"), []byte(cdkJSON), 0600); err != nil {
		return fmt.Errorf("failed to create cdk.json: %w", err)
	}

	// Create .gitignore
	gitignore := `cdk.out/
*.js
*.d.ts
`
	if err := os.WriteFile(filepath.Join(cdkDir, ".gitignore"), []byte(gitignore), 0600); err != nil {
		return fmt.Errorf("failed to create .gitignore: %w", err)
	}

	fmt.Println("✅ CDK app initialized successfully!")
	fmt.Println("📁 Created files:")
	fmt.Println("   cdk/")
	fmt.Println("   ├── main.go")
	fmt.Println("   ├── cdk.json")
	fmt.Println("   └── .gitignore")
	fmt.Println("\n🚀 Next steps:")
	fmt.Println("   lift build         # Build your Lambda function")
	fmt.Println("   lift cdk-deploy    # Deploy to AWS")

	return nil
}

func (c *CDKInitCommand) generateMicroserviceCDKApp(cdkDir string) error {
	mainContent := `package main

import (
	"os"
	
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/pay-theory/lift/pkg/cdk/stacks"
)

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	serviceName := os.Getenv("SERVICE_NAME")
	if serviceName == "" {
		serviceName = "my-service"
	}

	stacks.NewMicroserviceStack(app, "MicroserviceStack", &stacks.MicroserviceStackProps{
		StackProps: awscdk.StackProps{
			Env: env(),
		},
		ServiceName:    serviceName,
		CodePath:       "../dist/bootstrap",
		EnableDatabase: true,
		MemorySize:     1024,
		Environment: map[string]string{
			"LOG_LEVEL": "info",
			"STAGE":     "dev",
		},
	})

	app.Synth(nil)
}

func env() *awscdk.Environment {
	return nil
}
`
	return c.writeStackFiles(cdkDir, mainContent, "microservice")
}

func (c *CDKInitCommand) generateSaaSCDKApp(cdkDir string) error {
	mainContent := `package main

import (
	"os"
	
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/pay-theory/lift/pkg/cdk/stacks"
)

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	appName := os.Getenv("APP_NAME")
	if appName == "" {
		appName = "my-saas-app"
	}

	stacks.NewMultiTenantSaaSStack(app, "SaaSStack", &stacks.MultiTenantSaaSStackProps{
		StackProps: awscdk.StackProps{
			Env: env(),
		},
		AppName:           appName,
		CodePath:          "../dist/bootstrap",
		EnableAuth:        true,
		EnableFileStorage: true,
	})

	app.Synth(nil)
}

func env() *awscdk.Environment {
	return nil
}
`
	return c.writeStackFiles(cdkDir, mainContent, "saas")
}

func (c *CDKInitCommand) generateEventDrivenCDKApp(cdkDir string) error {
	mainContent := `package main

import (
	"os"
	
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	"github.com/pay-theory/lift/pkg/cdk/stacks"
)

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	appName := os.Getenv("APP_NAME")
	if appName == "" {
		appName = "my-event-app"
	}

	stacks.NewEventDrivenStack(app, "EventStack", &stacks.EventDrivenStackProps{
		StackProps: awscdk.StackProps{
			Env: env(),
		},
		AppName:                appName,
		ApiCodePath:            "../dist/api/bootstrap",
		EventProcessorCodePath: "../dist/processor/bootstrap",
		EnableDLQ:              true,
		EventBusName:           appName + "-events",
	})

	app.Synth(nil)
}

func env() *awscdk.Environment {
	return nil
}
`
	return c.writeStackFiles(cdkDir, mainContent, "event-driven")
}

func (c *CDKInitCommand) writeStackFiles(cdkDir, mainContent, stackType string) error {
	if err := os.WriteFile(filepath.Join(cdkDir, "main.go"), []byte(mainContent), 0600); err != nil {
		return fmt.Errorf("failed to create main.go: %w", err)
	}

	// Standard cdk.json
	cdkJSON := `{
  "app": "go mod download && go run main.go",
  "watch": {
    "include": ["**"],
    "exclude": ["cdk.out", "go.sum", "go.mod"]
  },
  "context": {
    "@aws-cdk/aws-lambda:recognizeLayerVersion": true,
    "@aws-cdk/core:checkSecretUsage": true,
    "@aws-cdk/core:target-partitions": ["aws", "aws-cn"],
    "@aws-cdk/aws-ec2:uniqueImdsv2TemplateName": true,
    "@aws-cdk/aws-iam:minimizePolicies": true
  }
}
`
	if err := os.WriteFile(filepath.Join(cdkDir, "cdk.json"), []byte(cdkJSON), 0600); err != nil {
		return fmt.Errorf("failed to create cdk.json: %w", err)
	}

	fmt.Printf("✅ CDK %s stack initialized!\n", stackType)
	fmt.Println("📁 Created CDK app in cdk/ directory")
	fmt.Println("\n🚀 Next steps:")
	fmt.Println("   lift build         # Build your function")
	fmt.Println("   lift cdk-deploy    # Deploy stack")

	return nil
}

// CDKDeployCommand deploys the CDK stack
type CDKDeployCommand struct{}

func (c *CDKDeployCommand) Name() string {
	return "cdk-deploy"
}

func (c *CDKDeployCommand) Description() string {
	return "Deploy your Lift app using CDK"
}

func (c *CDKDeployCommand) Usage() string {
	return "lift cdk-deploy [stack-name] [--all]"
}

func (c *CDKDeployCommand) Execute(ctx context.Context, args []string) error {
	// Build the Lambda function first
	fmt.Println("🔨 Building Lambda function...")
	if err := c.buildFunction(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	// Change to CDK directory
	if err := os.Chdir("cdk"); err != nil {
		return fmt.Errorf("CDK directory not found. Run 'lift cdk-init' first")
	}

	// Prepare CDK deploy command
	cdkArgs := []string{"deploy", "--require-approval", "never"}

	// Add stack name or --all flag
	if len(args) > 0 {
		if args[0] == "--all" {
			cdkArgs = append(cdkArgs, "--all")
		} else {
			cdkArgs = append(cdkArgs, args[0])
		}
	}

	// Run CDK deploy
	fmt.Println("🚀 Deploying with CDK...")
	cmd := exec.CommandContext(ctx, "cdk", cdkArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("CDK deploy failed: %w", err)
	}

	fmt.Println("✅ Deployment complete!")
	return nil
}

func (c *CDKDeployCommand) buildFunction() error {
	// Create dist directory
	if err := os.MkdirAll("dist", 0750); err != nil {
		return err
	}

	// Build for Lambda
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", "dist/bootstrap", "./cmd/main.go")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// CDKSynthCommand synthesizes the CDK stack
type CDKSynthCommand struct{}

func (c *CDKSynthCommand) Name() string {
	return "cdk-synth"
}

func (c *CDKSynthCommand) Description() string {
	return "Synthesize CloudFormation template from CDK"
}

func (c *CDKSynthCommand) Usage() string {
	return "lift cdk-synth [stack-name]"
}

func (c *CDKSynthCommand) Execute(ctx context.Context, args []string) error {
	// Change to CDK directory
	if err := os.Chdir("cdk"); err != nil {
		return fmt.Errorf("CDK directory not found. Run 'lift cdk-init' first")
	}

	// Prepare CDK synth command
	cdkArgs := []string{"synth"}
	if len(args) > 0 {
		cdkArgs = append(cdkArgs, args[0])
	}

	// Run CDK synth
	fmt.Println("📋 Synthesizing CloudFormation template...")
	cmd := exec.CommandContext(ctx, "cdk", cdkArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("CDK synth failed: %w", err)
	}

	return nil
}

// CDKDiffCommand shows differences between deployed stack and local code
type CDKDiffCommand struct{}

func (c *CDKDiffCommand) Name() string {
	return "cdk-diff"
}

func (c *CDKDiffCommand) Description() string {
	return "Show differences between deployed stack and local code"
}

func (c *CDKDiffCommand) Usage() string {
	return "lift cdk-diff [stack-name]"
}

func (c *CDKDiffCommand) Execute(ctx context.Context, args []string) error {
	// Change to CDK directory
	if err := os.Chdir("cdk"); err != nil {
		return fmt.Errorf("CDK directory not found. Run 'lift cdk-init' first")
	}

	// Prepare CDK diff command
	cdkArgs := []string{"diff"}
	if len(args) > 0 {
		cdkArgs = append(cdkArgs, args[0])
	}

	// Run CDK diff
	fmt.Println("🔍 Comparing stack with deployed resources...")
	cmd := exec.CommandContext(ctx, "cdk", cdkArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// CDK diff returns non-zero when there are differences, which is expected
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil
		}
		return fmt.Errorf("CDK diff failed: %w", err)
	}

	return nil
}

// CDKDestroyCommand destroys the CDK stack
type CDKDestroyCommand struct{}

func (c *CDKDestroyCommand) Name() string {
	return "cdk-destroy"
}

func (c *CDKDestroyCommand) Description() string {
	return "Destroy the deployed CDK stack"
}

func (c *CDKDestroyCommand) Usage() string {
	return "lift cdk-destroy [stack-name] [--all]"
}

func (c *CDKDestroyCommand) Execute(ctx context.Context, args []string) error {
	// Change to CDK directory
	if err := os.Chdir("cdk"); err != nil {
		return fmt.Errorf("CDK directory not found")
	}

	// Prepare CDK destroy command
	cdkArgs := []string{"destroy", "--force"}

	// Add stack name or --all flag
	if len(args) > 0 {
		if args[0] == "--all" {
			cdkArgs = append(cdkArgs, "--all")
		} else {
			cdkArgs = append(cdkArgs, args[0])
		}
	}

	// Confirm destruction
	fmt.Println("⚠️  WARNING: This will destroy all resources in the stack!")
	fmt.Print("Are you sure? (y/N): ")

	var response string
	if _, err := fmt.Scanln(&response); err != nil {
		// If user just presses enter or there's an error, treat as "no"
		fmt.Println("Destruction canceled")
		return nil
	}
	if !strings.HasPrefix(strings.ToLower(response), "y") {
		fmt.Println("Destruction canceled")
		return nil
	}

	// Run CDK destroy
	fmt.Println("💥 Destroying stack...")
	cmd := exec.CommandContext(ctx, "cdk", cdkArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("CDK destroy failed: %w", err)
	}

	fmt.Println("✅ Stack destroyed successfully")
	return nil
}

// BuildCommand builds the Lambda function
type BuildCommand struct{}

func (c *BuildCommand) Name() string {
	return "build"
}

func (c *BuildCommand) Description() string {
	return "Build Lambda function for deployment"
}

func (c *BuildCommand) Usage() string {
	return "lift build [--arch arm64|amd64]"
}

func (c *BuildCommand) Execute(ctx context.Context, args []string) error {
	arch := "arm64" // Default to ARM64

	// Parse architecture flag
	for i, arg := range args {
		if arg == "--arch" && i+1 < len(args) {
			arch = args[i+1]
			if arch != "arm64" && arch != "amd64" {
				return fmt.Errorf("invalid architecture: %s (must be arm64 or amd64)", arch)
			}
		}
	}

	// Create dist directory
	if err := os.MkdirAll("dist", 0750); err != nil {
		return fmt.Errorf("failed to create dist directory: %w", err)
	}

	// Build command
	fmt.Printf("🔨 Building for Linux/%s...\n", arch)
	cmd := exec.CommandContext(ctx, "go", "build", "-o", "dist/bootstrap", "./cmd/main.go")
	cmd.Env = append(os.Environ(), "GOOS=linux", fmt.Sprintf("GOARCH=%s", arch))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	// Get file size
	info, err := os.Stat("dist/bootstrap")
	if err == nil {
		size := info.Size() / 1024 / 1024 // Convert to MB
		fmt.Printf("✅ Build complete! (dist/bootstrap - %dMB)\n", size)
	} else {
		fmt.Println("✅ Build complete!")
	}

	return nil
}
