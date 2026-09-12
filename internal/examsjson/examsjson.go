// Package examsjson normalizes dist/exams.json in place. exams.json is
// hand-edited directly (question wording, answers, category) - this fills
// in the two fields that would otherwise have to be computed by hand: a
// stable uid (used by the front end as the browser persistence key,
// re-derived from the question's current text) and a default category
// ("uncategorized") for any question that doesn't have one set yet.
package examsjson

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/DHKLeung/life-in-the-uk-test/internal/jsonutil"
	"github.com/DHKLeung/life-in-the-uk-test/internal/model"
	"github.com/DHKLeung/life-in-the-uk-test/internal/ordered"
)

// categorySlug pairs a topic category's slug with its display name.
type categorySlug struct {
	slug, name string
}

// categoryMeta lists the topic categories used by the "drill by category"
// feature, in the order the front end shows their buttons in. Slugs must
// match the values used in exams.json questions' "category" field.
var categoryMeta = []categorySlug{
	{"values-principles", "Values, Principles & Being a Citizen"},
	{"uk-geography", "The UK: Geography, Nations & Symbols"},
	{"history", "British History"},
	{"government-law", "Government, Law & Civic Life"},
	{"religion-traditions", "Religion, Customs & Traditions"},
	{"arts-culture-sport", "Arts, Culture, Sport & Science"},
	{"uncategorized", "Uncategorized"},
}

// ComputeUID derives a stable id for a question from its text, so the same
// question keeps the same id across normalizations and across the multiple
// exams it may appear verbatim in. Used as the persistence key for a
// user's browser-local progress data.
func ComputeUID(questionText string) string {
	sum := sha1.Sum([]byte(questionText))
	return hex.EncodeToString(sum[:])[:10]
}

// file is exams.json's shape on disk.
type file struct {
	Categories *ordered.Map[string]           `json:"categories"`
	Exams      *ordered.Map[[]model.Question] `json:"exams"`
}

// input is the subset of exams.json's shape this package reads back in -
// only "exams" matters; "categories" is always rebuilt fresh from
// categoryMeta above, the authoritative list.
type input struct {
	Exams map[string][]model.Question `json:"exams"`
}

// AnnotateCategories tags every question in place with a stable uid, and
// defaults an empty Category to "uncategorized". Returns the questions
// left uncategorized, for the caller to warn about.
func AnnotateCategories(questionsByExam map[string][]model.Question) []string {
	var uncategorized []string
	for exam, questions := range questionsByExam {
		for i := range questions {
			q := &questions[i]
			q.UID = ComputeUID(q.Question)
			if q.Category == "" {
				q.Category = "uncategorized"
				uncategorized = append(uncategorized, q.Question)
			}
		}
		questionsByExam[exam] = questions
	}
	return uncategorized
}

// Normalize re-reads exams.json from path, re-derives every question's uid
// and default category (see AnnotateCategories), and rebuilds the
// "categories" legend from categoryMeta. It also returns the number of
// exams and the questions left uncategorized, for the caller to report.
func Normalize(path string) (data []byte, examCount int, uncategorized []string, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, nil, err
	}
	var in input
	if err := json.Unmarshal(b, &in); err != nil {
		return nil, 0, nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	uncategorized = AnnotateCategories(in.Exams)

	categoriesOut := ordered.NewMap[string]()
	for _, c := range categoryMeta {
		categoriesOut.Set(c.slug, c.name)
	}

	f := file{
		Categories: categoriesOut,
		Exams:      ordered.FromMapNumeric(in.Exams),
	}

	data, err = jsonutil.MarshalIndent(f)
	if err != nil {
		return nil, 0, nil, err
	}
	return data, len(in.Exams), uncategorized, nil
}

// NormalizeFile is Normalize followed by writing the result back to path.
func NormalizeFile(path string) (examCount int, uncategorized []string, err error) {
	data, examCount, uncategorized, err := Normalize(path)
	if err != nil {
		return 0, nil, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return 0, nil, err
	}
	return examCount, uncategorized, nil
}
