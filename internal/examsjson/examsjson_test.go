package examsjson

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DHKLeung/life-in-the-uk-test/internal/model"
)

func TestComputeUIDIsStableAndContentDerived(t *testing.T) {
	a := ComputeUID("What is the capital?")
	b := ComputeUID("What is the capital?")
	c := ComputeUID("A different question?")
	if a != b {
		t.Errorf("same text produced different uids: %q vs %q", a, b)
	}
	if a == c {
		t.Errorf("different text produced the same uid: %q", a)
	}
}

func TestAnnotateCategoriesKeepsExistingCategory(t *testing.T) {
	data := map[string][]model.Question{
		"1": {{ID: 1, Question: "Known question", Category: "uk-geography"}},
	}
	uncategorized := AnnotateCategories(data)
	if len(uncategorized) != 0 {
		t.Errorf("unexpected uncategorized: %v", uncategorized)
	}
	if data["1"][0].Category != "uk-geography" {
		t.Errorf("got category %q, want uk-geography", data["1"][0].Category)
	}
	if data["1"][0].UID == "" {
		t.Error("expected a uid to be set")
	}
}

func TestAnnotateCategoriesFallsBackToUncategorized(t *testing.T) {
	data := map[string][]model.Question{
		"1": {{ID: 1, Question: "Mystery question"}},
	}
	uncategorized := AnnotateCategories(data)
	if data["1"][0].Category != "uncategorized" {
		t.Errorf("got category %q, want uncategorized", data["1"][0].Category)
	}
	if len(uncategorized) != 1 || uncategorized[0] != "Mystery question" {
		t.Errorf("unexpected uncategorized: %v", uncategorized)
	}
}

func TestNormalizeFileRewritesCategoriesAndUIDs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exams.json")
	writeJSON(t, path, map[string]any{
		"exams": map[string][]model.Question{
			"1": {{ID: 1, Question: "Question 1", Reference: "Ref 1", Category: "values-principles",
				Answers: []model.Answer{{Text: "Ans 1", IsCorrect: true}}}},
		},
	})

	examCount, uncategorized, err := NormalizeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if examCount != 1 {
		t.Errorf("got examCount %d, want 1", examCount)
	}
	if len(uncategorized) != 0 {
		t.Errorf("unexpected uncategorized: %v", uncategorized)
	}

	var out struct {
		Categories map[string]string           `json:"categories"`
		Exams      map[string][]model.Question `json:"exams"`
	}
	readJSON(t, path, &out)

	if len(out.Categories) == 0 {
		t.Error("expected categories to be populated")
	}
	q := out.Exams["1"][0]
	if q.Question != "Question 1" || q.Answers[0].Text != "Ans 1" || !q.Answers[0].IsCorrect {
		t.Errorf("unexpected question: %+v", q)
	}
	if q.UID == "" {
		t.Error("expected a uid to be set")
	}
	if q.Category != "values-principles" {
		t.Errorf("got category %q", q.Category)
	}
}

func TestNormalizeFileTagsMissingCategoryAsUncategorized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exams.json")
	writeJSON(t, path, map[string]any{
		"exams": map[string][]model.Question{
			"1": {{ID: 1, Question: "Question 1"}},
		},
	})

	_, uncategorized, err := NormalizeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(uncategorized) != 1 {
		t.Fatalf("got %d uncategorized, want 1", len(uncategorized))
	}

	var out struct {
		Exams map[string][]model.Question `json:"exams"`
	}
	readJSON(t, path, &out)
	if out.Exams["1"][0].Category != "uncategorized" {
		t.Errorf("got category %q, want uncategorized", out.Exams["1"][0].Category)
	}
}

func TestNormalizeOrdersExamsNumerically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exams.json")
	writeJSON(t, path, map[string]any{
		"exams": map[string][]model.Question{
			"10": {{ID: 1, Question: "Q10"}},
			"2":  {{ID: 1, Question: "Q2"}},
		},
	})

	data, _, _, err := Normalize(path)
	if err != nil {
		t.Fatal(err)
	}
	i2 := indexOf(t, data, `"2"`)
	i10 := indexOf(t, data, `"10"`)
	if i2 > i10 {
		t.Errorf(`expected "2" to appear before "10" in %s`, data)
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func indexOf(t *testing.T, data []byte, substr string) int {
	t.Helper()
	for i := 0; i+len(substr) <= len(data); i++ {
		if string(data[i:i+len(substr)]) == substr {
			return i
		}
	}
	t.Fatalf("substring %q not found in %s", substr, data)
	return -1
}
