# QuickCart Feedback Intelligence System — Plan (v3)

## Version History
| Version | File | Status |
|---|---|---|
| v1 | `plan_v1.md` | ✅ Pipeline complete |
| v2 | `plan_v2.md` | ✅ Parallel processing + TUI built |
| **v3** | `plan.md` (this) | 🔄 Upgrade planning |

---

## What's Done ✅

### Pipeline (100% complete)
```
ingest → clean → AI enrich → SQLite store → report
```
| Metric | Value |
|---|---|
| Raw rows | 1,810 |
| Cleaned & enriched | 958 |
| Dropped (blank/short/dup) | 852 |
| Negative sentiment | 682 (71.2%) |
| Positive sentiment | 263 (27.4%) |
| Neutral sentiment | 13 (1.4%) |
| Rating contradictions flagged | 76 |
| Top category | Delivery (245) |

### TUI (v1.0 — functional)
```
tui/
├── main.go          ✅ 4-tab Bubble Tea app
├── db/db.go         ✅ SQLite queries
└── ui/
    ├── styles.go    ✅ Purple/cyan dark theme
    ├── dashboard.go ✅ Metric cards + bar charts
    ├── table.go     ✅ Filterable feedback table
    ├── runner.go    ✅ Live pipeline streaming (PYTHONUNBUFFERED fixed)
    └── report.go    ✅ Scrollable markdown report
```

### Bugs Fixed
- ✅ DB path resolution (`exeDir` not `exeDir/..`)
- ✅ Python stdout buffering (`-u` + `PYTHONUNBUFFERED=1`)
- ✅ Duplicate `runPipelineStreaming` method

---

## Proposed Upgrades for v3

### Track A — TUI Enhancements (UX Polish)

| # | Upgrade | Impact |
|---|---|---|
| A1 | **Detail popup** — press `enter` on table row → full feedback in overlay | High |
| A2 | **Live sentiment donut** in dashboard using block chars | Medium |
| A3 | **Category filter dropdown** in table (instead of cycling) | Medium |
| A4 | **Export filtered rows** to CSV from table view (`x` key) | High |
| A5 | **Search highlight** — highlight matched text in table rows | Medium |
| A6 | **Keyboard shortcut help overlay** (`?` key) | Low |

### Track B — Pipeline Improvements

| # | Upgrade | Impact |
|---|---|---|
| B1 | **Idempotent re-run** — skip already-enriched rows (check DB before AI call) | High |
| B2 | **Progress bar** in pipeline output (% complete with ETA) | Medium |
| B3 | **Cost tracker** — count tokens used, estimate API cost | Low |
| B4 | **Config file** (`config.yaml`) so API key isn't hardcoded | High |

### Track C — Submission Artifacts (Required)

| # | Artifact | Status |
|---|---|---|
| C1 | `README.md` — how to run, decisions, trade-offs | 🔲 Not done |
| C2 | `ai_usage_log.md` — AI interaction log | 🔲 Not done |
| C3 | Final review of `summary_report.md` | 🔲 Not done |

### Track D — Optional Web UI

| # | Upgrade | Notes |
|---|---|---|
| D1 | Flask dashboard with Chart.js charts | Only if time allows |
| D2 | Upload new CSV via browser | Advanced |

---

## Recommended Build Order for v3

```
[1] B4  config.yaml          → move API key out of code     (5 min)
[2] B1  idempotent re-run    → save API cost on re-runs     (15 min)
[3] A1  detail popup         → biggest UX win in TUI        (20 min)
[4] A4  export to CSV        → useful for demo              (10 min)
[5] A6  help overlay         → polish                       (10 min)
[6] C1  README.md            → required for submission      (15 min)
[7] C2  ai_usage_log.md      → required for submission      (10 min)
```

---

## File Structure After v3

```
quickcart-feedback/
├── venv/
├── pipeline/
│   ├── ingest.py
│   ├── clean.py
│   ├── enrich.py       ← B1: skip already enriched rows
│   ├── store.py
│   ├── report.py
│   └── project.md
├── tui/
│   ├── main.go
│   ├── db/db.go
│   └── ui/
│       ├── styles.go
│       ├── dashboard.go
│       ├── table.go    ← A1: detail popup, A4: export
│       ├── runner.go
│       ├── report.go
│       └── help.go     ← A6: new file
├── data/
├── output/
├── config.py           ← B4: move to config.yaml
├── main.py
├── quickcart-tui       ← compiled binary
├── requirements.txt
├── README.md           ← C1: new
├── ai_usage_log.md     ← C2: new
├── plan_v1.md
├── plan_v2.md
└── plan.md             ← this file (v3)
```

---

## Open Questions

1. **Which Track first?** — TUI polish (A) or submission artifacts (C)?
2. **Config file** — move API key to `config.yaml` or `.env` file?
3. **Idempotent re-run** — should re-running the pipeline skip already enriched rows or always re-enrich?
