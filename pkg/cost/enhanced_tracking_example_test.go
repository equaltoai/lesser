package cost

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestEnhancedTrackingExample_CalculateOperationCost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		meta     *OperationMetadata
		expected float64
	}{
		{
			name:     "GetItem",
			meta:     &OperationMetadata{OperationType: "GetItem"},
			expected: 0.000025,
		},
		{
			name:     "PutItem",
			meta:     &OperationMetadata{OperationType: "PutItem"},
			expected: 0.000125,
		},
		{
			name:     "Query without filters",
			meta:     &OperationMetadata{OperationType: "Query", ItemCount: 10},
			expected: 10 * 0.000025,
		},
		{
			name:     "Query with filters",
			meta:     &OperationMetadata{OperationType: "Query", ItemCount: 10, FilterExpressions: []string{"x=1"}},
			expected: 10 * 0.000025 * 1.5,
		},
		{
			name:     "Scan without filters",
			meta:     &OperationMetadata{OperationType: "Scan", ItemCount: 10},
			expected: 10 * 0.000025 * 2.0,
		},
		{
			name:     "Scan with filters",
			meta:     &OperationMetadata{OperationType: "Scan", ItemCount: 10, FilterExpressions: []string{"x=1"}},
			expected: 10 * 0.000025 * 2.0 * 2.0,
		},
		{
			name:     "BatchWriteItem",
			meta:     &OperationMetadata{OperationType: "BatchWriteItem", ItemCount: 10},
			expected: 10 * 0.000125,
		},
		{
			name:     "consistent read doubles",
			meta:     &OperationMetadata{OperationType: "GetItem", ConsistentRead: true},
			expected: 0.000025 * 2.0,
		},
		{
			name:     "unknown operation",
			meta:     &OperationMetadata{OperationType: "SomethingElse"},
			expected: 0,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := calculateOperationCost(tc.meta)
			if tc.expected == 0 {
				require.Zero(t, got)
				return
			}
			require.InEpsilon(t, tc.expected, got, 0.000001)
		})
	}
}

func TestEnhancedTrackingExample_AnalysisFunctions_DoNotPanic(t *testing.T) {
	tracker := NewEnhancedOperationTracker(zap.NewNop())

	tracker.TrackOperation("op-scan", &OperationMetadata{
		OperationType:     "Scan",
		TableName:         "t",
		ItemCount:         200,
		FilterExpressions: []string{"x=1"},
		ProjectionFields:  nil,
		ConsistentRead:    true,
		IndexName:         "gsi1",
		Conditions:        []QueryCondition{{Field: "pk", Operator: "=", Value: "v"}},
	})
	tracker.TrackOperation("op-low", &OperationMetadata{
		OperationType: "GetItem",
		TableName:     "t",
		ItemCount:     1,
	})

	EnhancedCostAnalysis(tracker)
	RealTimeOptimization(tracker, zap.NewNop())
}
