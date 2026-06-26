package feed

import "testing"

func TestParseRSS(t *testing.T) {
	t.Parallel()
	doc := []byte(`<?xml version="1.0"?>
<rss version="2.0">
  <channel>
    <title>Example News</title>
    <link>https://example.com</link>
    <item>
      <title>First Post</title>
      <link>https://example.com/1</link>
      <pubDate>Mon, 02 Jan 2006 15:04:05 MST</pubDate>
      <description>Summary with &lt;b&gt;markup&lt;/b&gt; inside.</description>
      <guid>https://example.com/1</guid>
    </item>
    <item>
      <title>Second Post</title>
      <link>https://example.com/2</link>
    </item>
  </channel>
</rss>`)
	parsed, err := Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Format != "rss" || parsed.Title != "Example News" || parsed.Link != "https://example.com" {
		t.Fatalf("unexpected feed header: %#v", parsed)
	}
	if len(parsed.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(parsed.Items))
	}
	if parsed.Items[0].Title != "First Post" || parsed.Items[0].Link != "https://example.com/1" {
		t.Fatalf("unexpected first item: %#v", parsed.Items[0])
	}
	if parsed.Items[0].Summary != "Summary with markup inside." {
		t.Fatalf("HTML not stripped from summary: %q", parsed.Items[0].Summary)
	}
}

func TestParseAtom(t *testing.T) {
	t.Parallel()
	doc := []byte(`<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Example Atom</title>
  <link href="https://example.com" rel="alternate"/>
  <entry>
    <title>Atom Entry</title>
    <link href="https://example.com/a" rel="alternate"/>
    <link href="https://example.com/a/edit" rel="edit"/>
    <updated>2006-01-02T15:04:05Z</updated>
    <id>urn:uuid:1</id>
    <summary>An atom summary.</summary>
  </entry>
</feed>`)
	parsed, err := Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Format != "atom" || parsed.Title != "Example Atom" || parsed.Link != "https://example.com" {
		t.Fatalf("unexpected feed header: %#v", parsed)
	}
	if len(parsed.Items) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(parsed.Items))
	}
	item := parsed.Items[0]
	if item.Title != "Atom Entry" || item.Link != "https://example.com/a" {
		t.Fatalf("alternate link not preferred: %#v", item)
	}
	if item.Published != "2006-01-02T15:04:05Z" || item.ID != "urn:uuid:1" {
		t.Fatalf("unexpected published/id: %#v", item)
	}
}

func TestParseRejectsNonFeed(t *testing.T) {
	t.Parallel()
	if _, err := Parse([]byte(`<html><body>not a feed</body></html>`)); err == nil {
		t.Fatal("expected error for non-feed document")
	}
	if _, err := Parse([]byte(`totally not xml`)); err == nil {
		t.Fatal("expected error for non-xml input")
	}
}
