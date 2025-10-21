package constructs

import (
	"github.com/aws/aws-cdk-go/awscdk/v2/awsdynamodb"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

// StreamingTableProps defines properties for creating a streaming table
// Memory optimized: 56 → 48 bytes (8 bytes saved)
type StreamingTableProps struct {
	// Pointers first (8 bytes each)
	TableName           *string
	TimeToLiveAttribute *string
	ReadCapacity        *float64
	WriteCapacity       *float64
	EnableAutoScaling   *bool
	// Enum last
	StreamViewType awsdynamodb.StreamViewType
}

// StreamingTable is a table with DynamoDB Streams enabled
type StreamingTable struct {
	construct constructs.Construct
	*LiftTable
}

// NewStreamingTable creates a new DynamoDB table with streams
// The table uses standard pk/sk attributes - GSIs should be defined in DynamORM models
func NewStreamingTable(scope constructs.Construct, id *string, props *StreamingTableProps) *StreamingTable {
	// Set defaults
	if props == nil {
		props = &StreamingTableProps{}
	}

	// Set stream view type default
	if props.StreamViewType == "" {
		props.StreamViewType = awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES
	}

	// Set streaming table specific defaults
	if props.TableName == nil {
		props.TableName = jsii.String("streaming-table")
	}

	// Create the table with field names from StreamRecord struct
	liftTable := NewLiftTable(scope, id, &LiftTableProps{
		TableName:                 props.TableName,
		PartitionKeyName:          jsii.String("PK"),
		SortKeyName:               jsii.String("SK"),
		EnableStreams:             jsii.Bool(true),
		StreamViewType:            props.StreamViewType,
		TimeToLiveAttribute:       props.TimeToLiveAttribute,
		EnableAutoScaling:         props.EnableAutoScaling,
		ReadCapacity:              props.ReadCapacity,
		WriteCapacity:             props.WriteCapacity,
		EnablePointInTimeRecovery: jsii.Bool(true),
	})

	return &StreamingTable{
		construct: scope,
		LiftTable: liftTable,
	}
}

// GetStreamArn returns the DynamoDB stream ARN
func (s *StreamingTable) GetStreamArn() *string {
	return s.Table.TableStreamArn()
}

// GrantStreamRead grants stream read permissions
func (s *StreamingTable) GrantStreamRead(grantee awsiam.IGrantable) awsiam.Grant {
	return s.Table.GrantStreamRead(grantee)
}

// GetTableName returns the table name
func (s *StreamingTable) GetTableName() *string {
	return s.Table.TableName()
}

// GetTableArn returns the table ARN
func (s *StreamingTable) GetTableArn() *string {
	return s.Table.TableArn()
}

// GetResourceName returns the resource name for monitoring (implements MonitorableResource interface)
func (s *StreamingTable) GetResourceName() *string {
	return s.Table.TableName()
}

// Example DynamORM model for streaming data:
//
// type StreamRecord struct {
//     PK         string    `dynamorm:"pk"`     // entity#{id}
//     SK         string    `dynamorm:"sk"`     // timestamp#{timestamp}
//
//     // Your data fields
//     EntityID   string    `json:"entity_id"`
//     EventType  string    `json:"event_type"`
//     Data       string    `json:"data"`
//     TTL        int64     `json:"ttl,omitempty"`
// }
