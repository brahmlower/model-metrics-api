package aa

import (
	"encoding/json"
	"os"
	"testing"
)

func TestModelRoundtrip(t *testing.T) {
	t.Parallel()

	f64 := func(v float64) *float64 { return &v }
	s := func(v string) *string { return &v }
	m := Model{
		ID:                  "test-id",
		Name:                "Test Model",
		Slug:                "test-model",
		ShortName:           "Test",
		ModelCreatorName:    "Test Corp",
		ModelCreatorSlug:    "test-corp",
		ModelCreatorID:      "creator-id",
		ModelCreatorColor:   "#000",
		ModelCreatorCountry: "us",
		ModelCreatorLogo:    "test.svg",
		IntelligenceIndex:   f64(42.5),
		CodingIndex:         f64(10.0),
		Price1MInputTokens:  f64(1.0),
		Price1MOutputTokens: f64(3.0),
		PriceClass:          s("medium"),
	}

	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	var m2 Model

	err = json.Unmarshal(b, &m2)
	if err != nil {
		t.Fatal(err)
	}

	if m2.ID != m.ID {
		t.Errorf("ID mismatch: %s vs %s", m2.ID, m.ID)
	}

	if m2.Name != m.Name {
		t.Error("Name mismatch")
	}

	if m2.IntelligenceIndex == nil || *m2.IntelligenceIndex != *m.IntelligenceIndex {
		t.Error("IntelligenceIndex mismatch")
	}
}

func TestModelFromJSON(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/model_sample.json")
	if err != nil {
		t.Fatal(err)
	}

	var m Model

	err = json.Unmarshal(data, &m)
	if err != nil {
		t.Fatal(err)
	}

	if m.ID == "" {
		t.Error("empty ID")
	}

	if m.Name == "" {
		t.Error("empty Name")
	}

	if m.Slug == "" {
		t.Error("empty Slug")
	}
}
