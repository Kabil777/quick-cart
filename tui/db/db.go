package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Stats holds aggregate statistics from the feedback DB
type Stats struct {
	Total       int
	Negative    int
	Positive    int
	Neutral     int
	Dropped     int
	Flagged     int // rating contradictions
	Categories  []CategoryCount
}

type CategoryCount struct {
	Name  string
	Count int
}

// Feedback represents one enriched row
type Feedback struct {
	ID                  string
	Timestamp           string
	Source              string
	Rating              float64
	FeedbackText        string
	Sentiment           string
	Category            string
	Summary             string
	RatingContradiction bool
	TimestampMissing    bool
}

// Open opens the SQLite database
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return db, nil
}

// PipelineRun represents one recorded pipeline execution
type PipelineRun struct {
	RunID            int
	StartedAt        string
	CompletedAt      string
	Status           string
	Mode             string
	RowsRaw          int
	RowsCleaned      int
	RowsDropped      int
	RowsEnriched     int
	Negative         int
	Positive         int
	Neutral          int
	TokensPrompt     int
	TokensCompletion int
	TokensTotal      int
	APICalls         int
	Retries          int
	Notes            string
}

// GetRuns fetches the last N runs from runs.db ordered newest first
func GetRuns(runsDB *sql.DB, limit int) ([]PipelineRun, error) {
	rows, err := runsDB.Query(`
		SELECT run_id, COALESCE(started_at,''), COALESCE(completed_at,''),
		       COALESCE(status,''), COALESCE(mode,''),
		       rows_raw, rows_cleaned, rows_dropped, rows_enriched,
		       negative, positive, neutral,
		       COALESCE(tokens_prompt,0), COALESCE(tokens_completion,0),
		       COALESCE(tokens_total,0), COALESCE(api_calls,0), COALESCE(retries,0),
		       COALESCE(notes,'')
		FROM pipeline_runs ORDER BY run_id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PipelineRun
	for rows.Next() {
		var r PipelineRun
		rows.Scan(&r.RunID, &r.StartedAt, &r.CompletedAt, &r.Status, &r.Mode,
			&r.RowsRaw, &r.RowsCleaned, &r.RowsDropped, &r.RowsEnriched,
			&r.Negative, &r.Positive, &r.Neutral,
			&r.TokensPrompt, &r.TokensCompletion, &r.TokensTotal,
			&r.APICalls, &r.Retries, &r.Notes)
		result = append(result, r)
	}
	return result, nil
}

// TokenStats holds cumulative token usage across all runs
type TokenStats struct {
	TotalRuns        int
	TotalTokens      int
	TotalPrompt      int
	TotalCompletion  int
	TotalAPICalls    int
	TotalRetries     int
	LatestRunID      int
	LatestTokens     int
	LatestAPICalls   int
}

// GetTokenStats returns cumulative and latest-run token usage from runs.db
func GetTokenStats(runsDB *sql.DB) (TokenStats, error) {
	var s TokenStats
	if runsDB == nil {
		return s, nil
	}
	err := runsDB.QueryRow(`
		SELECT
			COUNT(*),
			COALESCE(SUM(tokens_total),0),
			COALESCE(SUM(tokens_prompt),0),
			COALESCE(SUM(tokens_completion),0),
			COALESCE(SUM(api_calls),0),
			COALESCE(SUM(retries),0)
		FROM pipeline_runs WHERE status = 'complete'`).
		Scan(&s.TotalRuns, &s.TotalTokens, &s.TotalPrompt,
			&s.TotalCompletion, &s.TotalAPICalls, &s.TotalRetries)
	if err != nil {
		return s, err
	}
	_ = runsDB.QueryRow(`
		SELECT run_id, COALESCE(tokens_total,0), COALESCE(api_calls,0)
		FROM pipeline_runs WHERE status='complete'
		ORDER BY run_id DESC LIMIT 1`).
		Scan(&s.LatestRunID, &s.LatestTokens, &s.LatestAPICalls)
	return s, nil
}

// GetStats returns aggregate statistics
func GetStats(db *sql.DB) (Stats, error) {
	var s Stats

	_ = db.QueryRow(`SELECT COUNT(*) FROM feedback`).Scan(&s.Total)

	sentRows, err := db.Query(`SELECT sentiment, COUNT(*) FROM feedback GROUP BY sentiment`)
	if err != nil {
		return s, err
	}
	defer sentRows.Close()
	for sentRows.Next() {
		var sent string
		var cnt int
		sentRows.Scan(&sent, &cnt)
		switch sent {
		case "negative":
			s.Negative = cnt
		case "positive":
			s.Positive = cnt
		case "neutral":
			s.Neutral = cnt
		}
	}

	catRows, err := db.Query(`SELECT category, COUNT(*) FROM feedback GROUP BY category ORDER BY COUNT(*) DESC`)
	if err != nil {
		return s, err
	}
	defer catRows.Close()
	for catRows.Next() {
		var cat string
		var cnt int
		catRows.Scan(&cat, &cnt)
		s.Categories = append(s.Categories, CategoryCount{Name: cat, Count: cnt})
	}

	_ = db.QueryRow(`SELECT COUNT(*) FROM dropped_rows`).Scan(&s.Dropped)
	_ = db.QueryRow(`SELECT COUNT(*) FROM feedback WHERE rating_contradiction = 1`).Scan(&s.Flagged)

	return s, nil
}

// GetFeedback queries feedback rows with optional filters (legacy, SQL LIKE search)
func GetFeedback(db *sql.DB, sentiment, category, search string, limit int) ([]Feedback, error) {
	return GetAllFeedback(db, sentiment, category, limit)
}

// GetAllFeedback fetches rows filtered only by sentiment/category (SQL).
// Text search is handled Go-side with regex for all-column support.
func GetAllFeedback(db *sql.DB, sentiment, category string, limit int) ([]Feedback, error) {
	query := `SELECT id, COALESCE(timestamp_normalized,''), COALESCE(source,''),
		COALESCE(rating_int,0), feedback_text, sentiment, category, summary,
		rating_contradiction, timestamp_missing
		FROM feedback WHERE 1=1`
	args := []interface{}{}

	if sentiment != "" && sentiment != "all" {
		query += " AND sentiment = ?"
		args = append(args, sentiment)
	}
	if category != "" && category != "All" {
		query += " AND category = ?"
		args = append(args, category)
	}
	query += " ORDER BY id LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Feedback
	for rows.Next() {
		var f Feedback
		var rc, tm int
		rows.Scan(&f.ID, &f.Timestamp, &f.Source, &f.Rating,
			&f.FeedbackText, &f.Sentiment, &f.Category, &f.Summary, &rc, &tm)
		f.RatingContradiction = rc == 1
		f.TimestampMissing = tm == 1
		result = append(result, f)
	}
	return result, nil
}
