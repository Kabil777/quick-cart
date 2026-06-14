# QuickCart Feedback Intelligence System — Plan (v4)

## Version History
| Version | File | Phase | Status |
|---|---|---|---|
| v1 | `plan_v1.md` | Pipeline foundation | ✅ Done |
| v2 | `plan_v2.md` | Parallel processing + TUI v1 | ✅ Done |
| v3 | `plan_v3.md` | TUI upgrades + history tracking | ✅ Done |
| **v4** | `plan.md` (this) | Token tracking + search + fixes | ✅ Done |
| v5 | _(next)_ | Submission artifacts + final polish | 🔄 Planned |

---

## What Was Done — Phase 4 ✅

### 4.1 Token Utilization Tracking (end-to-end)

**Python pipeline:**
- `pipeline/enrich.py` — Added `TokenUsage` dataclass (thread-safe with `threading.Lock`)
- Accumulates `prompt_tokens`, `completion_tokens`, `total_tokens`, `api_calls`, `retries` per API call
- Both `enrich()` and `enrich_parallel()` now return `(df, token_usage)` tuple
- Live progress now shows: `↳ 958/958 rows enriched  [12,430 tokens used]`
- `pipeline/runs.py` — Added 5 new columns to `pipeline_runs` schema:
  `tokens_prompt`, `tokens_completion`, `tokens_total`, `api_calls`, `retries`
- Auto-migration: `ALTER TABLE ... ADD COLUMN` safely handles existing DBs
- `main.py` — Passes token data from `enrich()` return value into `complete_run()`

**Go TUI — Dashboard tab:**
- New **AI Token Utilization** row shows cumulative stats across all runs:
  `Prompt Tokens` | `Completion Tokens` | `Total Tokens` | `Efficiency (tkns/row)` | `API Calls`
- Pulled from `runs.db` via `GetTokenStats()` in `db/db.go`

**Go TUI — History tab:**
- Per-run detail panel now shows full token breakdown:
  - Prompt / Completion / Total tokens
  - API Calls made
  - Retries count (green=0, red=N)
  - Efficiency: tokens per enriched row

### 4.2 Runner Fix — "0 Lines Captured" Bug

**Root cause:** `cmd.Env` was set to only `["PYTHONUNBUFFERED=1", "PATH=/usr/bin:/bin"]`,
completely wiping the OS environment. Python couldn't locate venv packages → silent crash.

**Fix:** Changed to `cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")`
→ Inherits the full OS environment, then overrides stdout buffering on top.

### 4.3 Enhanced Search Bar — Regex Across All Columns

**Before:** SQL `LIKE` search on `feedback_text` and `summary` only.

**After:**
- All 7 columns searched: `id`, `sentiment`, `category`, `source`, `timestamp`, `summary`, `text`
- **LITERAL mode** (default): case-insensitive substring via `(?i)regexp.QuoteMeta(pattern)`
- **REGEX mode** (`ctrl+r`): full `(?i)` regexp — rg-style pattern matching
- **Match highlight**: matched text highlighted amber in the Summary column
- **"Match in" column**: shows which column(s) matched (e.g. `text,summary`)
- **Error display**: invalid regex shows red border + inline error message
- **Live filtering**: applies on every keystroke (no Enter needed)
- Go-side filtering on up to 2000 pre-loaded rows (SQL only filters sentiment/category)

### 4.4 AI Log in TUI Report Tab
- Tab `[4]` Report can toggle between `summary_report.md` and `ai_usage_log.md`
- Press `[a]` to switch, `[r]` to reload, rendered with Glamour markdown engine

---

## Current File Structure ✅

```
quickcart-feedback/
├── venv/
├── data/
│   └── customer_feedback_raw.csv
├── output/
│   ├── feedback.db            ← enriched data
│   ├── runs.db                ← pipeline run history + token usage
│   ├── cleaned_enriched.csv
│   └── summary_report.md
├── pipeline/
│   ├── __init__.py
│   ├── ingest.py
│   ├── clean.py
│   ├── enrich.py              ← TokenUsage dataclass, parallel batch
│   ├── store.py
│   ├── report.py
│   ├── runs.py                ← run history tracking
│   └── project.md
├── tui/
│   ├── go.mod / go.sum
│   ├── main.go                ← 5-tab Bubble Tea app
│   ├── db/
│   │   └── db.go              ← GetStats, GetRuns, GetTokenStats, GetAllFeedback
│   └── ui/
│       ├── styles.go          ← purple/cyan dark theme
│       ├── dashboard.go       ← stats + token utilization row
│       ├── table.go           ← regex search, all-column, match highlight
│       ├── runner.go          ← live subprocess stream, env fix
│       ├── report.go          ← Glamour markdown, AI log toggle
│       └── history.go        ← run list + per-run token detail
├── quickcart-tui              ← compiled binary (12 MB)
├── main.py
├── config.py
├── requirements.txt
├── ai_usage_log.md            ← submission artifact ✅
├── plan_v1.md / plan_v2.md / plan_v3.md
└── plan.md                    ← this file (v4)
```

---

## Stats So Far

| Metric | Value |
|---|---|
| Raw rows ingested | 1,810 |
| Cleaned & enriched | 958 |
| Rows dropped | 852 |
| Negative sentiment | 682 (71.2%) |
| Positive sentiment | 263 (27.4%) |
| Neutral sentiment | 13 (1.4%) |
| Rating contradictions | 76 |
| Top category | Delivery (245) |
| Pipeline stages | 5 |
| TUI tabs | 5 (Dashboard, Feedback, Run, Report, History) |
| Go binary size | 12 MB (static, no runtime deps) |

---

## Phase 5 — Planned 🔄

### P5.1 README.md (Required — submission artifact)

```markdown
# QuickCart Feedback Intelligence System
## Setup, How to Run, Architecture, Design Decisions
```

| Section | Content |
|---|---|
| Overview | What the system does |
| Setup | venv, requirements.txt, API key |
| Running | `python main.py`, `./quickcart-tui` |
| Architecture | Pipeline stages, TUI tabs |
| Design decisions | Why NVIDIA API, why Go TUI, idempotency |
| Known limitations | Token cost, rate limits |

**Estimated effort:** 20 min

### P5.2 Idempotent Pipeline Re-run (B1)

Skip already-enriched rows when re-running. Check DB for existing IDs before AI call.

```python
# enrich.py: skip rows already in feedback.db
existing_ids = set(db.execute("SELECT id FROM feedback").fetchall())
to_enrich = df[~df['id'].isin(existing_ids)]
```

**Benefit:** Re-running costs 0 tokens if no new data. Critical for cost control.
**Estimated effort:** 15 min

### P5.3 Final Polish

| Item | Detail |
|---|---|
| Dashboard auto-refresh | Press `R` in Dashboard to reload stats after pipeline run |
| History auto-refresh | After pipeline completes in TUI, refresh History tab automatically |
| TUI version string | Show app version in header |

---

## Open Questions for Phase 5

1. **README language** — English only, or include Tamil/bilingual notes?
2. **Idempotent re-run** — Check by row `id` or by content hash?
3. **Submission deadline** — Is README the last required artifact, or are there others?
