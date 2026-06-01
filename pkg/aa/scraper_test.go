package aa

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractRSCChunks(t *testing.T) {
	t.Parallel()

	html := `<script>self.__next_f.push([1,"hello world"])</script><script>self.__next_f.push([1,"second"])</script>`

	chunks := extractRSCChunks(html)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}

	if chunks[0] != "hello world" {
		t.Errorf("chunk[0] = %q", chunks[0])
	}

	if chunks[1] != "second" {
		t.Errorf("chunk[1] = %q", chunks[1])
	}
}

func TestExtractJSONArray(t *testing.T) {
	t.Parallel()

	s := `prefix"models":[{"id":"a"},{"id":"b"}]suffix`

	raw, err := extractJSONArray(s, `"models":[`)
	if err != nil {
		t.Fatal(err)
	}

	if string(raw) != `[{"id":"a"},{"id":"b"}]` {
		t.Errorf("got %s", raw)
	}
}

func TestExtractJSONArrayMissingMarker(t *testing.T) {
	t.Parallel()

	_, err := extractJSONArray("no marker here", `"models":[`)
	if !errors.Is(err, errMarkerNotFound) {
		t.Errorf("expected errMarkerNotFound, got %v", err)
	}
}

func TestScanBracketEnd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		start int
		want  int
	}{
		{`[1,2,3]`, 0, 6},
		{`[[1],[2]]`, 0, 8},
		{`["he]llo"]`, 0, 9},
		{`[{"k":"v"}]`, 0, 10},
	}
	for _, tt := range tests {
		got, err := scanBracketEnd(tt.input, tt.start)
		if err != nil {
			t.Errorf("input=%q err=%v", tt.input, err)

			continue
		}

		if got != tt.want {
			t.Errorf("input=%q got=%d want=%d", tt.input, got, tt.want)
		}
	}
}

func TestFindChunkBound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		start int
		want  int
	}{
		{"normal", `hello"`, 0, 5},
		{"escaped quote", `hel\"lo"`, 0, 7},
		{"no closing quote returns last index", `hello`, 0, 4},
		{"empty string", ``, 0, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := findChunkBound(tt.input, tt.start)
			if got != tt.want {
				t.Errorf("findChunkBound(%q, %d) = %d, want %d", tt.input, tt.start, got, tt.want)
			}
		})
	}
}

func TestScrape(t *testing.T) {
	t.Parallel()

	// Hardcode the chunk with "id" first to match the real site's RSC field order,
	// which is what the "models":[{"id" content check expects.
	const chunk = `RSC data "models":[{"id":"m1","slug":"test-model","name":"Test Model","modelCreatorName":"Acme","intelligenceIndex":0.95}] end`

	// JSON-encode the chunk so it can be embedded as a JS string literal.
	encodedChunk, err := json.Marshal(chunk)
	if err != nil {
		t.Fatal(err)
	}

	html := fmt.Sprintf(`<html><script>self.__next_f.push([1,%s])</script></html>`, string(encodedChunk))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, html)
	}))
	defer ts.Close()

	s := &Scraper{client: ts.Client(), url: ts.URL}

	models, err := s.Scrape(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if len(models) != 1 {
		t.Fatalf("got %d models, want 1", len(models))
	}

	if models[0].Slug != "test-model" {
		t.Errorf("slug = %s, want test-model", models[0].Slug)
	}

	if models[0].IntelligenceIndex == nil || *models[0].IntelligenceIndex != 0.95 {
		t.Errorf("intelligenceIndex = %v, want 0.95", models[0].IntelligenceIndex)
	}
}

func TestScrapeModelsNotFound(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<html><body>no RSC chunks here</body></html>`)
	}))
	defer ts.Close()

	s := &Scraper{client: ts.Client(), url: ts.URL}

	_, err := s.Scrape(t.Context())
	if !errors.Is(err, errModelsNotFound) {
		t.Errorf("expected errModelsNotFound, got %v", err)
	}
}

func TestBracketScannerNext(t *testing.T) {
	t.Parallel()

	sc := bracketScanner{}
	for _, c := range []byte(`[{"k":"v["}]`) {
		if sc.next(c) {
			t.Logf("closed at %q", c)
		}
	}

	sc2 := bracketScanner{}
	input := `[1]`

	var closed bool

	for _, c := range []byte(input) {
		if sc2.next(c) {
			closed = true
		}
	}

	if !closed {
		t.Error("expected bracket to close")
	}
}
