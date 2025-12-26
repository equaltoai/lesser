package models

// ExportRequest represents a data export request.
type ExportRequest struct {
	Type         string           `json:"type"`
	Format       string           `json:"format"`
	IncludeMedia bool             `json:"include_media"`
	DateRange    *ExportDateRange `json:"date_range,omitempty"`
	Options      map[string]any   `json:"options,omitempty"`
}

// ExportDateRange represents a date range for filtering exports.
type ExportDateRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// ExportJob represents an export job status.
type ExportJob struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	Type        string  `json:"type"`
	Format      string  `json:"format"`
	CreatedAt   string  `json:"created_at"`
	DownloadURL *string `json:"download_url,omitempty"`
	ExpiresAt   *string `json:"expires_at,omitempty"`
	FileSize    *int64  `json:"file_size,omitempty"`
	RecordCount *int    `json:"record_count,omitempty"`
	Error       *string `json:"error,omitempty"`
}

// ExportDownloadResponse represents the response from GET /api/v1/exports/{id}/download.
type ExportDownloadResponse struct {
	DownloadURL string  `json:"download_url"`
	ExpiresAt   *string `json:"expires_at,omitempty"`
}
