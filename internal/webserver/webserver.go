// Package webserver is a minimal static file server for local previewing.
package webserver

import (
	"net/http"
	"path/filepath"
	"strings"
)

// mimeTypes overrides Go's extension-based content-type sniffing for the
// types this project actually serves, so results are stable across
// platforms (in particular .md, which mime.TypeByExtension leaves
// unregistered on some systems, and .json, whose registered type varies).
var mimeTypes = map[string]string{
	".html": "text/html; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".json": "application/json; charset=utf-8",
	".md":   "text/markdown; charset=utf-8",
	".csv":  "text/csv; charset=utf-8",
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".ico":  "image/x-icon",
}

// NewHandler serves files under root. Requests for "/" serve index.html;
// anything resolving outside root (via "..") is rejected.
func NewHandler(root string) http.Handler {
	fs := http.Dir(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlPath := r.URL.Path
		if urlPath == "/" {
			urlPath = "/index.html"
		}

		f, err := fs.Open(urlPath)
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil || info.IsDir() {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		ext := strings.ToLower(filepath.Ext(urlPath))
		contentType := mimeTypes[ext]
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)
		http.ServeContent(w, r, urlPath, info.ModTime(), f)
	})
}
