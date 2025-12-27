package patterns

import (
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
)

func TestProcessSQSBatch_AllSuccess(t *testing.T) {
	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{MessageId: "a"},
			{MessageId: "b"},
		},
	}

	err := ProcessSQSBatch(nil, event, func(_ *lift.Context, _ events.SQSMessage) error { return nil })
	require.NoError(t, err)
}

func TestProcessSQSBatch_PartialFailure(t *testing.T) {
	event := events.SQSEvent{
		Records: []events.SQSMessage{
			{MessageId: "a"},
			{MessageId: "b"},
		},
	}

	err := ProcessSQSBatch(nil, event, func(_ *lift.Context, msg events.SQSMessage) error {
		if msg.MessageId == "b" {
			return assertErr("fail")
		}
		return nil
	})
	require.Error(t, err)

	batchErr, ok := err.(*SQSBatchError)
	require.True(t, ok)
	require.Equal(t, []string{"b"}, batchErr.Failed)
	require.Equal(t, []string{"a"}, batchErr.Succeeded)
	require.Contains(t, batchErr.Error(), "partial batch failure")
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
