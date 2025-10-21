package dev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pay-theory/lift/pkg/features"
	"github.com/pay-theory/lift/pkg/lift"
)

// DevServerConfig configures the development server
type DevServerConfig struct {
	BuildCommand  string        `json:"build_command"`
	LogLevel      string        `json:"log_level"`
	WatchPaths    []string      `json:"watch_paths"`
	Port          int           `json:"port"`
	ProfilerPort  int           `json:"profiler_port"`
	DashboardPort int           `json:"dashboard_port"`
	WatchInterval time.Duration `json:"watch_interval"`
	RestartDelay  time.Duration `json:"restart_delay"`
	HotReload     bool          `json:"hot_reload"`
	DebugMode     bool          `json:"debug_mode"`
	EnableCORS    bool          `json:"enable_cors"`
}

// DefaultDevServerConfig returns sensible defaults for development
func DefaultDevServerConfig() *DevServerConfig {
	return &DevServerConfig{
		Port:          8080,
		HotReload:     true,
		DebugMode:     true,
		ProfilerPort:  6060,
		DashboardPort: 3000,
		WatchPaths:    []string{".", "cmd", "pkg", "internal"},
		WatchInterval: 500 * time.Millisecond,
		BuildCommand:  "go build -o ./tmp/main ./cmd/main.go",
		RestartDelay:  1 * time.Second,
		EnableCORS:    true,
		LogLevel:      "debug",
	}
}

// DevServer provides development server with hot reload and debugging
type DevServer struct {
	startTime time.Time
	server    *http.Server
	profiler  *ProfilerServer
	dashboard *DevDashboard
	watcher   *FileWatcher
	features  *features.FeatureFlags
	app       *lift.App
	restartCh chan struct{}
	stopCh    chan struct{}
	stats     *DevStats
	config    *DevServerConfig
	mu        sync.RWMutex
	running   bool
}

// DevStats holds internal development server statistics (with mutex)
type DevStats struct {
	LastRestart    time.Time     `json:"last_restart"`
	Requests       int64         `json:"requests"`
	Errors         int64         `json:"errors"`
	Restarts       int64         `json:"restarts"`
	AverageLatency time.Duration `json:"average_latency"`
	Uptime         time.Duration `json:"uptime"`
	HotReloads     int64         `json:"hot_reloads"`
	BuildTime      time.Duration `json:"build_time"`
	mu             sync.RWMutex
}

// SafeDevStats holds development server statistics without mutex (safe for copying)
type SafeDevStats struct {
	LastRestart    time.Time     `json:"last_restart"`
	Requests       int64         `json:"requests"`
	Errors         int64         `json:"errors"`
	Restarts       int64         `json:"restarts"`
	AverageLatency time.Duration `json:"average_latency"`
	Uptime         time.Duration `json:"uptime"`
	HotReloads     int64         `json:"hot_reloads"`
	BuildTime      time.Duration `json:"build_time"`
}

// NewDevServer creates a new development server
func NewDevServer(app *lift.App, config *DevServerConfig) *DevServer {
	if config == nil {
		config = DefaultDevServerConfig()
	}

	server := &DevServer{
		app:       app,
		config:    config,
		restartCh: make(chan struct{}, 1),
		stopCh:    make(chan struct{}),
		stats:     &DevStats{},
		startTime: time.Now(),
	}

	// Initialize profiler if debug mode is enabled
	if config.DebugMode {
		server.profiler = NewProfilerServer(config.ProfilerPort)
	}

	// Initialize dashboard
	server.dashboard = NewDevDashboard(server, config.DashboardPort)

	// Initialize file watcher if hot reload is enabled
	if config.HotReload {
		server.watcher = NewFileWatcher(config.WatchPaths, config.WatchInterval)
	}

	return server
}

// Start starts the development server
func (s *DevServer) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server is already running")
	}
	s.running = true
	s.mu.Unlock()

	fmt.Printf("🚀 Starting Lift development server...\n")
	fmt.Printf("📡 Server: http://localhost:%d\n", s.config.Port)

	// Start profiler if enabled
	if s.profiler != nil {
		go func() {
			if err := s.profiler.Start(); err != nil {
				fmt.Printf("⚠️  Failed to start profiler: %v\n", err)
			}
		}()
		fmt.Printf("🔍 Profiler: http://localhost:%d/debug/pprof/\n", s.config.ProfilerPort)
	}

	// Start dashboard
	go func() {
		if err := s.dashboard.Start(); err != nil {
			fmt.Printf("⚠️  Failed to start dashboard: %v\n", err)
		}
	}()
	fmt.Printf("📊 Dashboard: http://localhost:%d\n", s.config.DashboardPort)

	// Start file watcher if enabled
	if s.watcher != nil {
		go s.watchForChanges(ctx)
		fmt.Printf("🔥 Hot reload: enabled\n")
	}

	fmt.Printf("💡 Press Ctrl+C to stop\n\n")

	// Start HTTP server
	return s.startHTTPServer(ctx)
}

// Stop stops the development server
func (s *DevServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	close(s.stopCh)

	// Stop components
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.server.Shutdown(ctx); err != nil {
			// Log error but continue shutdown process
			fmt.Printf("Warning: Error shutting down server: %v\n", err)
		}
	}

	if s.profiler != nil {
		if err := s.profiler.Stop(); err != nil {
			// Log error but continue shutdown process
			fmt.Printf("Warning: Error stopping profiler: %v\n", err)
		}
	}

	if s.dashboard != nil {
		if err := s.dashboard.Stop(); err != nil {
			// Log error but continue shutdown process
			fmt.Printf("Warning: Error stopping dashboard: %v\n", err)
		}
	}

	if s.watcher != nil {
		s.watcher.Stop()
	}

	fmt.Printf("\n👋 Development server stopped\n")
	return nil
}

// startHTTPServer starts the main HTTP server
func (s *DevServer) startHTTPServer(ctx context.Context) error {
	mux := http.NewServeMux()

	// Add CORS middleware if enabled
	var handler http.Handler = mux
	if s.config.EnableCORS {
		handler = s.corsMiddleware(mux)
	}

	// Add development middleware
	handler = s.devMiddleware(handler)

	// Register routes
	mux.HandleFunc("/", s.handleRequest)
	mux.HandleFunc("/dev/stats", s.handleStats)
	mux.HandleFunc("/dev/restart", s.handleRestart)
	mux.HandleFunc("/dev/health", s.handleHealth)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Port),
		Handler: handler,

		// Security timeouts to prevent DoS attacks
		ReadTimeout:       15 * time.Second, // Maximum time to read request including body
		ReadHeaderTimeout: 5 * time.Second,  // Maximum time to read request headers (prevents Slowloris)
		WriteTimeout:      15 * time.Second, // Maximum time to write response
		IdleTimeout:       60 * time.Second, // Maximum time for keep-alive connections

		// Additional security settings
		MaxHeaderBytes: 1 << 20, // 1 MB max header size
	}

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.server.ListenAndServe()
	}()

	// Wait for shutdown or error
	select {
	case err := <-errCh:
		if err != http.ErrServerClosed {
			return err
		}
		return nil
	case <-s.stopCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// handleRequest handles all application requests
func (s *DevServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Update stats
	s.stats.mu.Lock()
	s.stats.Requests++
	s.stats.mu.Unlock()

	// Create Lift context and handle request
	// This is a simplified implementation - in reality, we'd integrate with the full Lift routing
	response := map[string]any{
		"message":   "Development server response",
		"path":      r.URL.Path,
		"method":    r.Method,
		"timestamp": time.Now(),
		"dev_mode":  true,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Log error but continue - best effort dev server response
		fmt.Printf("Warning: Error encoding JSON response: %v\n", err)
	}

	// Update latency stats
	duration := time.Since(start)
	s.stats.mu.Lock()
	s.stats.AverageLatency = (s.stats.AverageLatency + duration) / 2
	s.stats.mu.Unlock()
}

// handleStats returns development server statistics
func (s *DevServer) handleStats(w http.ResponseWriter, _ *http.Request) {
	stats := s.GetStats()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		// Log error but continue - best effort dev server response
		fmt.Printf("Warning: Error encoding JSON stats: %v\n", err)
	}
}

// handleRestart triggers a server restart
func (s *DevServer) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	select {
	case s.restartCh <- struct{}{}:
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{
			"status": "restart triggered",
		}); err != nil {
			// Log error but continue - best effort dev server response
			fmt.Printf("Warning: Error encoding restart response: %v\n", err)
		}
	default:
		w.WriteHeader(http.StatusTooManyRequests)
		if err := json.NewEncoder(w).Encode(map[string]string{
			"error": "restart already in progress",
		}); err != nil {
			// Log error but continue - best effort dev server response
			fmt.Printf("Warning: Error encoding restart error response: %v\n", err)
		}
	}
}

// handleHealth returns server health status
func (s *DevServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	health := map[string]any{
		"status":    "healthy",
		"timestamp": time.Now(),
		"uptime":    time.Since(s.startTime),
		"version":   "dev",
		"config": map[string]any{
			"hot_reload": s.config.HotReload,
			"debug_mode": s.config.DebugMode,
			"port":       s.config.Port,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(health); err != nil {
		// Log error but continue - best effort dev server response
		fmt.Printf("Warning: Error encoding health response: %v\n", err)
	}
}

// devMiddleware adds development-specific middleware
func (s *DevServer) devMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add development headers
		w.Header().Set("X-Dev-Server", "lift")
		w.Header().Set("X-Hot-Reload", fmt.Sprintf("%v", s.config.HotReload))

		// Log request in development mode
		if s.config.LogLevel == "debug" {
			fmt.Printf("🌐 %s %s\n", r.Method, r.URL.Path)
		}

		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds CORS headers for development
func (s *DevServer) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// watchForChanges monitors file changes and triggers restarts
func (s *DevServer) watchForChanges(ctx context.Context) {
	if s.watcher == nil {
		return
	}

	if err := s.watcher.Start(); err != nil {
		fmt.Printf("⚠️  Failed to start file watcher: %v\n", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case event := <-s.watcher.Events():
			fmt.Printf("🔄 File changed: %s\n", event.Path)
			s.triggerRestart()
		case <-s.restartCh:
			s.performRestart()
		}
	}
}

// triggerRestart triggers a server restart
func (s *DevServer) triggerRestart() {
	select {
	case s.restartCh <- struct{}{}:
	default:
		// Restart already pending
	}
}

// performRestart performs the actual restart
func (s *DevServer) performRestart() {
	fmt.Printf("🔄 Restarting server...\n")

	start := time.Now()

	// Update stats
	s.stats.mu.Lock()
	s.stats.Restarts++
	s.stats.LastRestart = time.Now()
	s.stats.HotReloads++
	s.stats.mu.Unlock()

	// Simulate build process
	if s.config.BuildCommand != "" {
		fmt.Printf("🔨 Building...\n")
		// In a real implementation, we'd execute the build command
		time.Sleep(100 * time.Millisecond) // Simulate build time
	}

	// Wait for restart delay
	time.Sleep(s.config.RestartDelay)

	buildTime := time.Since(start)
	s.stats.mu.Lock()
	s.stats.BuildTime = buildTime
	s.stats.mu.Unlock()

	fmt.Printf("✅ Restart complete (%v)\n", buildTime)
}

// GetStats returns current development server statistics
func (s *DevServer) GetStats() SafeDevStats {
	s.stats.mu.RLock()
	defer s.stats.mu.RUnlock()

	// Create a safe copy without the mutex
	return SafeDevStats{
		Requests:       s.stats.Requests,
		Errors:         s.stats.Errors,
		Restarts:       s.stats.Restarts,
		LastRestart:    s.stats.LastRestart,
		AverageLatency: s.stats.AverageLatency,
		Uptime:         time.Since(s.startTime),
		HotReloads:     s.stats.HotReloads,
		BuildTime:      s.stats.BuildTime,
	}
}

// ProfilerServer provides pprof endpoints for performance profiling
type ProfilerServer struct {
	server *http.Server
	port   int
}

// NewProfilerServer creates a new profiler server
func NewProfilerServer(port int) *ProfilerServer {
	return &ProfilerServer{
		port: port,
	}
}

// Start starts the profiler server
func (p *ProfilerServer) Start() error {
	mux := http.NewServeMux()

	// Register pprof handlers
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	p.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", p.port),
		Handler: mux,

		// Security timeouts to prevent DoS attacks
		ReadTimeout:       10 * time.Second, // Shorter timeout for profiler
		ReadHeaderTimeout: 3 * time.Second,  // Prevent Slowloris attacks
		WriteTimeout:      30 * time.Second, // Longer write timeout for profile data
		IdleTimeout:       60 * time.Second, // Standard idle timeout

		// Additional security settings
		MaxHeaderBytes: 1 << 20, // 1 MB max header size
	}

	return p.server.ListenAndServe()
}

// Stop stops the profiler server
func (p *ProfilerServer) Stop() error {
	if p.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return p.server.Shutdown(ctx)
	}
	return nil
}

// FileWatcher monitors file changes
type FileWatcher struct {
	events   chan FileEvent
	stop     chan struct{}
	lastMod  map[string]time.Time
	paths    []string
	interval time.Duration
	mu       sync.RWMutex
}

// FileEvent represents a file change event
type FileEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Path      string    `json:"path"`
	Type      string    `json:"type"`
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(paths []string, interval time.Duration) *FileWatcher {
	return &FileWatcher{
		paths:    paths,
		interval: interval,
		events:   make(chan FileEvent, 100),
		stop:     make(chan struct{}),
		lastMod:  make(map[string]time.Time),
	}
}

// Start starts the file watcher
func (w *FileWatcher) Start() error {
	go w.watch()
	return nil
}

// Stop stops the file watcher
func (w *FileWatcher) Stop() {
	close(w.stop)
}

// Events returns the events channel
func (w *FileWatcher) Events() <-chan FileEvent {
	return w.events
}

// watch monitors files for changes
func (w *FileWatcher) watch() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			w.checkFiles()
		}
	}
}

// checkFiles checks all monitored files for changes
func (w *FileWatcher) checkFiles() {
	for _, path := range w.paths {
		w.checkPath(path)
	}
}

// checkPath checks a specific path for changes
func (w *FileWatcher) checkPath(path string) {
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip directories and non-Go files
		if info.IsDir() || filepath.Ext(filePath) != ".go" {
			return nil
		}

		w.mu.RLock()
		lastMod, exists := w.lastMod[filePath]
		w.mu.RUnlock()

		if !exists || info.ModTime().After(lastMod) {
			w.mu.Lock()
			w.lastMod[filePath] = info.ModTime()
			w.mu.Unlock()

			if exists { // Don't trigger on first scan
				select {
				case w.events <- FileEvent{
					Path:      filePath,
					Type:      "modified",
					Timestamp: time.Now(),
				}:
				default:
					// Channel full, skip event
				}
			}
		}

		return nil
	})

	_ = err // Log error but continue
}
