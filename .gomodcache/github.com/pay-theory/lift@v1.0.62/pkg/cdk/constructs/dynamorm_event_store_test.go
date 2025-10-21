package constructs

import (
	"testing"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awss3"
	"github.com/aws/jsii-runtime-go"
)

func TestNewDynamORMEventStore(t *testing.T) {
	// Create a test app and stack
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create event store with default settings
	eventStore := NewDynamORMEventStore(stack, jsii.String("TestEventStore"), &DynamORMEventStoreProps{
		Pattern: EventStorePattern_SINGLE_TABLE,
	})

	// Test that the event store was created successfully
	if eventStore == nil {
		t.Fatal("DynamORMEventStore should not be nil")
	}

	// Test that event table was created
	if eventStore.EventTable == nil {
		t.Fatal("Event table should be created")
	}

	// Test that snapshot table was created (default snapshot strategy is FREQUENCY)
	if eventStore.SnapshotTable == nil {
		t.Fatal("Snapshot table should be created with default settings")
	}

	// Test that metrics were created
	if eventStore.Metrics == nil {
		t.Fatal("Metrics should be created by default")
	}

	// Test that IAM roles were created
	if eventStore.EventReaderRole == nil {
		t.Fatal("Event reader role should be created")
	}
	if eventStore.EventWriterRole == nil {
		t.Fatal("Event writer role should be created")
	}
	if eventStore.SnapshotManagerRole == nil {
		t.Fatal("Snapshot manager role should be created")
	}

	// Synth the stack to validate
	app.Synth(nil)
}

func TestDynamORMEventStoreMultiTable(t *testing.T) {
	// Create a test app and stack
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create event store with multi-table pattern
	eventStore := NewDynamORMEventStore(stack, jsii.String("TestEventStore"), &DynamORMEventStoreProps{
		Pattern:           EventStorePattern_MULTI_TABLE,
		EventTableName:    jsii.String("events"),
		SnapshotTableName: jsii.String("snapshots"),
		SnapshotStrategy:  SnapshotStrategy_SIZE_BASED,
		SnapshotSizeLimit: intPtr(2048), // 2MB
		EnableMetrics:     jsii.Bool(true),
		EnableGSIs:        jsii.Bool(true),
	})

	// Test that the pattern is correct
	if eventStore.props.Pattern != EventStorePattern_MULTI_TABLE {
		t.Fatal("Pattern should be MULTI_TABLE")
	}

	// Test that both tables were created
	if eventStore.EventTable == nil {
		t.Fatal("Event table should be created")
	}
	if eventStore.SnapshotTable == nil {
		t.Fatal("Snapshot table should be created")
	}

	// Synth the stack to validate
	app.Synth(nil)
}

func TestDynamORMEventStoreMultiTenant(t *testing.T) {
	// Create a test app and stack
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create multi-tenant event store
	eventStore := NewDynamORMEventStore(stack, jsii.String("TestEventStore"), &DynamORMEventStoreProps{
		Pattern:                EventStorePattern_AGGREGATE_TABLE,
		EnableMultiTenant:      jsii.Bool(true),
		TenantAttribute:        jsii.String("tenant_id"),
		EnableEventVersioning:  jsii.Bool(true),
		EnableEventEncryption:  jsii.Bool(true),
		EnableEventCompression: jsii.Bool(true),
		EventStreamEnabled:     jsii.Bool(true),
		SnapshotStreamEnabled:  jsii.Bool(true),
		EnableDetailedMetrics:  jsii.Bool(true),
		AlertThresholds: &EventStoreAlertThresholds{
			HighEventRate:        jsii.Number(500),
			HighErrorRate:        jsii.Number(10),
			HighLatency:          jsii.Number(200),
			LowSnapshotFrequency: jsii.Number(2),
			HighStorageUsage:     jsii.Number(50),
		},
	})

	// Test multi-tenant configuration
	if eventStore.props.EnableMultiTenant == nil || !*eventStore.props.EnableMultiTenant {
		t.Fatal("Multi-tenant should be enabled")
	}

	// Test that tenant metrics exist
	if eventStore.Metrics["TenantEventRate"] == nil {
		t.Fatal("Tenant event rate metric should exist")
	}

	// Synth the stack to validate
	app.Synth(nil)
}

func TestDynamORMEventStoreWithArchival(t *testing.T) {
	// Create a test app and stack
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create event store with archival enabled
	eventStore := NewDynamORMEventStore(stack, jsii.String("TestEventStore"), &DynamORMEventStoreProps{
		Pattern:           EventStorePattern_SINGLE_TABLE,
		EnableArchival:    jsii.Bool(true),
		ArchivalAfter:     awscdk.Duration_Days(jsii.Number(180)),  // 6 months
		EventTTL:          awscdk.Duration_Days(jsii.Number(2555)), // 7 years
		SnapshotRetention: awscdk.Duration_Days(jsii.Number(365)),  // 1 year
	})

	// Test that archival bucket was created
	if eventStore.ArchivalBucket == nil {
		t.Fatal("Archival bucket should be created")
	}

	// Test archival configuration
	if eventStore.props.EnableArchival == nil || !*eventStore.props.EnableArchival {
		t.Fatal("Archival should be enabled")
	}

	// Synth the stack to validate
	app.Synth(nil)
}

func TestDynamORMEventStoreWithExistingBucket(t *testing.T) {
	// Create a test app and stack
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create existing S3 bucket
	existingBucket := awss3.NewBucket(stack, jsii.String("ExistingBucket"), &awss3.BucketProps{
		BucketName: jsii.String("existing-event-archive-bucket"),
	})

	// Create event store with existing archival bucket
	eventStore := NewDynamORMEventStore(stack, jsii.String("TestEventStore"), &DynamORMEventStoreProps{
		Pattern:        EventStorePattern_SINGLE_TABLE,
		EnableArchival: jsii.Bool(true),
		ArchivalBucket: existingBucket,
	})

	// Test that the existing bucket is used
	if eventStore.ArchivalBucket != existingBucket {
		t.Fatal("Existing archival bucket should be used")
	}

	// Synth the stack to validate
	app.Synth(nil)
}

func TestDynamORMEventStoreSnapshotDisabled(t *testing.T) {
	// Create a test app and stack
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create event store with snapshots disabled
	eventStore := NewDynamORMEventStore(stack, jsii.String("TestEventStore"), &DynamORMEventStoreProps{
		Pattern:          EventStorePattern_SINGLE_TABLE,
		SnapshotStrategy: SnapshotStrategy_DISABLED,
	})

	// Test that snapshot table was not created
	if eventStore.SnapshotTable != nil {
		t.Fatal("Snapshot table should not be created when snapshots are disabled")
	}

	// Test that snapshot metrics don't exist
	if eventStore.Metrics["SnapshotsCreated"] != nil {
		t.Fatal("Snapshot metrics should not exist when snapshots are disabled")
	}

	// Synth the stack to validate
	app.Synth(nil)
}

func TestDynamORMEventStoreEnvironmentVariables(t *testing.T) {
	// Create a test app and stack
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create event store
	eventStore := NewDynamORMEventStore(stack, jsii.String("TestEventStore"), &DynamORMEventStoreProps{
		Pattern:                EventStorePattern_AGGREGATE_TABLE,
		EnableMultiTenant:      jsii.Bool(true),
		TenantAttribute:        jsii.String("org_id"),
		EnableEventVersioning:  jsii.Bool(true),
		EnableEventEncryption:  jsii.Bool(true),
		EnableEventCompression: jsii.Bool(true),
		EventStreamEnabled:     jsii.Bool(true),
		SnapshotStreamEnabled:  jsii.Bool(true),
		EnableArchival:         jsii.Bool(true),
		SnapshotStrategy:       SnapshotStrategy_TIME_BASED,
		SnapshotFrequency:      intPtr(50),
	})

	// Get environment variables
	env := eventStore.GetEnvironmentVariables()

	// Test required environment variables
	if (*env)["DYNAMORM_EVENT_STORE_ENABLED"] == nil || *(*env)["DYNAMORM_EVENT_STORE_ENABLED"] != trueStr {
		t.Fatal("DYNAMORM_EVENT_STORE_ENABLED should be true")
	}

	if (*env)["DYNAMORM_EVENT_STORE_PATTERN"] == nil || *(*env)["DYNAMORM_EVENT_STORE_PATTERN"] != string(EventStorePattern_AGGREGATE_TABLE) {
		t.Fatal("DYNAMORM_EVENT_STORE_PATTERN should be AGGREGATE_TABLE")
	}

	if (*env)["DYNAMORM_EVENT_TABLE_NAME"] == nil {
		t.Fatal("DYNAMORM_EVENT_TABLE_NAME should be set")
	}

	if (*env)["DYNAMORM_SNAPSHOT_TABLE_NAME"] == nil {
		t.Fatal("DYNAMORM_SNAPSHOT_TABLE_NAME should be set")
	}

	if (*env)["DYNAMORM_SNAPSHOT_STRATEGY"] == nil || *(*env)["DYNAMORM_SNAPSHOT_STRATEGY"] != string(SnapshotStrategy_TIME_BASED) {
		t.Fatal("DYNAMORM_SNAPSHOT_STRATEGY should be TIME_BASED")
	}

	if (*env)["DYNAMORM_SNAPSHOT_FREQUENCY"] == nil || *(*env)["DYNAMORM_SNAPSHOT_FREQUENCY"] != "50" {
		t.Fatal("DYNAMORM_SNAPSHOT_FREQUENCY should be 50")
	}

	if (*env)["DYNAMORM_EVENT_VERSIONING"] == nil || *(*env)["DYNAMORM_EVENT_VERSIONING"] != trueStr {
		t.Fatal("DYNAMORM_EVENT_VERSIONING should be true")
	}

	if (*env)["DYNAMORM_EVENT_ENCRYPTION"] == nil || *(*env)["DYNAMORM_EVENT_ENCRYPTION"] != trueStr {
		t.Fatal("DYNAMORM_EVENT_ENCRYPTION should be true")
	}

	if (*env)["DYNAMORM_EVENT_COMPRESSION"] == nil || *(*env)["DYNAMORM_EVENT_COMPRESSION"] != trueStr {
		t.Fatal("DYNAMORM_EVENT_COMPRESSION should be true")
	}

	if (*env)["DYNAMORM_EVENT_STORE_MULTI_TENANT"] == nil || *(*env)["DYNAMORM_EVENT_STORE_MULTI_TENANT"] != trueStr {
		t.Fatal("DYNAMORM_EVENT_STORE_MULTI_TENANT should be true")
	}

	if (*env)["DYNAMORM_TENANT_ATTRIBUTE"] == nil || *(*env)["DYNAMORM_TENANT_ATTRIBUTE"] != "org_id" {
		t.Fatal("DYNAMORM_TENANT_ATTRIBUTE should be org_id")
	}

	if (*env)["DYNAMORM_ARCHIVAL_ENABLED"] == nil || *(*env)["DYNAMORM_ARCHIVAL_ENABLED"] != trueStr {
		t.Fatal("DYNAMORM_ARCHIVAL_ENABLED should be true")
	}

	// Synth the stack to validate
	app.Synth(nil)
}

func TestDynamORMEventStorePermissions(t *testing.T) {
	// Create a test app and stack
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create event store
	eventStore := NewDynamORMEventStore(stack, jsii.String("TestEventStore"), &DynamORMEventStoreProps{
		Pattern:        EventStorePattern_SINGLE_TABLE,
		EnableArchival: jsii.Bool(true),
	})

	// Create test Lambda functions
	readerFunction := awslambda.NewFunction(stack, jsii.String("ReaderFunction"), &awslambda.FunctionProps{
		FunctionName: jsii.String("event-reader"),
		Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {};")),
		Handler:      jsii.String("index.handler"),
		Runtime:      awslambda.Runtime_NODEJS_18_X(),
	})

	writerFunction := awslambda.NewFunction(stack, jsii.String("WriterFunction"), &awslambda.FunctionProps{
		FunctionName: jsii.String("event-writer"),
		Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {};")),
		Handler:      jsii.String("index.handler"),
		Runtime:      awslambda.Runtime_NODEJS_18_X(),
	})

	snapshotFunction := awslambda.NewFunction(stack, jsii.String("SnapshotFunction"), &awslambda.FunctionProps{
		FunctionName: jsii.String("snapshot-manager"),
		Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {};")),
		Handler:      jsii.String("index.handler"),
		Runtime:      awslambda.Runtime_NODEJS_18_X(),
	})

	adminFunction := awslambda.NewFunction(stack, jsii.String("AdminFunction"), &awslambda.FunctionProps{
		FunctionName: jsii.String("event-admin"),
		Code:         awslambda.Code_FromInline(jsii.String("exports.handler = async () => {};")),
		Handler:      jsii.String("index.handler"),
		Runtime:      awslambda.Runtime_NODEJS_18_X(),
	})

	// Grant different levels of access
	eventStore.GrantEventReaderAccess(readerFunction)
	eventStore.GrantEventWriterAccess(writerFunction)
	eventStore.GrantSnapshotManagerAccess(snapshotFunction)
	eventStore.GrantFullAccess(adminFunction)

	// Test that IAM roles were created
	if eventStore.EventReaderRole == nil {
		t.Fatal("Event reader role should be created")
	}
	if eventStore.EventWriterRole == nil {
		t.Fatal("Event writer role should be created")
	}
	if eventStore.SnapshotManagerRole == nil {
		t.Fatal("Snapshot manager role should be created")
	}

	// Synth the stack to validate
	app.Synth(nil)
}

func TestDynamORMEventStoreGetters(t *testing.T) {
	// Create a test app and stack
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create event store
	eventStore := NewDynamORMEventStore(stack, jsii.String("TestEventStore"), &DynamORMEventStoreProps{
		Pattern:        EventStorePattern_SINGLE_TABLE,
		EnableArchival: jsii.Bool(true),
	})

	// Test getters
	if eventStore.GetEventTable() == nil {
		t.Fatal("GetEventTable should not return nil")
	}

	if eventStore.GetSnapshotTable() == nil {
		t.Fatal("GetSnapshotTable should not return nil")
	}

	if eventStore.GetArchivalBucket() == nil {
		t.Fatal("GetArchivalBucket should not return nil")
	}

	if eventStore.GetEventStoreMetrics() == nil {
		t.Fatal("GetEventStoreMetrics should not return nil")
	}

	if eventStore.GetEventReaderRole() == nil {
		t.Fatal("GetEventReaderRole should not return nil")
	}

	if eventStore.GetEventWriterRole() == nil {
		t.Fatal("GetEventWriterRole should not return nil")
	}

	if eventStore.GetSnapshotManagerRole() == nil {
		t.Fatal("GetSnapshotManagerRole should not return nil")
	}

	// Synth the stack to validate
	app.Synth(nil)
}

func TestDynamORMEventStoreProvisioned(t *testing.T) {
	// Create a test app and stack
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create event store with provisioned capacity
	eventStore := NewDynamORMEventStore(stack, jsii.String("TestEventStore"), &DynamORMEventStoreProps{
		Pattern:           EventStorePattern_SINGLE_TABLE,
		ReadCapacity:      jsii.Number(100),
		WriteCapacity:     jsii.Number(100),
		EnableAutoScaling: jsii.Bool(true),
	})

	// Test that capacity was configured
	if eventStore.props.ReadCapacity == nil || *eventStore.props.ReadCapacity != 100 {
		t.Fatal("Read capacity should be 100")
	}
	if eventStore.props.WriteCapacity == nil || *eventStore.props.WriteCapacity != 100 {
		t.Fatal("Write capacity should be 100")
	}

	// Synth the stack to validate
	app.Synth(nil)
}

func TestDynamORMEventStoreCustomMetrics(t *testing.T) {
	// Create a test app and stack
	app := awscdk.NewApp(nil)
	stack := awscdk.NewStack(app, jsii.String("TestStack"), nil)

	// Create event store with custom alert thresholds
	eventStore := NewDynamORMEventStore(stack, jsii.String("TestEventStore"), &DynamORMEventStoreProps{
		Pattern:               EventStorePattern_SINGLE_TABLE,
		EnableDetailedMetrics: jsii.Bool(true),
		AlertThresholds: &EventStoreAlertThresholds{
			HighEventRate:        jsii.Number(2000),
			HighErrorRate:        jsii.Number(20),
			HighLatency:          jsii.Number(500),
			LowSnapshotFrequency: jsii.Number(5),
			HighStorageUsage:     jsii.Number(500),
		},
	})

	// Test that metrics were created
	if eventStore.Metrics == nil {
		t.Fatal("Metrics should be created")
	}

	// Test that specific metrics exist
	if eventStore.Metrics["EventsWritten"] == nil {
		t.Fatal("EventsWritten metric should exist")
	}
	if eventStore.Metrics["EventsRead"] == nil {
		t.Fatal("EventsRead metric should exist")
	}
	if eventStore.Metrics["EventStoreLatency"] == nil {
		t.Fatal("EventStoreLatency metric should exist")
	}
	if eventStore.Metrics["EventStoreErrors"] == nil {
		t.Fatal("EventStoreErrors metric should exist")
	}

	// Synth the stack to validate
	app.Synth(nil)
}
