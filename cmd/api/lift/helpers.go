package lift

import "time"

// Helper functions for job data extraction
func getStringFromJobData(data map[string]any, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}

func getIntFromJobData(data map[string]any, key string) int {
	if val, ok := data[key].(float64); ok {
		return int(val)
	}
	if val, ok := data[key].(int); ok {
		return val
	}
	return 0
}

func getInt64FromJobData(data map[string]any, key string) int64 {
	if val, ok := data[key].(float64); ok {
		return int64(val)
	}
	if val, ok := data[key].(int64); ok {
		return val
	}
	return 0
}

func getTimeFromJobData(data map[string]any, key string) time.Time {
	if val, ok := data[key].(string); ok {
		t, _ := time.Parse(time.RFC3339, val)
		return t
	}
	if val, ok := data[key].(time.Time); ok {
		return val
	}
	return time.Time{}
}