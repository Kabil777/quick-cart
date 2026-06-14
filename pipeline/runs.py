"""
pipeline/runs.py — Run history tracking in a separate runs.db
Records every pipeline execution with timing, row counts, and status.
"""

import sqlite3
from datetime import datetime
from config import RUNS_DB


def _conn(path: str = RUNS_DB) -> sqlite3.Connection:
    return sqlite3.connect(path)


def init_runs_db(path: str = RUNS_DB):
    """Create the pipeline_runs table if it doesn't exist."""
    conn = _conn(path)
    conn.executescript("""
        CREATE TABLE IF NOT EXISTS pipeline_runs (
            run_id        INTEGER PRIMARY KEY AUTOINCREMENT,
            started_at    TEXT NOT NULL,
            completed_at  TEXT,
            status        TEXT DEFAULT 'running',
            mode          TEXT,
            rows_raw      INTEGER DEFAULT 0,
            rows_cleaned  INTEGER DEFAULT 0,
            rows_dropped  INTEGER DEFAULT 0,
            rows_enriched INTEGER DEFAULT 0,
            negative      INTEGER DEFAULT 0,
            positive      INTEGER DEFAULT 0,
            neutral       INTEGER DEFAULT 0,
            tokens_prompt      INTEGER DEFAULT 0,
            tokens_completion  INTEGER DEFAULT 0,
            tokens_total       INTEGER DEFAULT 0,
            api_calls          INTEGER DEFAULT 0,
            retries            INTEGER DEFAULT 0,
            notes         TEXT
        );
        -- Add token columns to existing DBs that were created before this migration
        CREATE TABLE IF NOT EXISTS _migrations (id INTEGER PRIMARY KEY);
    """)
    # Safe column additions for existing databases (ignore errors if col exists)
    for col, typ in [
        ("tokens_prompt", "INTEGER DEFAULT 0"),
        ("tokens_completion", "INTEGER DEFAULT 0"),
        ("tokens_total", "INTEGER DEFAULT 0"),
        ("api_calls", "INTEGER DEFAULT 0"),
        ("retries", "INTEGER DEFAULT 0"),
    ]:
        try:
            conn.execute(f"ALTER TABLE pipeline_runs ADD COLUMN {col} {typ}")
        except Exception:
            pass  # column already exists
    conn.commit()
    conn.close()


def start_run(mode: str = "sequential", path: str = RUNS_DB) -> int:
    """Insert a new run record with status='running'. Returns run_id."""
    init_runs_db(path)
    conn = _conn(path)
    cur = conn.execute(
        "INSERT INTO pipeline_runs (started_at, status, mode) VALUES (?, 'running', ?)",
        (datetime.utcnow().strftime("%Y-%m-%d %H:%M:%S"), mode)
    )
    run_id = cur.lastrowid
    conn.commit()
    conn.close()
    print(f"[runs] Run #{run_id} started ({mode} mode)")
    return run_id


def complete_run(run_id: int, rows_raw: int, rows_cleaned: int,
                 rows_dropped: int, rows_enriched: int,
                 negative: int, positive: int, neutral: int,
                 tokens_prompt: int = 0, tokens_completion: int = 0,
                 tokens_total: int = 0, api_calls: int = 0, retries: int = 0,
                 path: str = RUNS_DB):
    """Mark a run as complete with final stats and token usage."""
    conn = _conn(path)
    conn.execute("""
        UPDATE pipeline_runs SET
            completed_at       = ?,
            status             = 'complete',
            rows_raw           = ?,
            rows_cleaned       = ?,
            rows_dropped       = ?,
            rows_enriched      = ?,
            negative           = ?,
            positive           = ?,
            neutral            = ?,
            tokens_prompt      = ?,
            tokens_completion  = ?,
            tokens_total       = ?,
            api_calls          = ?,
            retries            = ?
        WHERE run_id = ?
    """, (
        datetime.utcnow().strftime("%Y-%m-%d %H:%M:%S"),
        rows_raw, rows_cleaned, rows_dropped, rows_enriched,
        negative, positive, neutral,
        tokens_prompt, tokens_completion, tokens_total, api_calls, retries,
        run_id
    ))
    conn.commit()
    conn.close()
    print(f"[runs] Run #{run_id} completed ✓  (tokens: {tokens_total:,})")


def fail_run(run_id: int, error: str, path: str = RUNS_DB):
    """Mark a run as failed with error message."""
    conn = _conn(path)
    conn.execute("""
        UPDATE pipeline_runs SET
            completed_at = ?,
            status       = 'failed',
            notes        = ?
        WHERE run_id = ?
    """, (datetime.utcnow().strftime("%Y-%m-%d %H:%M:%S"), error[:500], run_id))
    conn.commit()
    conn.close()
    print(f"[runs] Run #{run_id} failed: {error[:80]}")


def get_runs(limit: int = 20, path: str = RUNS_DB) -> list[dict]:
    """Fetch the last N runs ordered by newest first."""
    init_runs_db(path)
    conn = _conn(path)
    cur = conn.execute("""
        SELECT run_id, started_at, completed_at, status, mode,
               rows_raw, rows_cleaned, rows_dropped, rows_enriched,
               negative, positive, neutral, notes
        FROM pipeline_runs
        ORDER BY run_id DESC
        LIMIT ?
    """, (limit,))
    cols = [d[0] for d in cur.description]
    rows = [dict(zip(cols, row)) for row in cur.fetchall()]
    conn.close()
    return rows
