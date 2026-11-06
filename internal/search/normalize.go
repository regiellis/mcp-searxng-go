package search

import (
	"bytes"
	"encoding/json"
	"math"
	urlpkg "net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/regiellis/mcp-searxng-go/pkg/types"
)

// Caps on the auxiliary blocks of a response. Results are bounded by the
// request limit; these keep a rich entity card from dominating an agent's
// context. Wikidata infoboxes for well-known entities run past 40 attributes.
const (
	maxInfoboxes          = 3
	maxInfoboxAttributes  = 30
	maxInfoboxURLs        = 15
	maxSuggestions        = 10
	maxCorrections        = 5
	maxAnswers            = 5
	maxUnresponsiveReport = 30
)

// searxResponse is the subset of SearXNG's JSON output that is consumed.
// Results and the auxiliary blocks are kept raw and decoded element by
// element, so one malformed entry is skipped instead of failing the search.
type searxResponse struct {
	Query               string            `json:"query"`
	Results             []json.RawMessage `json:"results"`
	Answers             []json.RawMessage `json:"answers"`
	Infoboxes           []json.RawMessage `json:"infoboxes"`
	Suggestions         flexStrings       `json:"suggestions"`
	Corrections         flexStrings       `json:"corrections"`
	UnresponsiveEngines []json.RawMessage `json:"unresponsive_engines"`
}

type searxResult struct {
	Title    flexString  `json:"title"`
	URL      flexString  `json:"url"`
	Content  flexString  `json:"content"`
	Engine   flexString  `json:"engine"`
	Engines  flexStrings `json:"engines"`
	Score    flexFloat   `json:"score"`
	Category flexString  `json:"category"`
	Template flexString  `json:"template"`
	Source   flexString  `json:"source"`

	Thumbnail    flexString `json:"thumbnail"`
	ThumbnailSrc flexString `json:"thumbnail_src"`
	ImgSrc       flexString `json:"img_src"`
	ContentURL   flexString `json:"content_url"`
	Embedded     flexString `json:"embedded"`
	IframeSrc    flexString `json:"iframe_src"`
	AudioSrc     flexString `json:"audio_src"`

	Width      flexInt    `json:"width"`
	Height     flexInt    `json:"height"`
	Resolution flexString `json:"resolution"`
	ImgFormat  flexString `json:"img_format"`
	MimeType   flexString `json:"mimetype"`
	FileSize   flexString `json:"filesize"`
	Size       flexString `json:"size"`

	Duration    flexString  `json:"duration"`
	Length      flexString  `json:"length"`
	Views       flexString  `json:"views"`
	PublishedAt flexString  `json:"publishedDate"`
	Channel     flexString  `json:"channel"`
	Author      flexString  `json:"author"`
	Tags        flexStrings `json:"tags"`

	// packages.html
	PackageName   flexString `json:"package_name"`
	Version       flexString `json:"version"`
	Maintainer    flexString `json:"maintainer"`
	Popularity    flexString `json:"popularity"`
	Homepage      flexString `json:"homepage"`
	SourceCodeURL flexString `json:"source_code_url"`
	LicenseName   flexString `json:"license_name"`
	LicenseURL    flexString `json:"license_url"`

	// paper.html
	DOI       flexString  `json:"doi"`
	Authors   flexStrings `json:"authors"`
	Journal   flexString  `json:"journal"`
	Publisher flexString  `json:"publisher"`
	Editor    flexString  `json:"editor"`
	Type      flexString  `json:"type"`
	Volume    flexString  `json:"volume"`
	Number    flexString  `json:"number"`
	Pages     flexString  `json:"pages"`
	ISSN      flexStrings `json:"issn"`
	ISBN      flexStrings `json:"isbn"`
	PDFURL    flexString  `json:"pdf_url"`
	HTMLURL   flexString  `json:"html_url"`
	Comments  flexString  `json:"comments"`

	// torrent.html
	MagnetLink flexString `json:"magnetlink"`
	Seed       flexInt    `json:"seed"`
	Leech      flexInt    `json:"leech"`

	// map.html. geojson is deliberately not decoded: a city boundary polygon
	// runs to thousands of coordinates and would swamp an agent's context.
	Latitude    flexFloat             `json:"latitude"`
	Longitude   flexFloat             `json:"longitude"`
	BoundingBox flexFloats            `json:"boundingbox"`
	OSM         *searxOSM             `json:"osm"`
	Address     map[string]flexString `json:"address"`
	Links       []searxMapLink        `json:"links"`
}

type searxOSM struct {
	Type flexString `json:"type"`
	ID   flexInt64  `json:"id"`
}

type searxMapLink struct {
	Label flexString `json:"label"`
	URL   flexString `json:"url"`
}

type searxLink struct {
	Title    flexString `json:"title"`
	URL      flexString `json:"url"`
	Official bool       `json:"official"`
}

type searxAnswer struct {
	Answer flexString `json:"answer"`
	URL    flexString `json:"url"`
	Engine flexString `json:"engine"`
}

type searxInfobox struct {
	Infobox    flexString        `json:"infobox"`
	ID         flexString        `json:"id"`
	Content    flexString        `json:"content"`
	ImgSrc     flexString        `json:"img_src"`
	Engine     flexString        `json:"engine"`
	Engines    flexStrings       `json:"engines"`
	URLs       []json.RawMessage `json:"urls"`
	Attributes []json.RawMessage `json:"attributes"`
}

type searxAttribute struct {
	Label  flexString      `json:"label"`
	Value  flexString      `json:"value"`
	Entity flexString      `json:"entity"`
	Image  json.RawMessage `json:"image"`
}

// decodedResults is the normalized content of one SearXNG response.
type decodedResults struct {
	results      []types.SearchResult
	available    int
	answers      []types.Answer
	infoboxes    []types.Infobox
	suggestions  []string
	corrections  []string
	unresponsive []types.UnresponsiveEngine
}

func decodeResponse(raw searxResponse, query string, limit int) decodedResults {
	all := make([]searxResult, 0, len(raw.Results))
	for _, item := range raw.Results {
		var result searxResult
		if err := json.Unmarshal(item, &result); err != nil {
			continue
		}
		all = append(all, result)
	}
	results := promotePackageMatches(normalizeResults(all, 0), query)
	available := len(results)
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return decodedResults{
		results:      results,
		available:    available,
		answers:      normalizeAnswers(raw.Answers),
		infoboxes:    normalizeInfoboxes(raw.Infoboxes),
		suggestions:  capStrings(uniqueTrimmed(raw.Suggestions), maxSuggestions),
		corrections:  capStrings(uniqueTrimmed(raw.Corrections), maxCorrections),
		unresponsive: normalizeUnresponsive(raw.UnresponsiveEngines),
	}
}

func normalizeResults(results []searxResult, limit int) []types.SearchResult {
	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}
	out := make([]types.SearchResult, 0, limit)
	for _, item := range results {
		if len(out) >= limit {
			break
		}
		if result, ok := normalizeResult(item); ok {
			out = append(out, result)
		}
	}
	return out
}

func normalizeResult(item searxResult) (types.SearchResult, bool) {
	link := string(item.URL)
	title := string(item.Title)
	if link == "" || title == "" {
		return types.SearchResult{}, false
	}
	domain := ""
	if parsed, err := urlpkg.Parse(link); err == nil {
		domain = strings.TrimSpace(parsed.Hostname())
	}
	engine := string(item.Engine)
	engines := []string(item.Engines)
	if len(engines) == 0 && engine != "" {
		engines = []string{engine}
	}
	kind := kindFromTemplate(string(item.Template))
	width, height := int(item.Width), int(item.Height)
	resolution := string(item.Resolution)
	if width == 0 && height == 0 {
		width, height = parseResolution(resolution)
	}

	result := types.SearchResult{
		Title:        title,
		URL:          link,
		Snippet:      string(item.Content),
		Kind:         kind,
		Category:     string(item.Category),
		Engine:       engine,
		Engines:      engines,
		Score:        roundScore(float64(item.Score)),
		Source:       string(item.Source),
		Domain:       domain,
		ThumbnailURL: firstNonEmpty(string(item.Thumbnail), string(item.ThumbnailSrc)),
		ContentURL:   firstNonEmpty(string(item.ContentURL), string(item.ImgSrc), string(item.Embedded)),
		EmbedURL:     string(item.IframeSrc),
		AudioURL:     string(item.AudioSrc),
		Width:        width,
		Height:       height,
		Resolution:   resolution,
		Format:       firstNonEmpty(string(item.ImgFormat), string(item.MimeType)),
		FileSize:     firstNonEmpty(string(item.FileSize), string(item.Size)),
		Duration:     formatDuration(firstNonEmpty(string(item.Duration), string(item.Length))),
		Views:        string(item.Views),
		PublishedAt:  string(item.PublishedAt),
		Channel:      string(item.Channel),
		Author:       string(item.Author),
		Tags:         []string(item.Tags),
	}

	switch kind {
	case "package":
		result.Package = packageInfo(item)
	case "paper":
		result.Paper = paperInfo(item)
	case "torrent":
		result.Torrent = &types.TorrentInfo{
			MagnetLink: string(item.MagnetLink),
			Seeders:    int(item.Seed),
			Leechers:   int(item.Leech),
		}
	case "map":
		result.Location = locationInfo(item)
	}
	return result, true
}

// Package name match tiers, best first. Results that are not packages, or
// packages that do not match, share packageNoMatch and keep SearXNG's order.
const (
	packageExactName = iota
	packageLastSegment
	packageContains
	packageNoMatch
)

// promotePackageMatches moves package results whose name matches the query
// ahead of the rest, before the limit is applied. SearXNG interleaves
// registries by engine, so for "tokio" a docker image can outrank the crate
// itself and the package an agent means can fall below the limit cut. The
// sort is stable: within a tier, and for every non-package result, SearXNG's
// own order is kept.
func promotePackageMatches(results []types.SearchResult, query string) []types.SearchResult {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return results
	}
	tiers := make([]int, len(results))
	promoted := false
	for i, result := range results {
		tiers[i] = packageMatchTier(result, query)
		if tiers[i] != packageNoMatch {
			promoted = true
		}
	}
	if !promoted {
		return results
	}
	order := make([]int, len(results))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return tiers[a] - tiers[b] })
	out := make([]types.SearchResult, len(results))
	for i, index := range order {
		out[i] = results[index]
	}
	return out
}

// packageMatchTier compares a lowercased query with a package name. The last
// path segment counts separately because registries name packages by path
// (pkg.go.dev reports golang.org/x/sync/errgroup, docker hub owner/image).
func packageMatchTier(result types.SearchResult, query string) int {
	if result.Kind != "package" {
		return packageNoMatch
	}
	name := result.Title
	if result.Package != nil && result.Package.Name != "" {
		name = result.Package.Name
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return packageNoMatch
	}
	if name == query {
		return packageExactName
	}
	if segment := name[strings.LastIndexAny(name, "/:")+1:]; segment == query {
		return packageLastSegment
	}
	if strings.Contains(name, query) {
		return packageContains
	}
	return packageNoMatch
}

// kindFromTemplate maps SearXNG's result template to a stable kind name.
// Unknown templates pass through without their ".html" suffix so new
// SearXNG result types still surface something meaningful.
func kindFromTemplate(template string) string {
	name := strings.TrimSuffix(strings.TrimSpace(template), ".html")
	switch name {
	case "", "default":
		return "web"
	case "images":
		return "image"
	case "videos":
		return "video"
	case "packages":
		return "package"
	default:
		return name
	}
}

func packageInfo(item searxResult) *types.PackageInfo {
	info := types.PackageInfo{
		Name:          string(item.PackageName),
		Version:       string(item.Version),
		Maintainer:    string(item.Maintainer),
		Popularity:    string(item.Popularity),
		Homepage:      string(item.Homepage),
		SourceCodeURL: string(item.SourceCodeURL),
		LicenseName:   string(item.LicenseName),
		LicenseURL:    string(item.LicenseURL),
	}
	if info == (types.PackageInfo{}) {
		return nil
	}
	return &info
}

func paperInfo(item searxResult) *types.PaperInfo {
	info := types.PaperInfo{
		DOI:       string(item.DOI),
		Authors:   []string(item.Authors),
		Journal:   string(item.Journal),
		Publisher: string(item.Publisher),
		Editor:    string(item.Editor),
		Type:      string(item.Type),
		Volume:    string(item.Volume),
		Number:    string(item.Number),
		Pages:     string(item.Pages),
		ISSN:      []string(item.ISSN),
		ISBN:      []string(item.ISBN),
		PDFURL:    string(item.PDFURL),
		HTMLURL:   string(item.HTMLURL),
		Comments:  string(item.Comments),
	}
	if info.DOI == "" && len(info.Authors) == 0 && info.Journal == "" && info.Publisher == "" &&
		info.Editor == "" && info.Type == "" && info.Volume == "" && info.Number == "" &&
		info.Pages == "" && len(info.ISSN) == 0 && len(info.ISBN) == 0 && info.PDFURL == "" &&
		info.HTMLURL == "" && info.Comments == "" {
		return nil
	}
	return &info
}

func locationInfo(item searxResult) *types.LocationInfo {
	info := &types.LocationInfo{
		Latitude:    float64(item.Latitude),
		Longitude:   float64(item.Longitude),
		BoundingBox: []float64(item.BoundingBox),
	}
	if item.OSM != nil {
		info.OSMType = string(item.OSM.Type)
		info.OSMID = int64(item.OSM.ID)
	}
	if len(item.Address) > 0 {
		address := make(map[string]string, len(item.Address))
		for key, value := range item.Address {
			if key = strings.TrimSpace(key); key != "" && value != "" {
				address[key] = string(value)
			}
		}
		if len(address) > 0 {
			info.Address = address
		}
	}
	for _, link := range item.Links {
		if link.URL == "" {
			continue
		}
		info.Links = append(info.Links, types.Link{Title: string(link.Label), URL: string(link.URL)})
	}
	return info
}

func normalizeAnswers(raw []json.RawMessage) []types.Answer {
	var out []types.Answer
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		if len(out) >= maxAnswers {
			break
		}
		var answer types.Answer
		// Older SearXNG releases emit answers as bare strings.
		if text := scalarText(item); text != "" {
			answer.Answer = text
		} else {
			var decoded searxAnswer
			if err := json.Unmarshal(item, &decoded); err != nil {
				continue
			}
			answer = types.Answer{
				Answer: string(decoded.Answer),
				URL:    string(decoded.URL),
				Engine: string(decoded.Engine),
			}
		}
		if answer.Answer == "" {
			continue
		}
		if _, ok := seen[answer.Answer]; ok {
			continue
		}
		seen[answer.Answer] = struct{}{}
		out = append(out, answer)
	}
	return out
}

func normalizeInfoboxes(raw []json.RawMessage) []types.Infobox {
	var out []types.Infobox
	for _, item := range raw {
		if len(out) >= maxInfoboxes {
			break
		}
		var decoded searxInfobox
		if err := json.Unmarshal(item, &decoded); err != nil {
			continue
		}
		box := types.Infobox{
			Title:    string(decoded.Infobox),
			ID:       string(decoded.ID),
			Content:  string(decoded.Content),
			ImageURL: string(decoded.ImgSrc),
			Engine:   string(decoded.Engine),
			Engines:  []string(decoded.Engines),
		}
		if box.Title == "" {
			continue
		}
		for _, rawURL := range decoded.URLs {
			if len(box.URLs) >= maxInfoboxURLs {
				break
			}
			var link searxLink
			if err := json.Unmarshal(rawURL, &link); err != nil || link.URL == "" {
				continue
			}
			box.URLs = append(box.URLs, types.Link{
				Title:    string(link.Title),
				URL:      string(link.URL),
				Official: link.Official,
			})
		}
		for _, rawAttr := range decoded.Attributes {
			if len(box.Attributes) >= maxInfoboxAttributes {
				break
			}
			var attr searxAttribute
			if err := json.Unmarshal(rawAttr, &attr); err != nil || attr.Label == "" {
				continue
			}
			entry := types.InfoboxAttribute{
				Label:    string(attr.Label),
				Value:    string(attr.Value),
				ImageURL: attributeImage(attr.Image),
				Entity:   string(attr.Entity),
			}
			if entry.Value == "" && entry.ImageURL == "" {
				continue
			}
			box.Attributes = append(box.Attributes, entry)
		}
		// Wikidata sometimes emits a bare card titled with its Q-ID and no
		// facts; it tells an agent nothing the results do not.
		if box.Content == "" && len(box.Attributes) == 0 {
			continue
		}
		out = append(out, box)
	}
	return out
}

// attributeImage reads an infobox attribute image given either as a URL
// string or as an object with a "src" key.
func attributeImage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if text := scalarText(raw); text != "" {
		return text
	}
	var image struct {
		Src flexString `json:"src"`
	}
	if err := json.Unmarshal(raw, &image); err != nil {
		return ""
	}
	return string(image.Src)
}

// normalizeUnresponsive reads SearXNG's [[engine, reason], ...] pairs.
func normalizeUnresponsive(raw []json.RawMessage) []types.UnresponsiveEngine {
	var out []types.UnresponsiveEngine
	seen := make(map[string]struct{}, len(raw))
	for _, item := range raw {
		if len(out) >= maxUnresponsiveReport {
			break
		}
		if trimmed := bytes.TrimSpace(item); len(trimmed) == 0 || trimmed[0] != '[' {
			continue
		}
		var pair flexStrings
		if err := json.Unmarshal(item, &pair); err != nil || len(pair) == 0 {
			continue
		}
		name := pair[0]
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		entry := types.UnresponsiveEngine{Engine: name}
		if len(pair) > 1 {
			entry.Reason = pair[1]
		}
		out = append(out, entry)
	}
	return out
}

var resolutionPattern = regexp.MustCompile(`^\s*(\d+)\s*[x×X]\s*(\d+)\s*$`)

// parseResolution reads "2464 x 1643" style image resolutions.
func parseResolution(text string) (int, int) {
	match := resolutionPattern.FindStringSubmatch(text)
	if match == nil {
		return 0, 0
	}
	width, errW := strconv.Atoi(match[1])
	height, errH := strconv.Atoi(match[2])
	if errW != nil || errH != nil {
		return 0, 0
	}
	return width, height
}

// formatDuration renders a bare seconds count as h:mm:ss / m:ss; anything
// already formatted ("08:47", "PT3M") passes through unchanged.
func formatDuration(text string) string {
	if text == "" {
		return ""
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || value < 0 {
		return text
	}
	total := int(math.Round(value))
	hours, minutes, seconds := total/3600, (total%3600)/60, total%60
	if hours > 0 {
		return strconv.Itoa(hours) + ":" + pad2(minutes) + ":" + pad2(seconds)
	}
	return strconv.Itoa(minutes) + ":" + pad2(seconds)
}

func pad2(value int) string {
	if value < 10 {
		return "0" + strconv.Itoa(value)
	}
	return strconv.Itoa(value)
}

// roundScore trims SearXNG's float noise (0.047619047619047616) to four
// decimals; scores only order results within a response.
func roundScore(score float64) float64 {
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0
	}
	return math.Round(score*10000) / 10000
}

func uniqueTrimmed(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func capStrings(values []string, limit int) []string {
	if limit > 0 && len(values) > limit {
		return values[:limit]
	}
	return values
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
