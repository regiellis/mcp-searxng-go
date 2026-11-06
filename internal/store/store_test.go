package store

import (
	"log/slog"
	"testing"

	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(t.TempDir(), slog.New(slog.NewTextHandler(ioDiscard{}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSaveCreatesThenAppends(t *testing.T) {
	t.Parallel()
	s := newStore(t)

	created, err := s.SaveResearch(types.SaveResearchRequest{
		Title: "Caching study",
		Query: "how does caching work",
		Note:  "First finding",
		Tags:  []string{"perf"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || len(created.Notes) != 1 || created.Title != "Caching study" {
		t.Fatalf("unexpected created session: %#v", created)
	}

	appended, err := s.SaveResearch(types.SaveResearchRequest{
		ID:      created.ID,
		Note:    "Second finding",
		Sources: []string{"https://example.com"},
		Tags:    []string{"perf", "memory"}, // perf dedupes
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(appended.Notes) != 2 {
		t.Fatalf("expected 2 notes after append, got %d", len(appended.Notes))
	}
	if len(appended.Tags) != 2 {
		t.Fatalf("expected merged/deduped tags [perf memory], got %#v", appended.Tags)
	}
	if appended.Notes[1].Sources[0] != "https://example.com" {
		t.Fatalf("note sources not stored: %#v", appended.Notes[1])
	}
	if !appended.UpdatedAt.After(created.UpdatedAt) && !appended.UpdatedAt.Equal(created.UpdatedAt) {
		t.Fatal("updated_at should advance or hold on append")
	}
}

func TestGetAndList(t *testing.T) {
	t.Parallel()
	s := newStore(t)

	a, _ := s.SaveResearch(types.SaveResearchRequest{Title: "A", Note: "n"})
	b, _ := s.SaveResearch(types.SaveResearchRequest{Title: "B", Note: "n"})

	got, err := s.GetResearch(a.ID)
	if err != nil || got.Title != "A" {
		t.Fatalf("get A failed: %#v err=%v", got, err)
	}

	list, err := s.ListResearch()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(list))
	}
	// Newest first: b was saved last.
	if list[0].ID != b.ID {
		t.Fatalf("expected newest (%s) first, got %#v", b.ID, list)
	}
}

func TestInvalidAndMissingIDs(t *testing.T) {
	t.Parallel()
	s := newStore(t)

	// Path-traversal / malformed ids are rejected before any file access.
	if _, err := s.GetResearch("../etc/passwd"); err == nil {
		t.Fatal("expected rejection of traversal id")
	}
	if _, err := s.SaveResearch(types.SaveResearchRequest{ID: "rs_bad", Note: "x"}); err == nil {
		t.Fatal("expected rejection of malformed id")
	}
	// Well-formed but absent id.
	if _, err := s.GetResearch("rs_0123456789ab"); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestTitleFallsBackToQuery(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	got, err := s.SaveResearch(types.SaveResearchRequest{Query: "what is mmap", Note: "n"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "what is mmap" {
		t.Fatalf("expected title to fall back to query, got %q", got.Title)
	}
}
