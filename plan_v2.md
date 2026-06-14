# QuickCart Feedback Intelligence System — Plan (v2)

## Version History
| Version | File | Date | Status |
|---|---|---|---|
| v1 | `plan_v1.md` | 2026-06-14 | Phase 1 pipeline — ✅ Complete |
| v2 | `plan.md` (this) | 2026-06-14 | Parallel processing upgrade + Phase 2 UI |

---

## Phase 1 — Pipeline ✅ COMPLETE

### Actual Results (from run)

| Metric | Value |
|---|---|
| Raw rows loaded | 1,810 |
| Blank text dropped | 25 |
| Too short dropped | 114 |
| Duplicates dropped | 713 |
| **Rows AI-enriched** | **958** |
| Missing timestamps (kept) | 118 |
| Rating contradictions flagged | 76 |
| 🔴 Negative sentiment | 682 (71.2%) |
| 🟢 Positive sentiment | 263 (27.4%) |
| 🟡 Neutral sentiment | 13 (1.4%) |

### Output Files Produced
```
output/
├── feedback.db              ✅ SQLite DB (958 rows)
├── cleaned_enriched.csv     ✅ Full enriched export
└── summary_report.md        ✅ Markdown report
```

### Pipeline File Structure
```
quickcart-feedback/
├── venv/                    ✅ Python virtual environment
├── pipeline/
│   ├── __init__.py
│   ├── ingest.py            ✅ Stage 1 — Load CSV
│   ├── clean.py             ✅ Stage 2 — Clean + Flag
│   ├── enrich.py            ✅ Stage 3 — AI enrichment (sequential)
│   ├── store.py             ✅ Stage 4 — SQLite + CSV
│   └── report.py            ✅ Stage 5 — Markdown report
├── data/customer_feedback_raw.csv
├── output/
├── config.py
├── main.py
├── requirements.txt
├── plan_v1.md               ✅ Archived v1 plan
└── plan.md                  ← this file (v2)
```

---

## Phase 1.5 — Parallel Processing Upgrade 🔄 IN PROGRESS

### Problem with current enrich.py
Sequential batching: each batch of 10 waits for the previous one.
- ~130 rows/min → 958 rows takes ~7 minutes
- API latency (reasoning model) is the bottleneck

### Solution: ThreadPoolExecutor (parallel batches)

**Target**: 5 concurrent API calls → ~3-4x speedup → ~2 min for 958 rows

#### Implementation Plan for `enrich.py`

```python
from concurrent.futures import ThreadPoolExecutor, as_completed

def enrich_parallel(df, skip_ai=False, max_workers=5):
    batches = [rows[i:i+AI_BATCH_SIZE] for i in range(0, len(rows), AI_BATCH_SIZE)]
    results_map = {}  # batch_index → results

    with ThreadPoolExecutor(max_workers=max_workers) as executor:
        future_to_idx = {
            executor.submit(_call_ai, batch): idx
            for idx, batch in enumerate(batches)
        }
        for future in as_completed(future_to_idx):
            idx = future_to_idx[future]
            results_map[idx] = future.result()
            # Progress tracking...

    # Reconstruct in order
    ordered = []
    for idx in sorted(results_map):
        ordered.extend(results_map[idx])
    return ordered
```

#### Key Considerations
| Issue | Handling |
|---|---|
| Rate limits | Cap `max_workers=5`, add jitter to retry |
| Order preservation | Use `results_map[idx]` keyed by batch index |
| Thread-safe retry | Each `_call_ai()` is stateless — safe to parallelize |
| Progress display | Use `threading.Lock()` for shared counter |
| Fallback | Same retry + fallback logic per batch |

#### Files to Modify
- `pipeline/enrich.py` — add `enrich_parallel()`, keep `enrich()` as fallback
- `main.py` — add `--parallel` flag, `--workers N` flag
- `config.py` — add `MAX_WORKERS = 5`

#### New CLI flags after upgrade
```bash
python main.py                        # sequential (safe default)
python main.py --parallel             # parallel (faster)
python main.py --parallel --workers 8 # custom worker count
```

---

## Phase 2 — CLI UI (After Parallel Upgrade)

### Tool: `rich` library
- Live progress bars during pipeline run
- Pretty tables for report display
- Color-coded sentiment (🔴 negative, 🟢 positive, 🟡 neutral)
- Commands: `run`, `report`, `query`

### CLI Commands Planned
```bash
python cli.py run                     # run full pipeline with live UI
python cli.py report                  # display summary report in terminal
python cli.py query --category Billing   # filter + show rows
python cli.py query --sentiment negative # filter by sentiment
python cli.py stats                   # quick dataset statistics
```

### File to Create
```
quickcart-feedback/
└── cli.py                            # Rich-powered CLI interface
```

---

## Phase 3 — Submission Artifacts

| File | Status |
|---|---|
| `README.md` | 🔲 To write |
| `ai_usage_log.md` | 🔲 To write |
| `output/summary_report.md` | ✅ Generated |
| `output/cleaned_enriched.csv` | ✅ Generated |
| `output/feedback.db` | ✅ Generated |

---

## Build Order (Updated)

```
✅ [1]  config.py
✅ [2]  pipeline/ingest.py
✅ [3]  pipeline/clean.py
✅ [4]  pipeline/enrich.py       (sequential)
✅ [5]  pipeline/store.py
✅ [6]  pipeline/report.py
✅ [7]  main.py
✅ [8]  End-to-end run verified
🔄 [9]  pipeline/enrich.py       → parallel upgrade  ← CURRENT
🔲 [10] main.py                  → --parallel / --workers flags
🔲 [11] cli.py                   → Rich CLI UI
🔲 [12] README.md
🔲 [13] ai_usage_log.md
```
