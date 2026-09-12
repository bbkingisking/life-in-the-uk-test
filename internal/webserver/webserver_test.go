package webserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setupRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>home</h1>"), 0o644)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# guide"), 0o644)
	os.WriteFile(filepath.Join(dir, "exams.json"), []byte("{}"), 0o644)
	return dir
}

func TestServesIndexAtRoot(t *testing.T) {
	h := NewHandler(setupRoot(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("got content-type %q", ct)
	}
	if rec.Body.String() != "<h1>home</h1>" {
		t.Errorf("got body %q", rec.Body.String())
	}
}

func TestSetsMimeTypeByExtension(t *testing.T) {
	h := NewHandler(setupRoot(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readme.md", nil))

	if ct := rec.Header().Get("Content-Type"); ct != "text/markdown; charset=utf-8" {
		t.Errorf("got content-type %q, want text/markdown", ct)
	}
}

func TestReturnsNotFoundForMissingFile(t *testing.T) {
	h := NewHandler(setupRoot(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope.html", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404", rec.Code)
	}
}

func TestRejectsPathTraversalOutsideRoot(t *testing.T) {
	root := setupRoot(t)
	// A secret file that exists next to, but not inside, root.
	os.WriteFile(filepath.Join(filepath.Dir(root), "secret.txt"), []byte("hush"), 0o644)

	h := NewHandler(root)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/../secret.txt", nil))

	if rec.Code == http.StatusOK {
		t.Fatalf("path traversal served a file outside root: %q", rec.Body.String())
	}
}
