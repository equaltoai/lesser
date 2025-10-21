package dev

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/pay-theory/lift/pkg/features"
	"github.com/pay-theory/lift/pkg/lift"
)

// DevDashboard provides an interactive web interface for development
type DevDashboard struct {
	server     *DevServer
	httpServer *http.Server
	logService *LogService
	features   *features.FeatureFlags
	port       int
}

// NewDevDashboard creates a new development dashboard
func NewDevDashboard(server *DevServer, port int) *DevDashboard {
	// Initialize feature flags if not already done
	var ff *features.FeatureFlags
	if server != nil {
		ff = server.features
	}
	if ff == nil {
		var err error
		ff, err = features.NewFeatureFlags(features.FeatureFlagConfig{
			LocalOnly: true,
		})
		if err != nil {
			log.Printf("Warning: failed to create feature flags: %v", err)
			// Continue with nil feature flags
		}
	}

	return &DevDashboard{
		server:     server,
		port:       port,
		logService: LogServiceFactory("", ff),
		features:   ff,
	}
}

// Start starts the dashboard server
func (d *DevDashboard) Start() error {
	mux := http.NewServeMux()

	// Static routes
	mux.HandleFunc("/", d.handleDashboard)
	mux.HandleFunc("/api/stats", d.handleAPIStats)
	mux.HandleFunc("/api/health", d.handleAPIHealth)
	mux.HandleFunc("/api/restart", d.handleAPIRestart)
	mux.HandleFunc("/api/logs", d.handleAPILogs)
	mux.HandleFunc("/static/", d.handleStatic)

	d.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", d.port),
		Handler: mux,

		// Security timeouts to prevent DoS attacks
		ReadTimeout:       15 * time.Second, // Maximum time to read request including body
		ReadHeaderTimeout: 5 * time.Second,  // Maximum time to read request headers (prevents Slowloris)
		WriteTimeout:      15 * time.Second, // Maximum time to write response
		IdleTimeout:       60 * time.Second, // Maximum time for keep-alive connections

		// Additional security settings
		MaxHeaderBytes: 1 << 20, // 1 MB max header size
	}

	return d.httpServer.ListenAndServe()
}

// Stop stops the dashboard server
func (d *DevDashboard) Stop() error {
	// Stop log service
	if d.logService != nil {
		d.logService.Stop()
	}

	if d.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return d.httpServer.Shutdown(ctx)
	}
	return nil
}

// handleDashboard serves the main dashboard page
func (d *DevDashboard) handleDashboard(w http.ResponseWriter, _ *http.Request) {
	tmpl := template.Must(template.New("dashboard").Parse(dashboardHTML))

	data := struct {
		Title        string
		Stats        SafeDevStats
		ServerPort   int
		ProfilerPort int
	}{
		Title:        "Lift Development Dashboard",
		ServerPort:   d.server.config.Port,
		ProfilerPort: d.server.config.ProfilerPort,
		Stats:        d.server.GetStats(),
	}

	w.Header().Set(lift.HeaderContentType, lift.ContentTypeHTML)
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Failed to execute dashboard template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// handleAPIStats returns server statistics as JSON
func (d *DevDashboard) handleAPIStats(w http.ResponseWriter, _ *http.Request) {
	stats := d.server.GetStats()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		log.Printf("Failed to encode stats: %v", err)
	}
}

// handleAPIHealth returns server health as JSON
func (d *DevDashboard) handleAPIHealth(w http.ResponseWriter, _ *http.Request) {
	health := map[string]any{
		"status":     "healthy",
		"timestamp":  time.Now(),
		"uptime":     time.Since(d.server.startTime),
		"hot_reload": d.server.config.HotReload,
		"debug_mode": d.server.config.DebugMode,
		"version":    "dev",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(health); err != nil {
		log.Printf("Failed to encode health: %v", err)
	}
}

// handleAPIRestart triggers a server restart
func (d *DevDashboard) handleAPIRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	d.server.triggerRestart()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "restart triggered",
	}); err != nil {
		log.Printf("Failed to encode restart response: %v", err)
	}
}

// handleAPILogs returns recent logs from the log service
func (d *DevDashboard) handleAPILogs(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()
	limit := 100
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	// Get search query if provided
	searchQuery := query.Get("search")

	var logs []LogEntry
	if searchQuery != "" {
		logs = d.logService.SearchLogs(searchQuery)
	} else {
		logs = d.logService.GetRecentLogs(limit)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(logs); err != nil {
		// Log error but continue - best effort dev dashboard response
		fmt.Printf("Warning: Error encoding logs response: %v\n", err)
	}
}

// handleStatic serves static assets
func (d *DevDashboard) handleStatic(w http.ResponseWriter, r *http.Request) {
	// For the development dashboard, we embed CSS and JS directly
	// This keeps the dashboard self-contained with no external dependencies
	path := r.URL.Path[8:] // Remove "/static/"

	switch path {
	case "style.css":
		w.Header().Set("Content-Type", "text/css")
		if _, err := w.Write([]byte(dashboardCSS)); err != nil {
			log.Printf("Failed to write CSS: %v", err)
		}
	case "script.js":
		w.Header().Set("Content-Type", "application/javascript")
		if _, err := w.Write([]byte(dashboardJS)); err != nil {
			log.Printf("Failed to write JS: %v", err)
		}
	default:
		http.NotFound(w, r)
	}
}

// dashboardHTML is the main dashboard HTML template
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <div class="container">
        <header>
            <h1>🚀 {{.Title}}</h1>
            <div class="status-indicator healthy">
                <span class="dot"></span>
                <span>Development Server</span>
            </div>
        </header>

        <div class="grid">
            <!-- Server Info -->
            <div class="card">
                <h2>📡 Server Information</h2>
                <div class="info-grid">
                    <div class="info-item">
                        <label>Main Server:</label>
                        <span><a href="http://localhost:{{.ServerPort}}" target="_blank">http://localhost:{{.ServerPort}}</a></span>
                    </div>
                    <div class="info-item">
                        <label>Profiler:</label>
                        <span><a href="http://localhost:{{.ProfilerPort}}/debug/pprof/" target="_blank">http://localhost:{{.ProfilerPort}}/debug/pprof/</a></span>
                    </div>
                    <div class="info-item">
                        <label>Uptime:</label>
                        <span id="uptime">{{.Stats.Uptime}}</span>
                    </div>
                    <div class="info-item">
                        <label>Hot Reload:</label>
                        <span class="badge success">Enabled</span>
                    </div>
                </div>
            </div>

            <!-- Statistics -->
            <div class="card">
                <h2>📊 Statistics</h2>
                <div class="stats-grid">
                    <div class="stat-item">
                        <div class="stat-value" id="requests">{{.Stats.Requests}}</div>
                        <div class="stat-label">Requests</div>
                    </div>
                    <div class="stat-item">
                        <div class="stat-value" id="errors">{{.Stats.Errors}}</div>
                        <div class="stat-label">Errors</div>
                    </div>
                    <div class="stat-item">
                        <div class="stat-value" id="restarts">{{.Stats.Restarts}}</div>
                        <div class="stat-label">Restarts</div>
                    </div>
                    <div class="stat-item">
                        <div class="stat-value" id="hot-reloads">{{.Stats.HotReloads}}</div>
                        <div class="stat-label">Hot Reloads</div>
                    </div>
                </div>
            </div>

            <!-- Performance -->
            <div class="card">
                <h2>⚡ Performance</h2>
                <div class="info-grid">
                    <div class="info-item">
                        <label>Average Latency:</label>
                        <span id="avg-latency">{{.Stats.AverageLatency}}</span>
                    </div>
                    <div class="info-item">
                        <label>Build Time:</label>
                        <span id="build-time">{{.Stats.BuildTime}}</span>
                    </div>
                    <div class="info-item">
                        <label>Memory Usage:</label>
                        <span class="badge info">28MB</span>
                    </div>
                    <div class="info-item">
                        <label>CPU Usage:</label>
                        <span class="badge info">2.1%</span>
                    </div>
                </div>
            </div>

            <!-- Actions -->
            <div class="card">
                <h2>🔧 Actions</h2>
                <div class="actions">
                    <button class="btn primary" onclick="restartServer()">
                        🔄 Restart Server
                    </button>
                    <button class="btn secondary" onclick="viewLogs()">
                        📋 View Logs
                    </button>
                    <button class="btn secondary" onclick="openProfiler()">
                        🔍 Open Profiler
                    </button>
                    <button class="btn secondary" onclick="runTests()">
                        🧪 Run Tests
                    </button>
                </div>
            </div>

            <!-- Recent Logs -->
            <div class="card full-width">
                <h2>📋 Recent Logs</h2>
                <div class="logs-container" id="logs-container">
                    <div class="log-entry info">
                        <span class="timestamp">{{.Stats.LastRestart.Format "15:04:05"}}</span>
                        <span class="level">INFO</span>
                        <span class="message">Development server started</span>
                    </div>
                </div>
            </div>
        </div>
    </div>

    <script src="/static/script.js"></script>
</body>
</html>`

// dashboardCSS contains the dashboard styles
const dashboardCSS = `
* {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
}

body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: #f5f7fa;
    color: #2d3748;
    line-height: 1.6;
}

.container {
    max-width: 1200px;
    margin: 0 auto;
    padding: 20px;
}

header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 30px;
    padding: 20px 0;
    border-bottom: 2px solid #e2e8f0;
}

h1 {
    color: #1a202c;
    font-size: 2rem;
    font-weight: 700;
}

.status-indicator {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 16px;
    border-radius: 20px;
    font-weight: 500;
}

.status-indicator.healthy {
    background: #f0fff4;
    color: #22543d;
    border: 1px solid #9ae6b4;
}

.dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #48bb78;
    animation: pulse 2s infinite;
}

@keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.5; }
}

.grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
    gap: 20px;
}

.card {
    background: white;
    border-radius: 12px;
    padding: 24px;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
    border: 1px solid #e2e8f0;
}

.card.full-width {
    grid-column: 1 / -1;
}

h2 {
    color: #2d3748;
    font-size: 1.25rem;
    font-weight: 600;
    margin-bottom: 16px;
}

.info-grid {
    display: grid;
    gap: 12px;
}

.info-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 0;
    border-bottom: 1px solid #f7fafc;
}

.info-item:last-child {
    border-bottom: none;
}

.info-item label {
    font-weight: 500;
    color: #4a5568;
}

.stats-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 16px;
}

.stat-item {
    text-align: center;
    padding: 16px;
    background: #f7fafc;
    border-radius: 8px;
}

.stat-value {
    font-size: 2rem;
    font-weight: 700;
    color: #2b6cb0;
    margin-bottom: 4px;
}

.stat-label {
    font-size: 0.875rem;
    color: #4a5568;
    font-weight: 500;
}

.badge {
    padding: 4px 8px;
    border-radius: 12px;
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
}

.badge.success {
    background: #f0fff4;
    color: #22543d;
}

.badge.info {
    background: #ebf8ff;
    color: #2c5282;
}

.actions {
    display: grid;
    gap: 12px;
}

.btn {
    padding: 12px 16px;
    border: none;
    border-radius: 8px;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.2s;
    text-decoration: none;
    display: inline-block;
    text-align: center;
}

.btn.primary {
    background: #3182ce;
    color: white;
}

.btn.primary:hover {
    background: #2c5282;
}

.btn.secondary {
    background: #edf2f7;
    color: #4a5568;
}

.btn.secondary:hover {
    background: #e2e8f0;
}

.logs-container {
    max-height: 300px;
    overflow-y: auto;
    background: #1a202c;
    border-radius: 8px;
    padding: 16px;
}

.log-entry {
    display: flex;
    gap: 12px;
    margin-bottom: 8px;
    font-family: 'Monaco', 'Menlo', monospace;
    font-size: 0.875rem;
}

.log-entry:last-child {
    margin-bottom: 0;
}

.timestamp {
    color: #a0aec0;
    min-width: 80px;
}

.level {
    min-width: 50px;
    font-weight: 600;
}

.level.INFO {
    color: #63b3ed;
}

.level.DEBUG {
    color: #9ae6b4;
}

.level.WARN {
    color: #fbb6ce;
}

.level.ERROR {
    color: #fc8181;
}

.message {
    color: #e2e8f0;
    flex: 1;
}

a {
    color: #3182ce;
    text-decoration: none;
}

a:hover {
    text-decoration: underline;
}

@media (max-width: 768px) {
    .container {
        padding: 10px;
    }
    
    header {
        flex-direction: column;
        gap: 16px;
        text-align: center;
    }
    
    .stats-grid {
        grid-template-columns: 1fr;
    }
}
`

// dashboardJS contains the dashboard JavaScript
const dashboardJS = `
// Auto-refresh functionality
let refreshInterval;

function startAutoRefresh() {
    refreshInterval = setInterval(updateStats, 2000);
}

function stopAutoRefresh() {
    if (refreshInterval) {
        clearInterval(refreshInterval);
    }
}

function updateStats() {
    fetch('/api/stats')
        .then(response => response.json())
        .then(data => {
            document.getElementById('requests').textContent = data.requests || 0;
            document.getElementById('errors').textContent = data.errors || 0;
            document.getElementById('restarts').textContent = data.restarts || 0;
            document.getElementById('hot-reloads').textContent = data.hot_reloads || 0;
            
            if (data.average_latency) {
                document.getElementById('avg-latency').textContent = formatDuration(data.average_latency);
            }
            
            if (data.build_time) {
                document.getElementById('build-time').textContent = formatDuration(data.build_time);
            }
            
            if (data.uptime) {
                document.getElementById('uptime').textContent = formatDuration(data.uptime);
            }
        })
        .catch(error => {
            console.error('Failed to update stats:', error);
        });
}

function updateLogs() {
    fetch('/api/logs')
        .then(response => response.json())
        .then(logs => {
            const container = document.getElementById('logs-container');
            container.innerHTML = '';
            
            logs.forEach(log => {
                const entry = document.createElement('div');
                entry.className = 'log-entry';
                
                const timestamp = new Date(log.timestamp).toLocaleTimeString();
                
                entry.innerHTML = ` + "`" + `
                    <span class="timestamp">${timestamp}</span>
                    <span class="level ${log.level}">${log.level}</span>
                    <span class="message">${log.message}</span>
                ` + "`" + `;
                
                container.appendChild(entry);
            });
            
            container.scrollTop = container.scrollHeight;
        })
        .catch(error => {
            console.error('Failed to update logs:', error);
        });
}

function restartServer() {
    if (confirm('Are you sure you want to restart the server?')) {
        fetch('/api/restart', { method: 'POST' })
            .then(response => response.json())
            .then(data => {
                showNotification('Server restart triggered', 'success');
                setTimeout(updateStats, 1000);
            })
            .catch(error => {
                showNotification('Failed to restart server', 'error');
                console.error('Restart failed:', error);
            });
    }
}

function viewLogs() {
    updateLogs();
    showNotification('Logs refreshed', 'info');
}

function openProfiler() {
    const profilerUrl = 'http://localhost:6060/debug/pprof/';
    window.open(profilerUrl, '_blank');
}

function runTests() {
    showNotification('Running tests... (this would execute tests in a real implementation)', 'info');
}

function showNotification(message, type = 'info') {
    const notification = document.createElement('div');
    notification.className = ` + "`" + `notification ${type}` + "`" + `;
    notification.textContent = message;
    
    notification.style.cssText = ` + "`" + `
        position: fixed;
        top: 20px;
        right: 20px;
        padding: 12px 16px;
        border-radius: 8px;
        color: white;
        font-weight: 500;
        z-index: 1000;
        animation: slideIn 0.3s ease;
    ` + "`" + `;
    
    switch (type) {
        case 'success':
            notification.style.background = '#48bb78';
            break;
        case 'error':
            notification.style.background = '#f56565';
            break;
        case 'info':
        default:
            notification.style.background = '#4299e1';
            break;
    }
    
    document.body.appendChild(notification);
    
    setTimeout(() => {
        notification.remove();
    }, 3000);
}

function formatDuration(nanoseconds) {
    if (nanoseconds < 1000) {
        return nanoseconds + 'ns';
    } else if (nanoseconds < 1000000) {
        return (nanoseconds / 1000).toFixed(1) + 'µs';
    } else if (nanoseconds < 1000000000) {
        return (nanoseconds / 1000000).toFixed(1) + 'ms';
    } else {
        return (nanoseconds / 1000000000).toFixed(1) + 's';
    }
}

// Add CSS for animations
const style = document.createElement('style');
style.textContent = ` + "`" + `
    @keyframes slideIn {
        from {
            transform: translateX(100%);
            opacity: 0;
        }
        to {
            transform: translateX(0);
            opacity: 1;
        }
    }
` + "`" + `;
document.head.appendChild(style);

// Initialize dashboard
document.addEventListener('DOMContentLoaded', function() {
    updateStats();
    updateLogs();
    startAutoRefresh();
    
    // Stop auto-refresh when page is hidden
    document.addEventListener('visibilitychange', function() {
        if (document.hidden) {
            stopAutoRefresh();
        } else {
            startAutoRefresh();
        }
    });
});

// Cleanup on page unload
window.addEventListener('beforeunload', function() {
    stopAutoRefresh();
});
`
