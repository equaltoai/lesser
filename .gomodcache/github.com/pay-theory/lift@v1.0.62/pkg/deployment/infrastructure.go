package deployment

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// InfrastructureProvider defines the supported IaC providers
type InfrastructureProvider string

const (
	ProviderPulumi         InfrastructureProvider = "pulumi"
	ProviderTerraform      InfrastructureProvider = "terraform"
	ProviderCloudFormation InfrastructureProvider = "cloudformation"
	ProviderCDK            InfrastructureProvider = "cdk"
)

// InfrastructureTemplate represents a complete infrastructure template
type InfrastructureTemplate struct {
	// Time structs (24 bytes each) - largest first
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Maps (8 bytes each)
	Resources  map[string]Resource  `json:"resources"`
	Outputs    map[string]Output    `json:"outputs"`
	Parameters map[string]Parameter `json:"parameters"`
	Metadata   map[string]any       `json:"metadata"`
	Tags       map[string]string    `json:"tags"`

	// Enums/constants (8 bytes)
	Provider InfrastructureProvider `json:"provider"`

	// Strings (16 bytes each)
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

// Resource represents an infrastructure resource
type Resource struct {
	Properties   map[string]any    `json:"properties"`
	Metadata     map[string]any    `json:"metadata,omitempty"`
	Tags         map[string]string `json:"tags,omitempty"`
	Type         string            `json:"type"`
	Name         string            `json:"name"`
	Condition    string            `json:"condition,omitempty"`
	Dependencies []string          `json:"dependencies,omitempty"`
}

// Output represents a template output
type Output struct {
	Value       any    `json:"value"`
	Description string `json:"description"`
	Export      bool   `json:"export,omitempty"`
}

// Parameter represents a template parameter
type Parameter struct {
	Default       any    `json:"default,omitempty"`
	MinLength     *int   `json:"min_length,omitempty"`
	MaxLength     *int   `json:"max_length,omitempty"`
	Type          string `json:"type"`
	Description   string `json:"description"`
	Pattern       string `json:"pattern,omitempty"`
	AllowedValues []any  `json:"allowed_values,omitempty"`
}

// InfrastructureConfig holds configuration for infrastructure generation
type InfrastructureConfig struct {
	Metadata        map[string]any    `json:"metadata"`
	Tags            map[string]string `json:"tags"`
	ApplicationName string            `json:"application_name"`
	Region          string            `json:"region"`
	Environment     string            `json:"environment"`
	Security        SecurityConfig    `json:"security"`
	Regions         []string          `json:"regions,omitempty"`
	Monitoring      MonitoringConfig  `json:"monitoring"`
	Database        DatabaseConfig    `json:"database"`
	Networking      NetworkingConfig  `json:"networking"`
	Lambda          LambdaConfig      `json:"lambda"`
	APIGateway      APIGatewayConfig  `json:"api_gateway"`
	MultiRegion     bool              `json:"multi_region"`
}

// LambdaConfig holds Lambda function configuration
type LambdaConfig struct {
	ReservedConcurrency *int              `json:"reserved_concurrency,omitempty"`
	Environment         map[string]string `json:"environment,omitempty"`
	VPCConfig           *VPCConfig        `json:"vpc_config,omitempty"`
	Runtime             string            `json:"runtime"`
	Handler             string            `json:"handler"`
	Layers              []string          `json:"layers,omitempty"`
	Timeout             int               `json:"timeout"`
	MemorySize          int               `json:"memory_size"`
	DeadLetterQueue     bool              `json:"dead_letter_queue"`
}

// APIGatewayConfig holds API Gateway configuration
type APIGatewayConfig struct {
	CustomDomain   *CustomDomainConfig  `json:"custom_domain,omitempty"`
	Caching        CachingConfig        `json:"caching"`
	Type           string               `json:"type"`
	StageName      string               `json:"stage_name"`
	Authentication AuthenticationConfig `json:"authentication"`
	CORS           CORSConfig           `json:"cors"`
	Throttling     ThrottlingConfig     `json:"throttling"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Encryption    EncryptionConfig `json:"encryption"`
	Type          string           `json:"type"`
	Tables        []TableConfig    `json:"tables,omitempty"`
	GlobalTables  bool             `json:"global_tables"`
	BackupEnabled bool             `json:"backup_enabled"`
	StreamEnabled bool             `json:"stream_enabled"`
}

// MonitoringConfig holds monitoring configuration
type MonitoringConfig struct {
	XRay       XRayConfig       `json:"xray"`
	Dashboard  DashboardConfig  `json:"dashboard"`
	Alarms     []AlarmConfig    `json:"alarms"`
	CloudWatch CloudWatchConfig `json:"cloudwatch"`
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	IAMRoles       []IAMRoleConfig     `json:"iam_roles"`
	KMSKeys        []KMSKeyConfig      `json:"kms_keys"`
	SecretsManager SecretsConfig       `json:"secrets_manager"`
	WAF            *WAFConfig          `json:"waf,omitempty"`
	VPCEndpoints   []VPCEndpointConfig `json:"vpc_endpoints,omitempty"`
}

// NetworkingConfig holds networking configuration
type NetworkingConfig struct {
	VPC            *VPCConfig            `json:"vpc,omitempty"`
	Subnets        []SubnetConfig        `json:"subnets,omitempty"`
	SecurityGroups []SecurityGroupConfig `json:"security_groups,omitempty"`
	VPCEndpoints   []VPCEndpointConfig   `json:"vpc_endpoints,omitempty"`
	NATGateways    bool                  `json:"nat_gateways"`
}

// Supporting configuration types
type VPCConfig struct {
	CIDR               string   `json:"cidr"`
	AvailabilityZones  []string `json:"availability_zones"`
	EnableDNSSupport   bool     `json:"enable_dns_support"`
	EnableDNSHostnames bool     `json:"enable_dns_hostnames"`
}

type CORSConfig struct {
	AllowOrigins     []string `json:"allow_origins"`
	AllowMethods     []string `json:"allow_methods"`
	AllowHeaders     []string `json:"allow_headers"`
	ExposeHeaders    []string `json:"expose_headers,omitempty"`
	MaxAge           int      `json:"max_age,omitempty"`
	AllowCredentials bool     `json:"allow_credentials"`
}

type AuthenticationConfig struct {
	Type        string             `json:"type"` // JWT, API_KEY, IAM, COGNITO
	Authorizers []AuthorizerConfig `json:"authorizers,omitempty"`
	APIKeys     []APIKeyConfig     `json:"api_keys,omitempty"`
}

type ThrottlingConfig struct {
	BurstLimit int `json:"burst_limit"`
	RateLimit  int `json:"rate_limit"`
}

type CachingConfig struct {
	KeyPattern string `json:"key_pattern,omitempty"`
	TTL        int    `json:"ttl"`
	Enabled    bool   `json:"enabled"`
}

type CustomDomainConfig struct {
	DomainName        string `json:"domain_name"`
	CertificateArn    string `json:"certificate_arn"`
	Route53HostedZone string `json:"route53_hosted_zone,omitempty"`
}

type TableConfig struct {
	Name          string              `json:"name"`
	BillingMode   string              `json:"billing_mode"`
	HashKey       string              `json:"hash_key"`
	RangeKey      string              `json:"range_key,omitempty"`
	Attributes    []AttributeConfig   `json:"attributes"`
	GlobalIndexes []GlobalIndexConfig `json:"global_indexes,omitempty"`
	LocalIndexes  []LocalIndexConfig  `json:"local_indexes,omitempty"`
	StreamEnabled bool                `json:"stream_enabled"`
	BackupEnabled bool                `json:"backup_enabled"`
}

type EncryptionConfig struct {
	KMSKeyId  string `json:"kms_key_id,omitempty"`
	Algorithm string `json:"algorithm,omitempty"`
	Enabled   bool   `json:"enabled"`
}

type CloudWatchConfig struct {
	LogGroups     []LogGroupConfig     `json:"log_groups"`
	MetricFilters []MetricFilterConfig `json:"metric_filters,omitempty"`
	RetentionDays int                  `json:"retention_days"`
}

type XRayConfig struct {
	TracingConfig string  `json:"tracing_config"`
	SamplingRate  float64 `json:"sampling_rate"`
	Enabled       bool    `json:"enabled"`
}

type AlarmConfig struct {
	Name               string   `json:"name"`
	MetricName         string   `json:"metric_name"`
	Namespace          string   `json:"namespace"`
	Statistic          string   `json:"statistic"`
	ComparisonOperator string   `json:"comparison_operator"`
	Actions            []string `json:"actions"`
	Threshold          float64  `json:"threshold"`
	EvaluationPeriods  int      `json:"evaluation_periods"`
	Period             int      `json:"period"`
}

type DashboardConfig struct {
	Name    string            `json:"name"`
	Widgets []DashboardWidget `json:"widgets"`
}

type DashboardWidget struct {
	Properties map[string]any `json:"properties"`
	Type       string         `json:"type"`
	Title      string         `json:"title"`
}

// Additional supporting types
type IAMRoleConfig struct {
	Name             string               `json:"name"`
	AssumeRolePolicy map[string]any       `json:"assume_role_policy"`
	Policies         []string             `json:"policies"`
	InlinePolicies   []InlinePolicyConfig `json:"inline_policies,omitempty"`
}

type KMSKeyConfig struct {
	Policy      map[string]any `json:"policy,omitempty"`
	Alias       string         `json:"alias"`
	Description string         `json:"description"`
}

type SecretsConfig struct {
	Secrets []SecretConfig `json:"secrets"`
}

type SecretConfig struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	KMSKeyId    string `json:"kms_key_id,omitempty"`
}

type WAFConfig struct {
	Name  string    `json:"name"`
	Rules []WAFRule `json:"rules"`
}

type WAFRule struct {
	Statement map[string]any `json:"statement"`
	Name      string         `json:"name"`
	Action    string         `json:"action"`
	Priority  int            `json:"priority"`
}

type VPCEndpointConfig struct {
	ServiceName string   `json:"service_name"`
	Type        string   `json:"type"` // Gateway, Interface
	SubnetIds   []string `json:"subnet_ids,omitempty"`
}

type SubnetConfig struct {
	Name             string `json:"name"`
	CIDR             string `json:"cidr"`
	AvailabilityZone string `json:"availability_zone"`
	Type             string `json:"type"` // public, private
}

type SecurityGroupConfig struct {
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	IngressRules []SecurityGroupRule `json:"ingress_rules"`
	EgressRules  []SecurityGroupRule `json:"egress_rules"`
}

type SecurityGroupRule struct {
	Protocol   string   `json:"protocol"`
	SourceSG   string   `json:"source_sg,omitempty"`
	CIDRBlocks []string `json:"cidr_blocks,omitempty"`
	FromPort   int      `json:"from_port"`
	ToPort     int      `json:"to_port"`
}

type AuthorizerConfig struct {
	Name string `json:"name"`
	Type string `json:"type"`
	URI  string `json:"uri,omitempty"`
}

type APIKeyConfig struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type AttributeConfig struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type GlobalIndexConfig struct {
	Name       string           `json:"name"`
	HashKey    string           `json:"hash_key"`
	RangeKey   string           `json:"range_key,omitempty"`
	Projection ProjectionConfig `json:"projection"`
}

type LocalIndexConfig struct {
	Name       string           `json:"name"`
	RangeKey   string           `json:"range_key"`
	Projection ProjectionConfig `json:"projection"`
}

type ProjectionConfig struct {
	Type       string   `json:"type"` // ALL, KEYS_ONLY, INCLUDE
	Attributes []string `json:"attributes,omitempty"`
}

type LogGroupConfig struct {
	Name          string `json:"name"`
	KMSKeyId      string `json:"kms_key_id,omitempty"`
	RetentionDays int    `json:"retention_days"`
}

type MetricFilterConfig struct {
	Name            string `json:"name"`
	LogGroupName    string `json:"log_group_name"`
	FilterPattern   string `json:"filter_pattern"`
	MetricName      string `json:"metric_name"`
	MetricNamespace string `json:"metric_namespace"`
	MetricValue     string `json:"metric_value"`
}

type InlinePolicyConfig struct {
	Policy map[string]any `json:"policy"`
	Name   string         `json:"name"`
}

// InfrastructureGenerator generates infrastructure templates
type InfrastructureGenerator struct {
	provider InfrastructureProvider
	config   InfrastructureConfig
	mu       sync.RWMutex
}

// NewInfrastructureGenerator creates a new infrastructure generator
func NewInfrastructureGenerator(provider InfrastructureProvider, config InfrastructureConfig) *InfrastructureGenerator {
	return &InfrastructureGenerator{
		provider: provider,
		config:   config,
	}
}

// GenerateTemplate generates a complete infrastructure template
func (ig *InfrastructureGenerator) GenerateTemplate() (*InfrastructureTemplate, error) {
	ig.mu.RLock()
	defer ig.mu.RUnlock()

	template := &InfrastructureTemplate{
		Provider:    ig.provider,
		Name:        fmt.Sprintf("%s-%s", ig.config.ApplicationName, ig.config.Environment),
		Version:     "1.0.0",
		Description: fmt.Sprintf("Infrastructure template for %s in %s environment", ig.config.ApplicationName, ig.config.Environment),
		Resources:   make(map[string]Resource),
		Outputs:     make(map[string]Output),
		Parameters:  make(map[string]Parameter),
		Metadata:    ig.config.Metadata,
		Tags:        ig.config.Tags,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Generate resources based on configuration
	if err := ig.generateLambdaResources(template); err != nil {
		return nil, fmt.Errorf("failed to generate Lambda resources: %w", err)
	}

	if err := ig.generateAPIGatewayResources(template); err != nil {
		return nil, fmt.Errorf("failed to generate API Gateway resources: %w", err)
	}

	if err := ig.generateDatabaseResources(template); err != nil {
		return nil, fmt.Errorf("failed to generate database resources: %w", err)
	}

	if err := ig.generateMonitoringResources(template); err != nil {
		return nil, fmt.Errorf("failed to generate monitoring resources: %w", err)
	}

	if err := ig.generateSecurityResources(template); err != nil {
		return nil, fmt.Errorf("failed to generate security resources: %w", err)
	}

	if err := ig.generateNetworkingResources(template); err != nil {
		return nil, fmt.Errorf("failed to generate networking resources: %w", err)
	}

	// Generate outputs
	ig.generateOutputs(template)

	// Generate parameters
	ig.generateParameters(template)

	return template, nil
}

// generateLambdaResources generates Lambda function resources
func (ig *InfrastructureGenerator) generateLambdaResources(template *InfrastructureTemplate) error {
	lambdaConfig := ig.config.Lambda

	// Lambda execution role
	executionRole := Resource{
		Type: "AWS::IAM::Role",
		Name: fmt.Sprintf("%s-lambda-execution-role", ig.config.ApplicationName),
		Properties: map[string]any{
			"AssumeRolePolicyDocument": map[string]any{
				"Version": "2012-10-17",
				"Statement": []map[string]any{
					{
						"Effect": "Allow",
						"Principal": map[string]any{
							"Service": "lambda.amazonaws.com",
						},
						"Action": "sts:AssumeRole",
					},
				},
			},
			"ManagedPolicyArns": []string{
				"arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole",
			},
		},
		Tags: ig.config.Tags,
	}

	if lambdaConfig.VPCConfig != nil {
		managedPoliciesInterface, ok := executionRole.Properties["ManagedPolicyArns"]
		if !ok {
			return fmt.Errorf("ManagedPolicyArns not found in execution role")
		}
		managedPolicies, ok := managedPoliciesInterface.([]string)
		if !ok {
			return fmt.Errorf("ManagedPolicyArns is not a string slice")
		}
		managedPolicies = append(managedPolicies, "arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole")
		executionRole.Properties["ManagedPolicyArns"] = managedPolicies
	}

	template.Resources["LambdaExecutionRole"] = executionRole

	// Lambda function
	lambdaFunction := Resource{
		Type: "AWS::Lambda::Function",
		Name: fmt.Sprintf("%s-function", ig.config.ApplicationName),
		Properties: map[string]any{
			"FunctionName": fmt.Sprintf("%s-%s", ig.config.ApplicationName, ig.config.Environment),
			"Runtime":      lambdaConfig.Runtime,
			"Handler":      lambdaConfig.Handler,
			"Timeout":      lambdaConfig.Timeout,
			"MemorySize":   lambdaConfig.MemorySize,
			"Role":         fmt.Sprintf("${%s.Arn}", "LambdaExecutionRole"),
			"Code": map[string]any{
				"ZipFile": "// Placeholder code - replace with actual deployment package",
			},
		},
		Dependencies: []string{"LambdaExecutionRole"},
		Tags:         ig.config.Tags,
	}

	if len(lambdaConfig.Environment) > 0 {
		lambdaFunction.Properties["Environment"] = map[string]any{
			"Variables": lambdaConfig.Environment,
		}
	}

	if lambdaConfig.ReservedConcurrency != nil {
		lambdaFunction.Properties["ReservedConcurrencyLimit"] = *lambdaConfig.ReservedConcurrency
	}

	if len(lambdaConfig.Layers) > 0 {
		lambdaFunction.Properties["Layers"] = lambdaConfig.Layers
	}

	if lambdaConfig.VPCConfig != nil {
		lambdaFunction.Properties["VpcConfig"] = map[string]any{
			"SecurityGroupIds": []string{"${SecurityGroup.GroupId}"},
			"SubnetIds":        []string{"${PrivateSubnet1.SubnetId}", "${PrivateSubnet2.SubnetId}"},
		}
		lambdaFunction.Dependencies = append(lambdaFunction.Dependencies, "SecurityGroup", "PrivateSubnet1", "PrivateSubnet2")
	}

	template.Resources["LambdaFunction"] = lambdaFunction

	// Dead Letter Queue if enabled
	if lambdaConfig.DeadLetterQueue {
		dlq := Resource{
			Type: "AWS::SQS::Queue",
			Name: fmt.Sprintf("%s-dlq", ig.config.ApplicationName),
			Properties: map[string]any{
				"QueueName":                fmt.Sprintf("%s-%s-dlq", ig.config.ApplicationName, ig.config.Environment),
				"MessageRetentionPeriod":   1209600, // 14 days
				"VisibilityTimeoutSeconds": 60,
			},
			Tags: ig.config.Tags,
		}

		template.Resources["DeadLetterQueue"] = dlq

		// Update Lambda function to use DLQ
		lambdaProps := lambdaFunction.Properties
		lambdaProps["DeadLetterConfig"] = map[string]any{
			"TargetArn": fmt.Sprintf("${%s.Arn}", "DeadLetterQueue"),
		}
		lambdaFunction.Dependencies = append(lambdaFunction.Dependencies, "DeadLetterQueue")
		template.Resources["LambdaFunction"] = lambdaFunction
	}

	return nil
}

// generateAPIGatewayResources generates API Gateway resources
func (ig *InfrastructureGenerator) generateAPIGatewayResources(template *InfrastructureTemplate) error {
	apiConfig := ig.config.APIGateway

	var apiResource Resource

	switch apiConfig.Type {
	case "HTTP":
		apiResource = Resource{
			Type: "AWS::ApiGatewayV2::Api",
			Name: fmt.Sprintf("%s-api", ig.config.ApplicationName),
			Properties: map[string]any{
				"Name":         fmt.Sprintf("%s-%s-api", ig.config.ApplicationName, ig.config.Environment),
				"ProtocolType": "HTTP",
				"Description":  fmt.Sprintf("HTTP API for %s", ig.config.ApplicationName),
			},
			Tags: ig.config.Tags,
		}

		// CORS configuration for HTTP API
		if len(apiConfig.CORS.AllowOrigins) > 0 {
			apiResource.Properties["CorsConfiguration"] = map[string]any{
				"AllowOrigins":     apiConfig.CORS.AllowOrigins,
				"AllowMethods":     apiConfig.CORS.AllowMethods,
				"AllowHeaders":     apiConfig.CORS.AllowHeaders,
				"ExposeHeaders":    apiConfig.CORS.ExposeHeaders,
				"MaxAge":           apiConfig.CORS.MaxAge,
				"AllowCredentials": apiConfig.CORS.AllowCredentials,
			}
		}

	default: // REST API
		apiResource = Resource{
			Type: "AWS::ApiGateway::RestApi",
			Name: fmt.Sprintf("%s-api", ig.config.ApplicationName),
			Properties: map[string]any{
				"Name":        fmt.Sprintf("%s-%s-api", ig.config.ApplicationName, ig.config.Environment),
				"Description": fmt.Sprintf("REST API for %s", ig.config.ApplicationName),
				"EndpointConfiguration": map[string]any{
					"Types": []string{"REGIONAL"},
				},
			},
			Tags: ig.config.Tags,
		}
	}

	template.Resources["APIGateway"] = apiResource

	// API Gateway deployment
	deployment := Resource{
		Type: "AWS::ApiGateway::Deployment",
		Name: fmt.Sprintf("%s-deployment", ig.config.ApplicationName),
		Properties: map[string]any{
			"RestApiId": fmt.Sprintf("${%s.RestApiId}", "APIGateway"),
			"StageName": apiConfig.StageName,
		},
		Dependencies: []string{"APIGateway"},
		Tags:         ig.config.Tags,
	}

	template.Resources["APIDeployment"] = deployment

	// Lambda permission for API Gateway
	lambdaPermission := Resource{
		Type: "AWS::Lambda::Permission",
		Name: fmt.Sprintf("%s-lambda-permission", ig.config.ApplicationName),
		Properties: map[string]any{
			"FunctionName": fmt.Sprintf("${%s.FunctionName}", "LambdaFunction"),
			"Action":       "lambda:InvokeFunction",
			"Principal":    "apigateway.amazonaws.com",
			"SourceArn":    fmt.Sprintf("${%s.ExecutionArn}/*/*", "APIGateway"),
		},
		Dependencies: []string{"LambdaFunction", "APIGateway"},
	}

	template.Resources["LambdaPermission"] = lambdaPermission

	return nil
}

// generateDatabaseResources generates database resources
func (ig *InfrastructureGenerator) generateDatabaseResources(template *InfrastructureTemplate) error {
	dbConfig := ig.config.Database

	if dbConfig.Type == "DynamoDB" {
		builder := newDynamoDBResourceBuilder(ig, dbConfig)
		return builder.generateTables(template)
	}

	return nil
}

// dynamoDBResourceBuilder builds DynamoDB table resources
type dynamoDBResourceBuilder struct {
	ig       *InfrastructureGenerator
	dbConfig DatabaseConfig
}

// newDynamoDBResourceBuilder creates a new DynamoDB resource builder
func newDynamoDBResourceBuilder(ig *InfrastructureGenerator, dbConfig DatabaseConfig) *dynamoDBResourceBuilder {
	return &dynamoDBResourceBuilder{
		ig:       ig,
		dbConfig: dbConfig,
	}
}

// generateTables generates all DynamoDB table resources
func (drb *dynamoDBResourceBuilder) generateTables(template *InfrastructureTemplate) error {
	for _, tableConfig := range drb.dbConfig.Tables {
		tableBuilder := newDynamoDBTableBuilder(drb.ig, drb.dbConfig, tableConfig)
		table := tableBuilder.build()

		tableName := fmt.Sprintf("DynamoTable%s", cases.Title(language.English).String(tableConfig.Name))
		template.Resources[tableName] = table
	}
	return nil
}

// dynamoDBTableBuilder builds individual DynamoDB table resources
type dynamoDBTableBuilder struct {
	ig          *InfrastructureGenerator
	dbConfig    DatabaseConfig
	tableConfig TableConfig
}

// newDynamoDBTableBuilder creates a new DynamoDB table builder
func newDynamoDBTableBuilder(ig *InfrastructureGenerator, dbConfig DatabaseConfig, tableConfig TableConfig) *dynamoDBTableBuilder {
	return &dynamoDBTableBuilder{
		ig:          ig,
		dbConfig:    dbConfig,
		tableConfig: tableConfig,
	}
}

// build creates a complete DynamoDB table resource
func (dtb *dynamoDBTableBuilder) build() Resource {
	table := dtb.buildBaseTable()
	dtb.configureStreaming(&table)
	dtb.configureEncryption(&table)
	dtb.configureBackup(&table)
	dtb.configureGlobalIndexes(&table)
	return table
}

// buildBaseTable creates the base table resource
func (dtb *dynamoDBTableBuilder) buildBaseTable() Resource {
	return Resource{
		Type: "AWS::DynamoDB::Table",
		Name: fmt.Sprintf("%s-%s", dtb.ig.config.ApplicationName, dtb.tableConfig.Name),
		Properties: map[string]any{
			"TableName":            fmt.Sprintf("%s-%s-%s", dtb.ig.config.ApplicationName, dtb.ig.config.Environment, dtb.tableConfig.Name),
			"BillingMode":          dtb.tableConfig.BillingMode,
			"AttributeDefinitions": dtb.ig.generateAttributeDefinitions(dtb.tableConfig.Attributes),
			"KeySchema":            dtb.ig.generateKeySchema(dtb.tableConfig.HashKey, dtb.tableConfig.RangeKey),
		},
		Tags: dtb.ig.config.Tags,
	}
}

// configureStreaming adds stream configuration if enabled
func (dtb *dynamoDBTableBuilder) configureStreaming(table *Resource) {
	if dtb.tableConfig.StreamEnabled {
		table.Properties["StreamSpecification"] = map[string]any{
			"StreamViewType": "NEW_AND_OLD_IMAGES",
		}
	}
}

// configureEncryption adds encryption configuration if enabled
func (dtb *dynamoDBTableBuilder) configureEncryption(table *Resource) {
	if !dtb.dbConfig.Encryption.Enabled {
		return
	}

	sseSpec := map[string]any{
		"SSEEnabled": true,
	}

	if dtb.dbConfig.Encryption.KMSKeyId != "" {
		sseSpec["KMSMasterKeyId"] = dtb.dbConfig.Encryption.KMSKeyId
	}

	table.Properties["SSESpecification"] = sseSpec
}

// configureBackup adds backup configuration if enabled
func (dtb *dynamoDBTableBuilder) configureBackup(table *Resource) {
	if dtb.tableConfig.BackupEnabled {
		table.Properties["PointInTimeRecoverySpecification"] = map[string]any{
			"PointInTimeRecoveryEnabled": true,
		}
	}
}

// configureGlobalIndexes adds global secondary indexes if configured
func (dtb *dynamoDBTableBuilder) configureGlobalIndexes(table *Resource) {
	if len(dtb.tableConfig.GlobalIndexes) == 0 {
		return
	}

	gsiBuilder := newGlobalSecondaryIndexBuilder(dtb.ig, dtb.tableConfig.GlobalIndexes)
	gsis := gsiBuilder.buildAll()
	table.Properties["GlobalSecondaryIndexes"] = gsis
}

// globalSecondaryIndexBuilder builds GSI configurations
type globalSecondaryIndexBuilder struct {
	ig         *InfrastructureGenerator
	gsiConfigs []GlobalIndexConfig
}

// newGlobalSecondaryIndexBuilder creates a new GSI builder
func newGlobalSecondaryIndexBuilder(ig *InfrastructureGenerator, gsiConfigs []GlobalIndexConfig) *globalSecondaryIndexBuilder {
	return &globalSecondaryIndexBuilder{
		ig:         ig,
		gsiConfigs: gsiConfigs,
	}
}

// buildAll creates all GSI configurations
func (gsib *globalSecondaryIndexBuilder) buildAll() []map[string]any {
	gsis := make([]map[string]any, 0, len(gsib.gsiConfigs))

	for _, gsi := range gsib.gsiConfigs {
		gsiDef := gsib.buildSingleGSI(gsi)
		gsis = append(gsis, gsiDef)
	}

	return gsis
}

// buildSingleGSI creates a single GSI configuration
func (gsib *globalSecondaryIndexBuilder) buildSingleGSI(gsi GlobalIndexConfig) map[string]any {
	gsiDef := map[string]any{
		"IndexName": gsi.Name,
		"KeySchema": gsib.ig.generateKeySchema(gsi.HashKey, gsi.RangeKey),
		"Projection": map[string]any{
			"ProjectionType": gsi.Projection.Type,
		},
	}

	gsib.configureGSIProjection(gsiDef, gsi)
	return gsiDef
}

// configureGSIProjection configures GSI projection attributes
func (gsib *globalSecondaryIndexBuilder) configureGSIProjection(gsiDef map[string]any, gsi GlobalIndexConfig) {
	if gsi.Projection.Type == "INCLUDE" && len(gsi.Projection.Attributes) > 0 {
		if projection, ok := gsiDef["Projection"].(map[string]any); ok {
			projection["NonKeyAttributes"] = gsi.Projection.Attributes
		}
	}
}

// generateMonitoringResources generates monitoring resources
func (ig *InfrastructureGenerator) generateMonitoringResources(template *InfrastructureTemplate) error {
	monConfig := ig.config.Monitoring

	// CloudWatch Log Groups
	for _, logGroup := range monConfig.CloudWatch.LogGroups {
		logGroupName := fmt.Sprintf("LogGroup%s", cases.Title(language.English).String(strings.ReplaceAll(logGroup.Name, "-", "")))

		logGroupResource := Resource{
			Type: "AWS::Logs::LogGroup",
			Name: logGroup.Name,
			Properties: map[string]any{
				"LogGroupName":    fmt.Sprintf("/aws/lambda/%s-%s", ig.config.ApplicationName, ig.config.Environment),
				"RetentionInDays": logGroup.RetentionDays,
			},
			Tags: ig.config.Tags,
		}

		if logGroup.KMSKeyId != "" {
			logGroupResource.Properties["KmsKeyId"] = logGroup.KMSKeyId
		}

		template.Resources[logGroupName] = logGroupResource
	}

	// CloudWatch Alarms
	for _, alarm := range monConfig.Alarms {
		alarmName := fmt.Sprintf("Alarm%s", cases.Title(language.English).String(strings.ReplaceAll(alarm.Name, "-", "")))

		alarmResource := Resource{
			Type: "AWS::CloudWatch::Alarm",
			Name: alarm.Name,
			Properties: map[string]any{
				"AlarmName":          fmt.Sprintf("%s-%s-%s", ig.config.ApplicationName, ig.config.Environment, alarm.Name),
				"AlarmDescription":   fmt.Sprintf("Alarm for %s", alarm.MetricName),
				"MetricName":         alarm.MetricName,
				"Namespace":          alarm.Namespace,
				"Statistic":          alarm.Statistic,
				"Threshold":          alarm.Threshold,
				"ComparisonOperator": alarm.ComparisonOperator,
				"EvaluationPeriods":  alarm.EvaluationPeriods,
				"Period":             alarm.Period,
			},
			Tags: ig.config.Tags,
		}

		if len(alarm.Actions) > 0 {
			alarmResource.Properties["AlarmActions"] = alarm.Actions
		}

		template.Resources[alarmName] = alarmResource
	}

	return nil
}

// generateSecurityResources generates security resources
func (ig *InfrastructureGenerator) generateSecurityResources(template *InfrastructureTemplate) error {
	secConfig := ig.config.Security

	// IAM Roles
	for _, role := range secConfig.IAMRoles {
		roleName := fmt.Sprintf("IAMRole%s", cases.Title(language.English).String(strings.ReplaceAll(role.Name, "-", "")))

		roleResource := Resource{
			Type: "AWS::IAM::Role",
			Name: role.Name,
			Properties: map[string]any{
				"RoleName":                 fmt.Sprintf("%s-%s-%s", ig.config.ApplicationName, ig.config.Environment, role.Name),
				"AssumeRolePolicyDocument": role.AssumeRolePolicy,
				"ManagedPolicyArns":        role.Policies,
			},
			Tags: ig.config.Tags,
		}

		if len(role.InlinePolicies) > 0 {
			policies := make([]map[string]any, 0, len(role.InlinePolicies))
			for _, policy := range role.InlinePolicies {
				policies = append(policies, map[string]any{
					"PolicyName":     policy.Name,
					"PolicyDocument": policy.Policy,
				})
			}
			roleResource.Properties["Policies"] = policies
		}

		template.Resources[roleName] = roleResource
	}

	// KMS Keys
	for _, key := range secConfig.KMSKeys {
		keyName := fmt.Sprintf("KMSKey%s", cases.Title(language.English).String(strings.ReplaceAll(key.Alias, "-", "")))

		keyResource := Resource{
			Type: "AWS::KMS::Key",
			Name: key.Alias,
			Properties: map[string]any{
				"Description": key.Description,
				"KeyPolicy": map[string]any{
					"Version": "2012-10-17",
					"Statement": []map[string]any{
						{
							"Effect": "Allow",
							"Principal": map[string]any{
								"AWS": "arn:aws:iam::${AWS::AccountId}:root",
							},
							"Action":   "kms:*",
							"Resource": "*",
						},
					},
				},
			},
			Tags: ig.config.Tags,
		}

		if key.Policy != nil {
			keyResource.Properties["KeyPolicy"] = key.Policy
		}

		template.Resources[keyName] = keyResource

		// KMS Alias
		aliasResource := Resource{
			Type: "AWS::KMS::Alias",
			Name: fmt.Sprintf("%s-alias", key.Alias),
			Properties: map[string]any{
				"AliasName":   fmt.Sprintf("alias/%s-%s-%s", ig.config.ApplicationName, ig.config.Environment, key.Alias),
				"TargetKeyId": fmt.Sprintf("${%s.KeyId}", keyName),
			},
			Dependencies: []string{keyName},
		}

		template.Resources[fmt.Sprintf("%sAlias", keyName)] = aliasResource
	}

	return nil
}

// generateNetworkingResources generates networking resources
func (ig *InfrastructureGenerator) generateNetworkingResources(template *InfrastructureTemplate) error {
	netConfig := ig.config.Networking

	if netConfig.VPC != nil {
		// VPC
		vpc := Resource{
			Type: "AWS::EC2::VPC",
			Name: fmt.Sprintf("%s-vpc", ig.config.ApplicationName),
			Properties: map[string]any{
				"CidrBlock":          netConfig.VPC.CIDR,
				"EnableDnsSupport":   netConfig.VPC.EnableDNSSupport,
				"EnableDnsHostnames": netConfig.VPC.EnableDNSHostnames,
			},
			Tags: ig.config.Tags,
		}

		template.Resources["VPC"] = vpc

		// Internet Gateway
		igw := Resource{
			Type:       "AWS::EC2::InternetGateway",
			Name:       fmt.Sprintf("%s-igw", ig.config.ApplicationName),
			Properties: map[string]any{},
			Tags:       ig.config.Tags,
		}

		template.Resources["InternetGateway"] = igw

		// VPC Gateway Attachment
		vpcGwAttachment := Resource{
			Type: "AWS::EC2::VPCGatewayAttachment",
			Name: fmt.Sprintf("%s-vpc-gw-attachment", ig.config.ApplicationName),
			Properties: map[string]any{
				"VpcId":             fmt.Sprintf("${%s.VpcId}", "VPC"),
				"InternetGatewayId": fmt.Sprintf("${%s.InternetGatewayId}", "InternetGateway"),
			},
			Dependencies: []string{"VPC", "InternetGateway"},
		}

		template.Resources["VPCGatewayAttachment"] = vpcGwAttachment

		// Subnets
		for i, subnet := range netConfig.Subnets {
			subnetName := fmt.Sprintf("%sSubnet%d", cases.Title(language.English).String(subnet.Type), i+1)

			subnetResource := Resource{
				Type: "AWS::EC2::Subnet",
				Name: subnet.Name,
				Properties: map[string]any{
					"VpcId":            fmt.Sprintf("${%s.VpcId}", "VPC"),
					"CidrBlock":        subnet.CIDR,
					"AvailabilityZone": subnet.AvailabilityZone,
				},
				Dependencies: []string{"VPC"},
				Tags:         ig.config.Tags,
			}

			if subnet.Type == "public" {
				subnetResource.Properties["MapPublicIpOnLaunch"] = true
			}

			template.Resources[subnetName] = subnetResource
		}

		// Security Groups
		for _, sg := range netConfig.SecurityGroups {
			sgName := fmt.Sprintf("SecurityGroup%s", cases.Title(language.English).String(strings.ReplaceAll(sg.Name, "-", "")))

			sgResource := Resource{
				Type: "AWS::EC2::SecurityGroup",
				Name: sg.Name,
				Properties: map[string]any{
					"GroupName":        fmt.Sprintf("%s-%s-%s", ig.config.ApplicationName, ig.config.Environment, sg.Name),
					"GroupDescription": sg.Description,
					"VpcId":            fmt.Sprintf("${%s.VpcId}", "VPC"),
				},
				Dependencies: []string{"VPC"},
				Tags:         ig.config.Tags,
			}

			if len(sg.IngressRules) > 0 {
				ingressRules := make([]map[string]any, 0, len(sg.IngressRules))
				for _, rule := range sg.IngressRules {
					ingressRule := map[string]any{
						"IpProtocol": rule.Protocol,
						"FromPort":   rule.FromPort,
						"ToPort":     rule.ToPort,
					}

					if len(rule.CIDRBlocks) > 0 {
						ingressRule["CidrIp"] = rule.CIDRBlocks[0] // Simplified for single CIDR
					}

					if rule.SourceSG != "" {
						ingressRule["SourceSecurityGroupId"] = rule.SourceSG
					}

					ingressRules = append(ingressRules, ingressRule)
				}
				sgResource.Properties["SecurityGroupIngress"] = ingressRules
			}

			template.Resources[sgName] = sgResource
		}
	}

	return nil
}

// generateOutputs generates template outputs
func (ig *InfrastructureGenerator) generateOutputs(template *InfrastructureTemplate) {
	// Lambda Function ARN
	template.Outputs["LambdaFunctionArn"] = Output{
		Value:       fmt.Sprintf("${%s.Arn}", "LambdaFunction"),
		Description: "ARN of the Lambda function",
		Export:      true,
	}

	// API Gateway URL
	template.Outputs["APIGatewayURL"] = Output{
		Value:       fmt.Sprintf("https://${%s.RestApiId}.execute-api.${AWS::Region}.amazonaws.com/${%s.StageName}", "APIGateway", "APIDeployment"),
		Description: "URL of the API Gateway",
		Export:      true,
	}

	// DynamoDB Table Names
	for _, tableConfig := range ig.config.Database.Tables {
		tableName := fmt.Sprintf("DynamoTable%s", cases.Title(language.English).String(tableConfig.Name))
		outputName := fmt.Sprintf("%sTableName", cases.Title(language.English).String(tableConfig.Name))

		template.Outputs[outputName] = Output{
			Value:       fmt.Sprintf("${%s.TableName}", tableName),
			Description: fmt.Sprintf("Name of the %s DynamoDB table", tableConfig.Name),
			Export:      true,
		}
	}
}

// generateParameters generates template parameters
func (ig *InfrastructureGenerator) generateParameters(template *InfrastructureTemplate) {
	template.Parameters["Environment"] = Parameter{
		Type:          "String",
		Description:   "Environment name",
		Default:       ig.config.Environment,
		AllowedValues: []any{"dev", "staging", "prod"},
	}

	template.Parameters["ApplicationName"] = Parameter{
		Type:        "String",
		Description: "Name of the application",
		Default:     ig.config.ApplicationName,
		MinLength:   &[]int{1}[0],
		MaxLength:   &[]int{64}[0],
	}
}

// Helper functions
func (ig *InfrastructureGenerator) generateAttributeDefinitions(attributes []AttributeConfig) []map[string]any {
	definitions := make([]map[string]any, 0, len(attributes))
	for _, attr := range attributes {
		definitions = append(definitions, map[string]any{
			"AttributeName": attr.Name,
			"AttributeType": attr.Type,
		})
	}
	return definitions
}

func (ig *InfrastructureGenerator) generateKeySchema(hashKey, rangeKey string) []map[string]any {
	schema := []map[string]any{
		{
			"AttributeName": hashKey,
			"KeyType":       "HASH",
		},
	}

	if rangeKey != "" {
		schema = append(schema, map[string]any{
			"AttributeName": rangeKey,
			"KeyType":       "RANGE",
		})
	}

	return schema
}

// ExportTemplate exports the template in the specified format
func (ig *InfrastructureGenerator) ExportTemplate(template *InfrastructureTemplate, format string) ([]byte, error) {
	switch strings.ToLower(format) {
	case "json":
		return json.MarshalIndent(template, "", "  ")
	case "yaml":
		// For YAML export, you would use a YAML library
		// For now, return JSON
		return json.MarshalIndent(template, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}

// ValidateTemplate validates the generated template
func (ig *InfrastructureGenerator) ValidateTemplate(template *InfrastructureTemplate) error {
	if template.Name == "" {
		return fmt.Errorf("template name is required")
	}

	if len(template.Resources) == 0 {
		return fmt.Errorf("template must contain at least one resource")
	}

	// Validate resource dependencies
	resourceNames := make(map[string]bool)
	for name := range template.Resources {
		resourceNames[name] = true
	}

	for name, resource := range template.Resources {
		for _, dep := range resource.Dependencies {
			if !resourceNames[dep] {
				return fmt.Errorf("resource %s has invalid dependency: %s", name, dep)
			}
		}
	}

	return nil
}
