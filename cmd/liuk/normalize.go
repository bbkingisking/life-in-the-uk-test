package main

import (
	"fmt"

	"github.com/DHKLeung/life-in-the-uk-test/internal/examsjson"
)

// runNormalize re-derives dist/exams.json's per-question uid and default
// category in place - see internal/examsjson's package doc. Run this after
// hand-editing dist/exams.json (adding a question, fixing wording).
func runNormalize(args []string) error {
	examCount, uncategorized, err := examsjson.NormalizeFile("dist/exams.json")
	if err != nil {
		return err
	}
	if len(uncategorized) > 0 {
		fmt.Printf("Warning: %d question(s) with no category, tagged 'uncategorized'.\n", len(uncategorized))
	}
	fmt.Printf("Normalized dist/exams.json (%d exams).\n", examCount)
	return nil
}
