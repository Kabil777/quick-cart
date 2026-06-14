"""
pipeline/store.py — Stage 4: Persist to SQLite DB and CSV
"""

import sqlite3
import pandas as pd
from datetime import datetime
from config import DB_PATH, OUTPUT_CSV


def _get_conn(db_path: str = DB_PATH) -> sqlite3.Connection:
    return sqlite3.connect(db_path)


def init_db(db_path: str = DB_PATH):
    """Create tables if they don't exist."""
    conn = _get_conn(db_path)
    conn.executescript("""
        CREATE TABLE IF NOT EXISTS feedback (
            id                    TEXT PRIMARY KEY,
            timestamp             TEXT,
            timestamp_normalized  TEXT,
            timestamp_missing     INTEGER DEFAULT 0,
            text_missing          INTEGER DEFAULT 0,

            source                TEXT,
            rating                TEXT,
            rating_int            REAL,
            rating_contradiction  INTEGER DEFAULT 0,
            feedback_text         TEXT,
            sentiment             TEXT,
            category              TEXT,
            summary               TEXT,
            processed_at          TEXT
        );

        CREATE TABLE IF NOT EXISTS dropped_rows (
            id           TEXT,
            drop_reason  TEXT,
            feedback_text TEXT,
            raw_row      TEXT
        );
    """)
    # ── Migrate: add text_missing if it doesn't exist (safe on existing DBs) ─
    try:
        conn.execute("ALTER TABLE feedback ADD COLUMN text_missing INTEGER DEFAULT 0")
    except Exception:
        pass  # column already exists
    conn.commit()
    conn.close()


def store_cleaned(df: pd.DataFrame, dropped_df: pd.DataFrame, db_path: str = DB_PATH):
    """Upsert cleaned + enriched rows into SQLite. Also store dropped rows."""
    conn = _get_conn(db_path)
    now = datetime.utcnow().strftime("%Y-%m-%d %H:%M:%S")

    # ── Insert enriched rows (INSERT OR REPLACE = idempotent) ──────────────
    for _, row in df.iterrows():
        conn.execute("""
            INSERT OR REPLACE INTO feedback (
                id, timestamp, timestamp_normalized, timestamp_missing, text_missing,
                source, rating, rating_int, rating_contradiction,
                feedback_text, sentiment, category, summary, processed_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """, (
            str(row["id"]),
            str(row.get("timestamp", "")),
            str(row.get("timestamp_normalized", "")) if pd.notna(row.get("timestamp_normalized")) else None,
            int(row.get("timestamp_missing", 0)),
            int(row.get("text_missing", 0)),
            str(row.get("source", "")),
            str(row.get("rating", "")),
            float(row["rating_int"]) if pd.notna(row.get("rating_int")) else None,
            int(row.get("rating_contradiction", 0)),
            str(row["feedback_text"]),
            str(row.get("sentiment", "")),
            str(row.get("category", "")),
            str(row.get("summary", "")),
            now,
        ))

    # ── Insert dropped rows ─────────────────────────────────────────────────
    if not dropped_df.empty:
        for _, row in dropped_df.iterrows():
            conn.execute("""
                INSERT INTO dropped_rows (id, drop_reason, feedback_text, raw_row)
                VALUES (?, ?, ?, ?)
            """, (
                str(row.get("id", "")),
                str(row.get("drop_reason", "")),
                str(row.get("feedback_text", "")),
                str(row.to_dict()),
            ))

    conn.commit()
    conn.close()
    print(f"[store] Saved {len(df):,} rows to DB: {db_path}")

    # ── Export to CSV ────────────────────────────────────────────────────────
    export_cols = [
        "id", "timestamp_normalized", "source", "rating_int",
        "feedback_text", "sentiment", "category", "summary",
        "rating_contradiction", "timestamp_missing", "text_missing",
    ]
    available = [c for c in export_cols if c in df.columns]
    df[available].to_csv(OUTPUT_CSV, index=False)
    print(f"[store] Exported CSV: {OUTPUT_CSV}")
