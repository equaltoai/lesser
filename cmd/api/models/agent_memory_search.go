package models

// AgentMemorySearchRequest is the request payload for /api/v1/agents/memory/search.
//
// This endpoint is agent-scoped and is intended to support "timeline-as-memory" retrieval.
type AgentMemorySearchRequest struct {
	Query          string         `json:"query,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	DateRange      *DateRange     `json:"date_range,omitempty"`
	IncludeThreads bool           `json:"include_threads,omitempty"`
	Limit          int            `json:"limit,omitempty"`
	ThreadID       string         `json:"thread_id,omitempty"`
	Options        map[string]any `json:"options,omitempty"`
}

// DateRange is an inclusive date-only range (YYYY-MM-DD) used for memory retrieval.
type DateRange struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// AgentMemorySearchResponse is the response payload for /api/v1/agents/memory/search.
type AgentMemorySearchResponse struct {
	Results     []AgentMemorySearchResult `json:"results"`
	Total       int                      `json:"total"`
	QueryTimeMS int                      `json:"query_time_ms"`
}

// AgentMemorySearchResult represents a single memory hit with lightweight context.
type AgentMemorySearchResult struct {
	Status         *Status                  `json:"status"`
	RelevanceScore float64                  `json:"relevance_score"`
	Context        *AgentMemorySearchContext `json:"context,omitempty"`
	Thread         []*Status                `json:"thread,omitempty"`
}

// AgentMemorySearchContext provides metadata that helps clients reason about a memory hit.
type AgentMemorySearchContext struct {
	ThreadRoot string   `json:"thread_root,omitempty"`
	ReplyCount int      `json:"reply_count,omitempty"`
	Tags       []string `json:"tags,omitempty"`

	// Event-sourced memory fields.
	EventType  string `json:"event_type,omitempty"`
	OriginalID string `json:"original_id,omitempty"`
}

