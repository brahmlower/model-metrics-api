package aa

import (
	"errors"
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
