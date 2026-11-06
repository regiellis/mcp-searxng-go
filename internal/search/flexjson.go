package search

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// SearXNG aggregates many engines, and their result fields are not uniformly
// typed: the same key arrives as a string from one engine, a number from
// another, and null or "" from a third (e.g. "seed": "51", "views": null,
// "length": "08:47" or 228). The flex* types below decode whatever shape
// arrives into one Go type and never fail, so a single odd field cannot sink
// a whole search response. Values of an unexpected shape decode to the zero
// value.

// flexString decodes a JSON string, number, or bool into a trimmed string.
type flexString string

func (f *flexString) UnmarshalJSON(data []byte) error {
	*f = flexString(scalarText(data))
	return nil
}

// flexInt decodes a JSON number or numeric string into an int.
type flexInt int

func (f *flexInt) UnmarshalJSON(data []byte) error {
	value, ok := parseNumber(scalarText(data))
	if ok {
		*f = flexInt(int(value))
	} else {
		*f = 0
	}
	return nil
}

// flexInt64 decodes a JSON number or numeric string into an int64.
type flexInt64 int64

func (f *flexInt64) UnmarshalJSON(data []byte) error {
	text := scalarText(data)
	if parsed, err := strconv.ParseInt(text, 10, 64); err == nil {
		*f = flexInt64(parsed)
		return nil
	}
	value, ok := parseNumber(text)
	if ok {
		*f = flexInt64(int64(value))
	} else {
		*f = 0
	}
	return nil
}

// flexFloat decodes a JSON number or numeric string into a float64.
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(data []byte) error {
	value, ok := parseNumber(scalarText(data))
	if ok {
		*f = flexFloat(value)
	} else {
		*f = 0
	}
	return nil
}

// flexStrings decodes a JSON array of scalars, or a single scalar, into a
// list of non-empty trimmed strings.
type flexStrings []string

func (f *flexStrings) UnmarshalJSON(data []byte) error {
	*f = nil
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil
	}
	if data[0] != '[' {
		if text := scalarText(data); text != "" {
			*f = flexStrings{text}
		}
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil
	}
	out := make(flexStrings, 0, len(items))
	for _, item := range items {
		if text := scalarText(item); text != "" {
			out = append(out, text)
		}
	}
	if len(out) > 0 {
		*f = out
	}
	return nil
}

// flexFloats decodes a JSON array of numbers or numeric strings.
type flexFloats []float64

func (f *flexFloats) UnmarshalJSON(data []byte) error {
	*f = nil
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil
	}
	out := make(flexFloats, 0, len(items))
	for _, item := range items {
		value, ok := parseNumber(scalarText(item))
		if !ok {
			// A partially numeric box is meaningless; drop it whole.
			return nil
		}
		out = append(out, value)
	}
	if len(out) > 0 {
		*f = out
	}
	return nil
}

// scalarText renders a JSON scalar as trimmed text. Strings are unquoted,
// numbers and bools keep their literal form, and null, objects, arrays, or
// malformed input yield "".
func scalarText(data []byte) string {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return ""
	}
	switch data[0] {
	case '"':
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return ""
		}
		return strings.TrimSpace(text)
	case 't', 'f':
		var flag bool
		if err := json.Unmarshal(data, &flag); err != nil {
			return ""
		}
		return strconv.FormatBool(flag)
	case 'n', '{', '[':
		return ""
	default:
		var number json.Number
		if err := json.Unmarshal(data, &number); err != nil {
			return ""
		}
		return number.String()
	}
}

func parseNumber(text string) (float64, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}
