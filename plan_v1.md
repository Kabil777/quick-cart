# QuickCart Feedback Intelligence System — Implementation Plan

## Overview

Two-phase build:
- **Phase 1** → Data pipeline (clean → enrich → store → report)
- **Phase 2** → UI layer on top of the pipeline (web or CLI)

---

## Tech Stack

| Layer | Choice | Reason |
|---|---|---|
| Language | Python 3.11+ | Fast, rich ecosystem |
| Data Processing | `pandas` | Efficient CSV/table operations |
| AI Enrichment | Google Gemini (gemini-1.5-flash) | Free tier, fast, structured JSON output |
| Database | SQLite (via `sqlite3`) | Zero setup, portable, file-based |
| Report Output | Markdown + CSV | Easy to read, submittable |
| UI (Phase 2) | CLI (`rich`) or Web (`Flask`) | TBD after pipeline is stable |

---

## Project File Structure

```
quickcart-feedback/
├── data/
│   └── customer_feedback_raw.csv        # Input file
├── pipeline/
│   ├── __init__.py
│   ├── ingest.py                        # Stage 1: Load CSV
│   ├── clean.py                         # Stage 2: Clean + Flag
│   ├── enrich.py                        # Stage 3: AI Enrichment
│   ├── store.py                         # Stage 4: Store in SQLite
│   └── report.py                        # Stage 5: Report Generation
├── output/
│   ├── cleaned_enriched.csv             # Final output file
│   ├── feedback.db                      # SQLite database
│   └── summary_report.md               # Human-readable report
├── config.py                            # API keys, constants
├── main.py                              # Entry point (runs full pipeline)
├── ai_usage_log.md                      # AI usage log (required)
└── README.md                            # How to run + decisions
```

---

## Phase 1 — Data Pipeline

### Stage 1 · Ingest (`ingest.py`)
- Load `customer_feedback_raw.csv` with `pandas`
- Detect encoding issues
- Return raw DataFrame

```
Input  → customer_feedback_raw.csv
Output → raw DataFrame (1810 rows)
```

---

### Stage 2 · Clean + Flag (`clean.py`)

**Step-by-step cleaning logic:**

| Step | Action | Reason |
|---|---|---|
| 2.1 | Strip whitespace from all string columns | Normalize |
| 2.2 | Drop rows where `feedback_text` is blank | No content to process |
| 2.3 | Drop rows where `feedback_text` length < 15 chars after strip | Meaningless (e.g., `"meh"`, `"👍"`) |
| 2.4 | Normalize `feedback_text`: lowercase copy for dedup key | Prepare for dedup |
| 2.5 | **Dedup** on `feedback_text_normalized` (keep first occurrence) | 205 duplicate messages found |
| 2.6 | Normalize `timestamp` → ISO 8601 (`YYYY-MM-DD HH:MM:SS`) | Multiple formats in data |
| 2.7 | Blank `timestamp` → store as `NULL` (do NOT drop) | Text is still valid |
| 2.8 | Validate `source` → must be one of 3 known values | Catch corrupt source values |
| 2.9 | Add flag: `rating_contradiction` = True if rating/text conflict | e.g. rating=5 but text is negative |
| 2.10 | Add flag: `timestamp_missing` = True if timestamp was blank | Audit trail |

```
Input  → raw DataFrame
Output → cleaned DataFrame + audit flags
         Dropped rows logged separately
```

**Rating Contradiction Logic:**
- If `rating ∈ {1,2}` AND text contains strongly positive words → flag
- If `rating ∈ {4,5}` AND text contains strongly negative words → flag
- Trust **text** over rating for downstream AI sentiment

---

### Stage 3 · AI Enrichment (`enrich.py`)

**For every cleaned row, derive 3 things via AI:**

```json
{
  "sentiment": "positive | negative | neutral",
  "category": "Billing | App Bug | Delivery | Staff/Support | Other",
  "summary": "One-line plain English summary of the actual issue"
}
```

**AI Strategy:**

| Setting | Value |
|---|---|
| Model | `gemini-1.5-flash` |
| Temperature | `0` (deterministic, consistent) |
| Output format | JSON (structured, parseable) |
| Batch size | 10 rows per API call |
| Retry logic | 1 retry on invalid output, fallback to `"Other"/"neutral"` |
| Idempotency | Skip rows already enriched (check DB before calling AI) |

**System Prompt Design:**
```
You are a customer feedback classifier for QuickCart, a food delivery app.
For each feedback message, return a JSON object with exactly these fields:
  - sentiment: one of ["positive", "negative", "neutral"]
  - category: one of ["Billing", "App Bug", "Delivery", "Staff/Support", "Other"]
  - summary: a single sentence (max 20 words) describing the core issue in plain English

Rules:
- Base sentiment on the TEXT, not the star rating
- Sarcasm counts as negative
- If the message doesn't fit Billing/App Bug/Delivery/Staff/Support → use "Other"
- Return ONLY valid JSON. No explanation.
```

**Output Validation:**
```python
VALID_SENTIMENTS = {"positive", "negative", "neutral"}
VALID_CATEGORIES = {"Billing", "App Bug", "Delivery", "Staff/Support", "Other"}

# After AI response:
assert result["sentiment"] in VALID_SENTIMENTS
assert result["category"] in VALID_CATEGORIES
assert len(result["summary"]) > 0
# On failure → retry once → fallback
```

```
Input  → cleaned DataFrame
Output → enriched DataFrame (+ sentiment, category, summary columns)
```

---

### Stage 4 · Store in DB (`store.py`)

**SQLite Schema:**

```sql
CREATE TABLE IF NOT EXISTS feedback (
    id                    TEXT PRIMARY KEY,
    timestamp             TEXT,              -- ISO 8601 or NULL
    source                TEXT,
    rating                INTEGER,           -- NULL if blank
    feedback_text         TEXT,
    sentiment             TEXT,              -- positive/negative/neutral
    category              TEXT,              -- fixed 5 values
    summary               TEXT,             -- AI one-liner
    rating_contradiction  INTEGER DEFAULT 0, -- 0 or 1 (bool)
    timestamp_missing     INTEGER DEFAULT 0, -- 0 or 1 (bool)
    processed_at          TEXT               -- when AI enrichment ran
);

CREATE TABLE IF NOT EXISTS dropped_rows (
    original_id  TEXT,
    reason       TEXT,     -- "blank_text" | "too_short" | "duplicate"
    raw_text     TEXT
);
```

- Also export full enriched table to `output/cleaned_enriched.csv`
- Idempotent: re-running won't duplicate rows (uses `INSERT OR REPLACE`)

```
Input  → enriched DataFrame
Output → feedback.db (SQLite) + cleaned_enriched.csv
```

---

### Stage 5 · Report Generation (`report.py`)

**Report Contents:**

```
1. Dataset Summary
   - Total raw rows
   - Rows dropped (reason breakdown)
   - Rows processed

2. Sentiment Breakdown
   - Positive: N (XX%)
   - Neutral:  N (XX%)
   - Negative: N (XX%)

3. Top 5 Categories by Volume
   - Bar chart (text-based) + counts

4. Representative Examples (2-3 per top category)
   - Actual feedback_text + AI summary

5. Rating Contradiction Alerts
   - Count + sample rows
```

Output: `output/summary_report.md`

---

### Stage 6 · Entry Point (`main.py`)

```python
# Orchestrates all stages in order
python main.py --input data/customer_feedback_raw.csv
```

Flags:
- `--skip-ai` → run clean only (for testing without API key)
- `--dry-run` → print what would be dropped, don't save

---

## Phase 2 — UI Layer (After Pipeline is Stable)

### Option A: CLI UI (with `rich`)
- Live progress bars during processing
- Pretty tables for report
- Commands: `run`, `report`, `query <category>`

### Option B: Web UI (Flask)
- Upload CSV via browser
- Dashboard: sentiment donut chart, category bar chart
- Table view: filter by sentiment/category
- Download enriched CSV

> **Decision**: Build CLI first (faster), then optionally wrap in Web UI

---

## Build Order (Sequence)

```
[1] config.py          → API keys, constants, category/sentiment enums
[2] ingest.py          → load CSV
[3] clean.py           → all cleaning + flagging logic
[4] enrich.py          → AI enrichment + validation + retry
[5] store.py           → SQLite + CSV export
[6] report.py          → summary report
[7] main.py            → wire everything together
[8] Test end-to-end    → run on actual CSV, verify output
[9] ai_usage_log.md    → document AI interactions
[10] README.md         → how to run, decisions, trade-offs
---- PHASE 2 ----
[11] UI layer          → CLI (rich) or Web (Flask)
```

---

## Key Design Decisions Summary

| Decision | Choice | Reason |
|---|---|---|
| Dedup method | By normalized `feedback_text` | ID is always unique; content duplicates exist |
| Blank timestamps | Keep as `NULL` | Don't lose valid feedback |
| Rating vs text conflict | Trust text, flag contradiction | Ratings are unreliable (sarcasm, errors) |
| Short messages | Drop < 15 chars | No actionable content |
| AI temperature | 0 | Reproducible, consistent output |
| AI output format | Structured JSON | Programmatically validatable |
| DB | SQLite | Zero setup, portable |
| Re-run safety | Idempotent via `INSERT OR REPLACE` | Cost-efficient (no re-calling AI) |

---

## Open Questions for You

1. **AI Provider** → Gemini (free tier) or OpenAI (GPT-4o-mini)? Do you have an API key ready?
2. **UI preference** → CLI (`rich` tables) or Web (Flask dashboard)?
3. **Output format priority** → SQLite DB, or CSV is enough for submission?
