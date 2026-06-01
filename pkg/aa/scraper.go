package aa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const pageURL = "https://artificialanalysis.ai/leaderboards/models"

var (
	errModelsNotFound   = errors.New("models not found in RSC payload")
	errMarkerNotFound   = errors.New("marker not found")
	errUnmatchedBracket = errors.New("unmatched bracket")
	errNilResponse      = errors.New("nil HTTP response")
)

// Scraper fetches model data from artificialanalysis.ai.
type Scraper struct {
	client *http.Client
	url    string
}

// NewScraper returns a new Scraper ready to use.
func NewScraper() *Scraper {
	return &Scraper{client: http.DefaultClient, url: pageURL}
}

// Scrape fetches and parses the current AI model list from artificialanalysis.ai.
func (s *Scraper) Scrape(ctx context.Context) ([]Model, error) {
	body, err := s.fetchPage(ctx)
	if err != nil {
		return nil, err
	}

	chunks := extractRSCChunks(string(body))
	for _, chunk := range chunks {
		if !strings.Contains(chunk, `"models":[{"id"`) ||
			!strings.Contains(chunk, `"intelligenceIndex"`) {
			continue
		}

		raw, extractErr := extractJSONArray(chunk, `"models":[`)
		if extractErr != nil {
			continue
		}

		raw = bytes.ReplaceAll(raw, []byte(`"$undefined"`), []byte("null"))

		var models []Model

		err = json.Unmarshal(raw, &models)
		if err != nil {
			return nil, fmt.Errorf("parse models: %w", err)
		}

		return models, nil
	}

	return nil, errModelsNotFound
}

func (s *Scraper) fetchPage(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	if resp == nil {
		return nil, errNilResponse
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return body, nil
}

// extractRSCChunks finds all self.__next_f.push([1,"..."]) calls in the HTML
// and returns the unquoted string content of each.
func extractRSCChunks(html string) []string {
	const needle = `self.__next_f.push([1,"`

	var chunks []string

	offset := 0
	for {
		idx := strings.Index(html[offset:], needle)
		if idx == -1 {
			break
		}

		// start is the position of the opening " of the JSON string.
		start := offset + idx + len(needle) - 1
		end := findChunkBound(html, start+1)

		var s string

		err := json.Unmarshal([]byte(html[start:end+1]), &s)
		if err == nil {
			chunks = append(chunks, s)
		}

		offset = end + 1
	}

	return chunks
}

// findChunkBound scans forward from start to find the closing " of a JSON
// string, skipping backslash-escaped characters. Returns the index of the
// closing quote, or len(html)-1 if not found.
func findChunkBound(html string, start int) int {
	for i := start; i < len(html); i++ {
		if html[i] == '\\' {
			i++ // skip escaped character

			continue
		}

		if html[i] == '"' {
			return i
		}
	}

	return len(html) - 1
}

// extractJSONArray finds startMarker in s and returns the JSON array starting
// at the '[' of the marker, respecting nested brackets and quoted strings.
func extractJSONArray(s, startMarker string) ([]byte, error) {
	idx := strings.Index(s, startMarker)
	if idx == -1 {
		return nil, fmt.Errorf("%q: %w", startMarker, errMarkerNotFound)
	}

	bracketPos := idx + len(startMarker) - 1 // position of '['

	end, err := scanBracketEnd(s, bracketPos)
	if err != nil {
		return nil, fmt.Errorf("%q: %w", startMarker, errUnmatchedBracket)
	}

	return []byte(s[bracketPos : end+1]), nil
}

// bracketScanner tracks state while scanning a JSON array for its closing bracket.
type bracketScanner struct {
	depth   int
	inStr   bool
	escaped bool
}

// next processes one byte and reports whether the closing bracket was found.
func (sc *bracketScanner) next(c byte) bool {
	if sc.escaped {
		sc.escaped = false

		return false
	}

	if c == '\\' && sc.inStr {
		sc.escaped = true

		return false
	}

	if c == '"' {
		sc.inStr = !sc.inStr

		return false
	}

	if sc.inStr {
		return false
	}

	switch c {
	case '[':
		sc.depth++
	case ']':
		sc.depth--
		if sc.depth == 0 {
			return true
		}
	default:
	}

	return false
}

// scanBracketEnd finds the index of the closing ']' matching the '[' at start,
// respecting nested brackets and quoted strings.
func scanBracketEnd(s string, start int) (int, error) {
	sc := bracketScanner{}

	for i := start; i < len(s); i++ {
		if sc.next(s[i]) {
			return i, nil
		}
	}

	return -1, errUnmatchedBracket
}
