package constructs

import "github.com/aws/aws-cdk-go/awscdk/v2/awssqs"

// QueuePair holds a primary queue and its DLQ (dead-letter queue).
//
// This lives in constructs (not stacks) to avoid stacks<->constructs import cycles.
type QueuePair struct {
	Primary awssqs.Queue
	DLQ     awssqs.Queue
}
