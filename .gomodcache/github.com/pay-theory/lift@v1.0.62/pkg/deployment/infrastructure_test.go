package deployment

import (
	"testing"
	"time"
)

func TestNewInfrastructureGenerator(t *testing.T) {
	config := InfrastructureConfig{
		ApplicationName: "test-app",
		Environment:     "dev",
		Region:          "us-east-1",
		Lambda: LambdaConfig{
			Runtime:    "go1.x",
			Handler:    "main",
			Timeout:    30,
			MemorySize: 128,
		},
		APIGateway: APIGatewayConfig{
			Type:      "REST",
			StageName: "dev",
		},
		Database: DatabaseConfig{
			Type: "DynamoDB",
			Tables: []TableConfig{
				{
					Name:        "users",
					BillingMode: "PAY_PER_REQUEST",
					HashKey:     "id",
					Attributes: []AttributeConfig{
						{Name: "id", Type: "S"},
					},
				},
			},
		},
		Tags: map[string]string{
			"Environment": "dev",
			"Application": "test-app",
		},
	}

	generator := NewInfrastructureGenerator(ProviderPulumi, config)

	if generator == nil {
		t.Fatal("Expected generator to be created")
	}

	if generator.provider != ProviderPulumi {
		t.Errorf("Expected provider %s, got %s", ProviderPulumi, generator.provider)
	}

	if generator.config.ApplicationName != "test-app" {
		t.Errorf("Expected application name 'test-app', got %s", generator.config.ApplicationName)
	}
}

func TestGenerateTemplate(t *testing.T) {
	config := InfrastructureConfig{
		ApplicationName: "test-app",
		Environment:     "dev",
		Region:          "us-east-1",
		Lambda: LambdaConfig{
			Runtime:    "go1.x",
			Handler:    "main",
			Timeout:    30,
			MemorySize: 128,
		},
		APIGateway: APIGatewayConfig{
			Type:      "REST",
			StageName: "dev",
		},
		Database: DatabaseConfig{
			Type: "DynamoDB",
			Tables: []TableConfig{
				{
					Name:        "users",
					BillingMode: "PAY_PER_REQUEST",
					HashKey:     "id",
					Attributes: []AttributeConfig{
						{Name: "id", Type: "S"},
					},
				},
			},
		},
		Tags: map[string]string{
			"Environment": "dev",
		},
	}

	generator := NewInfrastructureGenerator(ProviderPulumi, config)
	template, err := generator.GenerateTemplate()

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if template == nil {
		t.Fatal("Expected template to be generated")
	}

	// Verify template properties
	if template.Name != "test-app-dev" {
		t.Errorf("Expected template name 'test-app-dev', got %s", template.Name)
	}

	if template.Provider != ProviderPulumi {
		t.Errorf("Expected provider %s, got %s", ProviderPulumi, template.Provider)
	}

	// Verify Lambda resources
	if _, exists := template.Resources["LambdaFunction"]; !exists {
		t.Error("Expected LambdaFunction resource to exist")
	}

	if _, exists := template.Resources["LambdaExecutionRole"]; !exists {
		t.Error("Expected LambdaExecutionRole resource to exist")
	}

	// Verify API Gateway resources
	if _, exists := template.Resources["APIGateway"]; !exists {
		t.Error("Expected APIGateway resource to exist")
	}

	// Verify DynamoDB resources
	if _, exists := template.Resources["DynamoTableUsers"]; !exists {
		t.Error("Expected DynamoTableUsers resource to exist")
	}

	// Verify outputs
	if _, exists := template.Outputs["LambdaFunctionArn"]; !exists {
		t.Error("Expected LambdaFunctionArn output to exist")
	}

	if _, exists := template.Outputs["APIGatewayURL"]; !exists {
		t.Error("Expected APIGatewayURL output to exist")
	}
}

func TestValidateTemplate(t *testing.T) {
	generator := NewInfrastructureGenerator(ProviderPulumi, InfrastructureConfig{
		ApplicationName: "test-app",
		Environment:     "dev",
	})

	// Test valid template
	validTemplate := &InfrastructureTemplate{
		Name: "test-template",
		Resources: map[string]Resource{
			"TestResource": {
				Type: "AWS::Lambda::Function",
				Name: "test-function",
			},
		},
	}

	err := generator.ValidateTemplate(validTemplate)
	if err != nil {
		t.Errorf("Expected no error for valid template, got %v", err)
	}

	// Test template without name
	invalidTemplate := &InfrastructureTemplate{
		Resources: map[string]Resource{
			"TestResource": {
				Type: "AWS::Lambda::Function",
				Name: "test-function",
			},
		},
	}

	err = generator.ValidateTemplate(invalidTemplate)
	if err == nil {
		t.Error("Expected error for template without name")
	}

	// Test template without resources
	emptyTemplate := &InfrastructureTemplate{
		Name:      "test-template",
		Resources: map[string]Resource{},
	}

	err = generator.ValidateTemplate(emptyTemplate)
	if err == nil {
		t.Error("Expected error for template without resources")
	}

	// Test template with invalid dependencies
	invalidDepsTemplate := &InfrastructureTemplate{
		Name: "test-template",
		Resources: map[string]Resource{
			"TestResource": {
				Type:         "AWS::Lambda::Function",
				Name:         "test-function",
				Dependencies: []string{"NonExistentResource"},
			},
		},
	}

	err = generator.ValidateTemplate(invalidDepsTemplate)
	if err == nil {
		t.Error("Expected error for template with invalid dependencies")
	}
}

func TestExportTemplate(t *testing.T) {
	generator := NewInfrastructureGenerator(ProviderPulumi, InfrastructureConfig{})

	template := &InfrastructureTemplate{
		Name:      "test-template",
		Provider:  ProviderPulumi,
		Version:   "1.0.0",
		CreatedAt: time.Now(),
		Resources: map[string]Resource{
			"TestResource": {
				Type: "AWS::Lambda::Function",
				Name: "test-function",
			},
		},
	}

	// Test JSON export
	jsonData, err := generator.ExportTemplate(template, "json")
	if err != nil {
		t.Errorf("Expected no error for JSON export, got %v", err)
	}

	if len(jsonData) == 0 {
		t.Error("Expected JSON data to be generated")
	}

	// Test unsupported format
	_, err = generator.ExportTemplate(template, "unsupported")
	if err == nil {
		t.Error("Expected error for unsupported format")
	}
}

func TestGenerateAttributeDefinitions(t *testing.T) {
	generator := NewInfrastructureGenerator(ProviderPulumi, InfrastructureConfig{})

	attributes := []AttributeConfig{
		{Name: "id", Type: "S"},
		{Name: "timestamp", Type: "N"},
	}

	definitions := generator.generateAttributeDefinitions(attributes)

	if len(definitions) != 2 {
		t.Errorf("Expected 2 attribute definitions, got %d", len(definitions))
	}

	if definitions[0]["AttributeName"] != "id" {
		t.Errorf("Expected first attribute name 'id', got %v", definitions[0]["AttributeName"])
	}

	if definitions[0]["AttributeType"] != "S" {
		t.Errorf("Expected first attribute type 'S', got %v", definitions[0]["AttributeType"])
	}
}

func TestGenerateKeySchema(t *testing.T) {
	generator := NewInfrastructureGenerator(ProviderPulumi, InfrastructureConfig{})

	// Test with hash key only
	schema := generator.generateKeySchema("id", "")

	if len(schema) != 1 {
		t.Errorf("Expected 1 key schema element, got %d", len(schema))
	}

	if schema[0]["AttributeName"] != "id" {
		t.Errorf("Expected attribute name 'id', got %v", schema[0]["AttributeName"])
	}

	if schema[0]["KeyType"] != "HASH" {
		t.Errorf("Expected key type 'HASH', got %v", schema[0]["KeyType"])
	}

	// Test with hash and range key
	schema = generator.generateKeySchema("id", "timestamp")

	if len(schema) != 2 {
		t.Errorf("Expected 2 key schema elements, got %d", len(schema))
	}

	if schema[1]["AttributeName"] != "timestamp" {
		t.Errorf("Expected second attribute name 'timestamp', got %v", schema[1]["AttributeName"])
	}

	if schema[1]["KeyType"] != "RANGE" {
		t.Errorf("Expected second key type 'RANGE', got %v", schema[1]["KeyType"])
	}
}

func TestLambdaResourceGeneration(t *testing.T) {
	config := InfrastructureConfig{
		ApplicationName: "test-app",
		Environment:     "dev",
		Lambda: LambdaConfig{
			Runtime:             "go1.x",
			Handler:             "main",
			Timeout:             30,
			MemorySize:          128,
			ReservedConcurrency: &[]int{10}[0],
			Environment: map[string]string{
				"ENV": "dev",
			},
			Layers:          []string{"arn:aws:lambda:us-east-1:123456789012:layer:test-layer:1"},
			DeadLetterQueue: true,
		},
		Tags: map[string]string{
			"Environment": "dev",
		},
	}

	generator := NewInfrastructureGenerator(ProviderPulumi, config)
	template, err := generator.GenerateTemplate()

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify Lambda function properties
	lambdaResource, exists := template.Resources["LambdaFunction"]
	if !exists {
		t.Fatal("Expected LambdaFunction resource to exist")
	}

	props := lambdaResource.Properties

	if props["Runtime"] != "go1.x" {
		t.Errorf("Expected runtime 'go1.x', got %v", props["Runtime"])
	}

	if props["Handler"] != "main" {
		t.Errorf("Expected handler 'main', got %v", props["Handler"])
	}

	if props["Timeout"] != 30 {
		t.Errorf("Expected timeout 30, got %v", props["Timeout"])
	}

	if props["MemorySize"] != 128 {
		t.Errorf("Expected memory size 128, got %v", props["MemorySize"])
	}

	if props["ReservedConcurrencyLimit"] != 10 {
		t.Errorf("Expected reserved concurrency 10, got %v", props["ReservedConcurrencyLimit"])
	}

	// Verify environment variables
	env, exists := props["Environment"]
	if !exists {
		t.Error("Expected Environment property to exist")
	} else {
		envMap, ok := env.(map[string]any)
		if !ok {
			t.Errorf("Expected Environment to be map[string]any, got %T", env)
		} else if variables, ok := envMap["Variables"].(map[string]string); !ok {
			t.Errorf("Expected Variables to be map[string]string, got %T", envMap["Variables"])
		} else if variables["ENV"] != "dev" {
			t.Errorf("Expected ENV variable 'dev', got %v", variables["ENV"])
		}
	}

	// Verify layers
	layers, exists := props["Layers"]
	if !exists {
		t.Error("Expected Layers property to exist")
	} else {
		layersList, ok := layers.([]string)
		if !ok {
			t.Errorf("Expected Layers to be []string, got %T", layers)
		} else if len(layersList) != 1 {
			t.Errorf("Expected 1 layer, got %d", len(layersList))
		}
	}

	// Verify DLQ
	dlqConfig, exists := props["DeadLetterConfig"]
	if !exists {
		t.Error("Expected DeadLetterConfig property to exist")
	} else {
		dlqMap, ok := dlqConfig.(map[string]any)
		if !ok {
			t.Errorf("Expected DeadLetterConfig to be map[string]any, got %T", dlqConfig)
		} else if dlqMap["TargetArn"] == "" {
			t.Error("Expected TargetArn to be set")
		}
	}

	// Verify DLQ resource exists
	if _, exists := template.Resources["DeadLetterQueue"]; !exists {
		t.Error("Expected DeadLetterQueue resource to exist")
	}
}

func TestDynamoDBResourceGeneration(t *testing.T) {
	config := InfrastructureConfig{
		ApplicationName: "test-app",
		Environment:     "dev",
		Database: DatabaseConfig{
			Type:          "DynamoDB",
			GlobalTables:  true,
			BackupEnabled: true,
			Encryption: EncryptionConfig{
				Enabled:  true,
				KMSKeyId: "alias/test-key",
			},
			StreamEnabled: true,
			Tables: []TableConfig{
				{
					Name:        "users",
					BillingMode: "PAY_PER_REQUEST",
					HashKey:     "id",
					RangeKey:    "timestamp",
					Attributes: []AttributeConfig{
						{Name: "id", Type: "S"},
						{Name: "timestamp", Type: "N"},
						{Name: "email", Type: "S"},
					},
					GlobalIndexes: []GlobalIndexConfig{
						{
							Name:    "email-index",
							HashKey: "email",
							Projection: ProjectionConfig{
								Type: "ALL",
							},
						},
					},
					StreamEnabled: true,
					BackupEnabled: true,
				},
			},
		},
	}

	generator := NewInfrastructureGenerator(ProviderPulumi, config)
	template, err := generator.GenerateTemplate()

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify DynamoDB table
	tableResource, exists := template.Resources["DynamoTableUsers"]
	if !exists {
		t.Fatal("Expected DynamoTableUsers resource to exist")
	}

	props := tableResource.Properties

	if props["BillingMode"] != "PAY_PER_REQUEST" {
		t.Errorf("Expected billing mode 'PAY_PER_REQUEST', got %v", props["BillingMode"])
	}

	// Verify key schema
	keySchema, exists := props["KeySchema"]
	if !exists {
		t.Error("Expected KeySchema property to exist")
	} else {
		keySchemaList, ok := keySchema.([]map[string]any)
		if !ok {
			t.Errorf("Expected KeySchema to be []map[string]any, got %T", keySchema)
		} else if len(keySchemaList) != 2 {
			t.Errorf("Expected 2 key schema elements, got %d", len(keySchemaList))
		}
	}

	// Verify attribute definitions
	attrDefs, exists := props["AttributeDefinitions"]
	if !exists {
		t.Error("Expected AttributeDefinitions property to exist")
	} else {
		attrDefsList, ok := attrDefs.([]map[string]any)
		if !ok {
			t.Errorf("Expected AttributeDefinitions to be []map[string]any, got %T", attrDefs)
		} else if len(attrDefsList) != 3 {
			t.Errorf("Expected 3 attribute definitions, got %d", len(attrDefsList))
		}
	}

	// Verify stream specification
	streamSpec, exists := props["StreamSpecification"]
	if !exists {
		t.Error("Expected StreamSpecification property to exist")
	} else {
		streamSpecMap, ok := streamSpec.(map[string]any)
		if !ok {
			t.Errorf("Expected StreamSpecification to be map[string]any, got %T", streamSpec)
		} else if streamSpecMap["StreamViewType"] != "NEW_AND_OLD_IMAGES" {
			t.Errorf("Expected stream view type 'NEW_AND_OLD_IMAGES', got %v", streamSpecMap["StreamViewType"])
		}
	}

	// Verify SSE specification
	sseSpec, exists := props["SSESpecification"]
	if !exists {
		t.Error("Expected SSESpecification property to exist")
	} else {
		sseSpecMap, ok := sseSpec.(map[string]any)
		if !ok {
			t.Errorf("Expected SSESpecification to be map[string]any, got %T", sseSpec)
		} else if sseSpecMap["SSEEnabled"] != true {
			t.Error("Expected SSE to be enabled")
		}
	}

	// Verify point-in-time recovery
	pitrSpec, exists := props["PointInTimeRecoverySpecification"]
	if !exists {
		t.Error("Expected PointInTimeRecoverySpecification property to exist")
	} else {
		pitrSpecMap, ok := pitrSpec.(map[string]any)
		if !ok {
			t.Errorf("Expected PointInTimeRecoverySpecification to be map[string]any, got %T", pitrSpec)
		} else if pitrSpecMap["PointInTimeRecoveryEnabled"] != true {
			t.Error("Expected point-in-time recovery to be enabled")
		}
	}

	// Verify global secondary indexes
	gsis, exists := props["GlobalSecondaryIndexes"]
	if !exists {
		t.Error("Expected GlobalSecondaryIndexes property to exist")
	} else {
		gsisList, ok := gsis.([]map[string]any)
		if !ok {
			t.Errorf("Expected GlobalSecondaryIndexes to be []map[string]any, got %T", gsis)
			return
		}
		if len(gsisList) != 1 {
			t.Errorf("Expected 1 global secondary index, got %d", len(gsisList))
			return
		}

		gsi := gsisList[0]
		if gsi["IndexName"] != "email-index" {
			t.Errorf("Expected index name 'email-index', got %v", gsi["IndexName"])
		}
	}
}

func BenchmarkGenerateTemplate(b *testing.B) {
	config := InfrastructureConfig{
		ApplicationName: "benchmark-app",
		Environment:     "test",
		Region:          "us-east-1",
		Lambda: LambdaConfig{
			Runtime:    "go1.x",
			Handler:    "main",
			Timeout:    30,
			MemorySize: 128,
		},
		APIGateway: APIGatewayConfig{
			Type:      "REST",
			StageName: "test",
		},
		Database: DatabaseConfig{
			Type: "DynamoDB",
			Tables: []TableConfig{
				{
					Name:        "users",
					BillingMode: "PAY_PER_REQUEST",
					HashKey:     "id",
					Attributes: []AttributeConfig{
						{Name: "id", Type: "S"},
					},
				},
			},
		},
	}

	generator := NewInfrastructureGenerator(ProviderPulumi, config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := generator.GenerateTemplate()
		if err != nil {
			b.Fatalf("Template generation failed: %v", err)
		}
	}
}
