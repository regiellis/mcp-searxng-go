// Package store persists research sessions as JSON files inside a sandboxed
// directory, so an investigation can be saved, recalled, and appended to across
// separate tool calls. It is deterministic and needs no external service.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

// idPattern matches the IDs this package generates; user-supplied IDs are
// validated against it so they can never escape the storage directory.
var idPattern = regexp.MustCompile(`^rs_[0-9a-f]{12}$`)

// Store reads and writes research sessions under a single directory.
type Store struct {
	dir    string
	mu     sync.Mutex
	logger *slog.Logger
}

// NewStore creates the storage directory and returns a Store.
func NewStore(dir string, logger *slog.Logger) (*Store, error) {
	abs, err := filepath.Abs(strings.TrimSpace(dir))
	if err != nil {
		return nil, fmt.Errorf("resolve storage dir: %w", err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}
	return &Store{dir: abs, logger: logger}, nil
}

// Dir returns the storage root.
func (s *Store) Dir() string { return s.dir }

// SaveResearch creates a new session (when req.ID is empty) or appends to an
// existing one. A note is appended when provided; title, query, and tags are
// updated/merged when supplied.
func (s *Store) SaveResearch(req types.SaveResearchRequest) (types.ResearchSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	var session types.ResearchSession

	if id := strings.TrimSpace(req.ID); id != "" {
		if !idPattern.MatchString(id) {
			return types.ResearchSession{}, fmt.Errorf("invalid research id %q", id)
		}
		existing, err := s.read(id)
		if err != nil {
			return types.ResearchSession{}, err
		}
		session = existing
	} else {
		newID, err := newSessionID()
		if err != nil {
			return types.ResearchSession{}, err
		}
		session = types.ResearchSession{ID: newID, CreatedAt: now}
	}

	if title := strings.TrimSpace(req.Title); title != "" {
		session.Title = title
	}
	if session.Title == "" {
		session.Title = firstNonEmpty(strings.TrimSpace(req.Query), "Untitled research")
	}
	if query := strings.TrimSpace(req.Query); query != "" {
		session.Query = query
	}
	session.Tags = mergeTags(session.Tags, req.Tags)

	if note := strings.TrimSpace(req.Note); note != "" {
		session.Notes = append(session.Notes, types.ResearchNote{
			At:      now,
			Text:    note,
			Sources: cleanStrings(req.Sources),
		})
	}
	session.UpdatedAt = now

	if err := s.write(session); err != nil {
		return types.ResearchSession{}, err
	}
	if s.logger != nil {
		s.logger.Info("research saved", "id", session.ID, "notes", len(session.Notes))
	}
	return session, nil
}

// GetResearch returns a session by ID.
func (s *Store) GetResearch(id string) (types.ResearchSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	if !idPattern.MatchString(id) {
		return types.ResearchSession{}, fmt.Errorf("invalid research id %q", id)
	}
	return s.read(id)
}

// ListResearch returns summaries of all stored sessions, newest first.
func (s *Store) ListResearch() ([]types.ResearchSessionSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	summaries := make([]types.ResearchSessionSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !idPattern.MatchString(id) {
			continue
		}
		session, err := s.read(id)
		if err != nil {
			continue
		}
		summaries = append(summaries, types.ResearchSessionSummary{
			ID:        session.ID,
			Title:     session.Title,
			Query:     session.Query,
			Tags:      session.Tags,
			NoteCount: len(session.Notes),
			UpdatedAt: session.UpdatedAt,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})
	return summaries, nil
}

// read loads a session by validated ID. Caller holds the lock.
func (s *Store) read(id string) (types.ResearchSession, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, id+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return types.ResearchSession{}, fmt.Errorf("research session %q not found", id)
		}
		return types.ResearchSession{}, err
	}
	var session types.ResearchSession
	if err := json.Unmarshal(data, &session); err != nil {
		return types.ResearchSession{}, fmt.Errorf("decode research session %q: %w", id, err)
	}
	return session, nil
}

// write persists a session atomically (temp file + rename). Caller holds the lock.
func (s *Store) write(session types.ResearchSession) error {
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	final := filepath.Join(s.dir, session.ID+".json")
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

func newSessionID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate research id: %w", err)
	}
	return "rs_" + hex.EncodeToString(b), nil
}

func mergeTags(existing, incoming []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	out := make([]string, 0, len(existing)+len(incoming))
	for _, tag := range append(append([]string{}, existing...), incoming...) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cleanStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
