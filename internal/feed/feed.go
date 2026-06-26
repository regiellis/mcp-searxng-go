// Package feed parses RSS 2.0 and Atom feed documents into a common structured
// form. It is pure encoding/xml with no external dependencies and no network of
// its own — callers fetch the bytes (through the SSRF-guarded reader) and pass
// them in.
package feed

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"

	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

var tagRE = regexp.MustCompile(`<[^>]+>`)

// Parsed is the normalized result of parsing a feed document.
type Parsed struct {
	Format string // rss or atom
	Title  string
	Link   string
	Items  []types.FeedItem
}

type rssRoot struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Title string `xml:"title"`
		Link  string `xml:"link"`
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			PubDate     string `xml:"pubDate"`
			Description string `xml:"description"`
			GUID        string `xml:"guid"`
		} `xml:"item"`
	} `xml:"channel"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomRoot struct {
	XMLName xml.Name   `xml:"feed"`
	Title   string     `xml:"title"`
	Links   []atomLink `xml:"link"`
	Entries []struct {
		Title     string     `xml:"title"`
		Links     []atomLink `xml:"link"`
		Updated   string     `xml:"updated"`
		Published string     `xml:"published"`
		Summary   string     `xml:"summary"`
		Content   string     `xml:"content"`
		ID        string     `xml:"id"`
	} `xml:"entry"`
}

// Parse detects and parses an RSS 2.0 or Atom document.
func Parse(data []byte) (Parsed, error) {
	var rss rssRoot
	if err := xml.Unmarshal(data, &rss); err == nil && rss.XMLName.Local == "rss" {
		return fromRSS(rss), nil
	}
	var atom atomRoot
	if err := xml.Unmarshal(data, &atom); err == nil && atom.XMLName.Local == "feed" {
		return fromAtom(atom), nil
	}
	return Parsed{}, fmt.Errorf("not a recognized RSS or Atom feed")
}

func fromRSS(r rssRoot) Parsed {
	out := Parsed{
		Format: "rss",
		Title:  strings.TrimSpace(r.Channel.Title),
		Link:   strings.TrimSpace(r.Channel.Link),
		Items:  make([]types.FeedItem, 0, len(r.Channel.Items)),
	}
	for _, item := range r.Channel.Items {
		out.Items = append(out.Items, types.FeedItem{
			Title:     strings.TrimSpace(item.Title),
			Link:      strings.TrimSpace(item.Link),
			Published: strings.TrimSpace(item.PubDate),
			Summary:   cleanSummary(item.Description),
			ID:        strings.TrimSpace(item.GUID),
		})
	}
	return out
}

func fromAtom(a atomRoot) Parsed {
	out := Parsed{
		Format: "atom",
		Title:  strings.TrimSpace(a.Title),
		Link:   pickAtomLink(a.Links),
		Items:  make([]types.FeedItem, 0, len(a.Entries)),
	}
	for _, entry := range a.Entries {
		published := strings.TrimSpace(entry.Published)
		if published == "" {
			published = strings.TrimSpace(entry.Updated)
		}
		summary := entry.Summary
		if strings.TrimSpace(summary) == "" {
			summary = entry.Content
		}
		out.Items = append(out.Items, types.FeedItem{
			Title:     strings.TrimSpace(entry.Title),
			Link:      pickAtomLink(entry.Links),
			Published: published,
			Summary:   cleanSummary(summary),
			ID:        strings.TrimSpace(entry.ID),
		})
	}
	return out
}

// pickAtomLink prefers an explicit alternate link, then the first link without a
// rel (the conventional default), then any link.
func pickAtomLink(links []atomLink) string {
	for _, l := range links {
		if l.Rel == "alternate" {
			return strings.TrimSpace(l.Href)
		}
	}
	for _, l := range links {
		if l.Rel == "" {
			return strings.TrimSpace(l.Href)
		}
	}
	if len(links) > 0 {
		return strings.TrimSpace(links[0].Href)
	}
	return ""
}

// cleanSummary strips inline HTML tags and collapses whitespace so feed
// descriptions read as plain text.
func cleanSummary(s string) string {
	s = tagRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}
