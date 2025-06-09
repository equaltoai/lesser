package dynamodb

import (
	"math"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// safeInt32 converts an int to a pointer to int32, capping at math.MaxInt32
// to prevent overflow when passing to DynamoDB's Limit parameter.
func safeInt32(n int) *int32 {
	if n > math.MaxInt32 {
		n = math.MaxInt32
	}
	return aws.Int32(int32(n))
}
