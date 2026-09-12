package jsonutil

import "testing"

func TestMarshalDoesNotEscapeHTMLCharacters(t *testing.T) {
	got, err := Marshal("Arts, Culture, Sport & Science <tag>")
	if err != nil {
		t.Fatal(err)
	}
	want := `"Arts, Culture, Sport & Science <tag>"`
	if string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestMarshalIndentHasNoTrailingNewline(t *testing.T) {
	got, err := MarshalIndent(map[string]int{"a": 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[len(got)-1] == '\n' {
		t.Errorf("expected no trailing newline, got %q", got)
	}
}

func TestMarshalIndentUsesTwoSpaceIndent(t *testing.T) {
	got, err := MarshalIndent(map[string]int{"a": 1})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": 1\n}"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
