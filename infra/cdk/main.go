package main

import (
	"fmt"
	"os"

	"cdk/stacks"
	"io/ioutil"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/jsii-runtime-go"
	"gopkg.in/yaml.v2"
)

// Config represents the environment-specific configuration
type Config struct {
	Environment string `yaml:"environment"`
	AppName     string `yaml:"appName"`
	Domain      string `yaml:"domain"`
	MemorySize  int    `yaml:"memorySize"`
	Timeout     int    `yaml:"timeout"`
	LogLevel    string `yaml:"logLevel"`
	DNS         struct {
		RootDomain   string `yaml:"rootDomain"`
		HostedZoneID string `yaml:"hostedZoneId"`
	} `yaml:"dns"`
	Features struct {
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
	Media struct {
		CloudfrontDomain           string `yaml:"cloudfrontDomain"`
		CloudfrontKeyPairId        string `yaml:"cloudfrontKeyPairId"`
		CloudfrontPrivateKeySecret string `yaml:"cloudfrontPrivateKeySecret"`
		MediaConvertEndpoint       string `yaml:"mediaConvertEndpoint"`
		MediaConvertRole           string `yaml:"mediaConvertRole"`
		ManifestTTLHours           int    `yaml:"manifestTTLHours"`
	} `yaml:"media"`
	Push struct {
		VapidPublicKey string `yaml:"vapidPublicKey"`
		VapidSubject   string `yaml:"vapidSubject"`
		VapidSecretArn string `yaml:"vapidSecretArn"`
	} `yaml:"push"`
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
		"cloudfrontDomain":           config.Media.CloudfrontDomain,
		"cloudfrontKeyPairId":        config.Media.CloudfrontKeyPairId,
		"cloudfrontPrivateKeySecret": config.Media.CloudfrontPrivateKeySecret,
		"mediaConvertEndpoint":       config.Media.MediaConvertEndpoint,
		"mediaConvertRole":           config.Media.MediaConvertRole,
		"manifestTTLHours":           fmt.Sprintf("%d", config.Media.ManifestTTLHours),
		"vapidPublicKey":             config.Push.VapidPublicKey,
		"vapidSubject":               config.Push.VapidSubject,
		"vapidSecretArn":             config.Push.VapidSecretArn,
	}
}

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	defaultRootDomain := "lesser.host"
	defaultZones := []string{"dev", "staging", "live"}

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
	if domainCtx := app.Node().TryGetContext(jsii.String("domain")); domainCtx != nil {
		config.Domain = fmt.Sprintf("%v", domainCtx)
	}

	if secretCtx := app.Node().TryGetContext(jsii.String("cdnPrivateKeySecret")); secretCtx != nil {
		config.Media.CloudfrontPrivateKeySecret = fmt.Sprintf("%v", secretCtx)
	}

	if keyPairCtx := app.Node().TryGetContext(jsii.String("cdnKeyPairId")); keyPairCtx != nil {
		config.Media.CloudfrontKeyPairId = fmt.Sprintf("%v", keyPairCtx)
	}

	if vapidSecretCtx := app.Node().TryGetContext(jsii.String("vapidSecretArn")); vapidSecretCtx != nil {
		config.Push.VapidSecretArn = fmt.Sprintf("%v", vapidSecretCtx)
	}

	if vapidPublicCtx := app.Node().TryGetContext(jsii.String("vapidPublicKey")); vapidPublicCtx != nil {
		config.Push.VapidPublicKey = fmt.Sprintf("%v", vapidPublicCtx)
	}

	if vapidSubjectCtx := app.Node().TryGetContext(jsii.String("vapidSubject")); vapidSubjectCtx != nil {
		config.Push.VapidSubject = fmt.Sprintf("%v", vapidSubjectCtx)
	}

	// JWT secret is now auto-generated in SharedStack and retrieved by Lambda functions
	// No need to pass it via context anymore

	rootDomain := config.DNS.RootDomain
	if rootDomain == "" {
		rootDomain = defaultRootDomain
	}

	stages := defaultZones

	// Create shared stack (only once per account)
	// Resources are published to SSM Parameter Store and imported by environment stacks
	_ = stacks.NewSharedStack(app, "LesserSharedStack", &stacks.SharedStackProps{
		StackProps: awscdk.StackProps{
			Env:         getEnv(config),
			Description: jsii.String("Lesser shared resources - KMS keys, secrets"),
		},
		AppName:        config.AppName,
		RootDomain:     rootDomain,
		HostedZoneId:   config.DNS.HostedZoneID,
		HostedZoneName: rootDomain,
		Stages:         stages,
	})

	// Create monitoring stack
	// Get alert email from environment variable
	alertEmail := os.Getenv("ALERT_EMAIL")
	if alertEmail == "" {
		alertEmail = "alerts@example.com" // Default fallback
	}

	monitoringStack := stacks.NewMonitoringStack(app, fmt.Sprintf("LesserMonitoringStack-%s", env), &stacks.MonitoringStackProps{
		StackProps: awscdk.StackProps{
			Env:         getEnv(config),
			Description: jsii.String(fmt.Sprintf("Lesser monitoring resources for %s", env)),
		},
		AppName:     config.AppName,
		Environment: env,
		AlertEmail:  alertEmail,
	})

	configMap := configToMap(config)

	// Create main application stack
	// Note: Shared resources are imported from SSM Parameter Store, not passed directly
	lesserApiStack := stacks.NewLesserApiStack(app, fmt.Sprintf("LesserApiStack-%s", env), &stacks.LesserApiStackProps{
		StackProps: awscdk.StackProps{
			Env:         getEnv(config),
			Description: jsii.String(fmt.Sprintf("Lesser serverless application - %s", env)),
		},
		Environment:      env,
		Domain:           config.Domain,
		Config:           configMap,
		HostedZoneDomain: rootDomain,
		HostedZoneId:     config.DNS.HostedZoneID,
		CloudFrontDomain: config.Media.CloudfrontDomain,
		AppName:          "lesser", // Hard-coded to match SharedStack SSM parameter prefix
	})

	// Add dependencies
	// Note: No explicit dependency on SharedStack needed - SSM parameters will be resolved at deploy time
	lesserApiStack.AddDependency(monitoringStack.Stack, jsii.String("Monitoring must be set up before application"))

	app.Synth(nil)
}

// getEnv determines the AWS environment (account+region) in which our stack is to be deployed
func getEnv(config *Config) *awscdk.Environment {
	// CDK_DEFAULT_ACCOUNT and CDK_DEFAULT_REGION are CDK-specific deployment variables
	// These should remain as direct os.Getenv calls as they are CDK runtime variables
	account := os.Getenv("CDK_DEFAULT_ACCOUNT")
	region := os.Getenv("CDK_DEFAULT_REGION")

	// Use Lesser config region if CDK region is not specified
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
