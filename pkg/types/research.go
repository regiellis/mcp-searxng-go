package types

import "time"

// ResearchNote is one timestamped entry appended to a research session.
type ResearchNote struct {
	At      time.Time `json:"at"`
	Text    string    `json:"text"`
	Sources []string  `json:"sources,omitempty"` // optional supporting URLs
}

// ResearchSession is a persisted, append-only record of an investigation: a
// title and query plus the notes accumulated across calls.
type ResearchSession struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Query     string         `json:"query,omitempty"`
	Tags      []string       `json:"tags,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Notes     []ResearchNote `json:"notes"`
}

// ResearchSessionSummary is the compact form returned by list_research.
type ResearchSessionSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Query     string    `json:"query,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	NoteCount int       `json:"note_count"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SaveResearchRequest is the input for the save_research tool. With no id a new
// session is created; with an id an existing session is appended to/updated.
type SaveResearchRequest struct {
	ID      string   `json:"id,omitempty"`
	Title   string   `json:"title,omitempty"`
	Query   string   `json:"query,omitempty"`
	Note    string   `json:"note,omitempty"`
	Sources []string `json:"sources,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

// GetResearchRequest is the input for the get_research tool.
type GetResearchRequest struct {
	ID string `json:"id"`
}

// ListResearchResponse is returned by the list_research tool.
type ListResearchResponse struct {
	Sessions []ResearchSessionSummary `json:"sessions"`
}

// ReportSection is one headed block in an exported report.
type ReportSection struct {
	Heading string `json:"heading"`
	Body    string `json:"body"`
}

// ExportReportRequest is the input for the export_report tool. With an id, the
// named research session is rendered; otherwise the provided fields are used.
type ExportReportRequest struct {
	ID       string          `json:"id,omitempty"`
	Title    string          `json:"title,omitempty"`
	Query    string          `json:"query,omitempty"`
	Summary  string          `json:"summary,omitempty"`
	Sections []ReportSection `json:"sections,omitempty"`
	Sources  []string        `json:"sources,omitempty"`
}

// ExportReportResponse is returned by the export_report tool. Content is the
// rendered markdown (also written to Path inside the storage directory).
type ExportReportResponse struct {
	Path     string `json:"path"`
	Filename string `json:"filename"`
	Bytes    int    `json:"bytes"`
	Content  string `json:"content"`
}
