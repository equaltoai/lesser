package dev

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pay-theory/lift/pkg/features"
)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
}

// LogService manages log collection and retrieval
type LogService struct {
	stopWatching chan struct{}
	features     *features.FeatureFlags
	logFile      string
	logs         []LogEntry
	maxLogs      int
	mu           sync.RWMutex
	watching     bool
}

// NewLogService creates a new log service
func NewLogService(logFile string, ff *features.FeatureFlags) *LogService {
	ls := &LogService{
		logs:         make([]LogEntry, 0, 1000),
		maxLogs:      1000,
		logFile:      logFile,
		stopWatching: make(chan struct{}),
		features:     ff,
	}

	// Start watching log file if it exists
	if logFile != "" {
		go ls.watchLogFile()
	}

	return ls
}

// AddLog adds a log entry
func (ls *LogService) AddLog(entry LogEntry) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	ls.logs = append(ls.logs, entry)

	// Keep only the most recent logs
	if len(ls.logs) > ls.maxLogs {
		ls.logs = ls.logs[len(ls.logs)-ls.maxLogs:]
	}
}

// GetRecentLogs returns the most recent log entries
func (ls *LogService) GetRecentLogs(limit int) []LogEntry {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	if limit <= 0 || limit > len(ls.logs) {
		limit = len(ls.logs)
	}
	// Hard-cap to prevent excessive allocations even if analysis
	// cannot infer len(ls.logs) is bounded by ls.maxLogs.
	if limit > ls.maxLogs {
		limit = ls.maxLogs
	}

	// Return the most recent logs
	start := len(ls.logs) - limit
	if start < 0 {
		start = 0
	}

	// Copy into a bounded slice to avoid allocating based on a potentially
	// large, user-influenced value.
	subset := ls.logs[start:]
	if len(subset) > limit {
		subset = subset[:limit]
	}
	result := make([]LogEntry, len(subset))
	copy(result, subset)
	return result
}

// GetLogsSince returns logs since a given timestamp
func (ls *LogService) GetLogsSince(since time.Time) []LogEntry {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	var result []LogEntry
	for _, log := range ls.logs {
		if log.Timestamp.After(since) {
			result = append(result, log)
		}
	}

	return result
}

// SearchLogs searches for logs containing a query string
func (ls *LogService) SearchLogs(query string) []LogEntry {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	query = strings.ToLower(query)
	var result []LogEntry

	for _, log := range ls.logs {
		if strings.Contains(strings.ToLower(log.Message), query) {
			result = append(result, log)
			continue
		}

		// Also search in fields
		for _, v := range log.Fields {
			if str, ok := v.(string); ok {
				if strings.Contains(strings.ToLower(str), query) {
					result = append(result, log)
					break
				}
			}
		}
	}

	return result
}

// Clear clears all logs
func (ls *LogService) Clear() {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.logs = ls.logs[:0]
}

// Stop stops the log service
func (ls *LogService) Stop() {
	if ls.watching {
		close(ls.stopWatching)
		ls.watching = false
	}
}

// watchLogFile watches a log file for new entries
func (ls *LogService) watchLogFile() {
	if ls.logFile == "" {
		return
	}

	ls.watching = true
	defer func() { ls.watching = false }()

	// Initial read of existing logs
	ls.readExistingLogs()

	// Watch for new logs
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastSize int64

	for {
		select {
		case <-ls.stopWatching:
			return
		case <-ticker.C:
			info, err := os.Stat(ls.logFile)
			if err != nil {
				continue
			}

			if info.Size() > lastSize {
				ls.readNewLogs(lastSize)
				lastSize = info.Size()
			} else if info.Size() < lastSize {
				// File was truncated, re-read from beginning
				ls.Clear()
				ls.readExistingLogs()
				lastSize = info.Size()
			}
		}
	}
}

// readExistingLogs reads all existing logs from the file
func (ls *LogService) readExistingLogs() {
	file, err := os.Open(ls.logFile)
	if err != nil {
		return
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Silently ignore file close errors for read operations
			_ = closeErr
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		entry := ls.parseLogLine(scanner.Text())
		if entry != nil {
			ls.AddLog(*entry)
		}
	}
}

// readNewLogs reads new logs from the file starting at offset
func (ls *LogService) readNewLogs(offset int64) {
	file, err := os.Open(ls.logFile)
	if err != nil {
		return
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			// Silently ignore file close errors for read operations
			_ = closeErr
		}
	}()

	// Seek to offset
	_, err = file.Seek(offset, 0)
	if err != nil {
		return
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		entry := ls.parseLogLine(scanner.Text())
		if entry != nil {
			ls.AddLog(*entry)
		}
	}
}

// parseLogLine parses a log line into a LogEntry
func (ls *LogService) parseLogLine(line string) *LogEntry {
	// Try to parse as JSON first
	var entry LogEntry
	if err := json.Unmarshal([]byte(line), &entry); err == nil {
		return &entry
	}

	// Otherwise, parse as plain text log
	// Format: TIMESTAMP LEVEL MESSAGE
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 3 {
		// Fallback to simple format
		return &LogEntry{
			Timestamp: time.Now(),
			Level:     "INFO",
			Message:   line,
		}
	}

	timestamp, err := time.Parse(time.RFC3339, parts[0])
	if err != nil {
		timestamp = time.Now()
	}

	return &LogEntry{
		Timestamp: timestamp,
		Level:     parts[1],
		Message:   parts[2],
	}
}

// MockLogService provides mock logs for development
type MockLogService struct {
	*LogService
	startTime time.Time
}

// NewMockLogService creates a mock log service for development
func NewMockLogService(ff *features.FeatureFlags) *MockLogService {
	mls := &MockLogService{
		LogService: NewLogService("", ff),
		startTime:  time.Now(),
	}

	// Add some initial mock logs
	mls.generateInitialLogs()

	// Start generating periodic logs
	go mls.generatePeriodicLogs()

	return mls
}

// generateInitialLogs generates some initial mock logs
func (mls *MockLogService) generateInitialLogs() {
	logs := []LogEntry{
		{
			Timestamp: mls.startTime,
			Level:     "INFO",
			Message:   "Development server started",
			Fields: map[string]interface{}{
				"port":    3000,
				"version": "dev",
			},
		},
		{
			Timestamp: mls.startTime.Add(100 * time.Millisecond),
			Level:     "DEBUG",
			Message:   "File watcher initialized",
			Fields: map[string]interface{}{
				"directories": []string{"./pkg", "./cmd"},
			},
		},
		{
			Timestamp: mls.startTime.Add(200 * time.Millisecond),
			Level:     "INFO",
			Message:   "Hot reload enabled",
		},
		{
			Timestamp: mls.startTime.Add(500 * time.Millisecond),
			Level:     "INFO",
			Message:   "Dashboard available at http://localhost:3001",
		},
	}

	for _, log := range logs {
		mls.AddLog(log)
	}
}

// generatePeriodicLogs generates periodic mock logs
func (mls *MockLogService) generatePeriodicLogs() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	requestCount := 0

	for {
		select {
		case <-mls.stopWatching:
			return
		case <-ticker.C:
			// Generate random activity logs
			if requestCount%3 == 0 {
				mls.AddLog(LogEntry{
					Timestamp: time.Now(),
					Level:     "INFO",
					Message:   fmt.Sprintf("Handled request #%d", requestCount),
					Fields: map[string]interface{}{
						"method":   "GET",
						"path":     "/api/users",
						"duration": "12ms",
					},
				})
			}

			if requestCount%7 == 0 {
				mls.AddLog(LogEntry{
					Timestamp: time.Now(),
					Level:     "DEBUG",
					Message:   "Cache hit for user data",
					Fields: map[string]interface{}{
						"key":      "user:123",
						"hit_rate": "87%",
					},
				})
			}

			if requestCount%15 == 0 {
				mls.AddLog(LogEntry{
					Timestamp: time.Now(),
					Level:     "WARN",
					Message:   "Slow query detected",
					Fields: map[string]interface{}{
						"query":    "SELECT * FROM users WHERE...",
						"duration": "523ms",
					},
				})
			}

			requestCount++
		}
	}
}

// LogServiceFactory creates the appropriate log service based on environment
func LogServiceFactory(logFile string, ff *features.FeatureFlags) *LogService {
	// Use mock logs in development if enabled
	if ff != nil && ff.IsEnabled(features.MockServicesEnabled) {
		return NewMockLogService(ff).LogService
	}

	// Use real log service in production
	return NewLogService(logFile, ff)
}
