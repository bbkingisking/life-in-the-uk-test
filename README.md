# Life in the UK Test Exam Practice

An offline-friendly, interactive way to practice for the Life in the UK Test. Static HTML/CSS/JS site, no build step, no runtime dependencies.

## Features

- **Interactive Web UI**: Practice exams directly in your browser.
- **Multiple Modes**:
    - **Standard Exams**: Fixed exams from the official question bank.
    - **Random Exam**: Draws 24 random questions from the entire database.
    - **Marathon Exam**: A timeless mode to go through all available questions.
    - **Practice by Category**: Drill questions grouped by topic (history, government & law, geography, culture, and more), untimed.
- **Progress, Saved in Your Browser**: A lightweight persistence layer (localStorage) tracks per-question stats — nothing leaves your device, and none of the question content is sensitive.
    - **Review Mistakes**: Questions you've gotten wrong are added to a review queue, with basic stats (times seen, correct/wrong). Click one to revisit it; answering it correctly clears it from the queue. A "Drill All" button runs through everything still open.
    - **Not Sure List**: While answering any question, tap "🔖 Not sure" to bookmark it for later, independent of whether you got it right. Revisit or drill the whole list from its own section.
- **Timed Practice**: Standard and Random exams include a 45-minute countdown timer to simulate real exam conditions; Marathon and drill modes are untimed and stoppable at any point.
- **Auto-Shuffle**: Both questions and answer choices are randomized every time you start an exam to ensure genuine learning.
- **Advanced Results**: View your score, percentage, and time taken. Includes a detailed list of incorrect questions with their correct answers and explanations.
- **Mobile-First**: Designed to work perfectly on phones for on-the-go study.
- **Comprehensive Data**: Includes all questions, multiple-choice options, and detailed explanations (references), topic-tagged for the category drill.

## Project Structure

- `dist/`: The deployed site — everything the browser loads. This is surge's `--project` root (see `.github/workflows/deploy.yml`) — nothing outside it reaches the public site.
    - `dist/index.html`, `dist/style.css`, `dist/app.js`, `dist/knowledge-map.html`, `dist/vendor/shoelace/`: The web application (no framework, no build step).
    - `dist/exams.json`: The single canonical, hand-edited data file — every question, its answers, reference text, topic category, and a stable per-question uid (used as the front end's browser-persistence key). Edit it directly for corrections or new questions; there's no separate source it's built from (see "Update Data" below). Also what the "Download all exams" link serves.
    - `dist/island-walk-pools.json`: Data behind "Island walk" mode, regenerated fresh on every deploy (see `docs/KNOWLEDGE-WALK-DESIGN.md`).
- `cmd/liuk/`, `internal/`: The `liuk` build tool (normalize/serve). Go standard library only — no third-party dependencies. Tests live next to the code they test (`internal/*/*_test.go`).
- `docs/`: Design notes that aren't user-facing (currently `KNOWLEDGE-WALK-DESIGN.md`).

## Getting Started

### Prerequisites
A Go compiler (1.23+). Nothing else — no package install step, no virtual environment, no Node.

### Run the Website Locally
```bash
go run ./cmd/liuk serve
```
Then open `http://localhost:8000` (serves `dist/`). Or build the tool once and reuse the binary:
```bash
go build -o liuk ./cmd/liuk
./liuk serve
```

### Update Data
Edit `dist/exams.json` directly (fix wording, add a question, change a category), then normalize it to re-derive the fields you shouldn't have to fill in by hand — a new/changed question's uid, and a default `"uncategorized"` for one with no category yet:
```bash
go run ./cmd/liuk normalize
```
This rewrites the checked-in file in place — review the diff and commit the result, the same way you would any other hand edit.

### Run Tests
```bash
go test ./...
```
