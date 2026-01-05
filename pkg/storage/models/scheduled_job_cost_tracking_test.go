package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// ScheduledJobCostRecord Tests
// =============================================================================

func TestScheduledJobCostRecord_BeforeCreate(t *testing.T) {
	t.Run("sets timestamps and ID", func(t *testing.T) {
		record := &ScheduledJobCostRecord{
			JobName:  "test-job",
			Schedule: "daily",
			Status:   StatusSuccess,
		}

		err := record.BeforeCreate()

		require.NoError(t, err)
		assert.NotEmpty(t, record.ID, "ID should be generated")
		assert.False(t, record.CreatedAt.IsZero())
		assert.False(t, record.UpdatedAt.IsZero())
		assert.False(t, record.Timestamp.IsZero())
		assert.False(t, record.StartTime.IsZero())
	})

	t.Run("preserves existing ID", func(t *testing.T) {
		existingID := "existing-id-123"
		record := &ScheduledJobCostRecord{
			ID:       existingID,
			JobName:  "test-job",
			Schedule: "daily",
			Status:   StatusSuccess,
		}

		err := record.BeforeCreate()

		require.NoError(t, err)
		assert.Equal(t, existingID, record.ID)
	})

	t.Run("calculates duration from start/end times", func(t *testing.T) {
		startTime := time.Now().Add(-5 * time.Second)
		endTime := time.Now()
		record := &ScheduledJobCostRecord{
			JobName:   "test-job",
			Schedule:  "daily",
			Status:    StatusSuccess,
			StartTime: startTime,
			EndTime:   endTime,
		}

		err := record.BeforeCreate()

		require.NoError(t, err)
		assert.InDelta(t, 5000, record.Duration, 100) // ~5000ms with tolerance
	})

	t.Run("calculates actual start delay", func(t *testing.T) {
		scheduledTime := time.Now().Add(-10 * time.Second)
		startTime := time.Now().Add(-5 * time.Second)
		record := &ScheduledJobCostRecord{
			JobName:       "test-job",
			Schedule:      "daily",
			Status:        StatusSuccess,
			ScheduledTime: scheduledTime,
			StartTime:     startTime,
		}

		err := record.BeforeCreate()

		require.NoError(t, err)
		assert.InDelta(t, 5000, record.ActualStartDelay, 100) // ~5000ms delay
	})

	t.Run("calculates total cost in dollars", func(t *testing.T) {
		record := &ScheduledJobCostRecord{
			JobName:             "test-job",
			Schedule:            "daily",
			Status:              StatusSuccess,
			TotalCostMicroCents: 1_500_000, // $1.50
		}

		err := record.BeforeCreate()

		require.NoError(t, err)
		assert.InDelta(t, 1.5, record.TotalCostDollars, 0.001)
	})

	t.Run("sets success flag based on status", func(t *testing.T) {
		tests := []struct {
			status   string
			expected bool
		}{
			{StatusSuccess, true},
			{"failed", false},
			{"timeout", false},
			{"cancelled", false},
		}

		for _, tc := range tests {
			record := &ScheduledJobCostRecord{
				JobName:  "test-job",
				Schedule: "daily",
				Status:   tc.status,
			}

			err := record.BeforeCreate()
			require.NoError(t, err)
			assert.Equal(t, tc.expected, record.Success, "Status: %s", tc.status)
		}
	})

	t.Run("sets TTL to 90 days", func(t *testing.T) {
		record := &ScheduledJobCostRecord{
			JobName:  "test-job",
			Schedule: "daily",
			Status:   StatusSuccess,
		}

		err := record.BeforeCreate()

		require.NoError(t, err)
		expectedTTL := time.Now().Add(90 * 24 * time.Hour).Unix()
		assert.InDelta(t, expectedTTL, record.ExpiresAt, 10)
	})

	t.Run("sets up primary and GSI keys", func(t *testing.T) {
		record := &ScheduledJobCostRecord{
			JobName:  "test-job",
			Schedule: "daily",
			Status:   StatusSuccess,
		}

		err := record.BeforeCreate()

		require.NoError(t, err)
		assert.Contains(t, record.PK, "SCHEDULED_JOB_COST#test-job#daily")
		assert.Contains(t, record.SK, "RUN#")
		assert.Contains(t, record.GSI1PK, "SCHEDULED_JOB_STATUS#")
		assert.Contains(t, record.GSI2PK, "SCHEDULED_JOB_DATE#")
	})

	t.Run("returns validation error for missing JobName", func(t *testing.T) {
		record := &ScheduledJobCostRecord{
			Schedule: "daily",
			Status:   StatusSuccess,
		}

		err := record.BeforeCreate()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "JobName")
	})

	t.Run("returns validation error for invalid status", func(t *testing.T) {
		record := &ScheduledJobCostRecord{
			JobName:  "test-job",
			Schedule: "daily",
			Status:   "invalid_status",
		}

		err := record.BeforeCreate()

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidScheduledJobStatus)
	})

	t.Run("returns validation error for invalid schedule", func(t *testing.T) {
		record := &ScheduledJobCostRecord{
			JobName:  "test-job",
			Schedule: "every_two_days", // invalid
			Status:   StatusSuccess,
		}

		err := record.BeforeCreate()

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidScheduledJobSchedule)
	})
}

func TestScheduledJobCostRecord_BeforeUpdate(t *testing.T) {
	t.Run("updates UpdatedAt timestamp", func(t *testing.T) {
		oldTime := time.Now().Add(-time.Hour)
		record := &ScheduledJobCostRecord{
			ID:        "test-123",
			JobName:   "test-job",
			Schedule:  "daily",
			Status:    StatusSuccess,
			Timestamp: time.Now(),
			UpdatedAt: oldTime,
		}

		err := record.BeforeUpdate()

		require.NoError(t, err)
		assert.WithinDuration(t, time.Now(), record.UpdatedAt, 2*time.Second)
	})

	t.Run("recalculates duration", func(t *testing.T) {
		startTime := time.Now().Add(-10 * time.Second)
		endTime := time.Now()
		record := &ScheduledJobCostRecord{
			ID:        "test-123",
			JobName:   "test-job",
			Schedule:  "daily",
			Status:    StatusSuccess,
			Timestamp: time.Now(),
			StartTime: startTime,
			EndTime:   endTime,
		}

		err := record.BeforeUpdate()

		require.NoError(t, err)
		assert.InDelta(t, 10000, record.Duration, 100)
	})

	t.Run("recalculates cost in dollars", func(t *testing.T) {
		record := &ScheduledJobCostRecord{
			ID:                  "test-123",
			JobName:             "test-job",
			Schedule:            "daily",
			Status:              StatusSuccess,
			Timestamp:           time.Now(),
			TotalCostMicroCents: 2_500_000,
		}

		err := record.BeforeUpdate()

		require.NoError(t, err)
		assert.InDelta(t, 2.5, record.TotalCostDollars, 0.001)
	})
}

func TestScheduledJobCostRecord_Validate(t *testing.T) {
	testCases := []struct {
		name        string
		record      ScheduledJobCostRecord
		expectError bool
		errorType   error
	}{
		{
			name: "valid record",
			record: ScheduledJobCostRecord{
				ID:       "test-123",
				JobName:  "test-job",
				Schedule: "daily",
				Status:   StatusSuccess,
			},
			expectError: false,
		},
		{
			name: "missing ID",
			record: ScheduledJobCostRecord{
				JobName:  "test-job",
				Schedule: "daily",
				Status:   StatusSuccess,
			},
			expectError: true,
		},
		{
			name: "missing JobName",
			record: ScheduledJobCostRecord{
				ID:       "test-123",
				Schedule: "daily",
				Status:   StatusSuccess,
			},
			expectError: true,
		},
		{
			name: "missing Schedule",
			record: ScheduledJobCostRecord{
				ID:      "test-123",
				JobName: "test-job",
				Status:  StatusSuccess,
			},
			expectError: true,
		},
		{
			name: "missing Status",
			record: ScheduledJobCostRecord{
				ID:       "test-123",
				JobName:  "test-job",
				Schedule: "daily",
			},
			expectError: true,
		},
		{
			name: "invalid status",
			record: ScheduledJobCostRecord{
				ID:       "test-123",
				JobName:  "test-job",
				Schedule: "daily",
				Status:   "invalid",
			},
			expectError: true,
			errorType:   ErrInvalidScheduledJobStatus,
		},
		{
			name: "invalid schedule",
			record: ScheduledJobCostRecord{
				ID:       "test-123",
				JobName:  "test-job",
				Schedule: "biweekly", // not valid
				Status:   StatusSuccess,
			},
			expectError: true,
			errorType:   ErrInvalidScheduledJobSchedule,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.record.Validate()

			if tc.expectError {
				require.Error(t, err)
				if tc.errorType != nil {
					assert.ErrorIs(t, err, tc.errorType)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestScheduledJobCostRecord_ValidSchedulePatterns(t *testing.T) {
	validSchedules := []string{"minutely", "hourly", "daily", "weekly", "monthly", "yearly", "custom"}

	for _, schedule := range validSchedules {
		t.Run(schedule, func(t *testing.T) {
			record := &ScheduledJobCostRecord{
				ID:       "test-123",
				JobName:  "test-job",
				Schedule: schedule,
				Status:   StatusSuccess,
			}

			err := record.Validate()

			require.NoError(t, err)
		})
	}
}

func TestScheduledJobCostRecord_ValidStatuses(t *testing.T) {
	validStatuses := []string{StatusSuccess, "failed", "timeout", "cancelled", "running", "queued"}

	for _, status := range validStatuses {
		t.Run(status, func(t *testing.T) {
			record := &ScheduledJobCostRecord{
				ID:       "test-123",
				JobName:  "test-job",
				Schedule: "daily",
				Status:   status,
			}

			err := record.Validate()

			require.NoError(t, err)
		})
	}
}

func TestScheduledJobCostRecord_AddTag(t *testing.T) {
	t.Run("adds tag to nil map", func(t *testing.T) {
		record := &ScheduledJobCostRecord{}

		record.AddTag("env", "production")

		assert.NotNil(t, record.Tags)
		assert.Equal(t, "production", record.Tags["env"])
	})

	t.Run("adds multiple tags", func(t *testing.T) {
		record := &ScheduledJobCostRecord{}

		record.AddTag("env", "production")
		record.AddTag("team", "platform")

		assert.Len(t, record.Tags, 2)
		assert.Equal(t, "production", record.Tags["env"])
		assert.Equal(t, "platform", record.Tags["team"])
	})

	t.Run("overwrites existing tag", func(t *testing.T) {
		record := &ScheduledJobCostRecord{
			Tags: map[string]string{"env": "staging"},
		}

		record.AddTag("env", "production")

		assert.Equal(t, "production", record.Tags["env"])
	})
}

func TestScheduledJobCostRecord_SetJobProperty(t *testing.T) {
	t.Run("sets property on nil map", func(t *testing.T) {
		record := &ScheduledJobCostRecord{}

		record.SetJobProperty("batch_size", 100)

		assert.NotNil(t, record.JobProperties)
		assert.Equal(t, 100, record.JobProperties["batch_size"])
	})

	t.Run("sets multiple properties", func(t *testing.T) {
		record := &ScheduledJobCostRecord{}

		record.SetJobProperty("batch_size", 100)
		record.SetJobProperty("timeout_seconds", 300)
		record.SetJobProperty("dry_run", true)

		assert.Len(t, record.JobProperties, 3)
	})
}

func TestScheduledJobCostRecord_GetJobProperty(t *testing.T) {
	t.Run("returns value when exists", func(t *testing.T) {
		record := &ScheduledJobCostRecord{
			JobProperties: map[string]interface{}{
				"batch_size": 100,
			},
		}

		value, exists := record.GetJobProperty("batch_size")

		assert.True(t, exists)
		assert.Equal(t, 100, value)
	})

	t.Run("returns false when not exists", func(t *testing.T) {
		record := &ScheduledJobCostRecord{
			JobProperties: map[string]interface{}{},
		}

		value, exists := record.GetJobProperty("nonexistent")

		assert.False(t, exists)
		assert.Nil(t, value)
	})

	t.Run("returns false for nil map", func(t *testing.T) {
		record := &ScheduledJobCostRecord{}

		value, exists := record.GetJobProperty("anything")

		assert.False(t, exists)
		assert.Nil(t, value)
	})
}

func TestScheduledJobCostRecord_SetPerformanceMetric(t *testing.T) {
	t.Run("sets metric on nil map", func(t *testing.T) {
		record := &ScheduledJobCostRecord{}

		record.SetPerformanceMetric("latency_p99", 150.5)

		assert.NotNil(t, record.PerformanceMetrics)
		assert.InDelta(t, 150.5, record.PerformanceMetrics["latency_p99"], 0.001)
	})
}

func TestScheduledJobCostRecord_GetPerformanceMetric(t *testing.T) {
	t.Run("returns value when exists", func(t *testing.T) {
		record := &ScheduledJobCostRecord{
			PerformanceMetrics: map[string]float64{
				"latency_p99": 150.5,
			},
		}

		value, exists := record.GetPerformanceMetric("latency_p99")

		assert.True(t, exists)
		assert.InDelta(t, 150.5, value, 0.001)
	})

	t.Run("returns zero and false for nonexistent", func(t *testing.T) {
		record := &ScheduledJobCostRecord{
			PerformanceMetrics: map[string]float64{},
		}

		value, exists := record.GetPerformanceMetric("nonexistent")

		assert.False(t, exists)
		assert.Equal(t, 0.0, value)
	})

	t.Run("returns zero and false for nil map", func(t *testing.T) {
		record := &ScheduledJobCostRecord{}

		value, exists := record.GetPerformanceMetric("anything")

		assert.False(t, exists)
		assert.Equal(t, 0.0, value)
	})
}

func TestScheduledJobCostRecord_GetPK_GetSK(t *testing.T) {
	record := &ScheduledJobCostRecord{
		PK: "SCHEDULED_JOB_COST#test-job#daily",
		SK: "RUN#20240115T120000#test-123",
	}

	assert.Equal(t, "SCHEDULED_JOB_COST#test-job#daily", record.GetPK())
	assert.Equal(t, "RUN#20240115T120000#test-123", record.GetSK())
}

func TestScheduledJobCostRecord_UpdateKeys(t *testing.T) {
	record := &ScheduledJobCostRecord{
		Timestamp: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		Status:    StatusSuccess,
		JobName:   "test-job",
	}

	err := record.UpdateKeys()

	require.NoError(t, err)
	assert.Contains(t, record.GSI1PK, "SCHEDULED_JOB_STATUS#")
	assert.Contains(t, record.GSI2PK, "SCHEDULED_JOB_DATE#")
}

func TestScheduledJobCostRecord_TableName(t *testing.T) {
	record := ScheduledJobCostRecord{}
	assert.Equal(t, MainTableName, record.TableName())
}

// =============================================================================
// ScheduledJobCostAggregation Tests
// =============================================================================

func TestScheduledJobCostAggregation_BeforeCreate(t *testing.T) {
	t.Run("sets timestamps and keys", func(t *testing.T) {
		windowStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		windowEnd := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		agg := &ScheduledJobCostAggregation{
			JobName:              "test-job",
			Period:               "day",
			WindowStart:          windowStart,
			WindowEnd:            windowEnd,
			TotalCostMicroCents:  1_000_000,
			TotalExecutions:      10,
			SuccessfulExecutions: 8,
		}

		err := agg.BeforeCreate()

		require.NoError(t, err)
		assert.False(t, agg.CreatedAt.IsZero())
		assert.False(t, agg.UpdatedAt.IsZero())
		assert.Contains(t, agg.PK, "SCHEDULED_JOB_AGG#day#test-job")
		assert.Contains(t, agg.SK, "WINDOW#")
	})

	t.Run("calculates total cost in dollars", func(t *testing.T) {
		windowStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		windowEnd := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		agg := &ScheduledJobCostAggregation{
			JobName:             "test-job",
			Period:              "day",
			WindowStart:         windowStart,
			WindowEnd:           windowEnd,
			TotalCostMicroCents: 2_500_000,
			TotalExecutions:     10,
		}

		err := agg.BeforeCreate()

		require.NoError(t, err)
		assert.InDelta(t, 2.5, agg.TotalCostDollars, 0.001)
	})

	t.Run("calculates average cost per execution", func(t *testing.T) {
		windowStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		windowEnd := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		agg := &ScheduledJobCostAggregation{
			JobName:             "test-job",
			Period:              "day",
			WindowStart:         windowStart,
			WindowEnd:           windowEnd,
			TotalCostMicroCents: 1_000_000, // $1.00
			TotalExecutions:     10,
		}

		err := agg.BeforeCreate()

		require.NoError(t, err)
		assert.InDelta(t, 0.1, agg.AverageCostPerExecution, 0.001) // $0.10 per execution
	})

	t.Run("calculates success rate", func(t *testing.T) {
		windowStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		windowEnd := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		agg := &ScheduledJobCostAggregation{
			JobName:              "test-job",
			Period:               "day",
			WindowStart:          windowStart,
			WindowEnd:            windowEnd,
			TotalExecutions:      10,
			SuccessfulExecutions: 8,
		}

		err := agg.BeforeCreate()

		require.NoError(t, err)
		assert.InDelta(t, 80.0, agg.SuccessRate, 0.1)
	})

	t.Run("calculates cost per item processed", func(t *testing.T) {
		windowStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		windowEnd := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		agg := &ScheduledJobCostAggregation{
			JobName:             "test-job",
			Period:              "day",
			WindowStart:         windowStart,
			WindowEnd:           windowEnd,
			TotalCostMicroCents: 1_000_000, // $1.00
			TotalItemsProcessed: 100,
			TotalExecutions:     1,
		}

		err := agg.BeforeCreate()

		require.NoError(t, err)
		assert.InDelta(t, 0.01, agg.CostPerItemProcessed, 0.001) // $0.01 per item
	})

	t.Run("calculates cost per successful execution", func(t *testing.T) {
		windowStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		windowEnd := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		agg := &ScheduledJobCostAggregation{
			JobName:              "test-job",
			Period:               "day",
			WindowStart:          windowStart,
			WindowEnd:            windowEnd,
			TotalCostMicroCents:  1_000_000, // $1.00
			SuccessfulExecutions: 5,
			TotalExecutions:      10,
		}

		err := agg.BeforeCreate()

		require.NoError(t, err)
		assert.InDelta(t, 0.2, agg.CostPerSuccessfulExecution, 0.001) // $0.20 per successful
	})

	t.Run("sets TTL to 365 days", func(t *testing.T) {
		windowStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		windowEnd := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		agg := &ScheduledJobCostAggregation{
			JobName:         "test-job",
			Period:          "day",
			WindowStart:     windowStart,
			WindowEnd:       windowEnd,
			TotalExecutions: 1,
		}

		err := agg.BeforeCreate()

		require.NoError(t, err)
		expectedTTL := time.Now().Add(365 * 24 * time.Hour).Unix()
		assert.InDelta(t, expectedTTL, agg.ExpiresAt, 10)
	})
}

func TestScheduledJobCostAggregation_BeforeUpdate(t *testing.T) {
	t.Run("recalculates metrics", func(t *testing.T) {
		windowStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		windowEnd := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		agg := &ScheduledJobCostAggregation{
			JobName:              "test-job",
			Period:               "day",
			WindowStart:          windowStart,
			WindowEnd:            windowEnd,
			TotalCostMicroCents:  2_000_000,
			TotalExecutions:      20,
			SuccessfulExecutions: 18,
			TotalItemsProcessed:  200,
		}

		err := agg.BeforeUpdate()

		require.NoError(t, err)
		assert.InDelta(t, 2.0, agg.TotalCostDollars, 0.001)
		assert.InDelta(t, 0.1, agg.AverageCostPerExecution, 0.001)
		assert.InDelta(t, 90.0, agg.SuccessRate, 0.1)
	})
}

func TestScheduledJobCostAggregation_Validate(t *testing.T) {
	validWindowStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	validWindowEnd := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	testCases := []struct {
		name        string
		agg         ScheduledJobCostAggregation
		expectError bool
		errorType   error
	}{
		{
			name: "valid aggregation",
			agg: ScheduledJobCostAggregation{
				JobName:     "test-job",
				Period:      "day",
				WindowStart: validWindowStart,
				WindowEnd:   validWindowEnd,
			},
			expectError: false,
		},
		{
			name: "missing JobName",
			agg: ScheduledJobCostAggregation{
				Period:      "day",
				WindowStart: validWindowStart,
				WindowEnd:   validWindowEnd,
			},
			expectError: true,
		},
		{
			name: "missing Period",
			agg: ScheduledJobCostAggregation{
				JobName:     "test-job",
				WindowStart: validWindowStart,
				WindowEnd:   validWindowEnd,
			},
			expectError: true,
		},
		{
			name: "missing WindowStart",
			agg: ScheduledJobCostAggregation{
				JobName:   "test-job",
				Period:    "day",
				WindowEnd: validWindowEnd,
			},
			expectError: true,
			errorType:   ErrScheduledJobWindowStartRequired,
		},
		{
			name: "missing WindowEnd",
			agg: ScheduledJobCostAggregation{
				JobName:     "test-job",
				Period:      "day",
				WindowStart: validWindowStart,
			},
			expectError: true,
			errorType:   ErrScheduledJobWindowEndRequired,
		},
		{
			name: "WindowEnd before WindowStart",
			agg: ScheduledJobCostAggregation{
				JobName:     "test-job",
				Period:      "day",
				WindowStart: validWindowEnd,   // swapped
				WindowEnd:   validWindowStart, // swapped
			},
			expectError: true,
			errorType:   ErrScheduledJobWindowEndBeforeStart,
		},
		{
			name: "invalid period",
			agg: ScheduledJobCostAggregation{
				JobName:     "test-job",
				Period:      "biweekly", // invalid
				WindowStart: validWindowStart,
				WindowEnd:   validWindowEnd,
			},
			expectError: true,
			errorType:   ErrInvalidScheduledJobPeriod,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.agg.Validate()

			if tc.expectError {
				require.Error(t, err)
				if tc.errorType != nil {
					assert.ErrorIs(t, err, tc.errorType)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestScheduledJobCostAggregation_ValidPeriods(t *testing.T) {
	validPeriods := []string{"minute", "hour", "day", "week", "month", "year"}
	windowStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	for _, period := range validPeriods {
		t.Run(period, func(t *testing.T) {
			agg := &ScheduledJobCostAggregation{
				JobName:     "test-job",
				Period:      period,
				WindowStart: windowStart,
				WindowEnd:   windowEnd,
			}

			err := agg.Validate()

			require.NoError(t, err)
		})
	}
}

func TestScheduledJobCostAggregation_GetPK_GetSK(t *testing.T) {
	agg := &ScheduledJobCostAggregation{
		PK: "SCHEDULED_JOB_AGG#day#test-job",
		SK: "WINDOW#2024-01-01T00:00:00Z",
	}

	assert.Equal(t, "SCHEDULED_JOB_AGG#day#test-job", agg.GetPK())
	assert.Equal(t, "WINDOW#2024-01-01T00:00:00Z", agg.GetSK())
}

func TestScheduledJobCostAggregation_UpdateKeys(t *testing.T) {
	windowStart := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	agg := &ScheduledJobCostAggregation{
		JobName:     "test-job",
		Period:      "day",
		WindowStart: windowStart,
	}

	err := agg.UpdateKeys()

	require.NoError(t, err)
	assert.Equal(t, "SCHEDULED_JOB_AGG#day#test-job", agg.PK)
	assert.Contains(t, agg.SK, "WINDOW#")
}

func TestScheduledJobCostAggregation_TableName(t *testing.T) {
	agg := ScheduledJobCostAggregation{}
	assert.Equal(t, MainTableName, agg.TableName())
}

// =============================================================================
// ScheduledJobCostRecordBuilder Tests
// =============================================================================

func TestScheduledJobCostRecordBuilder(t *testing.T) {
	t.Run("builds record with all fields", func(t *testing.T) {
		scheduledTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		startTime := time.Date(2024, 1, 15, 12, 0, 5, 0, time.UTC)
		endTime := time.Date(2024, 1, 15, 12, 1, 0, 0, time.UTC)

		record := NewScheduledJobCostRecordBuilder().
			ForJob("aggregation-job", "hourly").
			WithStatus(StatusSuccess).
			WithTiming(scheduledTime, startTime, endTime).
			WithLambdaUsage(5, 30000, 512).
			WithDynamoDBUsage(100, 50, 10.5, 5.25).
			WithCosts(100, 50, 25, 10, 5, 3, 2).
			WithItemsProcessed(1000, 50, 10).
			WithError("", 0, 3).
			WithContext("production", "us-east-1", "aggregation-lambda", "req-123").
			WithCategory("aggregation", "high").
			WithCascadingCosts([]string{"downstream-job"}, 500, 10).
			WithTag("team", "platform").
			WithJobProperty("batch_size", 100).
			WithPerformanceMetric("latency_p99", 150.5).
			Build()

		assert.Equal(t, "aggregation-job", record.JobName)
		assert.Equal(t, "hourly", record.Schedule)
		assert.Equal(t, StatusSuccess, record.Status)
		assert.Equal(t, scheduledTime, record.ScheduledTime)
		assert.Equal(t, startTime, record.StartTime)
		assert.Equal(t, endTime, record.EndTime)
		assert.Equal(t, int64(5), record.LambdaInvocations)
		assert.Equal(t, int64(30000), record.LambdaDurationMs)
		assert.Equal(t, 512, record.LambdaMemoryUsedMB)
		assert.Equal(t, int64(100), record.DynamoDBReadOperations)
		assert.Equal(t, int64(50), record.DynamoDBWriteOperations)
		assert.Equal(t, int64(195), record.TotalCostMicroCents)
		assert.Equal(t, int64(1000), record.ItemsProcessed)
		assert.Equal(t, "production", record.Environment)
		assert.Equal(t, "us-east-1", record.Region)
		assert.Equal(t, "aggregation", record.JobCategory)
		assert.Equal(t, "high", record.Priority)
		assert.Contains(t, record.TriggeredJobs, "downstream-job")
		assert.Equal(t, "platform", record.Tags["team"])
		assert.Equal(t, 100, record.JobProperties["batch_size"])
		assert.InDelta(t, 150.5, record.PerformanceMetrics["latency_p99"], 0.001)
	})

	t.Run("initializes maps in builder", func(t *testing.T) {
		builder := NewScheduledJobCostRecordBuilder()
		record := builder.Build()

		assert.NotNil(t, record.Tags)
		assert.NotNil(t, record.JobProperties)
		assert.NotNil(t, record.PerformanceMetrics)
		assert.NotNil(t, record.TriggeredJobs)
	})
}

// =============================================================================
// Cost Calculation Function Tests
// =============================================================================

func TestCalculateScheduledJobCosts(t *testing.T) {
	t.Run("calculates all cost components", func(t *testing.T) {
		lambdaCost, dynamoDBCost, sqsCost, s3Cost, cloudWatchCost, dataTransferCost, totalCost :=
			CalculateScheduledJobCosts(
				1000,  // lambdaDurationMs
				256,   // memoryMB
				10000, // dynamoDBReadOps
				5000,  // dynamoDBWriteOps
				1000,  // sqsMessages
				100,   // s3Operations
				10,    // logSizeMB
				5,     // dataTransferMB
			)

		// Lambda: 1000ms * (256/128) * 2 = 4000 microcents
		assert.Equal(t, int64(4000), lambdaCost)

		// DynamoDB: (10000 * 25 / 1000) + (5000 * 125 / 1000) = 250 + 625 = 875
		assert.Equal(t, int64(875), dynamoDBCost)

		// SQS: 1000 * 40 / 1000 = 40
		assert.Equal(t, int64(40), sqsCost)

		// S3: 100 * 40 / 1000 = 4
		assert.Equal(t, int64(4), s3Cost)

		// CloudWatch: 10 * 50 = 500
		assert.Equal(t, int64(500), cloudWatchCost)

		// Data Transfer: 5 * 9 = 45
		assert.Equal(t, int64(45), dataTransferCost)

		// Total: 4000 + 875 + 40 + 4 + 500 + 45 = 5464
		assert.Equal(t, int64(5464), totalCost)
	})

	t.Run("handles zero values", func(t *testing.T) {
		lambdaCost, dynamoDBCost, sqsCost, s3Cost, cloudWatchCost, dataTransferCost, totalCost :=
			CalculateScheduledJobCosts(0, 128, 0, 0, 0, 0, 0, 0)

		assert.Equal(t, int64(0), lambdaCost)
		assert.Equal(t, int64(0), dynamoDBCost)
		assert.Equal(t, int64(0), sqsCost)
		assert.Equal(t, int64(0), s3Cost)
		assert.Equal(t, int64(0), cloudWatchCost)
		assert.Equal(t, int64(0), dataTransferCost)
		assert.Equal(t, int64(0), totalCost)
	})
}

// =============================================================================
// Stats Types Tests
// =============================================================================

func TestScheduledJobCategoryStats_TableName(t *testing.T) {
	stats := ScheduledJobCategoryStats{}
	assert.Equal(t, MainTableName, stats.TableName())
}

func TestScheduledJobEnvironmentStats_TableName(t *testing.T) {
	stats := ScheduledJobEnvironmentStats{}
	assert.Equal(t, MainTableName, stats.TableName())
}

func TestScheduledJobScheduleStats_TableName(t *testing.T) {
	stats := ScheduledJobScheduleStats{}
	assert.Equal(t, MainTableName, stats.TableName())
}

func TestScheduledJobCostRecordBuilder_TableName(t *testing.T) {
	builder := ScheduledJobCostRecordBuilder{}
	assert.Equal(t, MainTableName, builder.TableName())
}
