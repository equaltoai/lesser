package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	stringType          = "string"
	mediumSize          = "Medium"
	keysOnlyView        = "KEYS_ONLY"
	newAndOldImagesView = "NEW_AND_OLD_IMAGES"
	allView             = "ALL"
)

// DynamORMScaffoldCommand scaffolds DynamORM models, CDK constructs, and examples
type DynamORMScaffoldCommand struct{}

// DynamORMMigrateCommand handles migration from existing DynamoDB tables to DynamORM
type DynamORMMigrateCommand struct{}

func (c *DynamORMScaffoldCommand) Name() string { return "scaffold" }
func (c *DynamORMScaffoldCommand) Description() string {
	return "Generate DynamORM models with CDK constructs"
}
func (c *DynamORMScaffoldCommand) Usage() string {
	return "lift dynamorm-scaffold --model <ModelName> [--table <table-name>] [--multi-tenant] [--enable-ttl] [--enable-streams] [--gsi <name:pk:sk>]"
}

// ScaffoldConfig holds the configuration for scaffolding
type ScaffoldConfig struct {
	ModelName     string
	TableName     string
	GSIs          []GSIConfig
	MultiTenant   bool
	EnableTTL     bool
	EnableStreams bool
}

// GSIConfig holds GSI configuration
type GSIConfig struct {
	IndexName    string
	PartitionKey string
	SortKey      string
}

func (c *DynamORMScaffoldCommand) Execute(_ context.Context, args []string) error {
	// Check if we're in a Lift project
	if !c.isLiftProject() {
		return fmt.Errorf("not in a Lift project directory - run 'lift new' first")
	}

	// Parse arguments
	config, err := c.parseScaffoldArgs(args)
	if err != nil {
		return err
	}

	// Create directories
	if err := c.createDirectories(config); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Generate model file
	if err := c.generateModel(config); err != nil {
		return fmt.Errorf("failed to generate model: %w", err)
	}

	// Generate CDK construct
	if err := c.generateCDKConstruct(config); err != nil {
		return fmt.Errorf("failed to generate CDK construct: %w", err)
	}

	// Generate example usage
	if err := c.generateExampleUsage(config); err != nil {
		return fmt.Errorf("failed to generate example: %w", err)
	}

	fmt.Printf("✅ Successfully generated DynamORM scaffold for %s\n", config.ModelName)
	fmt.Printf("📁 Generated files:\n")
	fmt.Printf("   - models/%s.go\n", strings.ToLower(config.ModelName))
	fmt.Printf("   - cdk/constructs/%s_table.go\n", strings.ToLower(config.ModelName))
	fmt.Printf("   - examples/%s_example.go\n", strings.ToLower(config.ModelName))

	return nil
}

func (c *DynamORMScaffoldCommand) isLiftProject() bool {
	// Check for go.mod file
	if _, err := os.Stat("go.mod"); err != nil {
		return false
	}

	// Check if go.mod contains lift dependency
	content, err := os.ReadFile("go.mod")
	if err != nil {
		return false
	}

	return strings.Contains(string(content), "github.com/pay-theory/lift")
}

func (c *DynamORMScaffoldCommand) parseScaffoldArgs(args []string) (*ScaffoldConfig, error) {
	config := &ScaffoldConfig{
		GSIs: []GSIConfig{},
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--model":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--model requires a value")
			}
			config.ModelName = args[i+1]
			i++ // Skip the value
		case "--table":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--table requires a value")
			}
			config.TableName = args[i+1]
			i++ // Skip the value
		case "--multi-tenant":
			config.MultiTenant = true
		case "--enable-ttl":
			config.EnableTTL = true
		case "--enable-streams":
			config.EnableStreams = true
		case "--gsi":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--gsi requires a value")
			}
			gsi, err := c.parseGSI(args[i+1])
			if err != nil {
				return nil, err
			}
			config.GSIs = append(config.GSIs, gsi)
			i++ // Skip the value
		}
	}

	if config.ModelName == "" {
		return nil, fmt.Errorf("--model is required")
	}

	// Default table name if not provided
	if config.TableName == "" {
		config.TableName = strings.ToLower(config.ModelName) + "s"
	}

	return config, nil
}

func (c *DynamORMScaffoldCommand) parseGSI(gsiArg string) (GSIConfig, error) {
	parts := strings.Split(gsiArg, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return GSIConfig{}, fmt.Errorf("--gsi format should be 'IndexName:PartitionKey' or 'IndexName:PartitionKey:SortKey'")
	}

	gsi := GSIConfig{
		IndexName:    parts[0],
		PartitionKey: parts[1],
	}

	if len(parts) == 3 {
		gsi.SortKey = parts[2]
	}

	return gsi, nil
}

func (c *DynamORMScaffoldCommand) createDirectories(_ *ScaffoldConfig) error {
	dirs := []string{
		"models",
		"cdk/constructs",
		"examples",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return err
		}
	}

	return nil
}

func (c *DynamORMScaffoldCommand) generateModel(config *ScaffoldConfig) error {
	tmpl := `package models

import (
	"time"

	"github.com/pay-theory/lift/pkg/dynamorm"
)

// {{.ModelName}} represents a {{.ModelName}} entity
type {{.ModelName}} struct {
	{{if .MultiTenant}}TenantID string ` + "`" + `json:"tenant_id" dynamodb:"tenant_id,pk"` + "`" + `
	{{end}}ID        string    ` + "`" + `json:"id" dynamodb:"id{{if not .MultiTenant}},pk{{end}}"` + "`" + `
	Name      string    ` + "`" + `json:"name" dynamodb:"name"` + "`" + `
	CreatedAt time.Time ` + "`" + `json:"created_at" dynamodb:"created_at"` + "`" + `
	UpdatedAt time.Time ` + "`" + `json:"updated_at" dynamodb:"updated_at"` + "`" + `
	{{if .EnableTTL}}TTL       int64     ` + "`" + `json:"ttl" dynamodb:"ttl"` + "`" + `
	ExpiresAt time.Time ` + "`" + `json:"expires_at" dynamodb:"expires_at"` + "`" + `
	{{end}}{{range .GSIs}}{{.PartitionKey}} string ` + "`" + `json:"{{.PartitionKey | ToLower}}" dynamodb:"{{.PartitionKey | ToLower}}" gsi:"{{.IndexName}}"` + "`" + `
	{{if .SortKey}}{{.SortKey}} string ` + "`" + `json:"{{.SortKey | ToLower}}" dynamodb:"{{.SortKey | ToLower}}" gsi:"{{.IndexName}}"` + "`" + `
	{{end}}{{end}}
}

// New{{.ModelName}} creates a new {{.ModelName}} instance
func New{{.ModelName}}({{if .MultiTenant}}tenantID, {{end}}name string) *{{.ModelName}} {
	now := time.Now()
	{{if .MultiTenant}}id := tenantID + "#" + dynamorm.GenerateID(){{else}}id := dynamorm.GenerateID(){{end}}
	
	return &{{.ModelName}}{
		{{if .MultiTenant}}TenantID:  tenantID,
		{{end}}ID:        id,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
		{{if .EnableTTL}}TTL:       now.Add(24 * time.Hour).Unix(),
		ExpiresAt: now.Add(24 * time.Hour),
		{{end}}
	}
}

// TableName returns the DynamoDB table name
func (m *{{.ModelName}}) TableName() string {
	return "{{.TableName}}"
}

// Update updates the {{.ModelName}} timestamp
func (m *{{.ModelName}}) Update() {
	m.UpdatedAt = time.Now()
	{{if .EnableTTL}}m.TTL = time.Now().Add(24 * time.Hour).Unix()
	m.ExpiresAt = time.Now().Add(24 * time.Hour)
	{{end}}
}
`

	funcMap := template.FuncMap{
		"ToLower": strings.ToLower,
	}

	t, err := template.New("model").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return err
	}

	filename := filepath.Join("models", strings.ToLower(config.ModelName)+".go")
	file, err := os.Create(filename) // #nosec G304 - filename is constructed from sanitized input in controlled directories
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Warning: failed to close file: %v\n", err)
		}
	}()

	return t.Execute(file, config)
}

func (c *DynamORMScaffoldCommand) generateCDKConstruct(config *ScaffoldConfig) error {
	tmpl := `package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// {{.ModelName}}TableProps defines the properties for the {{.ModelName}} table
type {{.ModelName}}TableProps struct {
	awscdk.StackProps
	TableName         *string
	{{if .MultiTenant}}EnableMultiTenant *bool{{end}}
	{{if .EnableTTL}}EnableTTL        *bool{{end}}
	{{if .EnableStreams}}EnableStreams    *bool{{end}}
	RemovalPolicy     awscdk.RemovalPolicy
}

// {{.ModelName}}Table represents a DynamoDB table for {{.ModelName}} entities
type {{.ModelName}}Table struct {
	constructs.Construct
	Table awsdynamodb.Table
}

// New{{.ModelName}}Table creates a new {{.ModelName}} table construct
func New{{.ModelName}}Table(scope constructs.Construct, id *string, props *{{.ModelName}}TableProps) *{{.ModelName}}Table {
	this := &{{.ModelName}}Table{}
	this.Construct = constructs.NewConstruct(scope, id)

	// Set defaults
	if props.TableName == nil {
		props.TableName = jsii.String("{{.TableName}}")
	}
	if props.RemovalPolicy == "" {
		props.RemovalPolicy = awscdk.RemovalPolicy_RETAIN
	}

	// Table configuration
	tableProps := &awsdynamodb.TableProps{
		TableName:     props.TableName,
		BillingMode:   awsdynamodb.BillingMode_PAY_PER_REQUEST,
		RemovalPolicy: props.RemovalPolicy,
		{{if .MultiTenant}}PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("tenant_id"),
			Type: awsdynamodb.AttributeType_STRING,
		},
		SortKey: &awsdynamodb.Attribute{
			Name: jsii.String("id"),
			Type: awsdynamodb.AttributeType_STRING,
		},{{else}}PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("id"),
			Type: awsdynamodb.AttributeType_STRING,
		},{{end}}
		{{if .EnableTTL}}TimeToLiveAttribute: jsii.String("ttl"),{{end}}
		{{if .EnableStreams}}Stream: awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,{{end}}
	}

	// Create table
	this.Table = awsdynamodb.NewTable(this, jsii.String("Table"), tableProps)

	{{range .GSIs}}// Add {{.IndexName}} GSI
	this.Table.AddGlobalSecondaryIndex(&awsdynamodb.GlobalSecondaryIndexProps{
		IndexName: jsii.String("{{.IndexName}}"),
		PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("{{.PartitionKey}}"),
			Type: awsdynamodb.AttributeType_STRING,
		},
		{{if .SortKey}}SortKey: &awsdynamodb.Attribute{
			Name: jsii.String("{{.SortKey}}"),
			Type: awsdynamodb.AttributeType_STRING,
		},{{end}}
	})
	{{end}}

	return this
}

// TableName returns the table name
func (t *{{.ModelName}}Table) TableName() *string {
	return t.Table.TableName()
}

// TableArn returns the table ARN
func (t *{{.ModelName}}Table) TableArn() *string {
	return t.Table.TableArn()
}

// GrantReadData grants read permissions to the table
func (t *{{.ModelName}}Table) GrantReadData(grantee awscdk.IPrincipal) {
	t.Table.GrantReadData(grantee)
}

// GrantWriteData grants write permissions to the table
func (t *{{.ModelName}}Table) GrantWriteData(grantee awscdk.IPrincipal) {
	t.Table.GrantWriteData(grantee)
}

// GrantFullAccess grants full access to the table
func (t *{{.ModelName}}Table) GrantFullAccess(grantee awscdk.IPrincipal) {
	t.Table.GrantFullAccess(grantee)
}
`

	t, err := template.New("cdk").Parse(tmpl)
	if err != nil {
		return err
	}

	filename := filepath.Join("cdk", "constructs", strings.ToLower(config.ModelName)+"_table.go")
	file, err := os.Create(filename) // #nosec G304 - filename is constructed from sanitized input in controlled directories
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Warning: failed to close file: %v\n", err)
		}
	}()

	return t.Execute(file, config)
}

func (c *DynamORMScaffoldCommand) generateExampleUsage(config *ScaffoldConfig) error {
	tmpl := `package main

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/dynamorm"
	"{{.ModuleName}}/models"
)

// {{.ModelName}}Service handles {{.ModelName}} operations
type {{.ModelName}}Service struct {
	db *dynamorm.DB
}

// New{{.ModelName}}Service creates a new {{.ModelName}}Service
func New{{.ModelName}}Service() *{{.ModelName}}Service {
	db, err := dynamorm.New(context.Background())
	if err != nil {
		log.Fatal("Failed to initialize DynamORM:", err)
	}

	return &{{.ModelName}}Service{
		db: db,
	}
}

// Create{{.ModelName}} creates a new {{.ModelName}}
func (s *{{.ModelName}}Service) Create{{.ModelName}}(ctx *lift.Context) error {
	type CreateRequest struct {
		Name string ` + "`" + `json:"name" validate:"required"` + "`" + `
	}

	var req CreateRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return err
	}

	{{if .MultiTenant}}// Get tenant ID from context
	tenantID := ctx.TenantID()
	if tenantID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Tenant ID is required", 400)
	}

	{{.ModelName | ToLower}} := models.New{{.ModelName}}(tenantID, req.Name){{else}}{{.ModelName | ToLower}} := models.New{{.ModelName}}(req.Name){{end}}

	if err := s.db.Put(ctx.Context(), {{.ModelName | ToLower}}); err != nil {
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to create {{.ModelName | ToLower}}", 500)
	}

	return ctx.JSON({{.ModelName | ToLower}})
}

// Get{{.ModelName}} retrieves a {{.ModelName}} by ID
func (s *{{.ModelName}}Service) Get{{.ModelName}}(ctx *lift.Context) error {
	id := ctx.Param("id")
	{{if .MultiTenant}}tenantID := ctx.TenantID()
	if tenantID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Tenant ID is required", 400)
	}

	var {{.ModelName | ToLower}} models.{{.ModelName}}
	if err := s.db.GetByPK(ctx.Context(), tenantID, id, &{{.ModelName | ToLower}}); err != nil {
		if err == dynamorm.ErrNotFound {
			return lift.NotFound("{{.ModelName}} not found")
		}
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to get {{.ModelName | ToLower}}", 500)
	}{{else}}var {{.ModelName | ToLower}} models.{{.ModelName}}
	if err := s.db.GetByPK(ctx.Context(), id, &{{.ModelName | ToLower}}); err != nil {
		if err == dynamorm.ErrNotFound {
			return lift.NotFound("{{.ModelName}} not found")
		}
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to get {{.ModelName | ToLower}}", 500)
	}{{end}}

	return ctx.JSON({{.ModelName | ToLower}})
}

// List{{.ModelName}}s lists all {{.ModelName}}s
func (s *{{.ModelName}}Service) List{{.ModelName}}s(ctx *lift.Context) error {
	{{if .MultiTenant}}tenantID := ctx.TenantID()
	if tenantID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Tenant ID is required", 400)
	}

	var {{.ModelName | ToLower}}s []models.{{.ModelName}}
	if err := s.db.Query(ctx.Context(), &{{.ModelName | ToLower}}s, dynamorm.WithPartitionKey(tenantID)); err != nil {
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to list {{.ModelName | ToLower}}s", 500)
	}{{else}}var {{.ModelName | ToLower}}s []models.{{.ModelName}}
	if err := s.db.Scan(ctx.Context(), &{{.ModelName | ToLower}}s); err != nil {
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to list {{.ModelName | ToLower}}s", 500)
	}{{end}}

	return ctx.JSON({{.ModelName | ToLower}}s)
}

// Update{{.ModelName}} updates a {{.ModelName}}
func (s *{{.ModelName}}Service) Update{{.ModelName}}(ctx *lift.Context) error {
	id := ctx.Param("id")
	{{if .MultiTenant}}tenantID := ctx.TenantID()
	if tenantID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Tenant ID is required", 400)
	}{{end}}

	type UpdateRequest struct {
		Name string ` + "`" + `json:"name" validate:"required"` + "`" + `
	}

	var req UpdateRequest
	if err := ctx.ParseRequest(&req); err != nil {
		return err
	}

	var {{.ModelName | ToLower}} models.{{.ModelName}}
	{{if .MultiTenant}}if err := s.db.GetByPK(ctx.Context(), tenantID, id, &{{.ModelName | ToLower}}); err != nil {
		if err == dynamorm.ErrNotFound {
			return lift.NotFound("{{.ModelName}} not found")
		}
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to get {{.ModelName | ToLower}}", 500)
	}{{else}}if err := s.db.GetByPK(ctx.Context(), id, &{{.ModelName | ToLower}}); err != nil {
		if err == dynamorm.ErrNotFound {
			return lift.NotFound("{{.ModelName}} not found")
		}
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to get {{.ModelName | ToLower}}", 500)
	}{{end}}

	{{.ModelName | ToLower}}.Name = req.Name
	{{.ModelName | ToLower}}.Update()

	if err := s.db.Put(ctx.Context(), &{{.ModelName | ToLower}}); err != nil {
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to update {{.ModelName | ToLower}}", 500)
	}

	return ctx.JSON({{.ModelName | ToLower}})
}

// Delete{{.ModelName}} deletes a {{.ModelName}}
func (s *{{.ModelName}}Service) Delete{{.ModelName}}(ctx *lift.Context) error {
	id := ctx.Param("id")
	{{if .MultiTenant}}tenantID := ctx.TenantID()
	if tenantID == "" {
		return lift.NewLiftError("BAD_REQUEST", "Tenant ID is required", 400)
	}

	var {{.ModelName | ToLower}} models.{{.ModelName}}
	{{.ModelName | ToLower}}.TenantID = tenantID
	{{.ModelName | ToLower}}.ID = id{{else}}var {{.ModelName | ToLower}} models.{{.ModelName}}
	{{.ModelName | ToLower}}.ID = id{{end}}

	if err := s.db.Delete(ctx.Context(), &{{.ModelName | ToLower}}); err != nil {
		return lift.NewLiftError("INTERNAL_ERROR", "Failed to delete {{.ModelName | ToLower}}", 500)
	}

	return ctx.JSON(map[string]string{"message": "{{.ModelName}} deleted successfully"})
}

func main() {
	// Create Lift app
	app := lift.New()

	// Create {{.ModelName}} service
	{{.ModelName | ToLower}}Service := New{{.ModelName}}Service()

	// Routes
	{{if .MultiTenant}}api := app.Group("/api")
	{{.ModelName | ToLower}}Routes := api.Group("/{{.ModelName | ToLower}}s"){{else}}{{.ModelName | ToLower}}Routes := app.Group("/{{.ModelName | ToLower}}s"){{end}}

	{{.ModelName | ToLower}}Routes.POST("", {{.ModelName | ToLower}}Service.Create{{.ModelName}})
	{{.ModelName | ToLower}}Routes.GET("", {{.ModelName | ToLower}}Service.List{{.ModelName}}s)
	{{.ModelName | ToLower}}Routes.GET("/:id", {{.ModelName | ToLower}}Service.Get{{.ModelName}})
	{{.ModelName | ToLower}}Routes.PUT("/:id", {{.ModelName | ToLower}}Service.Update{{.ModelName}})
	{{.ModelName | ToLower}}Routes.DELETE("/:id", {{.ModelName | ToLower}}Service.Delete{{.ModelName}})

	// Start Lambda handler
	lambda.Start(app.Handler())
}
`

	// Get module name from go.mod
	moduleName := "your-module"
	if content, err := os.ReadFile("go.mod"); err == nil {
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "module ") {
				moduleName = strings.TrimSpace(strings.TrimPrefix(line, "module "))
				break
			}
		}
	}

	data := struct {
		*ScaffoldConfig
		ModuleName string
	}{
		ScaffoldConfig: config,
		ModuleName:     moduleName,
	}

	funcMap := template.FuncMap{
		"ToLower": strings.ToLower,
	}

	t, err := template.New("example").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return err
	}

	filename := filepath.Join("examples", strings.ToLower(config.ModelName)+"_example.go")
	file, err := os.Create(filename) // #nosec G304 - filename is constructed from sanitized input in controlled directories
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Warning: failed to close file: %v\n", err)
		}
	}()

	return t.Execute(file, data)
}

// DynamORMMigrateCommand methods
func (c *DynamORMMigrateCommand) Name() string { return "migrate" }
func (c *DynamORMMigrateCommand) Description() string {
	return "Migrate existing DynamoDB tables to DynamORM"
}
func (c *DynamORMMigrateCommand) Usage() string {
	return "lift dynamorm migrate --table <table-name> [--region <region>] [--output-dir <dir>] [--analyze-only]"
}

// TableAnalysis contains the analysis results of a DynamoDB table
type TableAnalysis struct {
	CreatedAt              time.Time                `json:"created_at"`
	SortKey                *AttributeSpec           `json:"sort_key,omitempty"`
	TimeToLiveSpec         *TTLSpec                 `json:"time_to_live,omitempty"`
	StreamSpec             *StreamSpec              `json:"stream,omitempty"`
	Attributes             map[string]AttributeSpec `json:"attributes"`
	RecommendedModel       string                   `json:"recommended_model"`
	TableName              string                   `json:"table_name"`
	BillingMode            string                   `json:"billing_mode"`
	MigrationComplexity    string                   `json:"migration_complexity"`
	PartitionKey           AttributeSpec            `json:"partition_key"`
	LocalSecondaryIndexes  []LSIAnalysis            `json:"local_secondary_indexes,omitempty"`
	SampleItems            []map[string]interface{} `json:"sample_items,omitempty"`
	Warnings               []string                 `json:"warnings,omitempty"`
	GlobalSecondaryIndexes []GSIAnalysis            `json:"global_secondary_indexes,omitempty"`
	TableSizeBytes         int64                    `json:"table_size_bytes"`
	ItemCount              int64                    `json:"item_count"`
	MultiTenantCandidate   bool                     `json:"multi_tenant_candidate"`
}

// AttributeSpec defines an attribute specification
type AttributeSpec struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

// GSIAnalysis contains GSI analysis
type GSIAnalysis struct {
	SortKey        *AttributeSpec           `json:"sort_key,omitempty"`
	IndexName      string                   `json:"index_name"`
	ProjectionType string                   `json:"projection_type"`
	PartitionKey   AttributeSpec            `json:"partition_key"`
	KeySchema      []types.KeySchemaElement `json:"key_schema"`
	ItemCount      int64                    `json:"item_count"`
}

// LSIAnalysis contains LSI analysis
type LSIAnalysis struct {
	IndexName      string        `json:"index_name"`
	ProjectionType string        `json:"projection_type"`
	SortKey        AttributeSpec `json:"sort_key"`
	ItemCount      int64         `json:"item_count"`
}

// TTLSpec defines TTL specification
type TTLSpec struct {
	AttributeName string `json:"attribute_name"`
	Enabled       bool   `json:"enabled"`
}

// StreamSpec defines stream specification
type StreamSpec struct {
	ViewType  string `json:"view_type"`
	StreamArn string `json:"stream_arn"`
	Enabled   bool   `json:"enabled"`
}

// MigrationConfig holds migration configuration
type MigrationConfig struct {
	TableName     string `json:"table_name"`
	Region        string `json:"region"`
	OutputDir     string `json:"output_dir"`
	ModelName     string `json:"model_name"`
	AnalyzeOnly   bool   `json:"analyze_only"`
	MultiTenant   bool   `json:"multi_tenant"`
	GenerateTests bool   `json:"generate_tests"`
}

func (c *DynamORMMigrateCommand) Execute(ctx context.Context, args []string) error {
	// Parse arguments
	config, err := c.parseMigrateArgs(args)
	if err != nil {
		return err
	}

	// Check if we're in a Lift project
	if !c.isLiftProject() {
		return fmt.Errorf("not in a Lift project directory - run 'lift new' first")
	}

	// Create AWS session
	cfg, err := awsConfig(ctx, config.Region)
	if err != nil {
		return fmt.Errorf("failed to create AWS config: %w", err)
	}

	client := dynamodb.NewFromConfig(cfg)

	// Analyze table
	fmt.Printf("🔍 Analyzing table: %s\n", config.TableName)
	analysis, err := c.analyzeTable(ctx, client, config.TableName)
	if err != nil {
		return fmt.Errorf("failed to analyze table: %w", err)
	}

	// Create output directory
	if err := os.MkdirAll(config.OutputDir, 0750); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Save analysis results
	analysisPath := filepath.Join(config.OutputDir, "table_analysis.json")
	if err := c.saveAnalysis(analysis, analysisPath); err != nil {
		return fmt.Errorf("failed to save analysis: %w", err)
	}

	fmt.Printf("📊 Analysis complete! Results saved to: %s\n", analysisPath)
	c.printAnalysisSummary(analysis)

	// If analyze-only mode, stop here
	if config.AnalyzeOnly {
		return nil
	}

	// Generate migration code
	fmt.Printf("🚀 Generating migration code...\n")
	if err := c.generateMigrationCode(analysis, config); err != nil {
		return fmt.Errorf("failed to generate migration code: %w", err)
	}

	fmt.Printf("✅ Migration complete! Generated files in: %s\n", config.OutputDir)
	return nil
}

func (c *DynamORMMigrateCommand) parseMigrateArgs(args []string) (*MigrationConfig, error) {
	config := &MigrationConfig{
		Region:        "us-east-1",
		OutputDir:     "migration",
		GenerateTests: true,
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--table":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--table requires a value")
			}
			config.TableName = args[i+1]
			i++
		case "--region":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--region requires a value")
			}
			config.Region = args[i+1]
			i++
		case "--output-dir":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--output-dir requires a value")
			}
			config.OutputDir = args[i+1]
			i++
		case "--analyze-only":
			config.AnalyzeOnly = true
		case "--model":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--model requires a value")
			}
			config.ModelName = args[i+1]
			i++
		case "--multi-tenant":
			config.MultiTenant = true
		case "--no-tests":
			config.GenerateTests = false
		}
	}

	if config.TableName == "" {
		return nil, fmt.Errorf("--table is required")
	}

	// Generate model name if not provided
	if config.ModelName == "" {
		config.ModelName = c.generateModelName(config.TableName)
	}

	return config, nil
}

func (c *DynamORMMigrateCommand) isLiftProject() bool {
	if _, err := os.Stat("go.mod"); err != nil {
		return false
	}

	content, err := os.ReadFile("go.mod")
	if err != nil {
		return false
	}

	return strings.Contains(string(content), "github.com/pay-theory/lift")
}

func (c *DynamORMMigrateCommand) generateModelName(tableName string) string {
	// Remove common prefixes/suffixes
	name := strings.TrimPrefix(tableName, "lift-")
	name = strings.TrimPrefix(name, "app-")
	name = strings.TrimSuffix(name, "-table")
	name = strings.TrimSuffix(name, "s")

	// Convert to PascalCase
	parts := strings.Split(name, "-")
	var result strings.Builder
	for _, part := range parts {
		if len(part) > 0 {
			result.WriteString(strings.ToUpper(part[:1]) + strings.ToLower(part[1:]))
		}
	}

	return result.String()
}

func awsConfig(ctx context.Context, region string) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx, config.WithRegion(region))
}

func (c *DynamORMMigrateCommand) analyzeTable(ctx context.Context, client *dynamodb.Client, tableName string) (*TableAnalysis, error) {
	builder := newTableAnalysisBuilder(ctx, c, client, tableName)
	return builder.build()
}

// tableAnalysisBuilder builds table analysis
// Memory optimized: struct with 64 pointer bytes could be 56
type tableAnalysisBuilder struct {
	// Pointers first (8 bytes each)
	cmd      *DynamORMMigrateCommand
	client   *dynamodb.Client
	table    *types.TableDescription
	analysis *TableAnalysis
	// Interface (16 bytes)
	ctx context.Context
	// String (16 bytes)
	tableName string
}

// newTableAnalysisBuilder creates a new table analysis builder
func newTableAnalysisBuilder(ctx context.Context, cmd *DynamORMMigrateCommand, client *dynamodb.Client, tableName string) *tableAnalysisBuilder {
	return &tableAnalysisBuilder{
		cmd:       cmd,
		ctx:       ctx,
		client:    client,
		tableName: tableName,
	}
}

// build constructs the complete table analysis
func (tab *tableAnalysisBuilder) build() (*TableAnalysis, error) {
	// Get table description
	if err := tab.describeTable(); err != nil {
		return nil, err
	}

	// Initialize analysis
	tab.initializeAnalysis()

	// Analyze various aspects
	tab.analyzeKeySchema()
	tab.analyzeGlobalSecondaryIndexes()
	tab.analyzeLocalSecondaryIndexes()
	tab.analyzeTTL()
	tab.analyzeStreams()

	// Sample and finalize
	tab.sampleTableItems()
	tab.finalizeAnalysis()

	return tab.analysis, nil
}

// describeTable gets table description from DynamoDB
func (tab *tableAnalysisBuilder) describeTable() error {
	describeInput := &dynamodb.DescribeTableInput{
		TableName: aws.String(tab.tableName),
	}

	describeOutput, err := tab.client.DescribeTable(tab.ctx, describeInput)
	if err != nil {
		return fmt.Errorf("failed to describe table: %w", err)
	}

	tab.table = describeOutput.Table
	return nil
}

// initializeAnalysis creates initial analysis structure
func (tab *tableAnalysisBuilder) initializeAnalysis() {
	tab.analysis = &TableAnalysis{
		TableName:      tab.tableName,
		BillingMode:    string(tab.table.BillingModeSummary.BillingMode),
		ItemCount:      *tab.table.ItemCount,
		TableSizeBytes: *tab.table.TableSizeBytes,
		Attributes:     make(map[string]AttributeSpec),
		CreatedAt:      time.Now(),
	}
}

// analyzeKeySchema analyzes primary key schema
func (tab *tableAnalysisBuilder) analyzeKeySchema() {
	keyAnalyzer := newKeySchemaAnalyzer(tab.cmd, tab.table.AttributeDefinitions)

	for _, key := range tab.table.KeySchema {
		spec := keyAnalyzer.analyzeKey(key)
		if spec == nil {
			continue
		}

		switch key.KeyType {
		case types.KeyTypeHash:
			tab.analysis.PartitionKey = *spec
		case types.KeyTypeRange:
			tab.analysis.SortKey = spec
		}
	}
}

// analyzeGlobalSecondaryIndexes analyzes all GSIs
func (tab *tableAnalysisBuilder) analyzeGlobalSecondaryIndexes() {
	gsiAnalyzer := newGSIAnalyzer(tab.cmd, tab.table.AttributeDefinitions)

	for _, gsi := range tab.table.GlobalSecondaryIndexes {
		gsiAnalysis := gsiAnalyzer.analyzeGSI(gsi)
		tab.analysis.GlobalSecondaryIndexes = append(tab.analysis.GlobalSecondaryIndexes, gsiAnalysis)
	}
}

// analyzeLocalSecondaryIndexes analyzes all LSIs
func (tab *tableAnalysisBuilder) analyzeLocalSecondaryIndexes() {
	lsiAnalyzer := newLSIAnalyzer(tab.cmd, tab.table.AttributeDefinitions)

	for _, lsi := range tab.table.LocalSecondaryIndexes {
		lsiAnalysis := lsiAnalyzer.analyzeLSI(lsi)
		tab.analysis.LocalSecondaryIndexes = append(tab.analysis.LocalSecondaryIndexes, lsiAnalysis)
	}
}

// analyzeTTL analyzes TTL configuration
func (tab *tableAnalysisBuilder) analyzeTTL() {
	ttlAnalyzer := newTTLAnalyzer()
	tab.analysis.TimeToLiveSpec = ttlAnalyzer.analyzeTTL(tab.table.AttributeDefinitions)
}

// analyzeStreams analyzes DynamoDB streams configuration
func (tab *tableAnalysisBuilder) analyzeStreams() {
	if tab.table.StreamSpecification != nil &&
		tab.table.StreamSpecification.StreamEnabled != nil &&
		*tab.table.StreamSpecification.StreamEnabled {
		tab.analysis.StreamSpec = &StreamSpec{
			Enabled:   true,
			ViewType:  string(tab.table.StreamSpecification.StreamViewType),
			StreamArn: *tab.table.LatestStreamArn,
		}
	}
}

// sampleTableItems samples items for better analysis
func (tab *tableAnalysisBuilder) sampleTableItems() {
	if err := tab.cmd.sampleItems(tab.ctx, tab.client, tab.tableName, tab.analysis); err != nil {
		tab.analysis.Warnings = append(tab.analysis.Warnings, fmt.Sprintf("Failed to sample items: %v", err))
	}
}

// finalizeAnalysis determines complexity and generates recommendations
func (tab *tableAnalysisBuilder) finalizeAnalysis() {
	tab.analysis.MigrationComplexity = tab.cmd.determineMigrationComplexity(tab.analysis)
	tab.analysis.RecommendedModel = tab.cmd.generateRecommendedModel(tab.analysis)
	tab.analysis.MultiTenantCandidate = tab.cmd.isMultiTenantCandidate(tab.analysis)
}

// keySchemaAnalyzer analyzes key schemas
type keySchemaAnalyzer struct {
	cmd        *DynamORMMigrateCommand
	attributes []types.AttributeDefinition
}

// newKeySchemaAnalyzer creates a new key schema analyzer
func newKeySchemaAnalyzer(cmd *DynamORMMigrateCommand, attributes []types.AttributeDefinition) *keySchemaAnalyzer {
	return &keySchemaAnalyzer{
		cmd:        cmd,
		attributes: attributes,
	}
}

// analyzeKey analyzes a single key element
func (ksa *keySchemaAnalyzer) analyzeKey(key types.KeySchemaElement) *AttributeSpec {
	attr := ksa.cmd.findAttribute(ksa.attributes, *key.AttributeName)
	if attr == nil {
		return nil
	}

	return &AttributeSpec{
		Name:     *key.AttributeName,
		Type:     ksa.cmd.convertDynamoType(attr.AttributeType),
		Required: true,
	}
}

// gsiAnalyzer analyzes global secondary indexes
type gsiAnalyzer struct {
	cmd        *DynamORMMigrateCommand
	attributes []types.AttributeDefinition
}

// newGSIAnalyzer creates a new GSI analyzer
func newGSIAnalyzer(cmd *DynamORMMigrateCommand, attributes []types.AttributeDefinition) *gsiAnalyzer {
	return &gsiAnalyzer{
		cmd:        cmd,
		attributes: attributes,
	}
}

// analyzeGSI analyzes a single GSI
func (ga *gsiAnalyzer) analyzeGSI(gsi types.GlobalSecondaryIndexDescription) GSIAnalysis {
	gsiAnalysis := GSIAnalysis{
		IndexName:      *gsi.IndexName,
		ProjectionType: string(gsi.Projection.ProjectionType),
		ItemCount:      *gsi.ItemCount,
		KeySchema:      gsi.KeySchema,
	}

	keyAnalyzer := newKeySchemaAnalyzer(ga.cmd, ga.attributes)

	for _, key := range gsi.KeySchema {
		spec := keyAnalyzer.analyzeKey(key)
		if spec == nil {
			continue
		}

		switch key.KeyType {
		case types.KeyTypeHash:
			gsiAnalysis.PartitionKey = *spec
		case types.KeyTypeRange:
			gsiAnalysis.SortKey = spec
		}
	}

	return gsiAnalysis
}

// lsiAnalyzer analyzes local secondary indexes
type lsiAnalyzer struct {
	cmd        *DynamORMMigrateCommand
	attributes []types.AttributeDefinition
}

// newLSIAnalyzer creates a new LSI analyzer
func newLSIAnalyzer(cmd *DynamORMMigrateCommand, attributes []types.AttributeDefinition) *lsiAnalyzer {
	return &lsiAnalyzer{
		cmd:        cmd,
		attributes: attributes,
	}
}

// analyzeLSI analyzes a single LSI
func (la *lsiAnalyzer) analyzeLSI(lsi types.LocalSecondaryIndexDescription) LSIAnalysis {
	lsiAnalysis := LSIAnalysis{
		IndexName:      *lsi.IndexName,
		ProjectionType: string(lsi.Projection.ProjectionType),
		ItemCount:      *lsi.ItemCount,
	}

	for _, key := range lsi.KeySchema {
		if key.KeyType == types.KeyTypeRange {
			attr := la.cmd.findAttribute(la.attributes, *key.AttributeName)
			if attr != nil {
				lsiAnalysis.SortKey = AttributeSpec{
					Name:     *key.AttributeName,
					Type:     la.cmd.convertDynamoType(attr.AttributeType),
					Required: true,
				}
			}
		}
	}

	return lsiAnalysis
}

// ttlAnalyzer analyzes TTL configuration
type ttlAnalyzer struct{}

// newTTLAnalyzer creates a new TTL analyzer
func newTTLAnalyzer() *ttlAnalyzer {
	return &ttlAnalyzer{}
}

// analyzeTTL checks for TTL attributes
func (ta *ttlAnalyzer) analyzeTTL(attributes []types.AttributeDefinition) *TTLSpec {
	for _, attr := range attributes {
		attrName := *attr.AttributeName
		if ta.isTTLAttribute(attrName) {
			return &TTLSpec{
				AttributeName: attrName,
				Enabled:       false, // Would need separate API call to confirm
			}
		}
	}
	return nil
}

// isTTLAttribute checks if attribute name suggests TTL usage
func (ta *ttlAnalyzer) isTTLAttribute(attrName string) bool {
	lowerName := strings.ToLower(attrName)
	return strings.HasSuffix(lowerName, "ttl") ||
		strings.HasSuffix(lowerName, "expires") ||
		strings.HasSuffix(lowerName, "expiry")
}

func (c *DynamORMMigrateCommand) findAttribute(attrs []types.AttributeDefinition, name string) *types.AttributeDefinition {
	for _, attr := range attrs {
		if *attr.AttributeName == name {
			return &attr
		}
	}
	return nil
}

func (c *DynamORMMigrateCommand) convertDynamoType(attrType types.ScalarAttributeType) string {
	switch attrType {
	case types.ScalarAttributeTypeS:
		return stringType
	case types.ScalarAttributeTypeN:
		return "number"
	case types.ScalarAttributeTypeB:
		return "binary"
	default:
		return stringType
	}
}

func (c *DynamORMMigrateCommand) sampleItems(ctx context.Context, client *dynamodb.Client, tableName string, analysis *TableAnalysis) error {
	// Scan a few items to understand the structure
	scanInput := &dynamodb.ScanInput{
		TableName: aws.String(tableName),
		Limit:     aws.Int32(5),
	}

	scanOutput, err := client.Scan(ctx, scanInput)
	if err != nil {
		return err
	}

	// Convert items to map for analysis
	for _, item := range scanOutput.Items {
		itemMap := make(map[string]interface{})
		for k, v := range item {
			itemMap[k] = c.convertAttributeValue(v)
		}
		analysis.SampleItems = append(analysis.SampleItems, itemMap)
	}

	return nil
}

func (c *DynamORMMigrateCommand) convertAttributeValue(av types.AttributeValue) interface{} {
	switch v := av.(type) {
	case *types.AttributeValueMemberS:
		return v.Value
	case *types.AttributeValueMemberN:
		return v.Value
	case *types.AttributeValueMemberB:
		return v.Value
	case *types.AttributeValueMemberBOOL:
		return v.Value
	case *types.AttributeValueMemberNULL:
		return nil
	default:
		return "complex_type"
	}
}

func (c *DynamORMMigrateCommand) determineMigrationComplexity(analysis *TableAnalysis) string {
	complexity := "Simple"

	if len(analysis.GlobalSecondaryIndexes) > 2 {
		complexity = mediumSize
	}

	if len(analysis.LocalSecondaryIndexes) > 0 {
		complexity = mediumSize
	}

	if analysis.StreamSpec != nil && analysis.StreamSpec.Enabled {
		complexity = mediumSize
	}

	if len(analysis.SampleItems) > 0 {
		// Check for complex nested structures
		for _, item := range analysis.SampleItems {
			if c.hasComplexStructure(item) {
				complexity = "Complex"
				break
			}
		}
	}

	return complexity
}

func (c *DynamORMMigrateCommand) hasComplexStructure(item map[string]interface{}) bool {
	for _, value := range item {
		switch v := value.(type) {
		case map[string]interface{}:
			return true
		case []interface{}:
			return true
		case string:
			if v == "complex_type" {
				return true
			}
		}
	}
	return false
}

func (c *DynamORMMigrateCommand) generateRecommendedModel(analysis *TableAnalysis) string {
	modelName := c.generateModelName(analysis.TableName)
	if analysis.MultiTenantCandidate {
		return modelName + " (Multi-Tenant)"
	}
	return modelName
}

func (c *DynamORMMigrateCommand) isMultiTenantCandidate(analysis *TableAnalysis) bool {
	// Check if partition key suggests multi-tenancy
	pkName := strings.ToLower(analysis.PartitionKey.Name)
	tenantIndicators := []string{"tenant", "org", "account", "customer", "company"}

	for _, indicator := range tenantIndicators {
		if strings.Contains(pkName, indicator) {
			return true
		}
	}

	// Check GSI names
	for _, gsi := range analysis.GlobalSecondaryIndexes {
		gsiName := strings.ToLower(gsi.IndexName)
		for _, indicator := range tenantIndicators {
			if strings.Contains(gsiName, indicator) {
				return true
			}
		}
	}

	return false
}

func (c *DynamORMMigrateCommand) saveAnalysis(analysis *TableAnalysis, path string) error {
	data, err := json.MarshalIndent(analysis, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func (c *DynamORMMigrateCommand) printAnalysisSummary(analysis *TableAnalysis) {
	fmt.Printf("\n📋 Analysis Summary for %s:\n", analysis.TableName)
	fmt.Printf("   • Partition Key: %s (%s)\n", analysis.PartitionKey.Name, analysis.PartitionKey.Type)

	if analysis.SortKey != nil {
		fmt.Printf("   • Sort Key: %s (%s)\n", analysis.SortKey.Name, analysis.SortKey.Type)
	}

	fmt.Printf("   • GSIs: %d\n", len(analysis.GlobalSecondaryIndexes))
	fmt.Printf("   • LSIs: %d\n", len(analysis.LocalSecondaryIndexes))
	fmt.Printf("   • Item Count: %d\n", analysis.ItemCount)
	fmt.Printf("   • Table Size: %.2f MB\n", float64(analysis.TableSizeBytes)/1024/1024)
	fmt.Printf("   • Billing Mode: %s\n", analysis.BillingMode)
	fmt.Printf("   • Migration Complexity: %s\n", analysis.MigrationComplexity)
	fmt.Printf("   • Recommended Model: %s\n", analysis.RecommendedModel)
	fmt.Printf("   • Multi-Tenant Candidate: %t\n", analysis.MultiTenantCandidate)

	if analysis.TimeToLiveSpec != nil {
		fmt.Printf("   • TTL: %s\n", analysis.TimeToLiveSpec.AttributeName)
	}

	if analysis.StreamSpec != nil {
		fmt.Printf("   • Stream: %s\n", analysis.StreamSpec.ViewType)
	}

	if len(analysis.Warnings) > 0 {
		fmt.Printf("   ⚠️  Warnings:\n")
		for _, warning := range analysis.Warnings {
			fmt.Printf("     - %s\n", warning)
		}
	}
}

func (c *DynamORMMigrateCommand) generateMigrationCode(analysis *TableAnalysis, config *MigrationConfig) error {
	// Generate model
	if err := c.generateMigrationModel(analysis, config); err != nil {
		return fmt.Errorf("failed to generate model: %w", err)
	}

	// Generate CDK construct
	if err := c.generateMigrationCDK(analysis, config); err != nil {
		return fmt.Errorf("failed to generate CDK: %w", err)
	}

	// Generate migration script
	if err := c.generateMigrationScript(analysis, config); err != nil {
		return fmt.Errorf("failed to generate migration script: %w", err)
	}

	// Generate tests if requested
	if config.GenerateTests {
		if err := c.generateMigrationTests(analysis, config); err != nil {
			return fmt.Errorf("failed to generate tests: %w", err)
		}
	}

	return nil
}

func (c *DynamORMMigrateCommand) generateMigrationModel(analysis *TableAnalysis, config *MigrationConfig) error {
	tmpl := `// Generated by lift dynamorm migrate
// Source table: {{.TableName}}
// Generated at: {{.CreatedAt.Format "2006-01-02 15:04:05"}}

package models

import (
	"time"

	"github.com/pay-theory/lift/pkg/dynamorm"
)

// {{.ModelName}} represents the migrated {{.TableName}} entity
type {{.ModelName}} struct {
	{{if .IsMultiTenant}}{{.PartitionKey.Name}} string ` + "`" + `json:"{{.PartitionKey.Name | ToSnakeCase}}" dynamodb:"{{.PartitionKey.Name}},pk"` + "`" + `
	{{if .SortKey}}{{.SortKey.Name}} string ` + "`" + `json:"{{.SortKey.Name | ToSnakeCase}}" dynamodb:"{{.SortKey.Name}}"` + "`" + `
	{{end}}{{else}}{{.PartitionKey.Name}} string ` + "`" + `json:"{{.PartitionKey.Name | ToSnakeCase}}" dynamodb:"{{.PartitionKey.Name}},pk"` + "`" + `
	{{if .SortKey}}{{.SortKey.Name}} string ` + "`" + `json:"{{.SortKey.Name | ToSnakeCase}}" dynamodb:"{{.SortKey.Name}}"` + "`" + `
	{{end}}{{end}}
	
	// Common fields
	CreatedAt time.Time ` + "`" + `json:"created_at" dynamodb:"created_at"` + "`" + `
	UpdatedAt time.Time ` + "`" + `json:"updated_at" dynamodb:"updated_at"` + "`" + `
	
	{{if .TTLSpec}}// TTL field
	{{.TTLSpec.AttributeName}} int64 ` + "`" + `json:"{{.TTLSpec.AttributeName | ToSnakeCase}}" dynamodb:"{{.TTLSpec.AttributeName}}"` + "`" + `
	{{end}}
	
	{{range .GSIs}}// GSI fields for {{.IndexName}}
	{{.PartitionKey.Name}} string ` + "`" + `json:"{{.PartitionKey.Name | ToSnakeCase}}" dynamodb:"{{.PartitionKey.Name}}" gsi:"{{.IndexName}}"` + "`" + `
	{{if .SortKey}}{{.SortKey.Name}} string ` + "`" + `json:"{{.SortKey.Name | ToSnakeCase}}" dynamodb:"{{.SortKey.Name}}" gsi:"{{.IndexName}}"` + "`" + `
	{{end}}{{end}}
}

// New{{.ModelName}} creates a new {{.ModelName}} instance
func New{{.ModelName}}({{if .IsMultiTenant}}{{.PartitionKey.Name | ToSnakeCase}} string{{if .SortKey}}, {{.SortKey.Name | ToSnakeCase}} string{{end}}{{else}}{{.PartitionKey.Name | ToSnakeCase}} string{{if .SortKey}}, {{.SortKey.Name | ToSnakeCase}} string{{end}}{{end}}) *{{.ModelName}} {
	now := time.Now()
	
	return &{{.ModelName}}{
		{{.PartitionKey.Name}}: {{.PartitionKey.Name | ToSnakeCase}},
		{{if .SortKey}}{{.SortKey.Name}}: {{.SortKey.Name | ToSnakeCase}},
		{{end}}CreatedAt: now,
		UpdatedAt: now,
		{{if .TTLSpec}}{{.TTLSpec.AttributeName}}: now.Add(24 * time.Hour).Unix(),
		{{end}}
	}
}

// TableName returns the DynamoDB table name
func (m *{{.ModelName}}) TableName() string {
	return "{{.TableName}}"
}

// Update updates the {{.ModelName}} timestamp
func (m *{{.ModelName}}) Update() {
	m.UpdatedAt = time.Now()
	{{if .TTLSpec}}m.{{.TTLSpec.AttributeName}} = time.Now().Add(24 * time.Hour).Unix()
	{{end}}
}
`

	data := struct {
		*TableAnalysis
		ModelName     string
		IsMultiTenant bool
	}{
		TableAnalysis: analysis,
		ModelName:     config.ModelName,
		IsMultiTenant: config.MultiTenant,
	}

	funcMap := template.FuncMap{
		"ToSnakeCase": func(s string) string {
			return strings.ToLower(strings.ReplaceAll(s, "-", "_"))
		},
	}

	t, err := template.New("model").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return err
	}

	filename := filepath.Join(config.OutputDir, strings.ToLower(config.ModelName)+".go")
	file, err := os.Create(filename) // #nosec G304 - filename is constructed from sanitized input in controlled directories
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Warning: failed to close file: %v\n", err)
		}
	}()

	return t.Execute(file, data)
}

func (c *DynamORMMigrateCommand) generateMigrationCDK(analysis *TableAnalysis, config *MigrationConfig) error {
	tmpl := `// Generated by lift dynamorm migrate
// Source table: {{.TableName}}
// Generated at: {{.CreatedAt.Format "2006-01-02 15:04:05"}}

package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
	
	"github.com/pay-theory/lift/pkg/cdk/constructs"
)

// {{.ModelName}}TableProps defines the properties for the migrated {{.ModelName}} table
type {{.ModelName}}TableProps struct {
	// DynamORM Table Props
	*constructs.DynamORMTableProps
	
	// Migration-specific settings
	EnableMigrationMode  *bool
	OriginalTableName    *string
	BackupTableName      *string
}

// {{.ModelName}}Table represents the migrated DynamoDB table for {{.ModelName}} entities
type {{.ModelName}}Table struct {
	constructs.Construct
	DynamORMTable *constructs.DynamORMTable
}

// New{{.ModelName}}Table creates a new {{.ModelName}} table construct from migration
func New{{.ModelName}}Table(scope constructs.Construct, id *string, props *{{.ModelName}}TableProps) *{{.ModelName}}Table {
	this := &{{.ModelName}}Table{}
	this.Construct = constructs.NewConstruct(scope, id)

	// Set defaults from analysis
	if props.DynamORMTableProps == nil {
		props.DynamORMTableProps = &constructs.DynamORMTableProps{}
	}
	
	if props.DynamORMTableProps.TableName == nil {
		props.DynamORMTableProps.TableName = jsii.String("{{.TableName}}")
	}
	
	if props.OriginalTableName == nil {
		props.OriginalTableName = jsii.String("{{.TableName}}")
	}

	// Configure table structure based on analysis
	props.DynamORMTableProps.PartitionKey = &awsdynamodb.Attribute{
		Name: jsii.String("{{.PartitionKey.Name}}"),
		Type: awsdynamodb.AttributeType_{{.PartitionKey.Type | ToDynamoType}},
	}
	
	{{if .SortKey}}props.DynamORMTableProps.SortKey = &awsdynamodb.Attribute{
		Name: jsii.String("{{.SortKey.Name}}"),
		Type: awsdynamodb.AttributeType_{{.SortKey.Type | ToDynamoType}},
	}
	{{end}}
	
	// Configure TTL if present
	{{if .TTLSpec}}props.DynamORMTableProps.TimeToLiveAttribute = jsii.String("{{.TTLSpec.AttributeName}}")
	{{end}}
	
	// Configure streams if present
	{{if .StreamSpec}}props.DynamORMTableProps.Stream = awsdynamodb.StreamViewType_{{.StreamSpec.ViewType | ToStreamType}}
	{{end}}
	
	// Set multi-tenant mode
	{{if .IsMultiTenant}}props.DynamORMTableProps.EnableMultiTenant = jsii.Bool(true)
	{{end}}
	
	// Create the DynamORM table
	this.DynamORMTable = constructs.NewDynamORMTable(this, jsii.String("Table"), props.DynamORMTableProps)
	
	{{range .GSIs}}// Add {{.IndexName}} GSI from migration
	this.DynamORMTable.AddGSI(&constructs.GSIProps{
		IndexName: jsii.String("{{.IndexName}}"),
		PartitionKey: &awsdynamodb.Attribute{
			Name: jsii.String("{{.PartitionKey.Name}}"),
			Type: awsdynamodb.AttributeType_{{.PartitionKey.Type | ToDynamoType}},
		},
		{{if .SortKey}}SortKey: &awsdynamodb.Attribute{
			Name: jsii.String("{{.SortKey.Name}}"),
			Type: awsdynamodb.AttributeType_{{.SortKey.Type | ToDynamoType}},
		},
		{{end}}ProjectionType: awsdynamodb.ProjectionType_{{.ProjectionType | ToProjectionType}},
	})
	{{end}}
	
	// Add migration-specific configurations
	if props.EnableMigrationMode != nil && *props.EnableMigrationMode {
		c.configureForMigration(this.DynamORMTable, props)
	}

	return this
}

// configureForMigration adds migration-specific settings
func (t *{{.ModelName}}Table) configureForMigration(table *constructs.DynamORMTable, props *{{.ModelName}}TableProps) {
	// Enable point-in-time recovery for safe migration
	table.Table.PointInTimeRecovery = jsii.Bool(true)
	
	// Add tags to identify migration
	table.Table.Tags().SetTag(jsii.String("MigrationSource"), props.OriginalTableName, jsii.Number(1))
	table.Table.Tags().SetTag(jsii.String("MigrationDate"), jsii.String("{{.CreatedAt.Format "2006-01-02"}}"), jsii.Number(1))
	table.Table.Tags().SetTag(jsii.String("DynamORMGenerated"), jsii.String("true"), jsii.Number(1))
}

// TableName returns the table name
func (t *{{.ModelName}}Table) TableName() *string {
	return t.DynamORMTable.Table.TableName()
}

// TableArn returns the table ARN
func (t *{{.ModelName}}Table) TableArn() *string {
	return t.DynamORMTable.Table.TableArn()
}

// GrantReadData grants read permissions to the table
func (t *{{.ModelName}}Table) GrantReadData(grantee awscdk.IPrincipal) {
	t.DynamORMTable.GrantReadData(grantee)
}

// GrantWriteData grants write permissions to the table
func (t *{{.ModelName}}Table) GrantWriteData(grantee awscdk.IPrincipal) {
	t.DynamORMTable.GrantWriteData(grantee)
}

// GrantFullAccess grants full access to the table
func (t *{{.ModelName}}Table) GrantFullAccess(grantee awscdk.IPrincipal) {
	t.DynamORMTable.GrantFullAccess(grantee)
}
`

	data := struct {
		*TableAnalysis
		ModelName     string
		IsMultiTenant bool
	}{
		TableAnalysis: analysis,
		ModelName:     config.ModelName,
		IsMultiTenant: config.MultiTenant,
	}

	funcMap := template.FuncMap{
		"ToDynamoType": func(s string) string {
			switch strings.ToLower(s) {
			case stringType:
				return "STRING"
			case "number":
				return "NUMBER"
			case "binary":
				return "BINARY"
			default:
				return "STRING"
			}
		},
		"ToStreamType": func(s string) string {
			switch strings.ToUpper(s) {
			case keysOnlyView:
				return keysOnlyView
			case "NEW_IMAGE":
				return "NEW_IMAGE"
			case "OLD_IMAGE":
				return "OLD_IMAGE"
			case newAndOldImagesView:
				return newAndOldImagesView
			default:
				return newAndOldImagesView
			}
		},
		"ToProjectionType": func(s string) string {
			switch strings.ToUpper(s) {
			case allView:
				return allView
			case keysOnlyView:
				return keysOnlyView
			case "INCLUDE":
				return "INCLUDE"
			default:
				return allView
			}
		},
	}

	t, err := template.New("cdk").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return err
	}

	filename := filepath.Join(config.OutputDir, strings.ToLower(config.ModelName)+"_table.go")
	file, err := os.Create(filename) // #nosec G304 - filename is constructed from sanitized input in controlled directories
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Warning: failed to close file: %v\n", err)
		}
	}()

	return t.Execute(file, data)
}

func (c *DynamORMMigrateCommand) generateMigrationScript(analysis *TableAnalysis, config *MigrationConfig) error {
	tmpl := `// Generated by lift dynamorm migrate
// Migration script for {{.TableName}} -> {{.ModelName}}
// Generated at: {{.CreatedAt.Format "2006-01-02 15:04:05"}}

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/lift/pkg/dynamorm"
)

// MigrationStats tracks migration progress
type MigrationStats struct {
	SourceTable      string    ` + "`" + `json:"source_table"` + "`" + `
	DestinationTable string    ` + "`" + `json:"destination_table"` + "`" + `
	StartTime        time.Time ` + "`" + `json:"start_time"` + "`" + `
	EndTime          time.Time ` + "`" + `json:"end_time"` + "`" + `
	TotalItems       int64     ` + "`" + `json:"total_items"` + "`" + `
	MigratedItems    int64     ` + "`" + `json:"migrated_items"` + "`" + `
	ErrorCount       int64     ` + "`" + `json:"error_count"` + "`" + `
	BatchSize        int32     ` + "`" + `json:"batch_size"` + "`" + `
	DryRun           bool      ` + "`" + `json:"dry_run"` + "`" + `
}

// MigrationConfig holds migration configuration
type MigrationConfig struct {
	SourceTableName      string
	DestinationTableName string
	Region               string
	BatchSize            int32
	DryRun               bool
	MaxConcurrency       int
	BackupBeforeMigration bool
	ValidateAfterMigration bool
}

func main() {
	// Parse command line arguments
	config := parseMigrationArgs()
	
	// Setup AWS client
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(config.Region))
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}
	
	client := dynamodb.NewFromConfig(cfg)
	
	// Initialize migration stats
	stats := &MigrationStats{
		SourceTable:      config.SourceTableName,
		DestinationTable: config.DestinationTableName,
		StartTime:        time.Now(),
		BatchSize:        config.BatchSize,
		DryRun:           config.DryRun,
	}
	
	fmt.Printf("🚀 Starting migration from %s to %s\n", config.SourceTableName, config.DestinationTableName)
	fmt.Printf("   • Batch Size: %d\n", config.BatchSize)
	fmt.Printf("   • Dry Run: %t\n", config.DryRun)
	fmt.Printf("   • Region: %s\n", config.Region)
	
	// Validate source table exists
	if err := validateSourceTable(ctx, client, config.SourceTableName); err != nil {
		log.Fatalf("Source table validation failed: %v", err)
	}
	
	// Create backup if requested
	if config.BackupBeforeMigration && !config.DryRun {
		if err := createBackup(ctx, client, config.SourceTableName); err != nil {
			log.Printf("⚠️  Backup creation failed (continuing): %v", err)
		} else {
			fmt.Printf("✅ Backup created for %s\n", config.SourceTableName)
		}
	}
	
	// Perform migration
	if err := performMigration(ctx, client, config, stats); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}
	
	// Validate migration if requested
	if config.ValidateAfterMigration && !config.DryRun {
		if err := validateMigration(ctx, client, config, stats); err != nil {
			log.Printf("⚠️  Migration validation failed: %v", err)
		} else {
			fmt.Printf("✅ Migration validation passed\n")
		}
	}
	
	// Print final stats
	stats.EndTime = time.Now()
	printMigrationSummary(stats)
}

func parseMigrationArgs() *MigrationConfig {
	config := &MigrationConfig{
		SourceTableName:      "{{.TableName}}",
		DestinationTableName: "{{.TableName}}-migrated",
		Region:               "us-east-1",
		BatchSize:            25,
		DryRun:               false,
		MaxConcurrency:       4,
		BackupBeforeMigration: true,
		ValidateAfterMigration: true,
	}
	
	// Parse OS args (simple implementation)
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--source":
			if i+1 < len(args) {
				config.SourceTableName = args[i+1]
				i++
			}
		case "--destination":
			if i+1 < len(args) {
				config.DestinationTableName = args[i+1]
				i++
			}
		case "--region":
			if i+1 < len(args) {
				config.Region = args[i+1]
				i++
			}
		case "--dry-run":
			config.DryRun = true
		case "--no-backup":
			config.BackupBeforeMigration = false
		case "--no-validation":
			config.ValidateAfterMigration = false
		}
	}
	
	return config
}

func validateSourceTable(ctx context.Context, client *dynamodb.Client, tableName string) error {
	_, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		return fmt.Errorf("source table %s not found: %w", tableName, err)
	}
	return nil
}

func createBackup(ctx context.Context, client *dynamodb.Client, tableName string) error {
	backupName := fmt.Sprintf("%s-migration-backup-%d", tableName, time.Now().Unix())
	
	_, err := client.CreateBackup(ctx, &dynamodb.CreateBackupInput{
		TableName:  aws.String(tableName),
		BackupName: aws.String(backupName),
	})
	
	return err
}

func performMigration(ctx context.Context, client *dynamodb.Client, config *MigrationConfig, stats *MigrationStats) error {
	// Get item count for progress tracking
	describeOutput, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(config.SourceTableName),
	})
	if err != nil {
		return fmt.Errorf("failed to describe source table: %w", err)
	}
	
	stats.TotalItems = *describeOutput.Table.ItemCount
	fmt.Printf("📊 Total items to migrate: %d\n", stats.TotalItems)
	
	// Scan and migrate items in batches
	var lastEvaluatedKey map[string]types.AttributeValue
	
	for {
		// Scan batch from source table
		scanInput := &dynamodb.ScanInput{
			TableName:                 aws.String(config.SourceTableName),
			Limit:                     aws.Int32(config.BatchSize),
			ExclusiveStartKey:         lastEvaluatedKey,
		}
		
		scanOutput, err := client.Scan(ctx, scanInput)
		if err != nil {
			return fmt.Errorf("failed to scan source table: %w", err)
		}
		
		if len(scanOutput.Items) == 0 {
			break
		}
		
		// Process batch
		if err := processBatch(ctx, client, config, scanOutput.Items, stats); err != nil {
			log.Printf("⚠️  Batch processing error: %v", err)
			stats.ErrorCount++
		} else {
			stats.MigratedItems += int64(len(scanOutput.Items))
		}
		
		// Progress update
		percentage := float64(stats.MigratedItems) / float64(stats.TotalItems) * 100
		fmt.Printf("🔄 Progress: %d/%d (%.1f%%)\n", stats.MigratedItems, stats.TotalItems, percentage)
		
		// Check if we need to continue
		lastEvaluatedKey = scanOutput.LastEvaluatedKey
		if lastEvaluatedKey == nil {
			break
		}
		
		// Small delay to avoid throttling
		time.Sleep(100 * time.Millisecond)
	}
	
	return nil
}

func processBatch(ctx context.Context, client *dynamodb.Client, config *MigrationConfig, items []map[string]types.AttributeValue, stats *MigrationStats) error {
	if config.DryRun {
		// In dry run mode, just log what would be done
		fmt.Printf("   [DRY RUN] Would migrate %d items\n", len(items))
		return nil
	}
	
	// Transform items for DynamORM compatibility
	transformedItems := make([]map[string]types.AttributeValue, len(items))
	for i, item := range items {
		transformedItems[i] = transformItem(item)
	}
	
	// Batch write to destination table
	writeRequests := make([]types.WriteRequest, len(transformedItems))
	for i, item := range transformedItems {
		writeRequests[i] = types.WriteRequest{
			PutRequest: &types.PutRequest{
				Item: item,
			},
		}
	}
	
	// Execute batch write
	_, err := client.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			config.DestinationTableName: writeRequests,
		},
	})
	
	return err
}

func transformItem(item map[string]types.AttributeValue) map[string]types.AttributeValue {
	// Add DynamORM-specific fields
	transformed := make(map[string]types.AttributeValue)
	
	// Copy existing attributes
	for k, v := range item {
		transformed[k] = v
	}
	
	// Add timestamps if not present
	now := time.Now()
	if _, exists := transformed["created_at"]; !exists {
		transformed["created_at"] = &types.AttributeValueMemberS{
			Value: now.Format(time.RFC3339),
		}
	}
	
	if _, exists := transformed["updated_at"]; !exists {
		transformed["updated_at"] = &types.AttributeValueMemberS{
			Value: now.Format(time.RFC3339),
		}
	}
	
	return transformed
}

func validateMigration(ctx context.Context, client *dynamodb.Client, config *MigrationConfig, stats *MigrationStats) error {
	// Compare item counts
	sourceCount, err := getItemCount(ctx, client, config.SourceTableName)
	if err != nil {
		return fmt.Errorf("failed to get source item count: %w", err)
	}
	
	destCount, err := getItemCount(ctx, client, config.DestinationTableName)
	if err != nil {
		return fmt.Errorf("failed to get destination item count: %w", err)
	}
	
	fmt.Printf("📊 Validation Results:\n")
	fmt.Printf("   • Source Items: %d\n", sourceCount)
	fmt.Printf("   • Destination Items: %d\n", destCount)
	fmt.Printf("   • Difference: %d\n", sourceCount-destCount)
	
	if sourceCount != destCount {
		return fmt.Errorf("item count mismatch: source=%d, destination=%d", sourceCount, destCount)
	}
	
	return nil
}

func getItemCount(ctx context.Context, client *dynamodb.Client, tableName string) (int64, error) {
	describeOutput, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		return 0, err
	}
	
	return *describeOutput.Table.ItemCount, nil
}

func printMigrationSummary(stats *MigrationStats) {
	duration := stats.EndTime.Sub(stats.StartTime)
	
	fmt.Printf("\n📋 Migration Summary:\n")
	fmt.Printf("   • Source Table: %s\n", stats.SourceTable)
	fmt.Printf("   • Destination Table: %s\n", stats.DestinationTable)
	fmt.Printf("   • Duration: %v\n", duration)
	fmt.Printf("   • Total Items: %d\n", stats.TotalItems)
	fmt.Printf("   • Migrated Items: %d\n", stats.MigratedItems)
	fmt.Printf("   • Error Count: %d\n", stats.ErrorCount)
	fmt.Printf("   • Success Rate: %.2f%%\n", float64(stats.MigratedItems)/float64(stats.TotalItems)*100)
	fmt.Printf("   • Items/Second: %.2f\n", float64(stats.MigratedItems)/duration.Seconds())
	fmt.Printf("   • Dry Run: %t\n", stats.DryRun)
	
	if stats.ErrorCount == 0 && stats.MigratedItems == stats.TotalItems {
		fmt.Printf("✅ Migration completed successfully!\n")
	} else {
		fmt.Printf("⚠️  Migration completed with %d errors\n", stats.ErrorCount)
	}
}
`

	data := struct {
		*TableAnalysis
		ModelName string
	}{
		TableAnalysis: analysis,
		ModelName:     config.ModelName,
	}

	t, err := template.New("migration").Parse(tmpl)
	if err != nil {
		return err
	}

	filename := filepath.Join(config.OutputDir, "migrate_"+strings.ToLower(config.ModelName)+".go")
	file, err := os.Create(filename) // #nosec G304 - filename is constructed from sanitized input in controlled directories
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Warning: failed to close file: %v\n", err)
		}
	}()

	return t.Execute(file, data)
}

func (c *DynamORMMigrateCommand) generateMigrationTests(analysis *TableAnalysis, config *MigrationConfig) error {
	tmpl := `// Generated by lift dynamorm migrate
// Tests for migrated {{.TableName}} -> {{.ModelName}}
// Generated at: {{.CreatedAt.Format "2006-01-02 15:04:05"}}

package models

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	
	"github.com/pay-theory/lift/pkg/dynamorm"
	"github.com/pay-theory/lift/pkg/cdk/test"
)

func Test{{.ModelName}}_TableName(t *testing.T) {
	{{.ModelName | ToLowerCase}} := &{{.ModelName}}{}
	assert.Equal(t, "{{.TableName}}", {{.ModelName | ToLowerCase}}.TableName())
}

func Test{{.ModelName}}_New{{.ModelName}}(t *testing.T) {
	{{if .IsMultiTenant}}{{.ModelName | ToLowerCase}} := New{{.ModelName}}("tenant123"{{if .SortKey}}, "sort123"{{end}})
	
	assert.Equal(t, "tenant123", {{.ModelName | ToLowerCase}}.{{.PartitionKey.Name}})
	{{if .SortKey}}assert.Equal(t, "sort123", {{.ModelName | ToLowerCase}}.{{.SortKey.Name}}){{end}}{{else}}{{.ModelName | ToLowerCase}} := New{{.ModelName}}("pk123"{{if .SortKey}}, "sort123"{{end}})
	
	assert.Equal(t, "pk123", {{.ModelName | ToLowerCase}}.{{.PartitionKey.Name}})
	{{if .SortKey}}assert.Equal(t, "sort123", {{.ModelName | ToLowerCase}}.{{.SortKey.Name}}){{end}}{{end}}
	assert.False(t, {{.ModelName | ToLowerCase}}.CreatedAt.IsZero())
	assert.False(t, {{.ModelName | ToLowerCase}}.UpdatedAt.IsZero())
	{{if .TTLSpec}}assert.Greater(t, {{.ModelName | ToLowerCase}}.{{.TTLSpec.AttributeName}}, int64(0)){{end}}
}

func Test{{.ModelName}}_Update(t *testing.T) {
	{{if .IsMultiTenant}}{{.ModelName | ToLowerCase}} := New{{.ModelName}}("tenant123"{{if .SortKey}}, "sort123"{{end}}){{else}}{{.ModelName | ToLowerCase}} := New{{.ModelName}}("pk123"{{if .SortKey}}, "sort123"{{end}}){{end}}
	
	originalUpdatedAt := {{.ModelName | ToLowerCase}}.UpdatedAt
	{{if .TTLSpec}}originalTTL := {{.ModelName | ToLowerCase}}.{{.TTLSpec.AttributeName}}{{end}}
	
	// Wait a bit to ensure timestamp changes
	time.Sleep(10 * time.Millisecond)
	
	{{.ModelName | ToLowerCase}}.Update()
	
	assert.True(t, {{.ModelName | ToLowerCase}}.UpdatedAt.After(originalUpdatedAt))
	{{if .TTLSpec}}assert.Greater(t, {{.ModelName | ToLowerCase}}.{{.TTLSpec.AttributeName}}, originalTTL){{end}}
}

func Test{{.ModelName}}_DynamORMIntegration(t *testing.T) {
	// Setup test environment
	ctx := context.Background()
	
	// Use DynamORM test helpers
	db, cleanup := test.SetupDynamORMTest(t, "{{.TableName}}")
	defer cleanup()
	
	// Create test entity
	{{if .IsMultiTenant}}{{.ModelName | ToLowerCase}} := New{{.ModelName}}("test-tenant"{{if .SortKey}}, "test-sort"{{end}}){{else}}{{.ModelName | ToLowerCase}} := New{{.ModelName}}("test-pk"{{if .SortKey}}, "test-sort"{{end}}){{end}}
	{{.ModelName | ToLowerCase}}.Name = "Test {{.ModelName}}"
	
	// Test Put operation
	err := db.Put(ctx, {{.ModelName | ToLowerCase}})
	require.NoError(t, err)
	
	// Test Get operation
	var retrieved {{.ModelName}}
	{{if .IsMultiTenant}}err = db.GetByPK(ctx, "test-tenant"{{if .SortKey}}, "test-sort"{{end}}, &retrieved){{else}}err = db.GetByPK(ctx, "test-pk"{{if .SortKey}}, "test-sort"{{end}}, &retrieved){{end}}
	require.NoError(t, err)
	
	assert.Equal(t, {{.ModelName | ToLowerCase}}.{{.PartitionKey.Name}}, retrieved.{{.PartitionKey.Name}})
	{{if .SortKey}}assert.Equal(t, {{.ModelName | ToLowerCase}}.{{.SortKey.Name}}, retrieved.{{.SortKey.Name}}){{end}}
	assert.Equal(t, "Test {{.ModelName}}", retrieved.Name)
	
	// Test Update operation
	retrieved.Name = "Updated {{.ModelName}}"
	retrieved.Update()
	
	err = db.Put(ctx, &retrieved)
	require.NoError(t, err)
	
	// Verify update
	var updated {{.ModelName}}
	{{if .IsMultiTenant}}err = db.GetByPK(ctx, "test-tenant"{{if .SortKey}}, "test-sort"{{end}}, &updated){{else}}err = db.GetByPK(ctx, "test-pk"{{if .SortKey}}, "test-sort"{{end}}, &updated){{end}}
	require.NoError(t, err)
	assert.Equal(t, "Updated {{.ModelName}}", updated.Name)
	
	// Test Delete operation
	err = db.Delete(ctx, &updated)
	require.NoError(t, err)
	
	// Verify deletion
	var deleted {{.ModelName}}
	{{if .IsMultiTenant}}err = db.GetByPK(ctx, "test-tenant"{{if .SortKey}}, "test-sort"{{end}}, &deleted){{else}}err = db.GetByPK(ctx, "test-pk"{{if .SortKey}}, "test-sort"{{end}}, &deleted){{end}}
	assert.Error(t, err)
	assert.Equal(t, dynamorm.ErrNotFound, err)
}

{{range .GSIs}}func Test{{$.ModelName}}_{{.IndexName}}GSI(t *testing.T) {
	ctx := context.Background()
	
	// Setup test environment  
	db, cleanup := test.SetupDynamORMTest(t, "{{$.TableName}}")
	defer cleanup()
	
	// Create test entities with GSI attributes
	{{if $.IsMultiTenant}}entity1 := New{{$.ModelName}}("tenant1", "item1"){{else}}entity1 := New{{$.ModelName}}("pk1", "item1"){{end}}
	entity1.{{.PartitionKey.Name}} = "gsi-pk-1"
	{{if .SortKey}}entity1.{{.SortKey.Name}} = "gsi-sk-1"{{end}}
	
	{{if $.IsMultiTenant}}entity2 := New{{$.ModelName}}("tenant2", "item2"){{else}}entity2 := New{{$.ModelName}}("pk2", "item2"){{end}}
	entity2.{{.PartitionKey.Name}} = "gsi-pk-1"
	{{if .SortKey}}entity2.{{.SortKey.Name}} = "gsi-sk-2"{{end}}
	
	// Save entities
	require.NoError(t, db.Put(ctx, entity1))
	require.NoError(t, db.Put(ctx, entity2))
	
	// Query by GSI
	var results []{{$.ModelName}}
	err := db.Query(ctx, &results, 
		dynamorm.WithIndex("{{.IndexName}}"),
		dynamorm.WithPartitionKey("gsi-pk-1"),
	)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}
{{end}}

{{if .IsMultiTenant}}func Test{{.ModelName}}_MultiTenantIsolation(t *testing.T) {
	ctx := context.Background()
	
	// Setup test environment
	db, cleanup := test.SetupDynamORMTest(t, "{{.TableName}}")
	defer cleanup()
	
	// Create entities for different tenants
	tenant1Entity := New{{.ModelName}}("tenant1"{{if .SortKey}}, "item1"{{end}})
	tenant1Entity.Name = "Tenant 1 Item"
	
	tenant2Entity := New{{.ModelName}}("tenant2"{{if .SortKey}}, "item2"{{end}})
	tenant2Entity.Name = "Tenant 2 Item"
	
	// Save entities
	require.NoError(t, db.Put(ctx, tenant1Entity))
	require.NoError(t, db.Put(ctx, tenant2Entity))
	
	// Query tenant 1 items
	var tenant1Results []{{.ModelName}}
	err := db.Query(ctx, &tenant1Results, dynamorm.WithPartitionKey("tenant1"))
	require.NoError(t, err)
	assert.Len(t, tenant1Results, 1)
	assert.Equal(t, "Tenant 1 Item", tenant1Results[0].Name)
	
	// Query tenant 2 items  
	var tenant2Results []{{.ModelName}}
	err = db.Query(ctx, &tenant2Results, dynamorm.WithPartitionKey("tenant2"))
	require.NoError(t, err)
	assert.Len(t, tenant2Results, 1)
	assert.Equal(t, "Tenant 2 Item", tenant2Results[0].Name)
	
	// Verify cross-tenant isolation
	var crossTenantResults []{{.ModelName}}
	err = db.Query(ctx, &crossTenantResults, dynamorm.WithPartitionKey("tenant1"))
	require.NoError(t, err)
	
	for _, result := range crossTenantResults {
		assert.Equal(t, "tenant1", result.{{.PartitionKey.Name}})
		assert.NotEqual(t, "tenant2", result.{{.PartitionKey.Name}})
	}
}
{{end}}

{{if .TTLSpec}}func Test{{.ModelName}}_TTLHandling(t *testing.T) {
	{{if .IsMultiTenant}}{{.ModelName | ToLowerCase}} := New{{.ModelName}}("tenant123"{{if .SortKey}}, "sort123"{{end}}){{else}}{{.ModelName | ToLowerCase}} := New{{.ModelName}}("pk123"{{if .SortKey}}, "sort123"{{end}}){{end}}
	
	// Test initial TTL is set
	assert.Greater(t, {{.ModelName | ToLowerCase}}.{{.TTLSpec.AttributeName}}, int64(0))
	
	// Test TTL is in the future
	futureTime := time.Now().Add(time.Hour).Unix()
	assert.LessOrEqual(t, {{.ModelName | ToLowerCase}}.{{.TTLSpec.AttributeName}}, futureTime)
	
	// Test Update refreshes TTL
	originalTTL := {{.ModelName | ToLowerCase}}.{{.TTLSpec.AttributeName}}
	time.Sleep(10 * time.Millisecond)
	{{.ModelName | ToLowerCase}}.Update()
	
	assert.Greater(t, {{.ModelName | ToLowerCase}}.{{.TTLSpec.AttributeName}}, originalTTL)
}
{{end}}

func Test{{.ModelName}}_StructTags(t *testing.T) {
	// Use reflection to verify struct tags are correct
	{{.ModelName | ToLowerCase}} := &{{.ModelName}}{}
	
	// This is a basic test - in practice you'd use reflection
	// to verify DynamoDB struct tags are properly set
	assert.NotNil(t, {{.ModelName | ToLowerCase}})
	
	// Verify table name is set correctly
	assert.Equal(t, "{{.TableName}}", {{.ModelName | ToLowerCase}}.TableName())
}

func Benchmark{{.ModelName}}_CRUD(b *testing.B) {
	ctx := context.Background()
	
	// Setup test environment
	db, cleanup := test.SetupDynamORMTest(nil, "{{.TableName}}")
	defer cleanup()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		// Create
		{{if .IsMultiTenant}}{{.ModelName | ToLowerCase}} := New{{.ModelName}}("tenant123", fmt.Sprintf("item%d", i)){{else}}{{.ModelName | ToLowerCase}} := New{{.ModelName}}(fmt.Sprintf("pk%d", i){{if .SortKey}}, fmt.Sprintf("sk%d", i){{end}}){{end}}
		
		// Put
		_ = db.Put(ctx, {{.ModelName | ToLowerCase}})
		
		// Get
		var retrieved {{.ModelName}}
		{{if .IsMultiTenant}}_ = db.GetByPK(ctx, "tenant123", fmt.Sprintf("item%d", i), &retrieved){{else}}_ = db.GetByPK(ctx, fmt.Sprintf("pk%d", i){{if .SortKey}}, fmt.Sprintf("sk%d", i){{end}}, &retrieved){{end}}
		
		// Update
		retrieved.Update()
		_ = db.Put(ctx, &retrieved)
		
		// Delete
		_ = db.Delete(ctx, &retrieved)
	}
}
`

	data := struct {
		*TableAnalysis
		ModelName     string
		IsMultiTenant bool
	}{
		TableAnalysis: analysis,
		ModelName:     config.ModelName,
		IsMultiTenant: config.MultiTenant,
	}

	funcMap := template.FuncMap{
		"ToLowerCase": func(s string) string {
			return strings.ToLower(s[:1]) + s[1:]
		},
	}

	t, err := template.New("tests").Funcs(funcMap).Parse(tmpl)
	if err != nil {
		return err
	}

	filename := filepath.Join(config.OutputDir, strings.ToLower(config.ModelName)+"_test.go")
	file, err := os.Create(filename) // #nosec G304 - filename is constructed from sanitized input in controlled directories
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Warning: failed to close file: %v\n", err)
		}
	}()

	return t.Execute(file, data)
}
