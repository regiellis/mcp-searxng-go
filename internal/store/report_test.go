package store

import (
	"os"
	"strings"
	"testing"

	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

func TestExportReportFromFields(t *testing.T) {
	t.Parallel()
	s := newStore(t)

	resp, err := s.ExportReport(types.ExportReportRequest{
		Title:   "Caching Report",
		Query:   "how does caching work",
		Summary: "Caching trades memory for speed.",
		Sections: []types.ReportSection{
			{Heading: "Background", Body: "Caches store hot data."},
		},
		Sources: []string{"https://example.com/a", "https://example.com/b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(resp.Filename, ".md") || resp.Bytes == 0 {
		t.Fatalf("unexpected response: %#v", resp)
	}
	for _, want := range []string{"# Caching Report", "## Summary", "## Background", "## Sources", "- https://example.com/a"} {
		if !strings.Contains(resp.Content, want) {
			t.Fatalf("report missing %q:\n%s", want, resp.Content)
		}
	}
	// The file was actually written and matches the returned content.
	onDisk, err := os.ReadFile(resp.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != resp.Content {
		t.Fatal("written file does not match returned content")
	}
}

func TestExportReportFromSession(t *testing.T) {
	t.Parallel()
	s := newStore(t)

	created, err := s.SaveResearch(types.SaveResearchRequest{
		Title:   "Session Report",
		Query:   "q",
		Note:    "A key finding.",
		Sources: []string{"https://src.example"},
	})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := s.ExportReport(types.ExportReportRequest{ID: created.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.Content, "# Session Report") {
		t.Fatalf("missing title from session: %s", resp.Content)
	}
	if !strings.Contains(resp.Content, "A key finding.") {
		t.Fatalf("missing note body: %s", resp.Content)
	}
	// Note sources are aggregated into the Sources section.
	if !strings.Contains(resp.Content, "- https://src.example") {
		t.Fatalf("missing aggregated source: %s", resp.Content)
	}
}

func TestExportReportRejectsBadSessionID(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	if _, err := s.ExportReport(types.ExportReportRequest{ID: "../escape"}); err == nil {
		t.Fatal("expected rejection of traversal id")
	}
}

func TestFileSlug(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Caching Report": "caching-report",
		"  !!!  ":        "report",
		"a/b\\c":         "a-b-c",
	}
	for in, want := range cases {
		if got := fileSlug(in); got != want {
			t.Fatalf("fileSlug(%q) = %q, want %q", in, got, want)
		}
	}
}
