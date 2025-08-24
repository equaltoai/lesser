package main

import (
	"fmt"
	"os"

	"cdk/stacks"
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/jsii-runtime-go"
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

// Config represents the environment-specific configuration
type Config struct {
	Environment string `yaml:"environment"`
	AppName     string `yaml:"appName"`
	Domain      string `yaml:"domain"`
	MemorySize  int    `yaml:"memorySize"`
	Timeout     int    `yaml:"timeout"`
	LogLevel    string `yaml:"logLevel"`
	Features    struct {
		EnableMultiTenant         bool `yaml:"enableMultiTenant"`
		EnableRateLimiting        bool `yaml:"enableRateLimiting"`
		EnableMonitoring          bool `yaml:"enableMonitoring"`
		EnableDeletionProtection  bool `yaml:"enableDeletionProtection"`
		EnablePointInTimeRecovery bool `yaml:"enablePointInTimeRecovery"`
	} `yaml:"features"`
	AWS struct {
		Region       string `yaml:"region"`
		Architecture string `yaml:"architecture"`
		Runtime      string `yaml:"runtime"`
	} `yaml:"aws"`
	Monitoring struct {
		DetailedMetrics               bool    `yaml:"detailedMetrics"`
		BusinessMetrics               bool    `yaml:"businessMetrics"`
		RealTimeStreaming             bool    `yaml:"realTimeStreaming"`
		ErrorRateThreshold            float64 `yaml:"errorRateThreshold,omitempty"`
		LatencyP99Threshold           int     `yaml:"latencyP99Threshold,omitempty"`
		ThrottleCountThreshold        int     `yaml:"throttleCountThreshold,omitempty"`
		ConcurrentExecutionsThreshold int     `yaml:"concurrentExecutionsThreshold,omitempty"`
	} `yaml:"monitoring"`
	Cost struct {
		Optimized           bool `yaml:"optimized"`
		ReservedConcurrency int  `yaml:"reservedConcurrency"`
	} `yaml:"cost"`
}

// loadEnvironmentConfig loads environment-specific configuration
func loadEnvironmentConfig(env string) (*Config, error) {
	configFile := fmt.Sprintf("config/%s.yaml", env)
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configFile, err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", configFile, err)
	}

	return &config, nil
}

// configToMap converts Config struct to map[string]interface{}
func configToMap(config *Config) map[string]interface{} {
	return map[string]interface{}{
		"environment": config.Environment,
		"appName":     config.AppName,
		"domain":      config.Domain,
		"memorySize":  float64(config.MemorySize),
		"timeout":     float64(config.Timeout),
		"logLevel":    config.LogLevel,
		"features": map[string]interface{}{
			"enableMultiTenant":         config.Features.EnableMultiTenant,
			"enableRateLimiting":        config.Features.EnableRateLimiting,
			"enableMonitoring":          config.Features.EnableMonitoring,
			"enableDeletionProtection":  config.Features.EnableDeletionProtection,
			"enablePointInTimeRecovery": config.Features.EnablePointInTimeRecovery,
		},
		"aws": map[string]interface{}{
			"region":       config.AWS.Region,
			"architecture": config.AWS.Architecture,
			"runtime":      config.AWS.Runtime,
		},
		"monitoring": map[string]interface{}{
			"detailedMetrics":               config.Monitoring.DetailedMetrics,
			"businessMetrics":               config.Monitoring.BusinessMetrics,
			"realTimeStreaming":             config.Monitoring.RealTimeStreaming,
			"errorRateThreshold":            config.Monitoring.ErrorRateThreshold,
			"latencyP99Threshold":           float64(config.Monitoring.LatencyP99Threshold),
			"throttleCountThreshold":        float64(config.Monitoring.ThrottleCountThreshold),
			"concurrentExecutionsThreshold": float64(config.Monitoring.ConcurrentExecutionsThreshold),
		},
		"cost": map[string]interface{}{
			"optimized":           config.Cost.Optimized,
			"reservedConcurrency": float64(config.Cost.ReservedConcurrency),
		},
	}
}

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	// Get environment from context
	environment := app.Node().TryGetContext(jsii.String("environment"))
	if environment == nil {
		environment = "dev"
	}
	env := environment.(string)

	// Load environment-specific configuration
	config, err := loadEnvironmentConfig(env)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Override domain from context if provided
	domain := app.Node().TryGetContext(jsii.String("domain"))
	if domain != nil {
		config.Domain = domain.(string)
	}

	// Get JWT secret from context (required for production)
	jwtSecret := app.Node().TryGetContext(jsii.String("jwtSecret"))
	if jwtSecret == nil && env == "production" {
		fmt.Printf("JWT secret is required for production environment. Pass via context: --context jwtSecret=<secret>\n")
		os.Exit(1)
	}

	// Create shared stack (only once per account)
	sharedStack := stacks.NewSharedStack(app, "LesserSharedStack", &stacks.SharedStackProps{
		StackProps: awscdk.StackProps{
			Env:         getEnv(config),
			Description: jsii.String("Lesser shared resources - KMS keys, secrets"),
		},
		AppName: config.AppName,
	})

	// Create monitoring stack
	monitoringStack := stacks.NewMonitoringStack(app, fmt.Sprintf("LesserMonitoringStack-%s", env), &stacks.MonitoringStackProps{
		StackProps: awscdk.StackProps{
			Env:         getEnv(config),
			Description: jsii.String(fmt.Sprintf("Lesser monitoring resources for %s", env)),
		},
		AppName:     config.AppName,
		Environment: env,
		AlertEmail:  os.Getenv("ALERT_EMAIL"),
	})

	var jwtSecretString *string
	if jwtSecret != nil {
		jwtSecretString = jsii.String(jwtSecret.(string))
	}

	// Create main application stack (creates its own certificate like Pulumi did)
	lesserStack := stacks.NewLesserStack(app, fmt.Sprintf("LesserStack-%s", env), &stacks.LesserStackProps{
		StackProps: awscdk.StackProps{
			Env:         getEnv(config),
			Description: jsii.String(fmt.Sprintf("Lesser serverless application - %s", env)),
		},
		Environment: env,
		Domain:      config.Domain,
		JWTSecret:   jwtSecretString,
		Config:      configToMap(config),
	})

	// Add dependencies
	lesserStack.AddDependency(sharedStack.Stack, jsii.String("Shared resources must be created first"))
	lesserStack.AddDependency(monitoringStack.Stack, jsii.String("Monitoring must be set up before application"))

	app.Synth(nil)
}

// getEnv determines the AWS environment (account+region) in which our stack is to be deployed
func getEnv(config *Config) *awscdk.Environment {
	// Try to get from environment variables first
	account := os.Getenv("CDK_DEFAULT_ACCOUNT")
	region := os.Getenv("CDK_DEFAULT_REGION")

	// Use config region if not specified in environment
	if region == "" {
		region = config.AWS.Region
	}

	// If account is specified, use it
	if account != "" {
		return &awscdk.Environment{
			Account: jsii.String(account),
			Region:  jsii.String(region),
		}
	}

	// Otherwise, environment-agnostic (will use current CLI configuration)
	return &awscdk.Environment{
		Region: jsii.String(region),
	}
}
