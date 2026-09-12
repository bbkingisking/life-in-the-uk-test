// Command liuk is this project's build tool: normalizing dist/exams.json
// (the site's one hand-edited data file - see internal/examsjson's package
// doc) in place after an edit, and serving dist/ (the site) locally for
// preview. Run from the repo root: `go build ./cmd/liuk` (or `go run
// ./cmd/liuk`) is all that's needed, no separate install step, zero
// third-party dependencies.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "normalize":
		err = runNormalize(os.Args[2:])
	case "serve":
		err = runServe(os.Args[2:])
	case "leanwalk":
		err = runLeanWalk(os.Args[2:])
	case "leanwalk-relabel":
		err = runLeanWalkRelabel(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "liuk: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "liuk:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `Usage: liuk <command>

Commands:
  normalize        Re-derive dist/exams.json's per-question uid and
                   default category in place - run after hand-editing it
  serve [port]     Serve dist/ locally (default port 8000)
  leanwalk [out]   Generate a pool of "lean walk" traversals of the
                   Knowledge Map's giant island (see
                   docs/KNOWLEDGE-WALK-DESIGN.md); prototype, not wired
                   into any other command yet (default out:
                   dist/island-walk-pools.json)
`)
}
