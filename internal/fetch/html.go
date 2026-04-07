package fetch

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

// ExtractHTMLText returns a best-effort title and readable text.
func ExtractHTMLText(r io.Reader, maxChars int) (string, string, bool, error) {
	tokenizer := html.NewTokenizer(r)
	var title string
	var builder strings.Builder
	var inTitle bool
	var skipDepth int
	truncated := false

	writeText := func(text string) {
		text = strings.TrimSpace(text)
		if text == "" || truncated {
			return
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(text)
		if builder.Len() > maxChars {
			text := builder.String()
			builder.Reset()
			builder.WriteString(text[:maxChars])
			truncated = true
		}
	}

	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			if tokenizer.Err() == io.EOF {
				return strings.TrimSpace(title), strings.TrimSpace(builder.String()), truncated, nil
			}
			return "", "", truncated, tokenizer.Err()
		case html.StartTagToken:
			name, _ := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if tag == "script" || tag == "style" || tag == "noscript" {
				skipDepth++
			}
			if tag == "title" {
				inTitle = true
			}
		case html.EndTagToken:
			name, _ := tokenizer.TagName()
			tag := strings.ToLower(string(name))
			if tag == "script" || tag == "style" || tag == "noscript" {
				if skipDepth > 0 {
					skipDepth--
				}
			}
			if tag == "title" {
				inTitle = false
			}
		case html.TextToken:
			if skipDepth > 0 {
				continue
			}
			text := string(tokenizer.Text())
			if inTitle {
				title = strings.TrimSpace(title + " " + text)
				continue
			}
			writeText(text)
		}
	}
}
