package mcp

import (
	"slices"
	"strings"
)

// supportedCategories are the SearXNG categories this server forwards, in
// the order tool schemas list them. Each was probed against the live
// instance (SearXNG 2026.11.5, 2026-11-06) and returned results:
//
//	general, images, videos, news   web and media (the original four)
//	music                           youtube, genius, soundcloud, mixcloud
//	it                              github, docker hub, stackoverflow
//	packages                        pkg.go.dev, crates.io, npm, pypi, docker hub, ...
//	repos                           github, gitlab, codeberg
//	q&a                             stackoverflow and Discourse forums
//	science                         arxiv, pubmed, europepmc, semantic scholar
//	scientific publications         the paper-only subset of science, plus crossref
//	files                           wikicommons files and torrent indexes
//	map                             openstreetmap, photon (places with coordinates)
//	social media                    mastodon, lemmy
//
// "shopping" is deliberately absent: no shopping engine is enabled, so the
// category returns nothing; shopping_search stays a general-web preset.
var supportedCategories = []string{
	"general",
	"images",
	"videos",
	"news",
	"music",
	"it",
	"packages",
	"repos",
	"q&a",
	"science",
	"scientific publications",
	"files",
	"map",
	"social media",
}

// defaultMultiCategories is what multi_search, quick_look, and monitor_query
// fan out to when the caller names none.
var defaultMultiCategories = []string{"general", "images", "videos", "news"}

func validCategory(category string) bool {
	return slices.Contains(supportedCategories, strings.TrimSpace(category))
}

// categoryEnum returns a copy of supportedCategories for a JSON schema enum,
// so a schema consumer cannot mutate the shared list.
func categoryEnum() []string {
	return slices.Clone(supportedCategories)
}

func categoryProperty() map[string]any {
	return map[string]any{
		"type": "string",
		"enum": categoryEnum(),
	}
}

func categoryListProperty() map[string]any {
	return map[string]any{
		"type":  "array",
		"items": categoryProperty(),
	}
}
