package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/DHKLeung/life-in-the-uk-test/internal/webserver"
)

// runServe serves dist/ (the site) over HTTP for local previewing. Run from
// the repo root, same as every other liuk command.
// Usage: liuk serve [port]  (defaults to 8000, or $PORT)
func runServe(args []string) error {
	port := 8000
	if p := os.Getenv("PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}
	if len(args) > 0 {
		n, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid port %q", args[0])
		}
		port = n
	}

	root := "dist"
	display := root
	if abs, err := filepath.Abs(root); err == nil {
		display = abs
	}

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("Serving %s at http://localhost:%d\n", display, port)
	return http.ListenAndServe(addr, webserver.NewHandler(root))
}
