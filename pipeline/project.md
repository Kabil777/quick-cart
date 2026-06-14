# QuickCart Feedback Pipeline — Project Documentation

> **Location**: `quickcart-feedback/pipeline/`
> **Status**: ✅ Complete & Verified
> **Last run**: 2026-06-14 | 958 rows enriched

---

## Overview

The `pipeline/` module is the core of the QuickCart Feedback Intelligence System.
It transforms a raw, messy CSV of ~1,810 customer messages into a clean, AI-enriched, structured dataset stored in SQLite and exported to CSV.

```
Raw CSV (1,810 rows)
       │
       ▼
 ┌─────────────┐
 │  ingest.py  │  Stage 1 — Load
 └──────┬──────┘
        │  1,810 rows
        ▼
 ┌─────────────┐
 │   clean.py  │  Stage 2 — Clean + Flag
 └──────┬──────┘
        │  958 rows kept │ 852 dropped
        ▼
 ┌─────────────┐
 │  enrich.py  │  Stage 3 — AI Enrichment (sequential or parallel)
 └──────┬──────┘
        │  958 rows + sentiment + category + summary
        ▼
 ┌─────────────┐
 │   store.py  │  Stage 4 — SQLite DB + CSV export
 └──────┬──────┘
        │
        ▼
 ┌─────────────┐
 │  report.py  │  Stage 5 — Markdown summary report
 └─────────────┘
```

---

## File Reference

| File | Stage | Responsibility |
|---|---|---|
| `ingest.py` | 1 | Load CSV, normalize column names |
| `clean.py` | 2 | Drop invalid rows, dedup, normalize timestamps, flag contradictions |
| `enrich.py` | 3 | AI enrichment — sentiment, category, summary (sequential + parallel) |
| `store.py` | 4 | Upsert to SQLite, export CSV |
| `report.py` | 5 | Generate Markdown summary report |
| `__init__.py` | — | Package marker |

---

## Stage 1 · `ingest.py`

### Function: `load_raw(path) → DataFrame`

- Loads CSV with `dtype=str` (no type assumptions on raw data)
- Handles encoding issues gracefully (`errors='replace'`)
- Normalizes column names to lowercase + underscores
- Warns on bad lines

**Input**: `data/customer_feedback_raw.csv`
**Output**: Raw `DataFrame` — 1,810 rows × 5 columns (`id`, `timestamp`, `source`, `rating`, `feedback_text`)

---

## Stage 2 · `clean.py`

### Function: `clean(df) → (cleaned_df, dropped_df)`

#### Cleaning Steps (in order)

| Step | Action | Rows Affected |
|---|---|---|
| 2.1 | Strip whitespace from all columns | All |
| 2.2 | Drop blank `feedback_text` | **25 dropped** |
| 2.3 | Drop short/meaningless text < 15 chars or no letters | **114 dropped** |
| 2.4 | Normalize text key (lowercase, collapse whitespace, strip punctuation) | — |
| 2.5 | Deduplicate on normalized text (keep first) | **713 dropped** |
| 2.6 | Normalize `timestamp` to ISO 8601 via `dateutil.parser` | Multiple formats handled |
| 2.7 | Blank timestamps to NULL (rows **kept**) | 118 flagged |
| 2.8 | Validate `source` against known enum values | Unknown to `"unknown"` |
| 2.9 | Parse `rating` safely to float (NaN if blank) | — |
| 2.10 | Flag `rating_contradiction` (high rating + negative text, or vice versa) | **76 flagged** |

#### Timestamp Formats Handled
`02-Feb-24`, `02/14/2024`, `2024-03-25 16:31:48`, `March 18, 2024`, `13-Mar-24` — all normalized to `YYYY-MM-DD HH:MM:SS`

#### Rating Contradiction Logic
- Rating >= 4 AND text contains negative keywords (crash, charged twice, refund...) flagged
- Rating <= 2 AND text contains positive keywords (love, fantastic, excellent...) flagged
- Downstream AI uses **text**, not rating, for sentiment

**Input**: Raw DataFrame (1,810 rows)
**Output**:
- `cleaned_df` — 958 rows with added flag columns
- `dropped_df` — 852 dropped rows with `drop_reason`

---

## Stage 3 · `enrich.py`

### Functions
- `enrich(df, skip_ai)` — sequential mode (safe default)
- `enrich_parallel(df, skip_ai, max_workers)` — parallel mode (3-4x faster)

### AI Configuration

| Setting | Value | Reason |
|---|---|---|
| Provider | NVIDIA API (`integrate.api.nvidia.com`) | Free credits, OpenAI-compatible |
| Model | `openai/gpt-oss-20b` | Fast reasoning model |
| Temperature | `0` | Deterministic, consistent output |
| Batch size | 10 rows per call | Efficient API usage |
| Max workers | 5 (parallel mode) | Balances speed vs. rate limits |
| Retry | Up to 2 retries (exponential backoff) | Handles transient failures |
| Fallback | `"neutral"` / `"Other"` | Never leaves a row unenriched |

### System Prompt Strategy
- Explicit instruction: base sentiment on **text only**, not rating
- Sarcasm counts as Negative
- Fixed enum enforcement for both `sentiment` and `category`
- JSON-only output (no markdown, no explanation)

### Output Validation (per row)
```
sentiment in {"positive", "negative", "neutral"}
category  in {"Billing", "App Bug", "Delivery", "Staff/Support", "Other"}
summary   → non-empty string
len(results) == len(batch)  → length check
```

### Parallel Mode Design
- `ThreadPoolExecutor(max_workers=5)` — concurrent batch submissions
- Thread-local `OpenAI` client (one per thread, thread-safe)
- `results_map[batch_idx]` — preserves original row order
- `threading.Lock()` — thread-safe progress counter
- Exponential backoff on retry (`2^attempt` seconds)

**Input**: Cleaned DataFrame (958 rows)
**Output**: Same DataFrame + `sentiment`, `category`, `summary` columns

### Verified Results
| Sentiment | Count | % |
|---|---|---|
| Negative | 682 | 71.2% |
| Positive | 263 | 27.4% |
| Neutral | 13 | 1.4% |

| Category | Count |
|---|---|
| Delivery | 245 |
| App Bug | 222 |
| Staff/Support | 194 |
| Billing | 167 |
| Other | 130 |

---

## Stage 4 · `store.py`

### Functions
- `init_db()` — creates tables if not exist
- `store_cleaned(df, dropped_df)` — upsert enriched rows + dropped rows

### SQLite Schema

```sql
-- Main enriched feedback table
CREATE TABLE feedback (
    id                    TEXT PRIMARY KEY,
    timestamp             TEXT,
    timestamp_normalized  TEXT,
    timestamp_missing     INTEGER,
    source                TEXT,
    rating                TEXT,
    rating_int            REAL,
    rating_contradiction  INTEGER,
    feedback_text         TEXT,
    sentiment             TEXT,
    category              TEXT,
    summary               TEXT,
    processed_at          TEXT
);

-- Rows removed during cleaning
CREATE TABLE dropped_rows (
    id            TEXT,
    drop_reason   TEXT,
    feedback_text TEXT,
    raw_row       TEXT
);
```

### Idempotency
Uses `INSERT OR REPLACE` — re-running the pipeline never creates duplicates.

**Output**:
- `output/feedback.db` — SQLite database (958 enriched + 852 dropped)
- `output/cleaned_enriched.csv` — 958 rows x 10 columns

---

## Stage 5 · `report.py`

### Function: `generate()`

Reads from `feedback.db` and produces `output/summary_report.md` containing:

1. **Dataset Summary** — raw/kept/dropped counts, flags
2. **Sentiment Breakdown** — counts, percentages, text bar chart
3. **Top 5 Categories by Volume** — ranked table with bars
4. **Representative Examples** — 2-3 real messages per top category
5. **Rating Contradiction Alerts** — sample of flagged rows

---

## Running the Pipeline

```bash
# Activate venv first (always)
cd /home/kabil/quickcart-feedback
source venv/bin/activate

# Full pipeline (sequential)
python main.py

# Full pipeline (parallel — 3-4x faster)
python main.py --parallel

# Custom workers
python main.py --parallel --workers 8

# Clean only, no AI (test mode)
python main.py --skip-ai

# Preview drops, no save
python main.py --dry-run

# Regenerate report from existing DB
python main.py --report-only
```

---

## Health Check (Last Verified)

```
✅ All module imports OK
✅ output/feedback.db        — 958 enriched rows, 852 dropped rows
✅ output/cleaned_enriched.csv — 958 rows x 10 columns
✅ output/summary_report.md  — generated
✅ venv/                     — isolated, all deps installed
✅ plan_v1.md / plan.md      — versioned plans present
```
