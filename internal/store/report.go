package store

import (
	"fmt"
	"strings"
	"time"

	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

// ExportReport renders a markdown report and writes it into the storage
// directory, returning the path and the rendered content. When req.ID is set the
// named research session is loaded and rendered (its notes become sections);
// otherwise the supplied fields are rendered directly. No LLM is involved — the
// markdown is assembled deterministically from the given content.
func (s *Store) ExportReport(req types.ExportReportRequest) (types.ExportReportResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	title := strings.TrimSpace(req.Title)
	query := strings.TrimSpace(req.Query)
	summary := strings.TrimSpace(req.Summary)
	sections := append([]types.ReportSection(nil), req.Sections...)
	sources := cleanStrings(req.Sources)

	if id := strings.TrimSpace(req.ID); id != "" {
		if !idPattern.MatchString(id) {
			return types.ExportReportResponse{}, fmt.Errorf("invalid research id %q", id)
		}
		session, err := s.read(id)
		if err != nil {
			return types.ExportReportResponse{}, err
		}
		if title == "" {
			title = session.Title
		}
		if query == "" {
			query = session.Query
		}
		aggregated := make([]string, 0)
		for i, note := range session.Notes {
			heading := fmt.Sprintf("Note %d — %s", i+1, note.At.UTC().Format("2006-01-02 15:04 UTC"))
			body := note.Text
			if len(note.Sources) > 0 {
				body += "\n\nSources:\n" + bulletList(note.Sources)
				aggregated = append(aggregated, note.Sources...)
			}
			sections = append(sections, types.ReportSection{Heading: heading, Body: body})
		}
		if len(sources) == 0 {
			sources = cleanStrings(aggregated)
		}
	}

	if title == "" {
		title = firstNonEmpty(query, "Research report")
	}

	content := renderMarkdown(title, query, summary, sections, sources, time.Now().UTC())

	suffix, err := newReportSuffix()
	if err != nil {
		return types.ExportReportResponse{}, err
	}
	filename := "report_" + fileSlug(title) + "_" + suffix + ".md"
	path, err := s.writeFile(filename, []byte(content))
	if err != nil {
		return types.ExportReportResponse{}, err
	}
	if s.logger != nil {
		s.logger.Info("report exported", "path", path, "bytes", len(content))
	}
	return types.ExportReportResponse{
		Path:     path,
		Filename: filename,
		Bytes:    len(content),
		Content:  content,
	}, nil
}

func renderMarkdown(title, query, summary string, sections []types.ReportSection, sources []string, at time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	if query != "" {
		fmt.Fprintf(&b, "**Query:** %s\n\n", query)
	}
	fmt.Fprintf(&b, "*Generated %s*\n", at.Format("2006-01-02 15:04 UTC"))

	if summary != "" {
		b.WriteString("\n## Summary\n\n")
		b.WriteString(summary)
		b.WriteString("\n")
	}
	for _, sec := range sections {
		heading := strings.TrimSpace(sec.Heading)
		if heading == "" {
			heading = "Section"
		}
		fmt.Fprintf(&b, "\n## %s\n\n", heading)
		b.WriteString(strings.TrimSpace(sec.Body))
		b.WriteString("\n")
	}
	if len(sources) > 0 {
		b.WriteString("\n## Sources\n\n")
		b.WriteString(bulletList(sources))
		b.WriteString("\n")
	}
	return b.String()
}

func bulletList(items []string) string {
	var b strings.Builder
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			fmt.Fprintf(&b, "- %s\n", item)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// fileSlug reduces a title to a short, filename-safe token.
func fileSlug(title string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case !lastDash && b.Len() > 0:
			b.WriteByte('-')
			lastDash = true
		}
		if b.Len() >= 40 {
			break
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "report"
	}
	return slug
}
