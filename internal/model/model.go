// Package model holds the question/answer shapes used by dist/exams.json,
// the site's one hand-edited data file, and by internal/examsjson, which
// normalizes it in place.
package model

// Answer is one multiple-choice option for a Question.
type Answer struct {
	Text      string `json:"text"`
	IsCorrect bool   `json:"isCorrect"`
}

// Question is a single exam question. UID and Category are filled in by
// internal/examsjson's normalization pass if left blank by hand - see its
// package doc.
type Question struct {
	ID        int      `json:"id"`
	Question  string   `json:"question"`
	Reference string   `json:"reference"`
	Answers   []Answer `json:"answers"`
	UID       string   `json:"uid,omitempty"`
	Category  string   `json:"category,omitempty"`
}
