# QuickCart Feedback Intelligence System — Plan (v5)

## Version History

| Version | File | Phase | Status |
|---|---|---|---|
| v1 | `plan_v1.md` | Pipeline foundation (5 stages) | ✅ Done |
| v2 | `plan_v2.md` | Parallel enrichment + TUI v1 | ✅ Done |
| v3 | `plan_v3.md` | TUI upgrades + run history | ✅ Done |
| v4 | `plan_v4.md` | Token tracking + regex search + runner fix | ✅ Done |
| **v5** | `plan.md` (this) | CSV picker + cron scheduler + README + polish | ✅ Done |
| v6 | _(next)_ | Idempotent re-run + export CSV + help overlay | 🔄 Planned |

---

## What Was Done — Phase 5 ✅

### 5.1 CSV File Picker (TUI — Run Pipeline tab)

**Problem:** Pipeline was hardcoded to always use `data/customer_feedback_raw.csv`.

**Solution:** Added a text input field in the Run Pipeline tab:

```
📂 CSV File: ✓
┌──────────────────────────────────────┐
│ data/customer_feedback_raw.csv       │
└──────────────────────────────────────┘
```

| Key | Action |
|---|---|
| `f` | Open file input, type any CSV path |
| `enter` / `esc` | Confirm / close |
| `d` | Reset to default CSV |

- Validates file exists in real-time (`✓` green / `✗` red)
- Passes `--input <path>` flag to `python main.py`
- Default path: `data/customer_feedback_raw.csv`
- Input is a `bubbles/textinput` with `CharLimit=200`

### 5.2 Cron Auto-Scheduler (TUI — Run Pipeline tab)

**Problem:** Pipeline had to be manually triggered every time.

**Solution:** Built a Go-native cron system using `tea.Every(time.Second, ...)`:

```
⏰ Schedule: [ACTIVE]  Interval: 1 hour
  Next run in: 58m 42s   (last: 04:52:11)
```

**Intervals available (press `c` to cycle):**
```
Manual → 30 min → 1 hour → 6 hours → 12 hours → 24 hours
```

| Key | Action |
|---|---|
| `c` | Cycle through schedule intervals |
| `x` | Disable (back to Manual) |
| `enter` | Manual run (resets countdown) |

**Architecture:**
- `ScheduleTickMsg` (every 1s) → updates countdown display
- `cronTick()` command re-fires itself with `tea.Every`
- When countdown hits 0 → triggers `startRun()` → resets countdown
- Messages are **routed globally** in `main.go` regardless of active tab — cron keeps ticking even when user is on Dashboard or Feedback tab
- `lastAutoRun` timestamp shown after each auto-triggered run

### 5.3 Runner Background Routing (main.go fix)

`ScheduleTickMsg`, `PipelineLineMsg`, `PipelineDoneMsg` now routed **before** the tab switch — so the runner state machine receives them even when the user is on a different tab.

```go
// Always route these — cron and pipeline don't pause when tab changes
switch msg.(type) {
case ui.ScheduleTickMsg, ui.PipelineLineMsg, ui.PipelineDoneMsg:
    m.runner, cmd = m.runner.Update(msg)
    return m, tea.Batch(cmds...)
}
```

### 5.4 README.md (Submission artifact) ✅

Full project documentation written to `README.md`:
- Setup (venv, pip, API key via env var)
- All run modes (`--parallel`, `--skip-ai`, `--dry-run`, `--input`)
- TUI build and launch instructions
- All key bindings per tab
- Architecture decisions (NVIDIA API, Go TUI, runs.db separation)
- Data quality findings
- 3 TUI screenshots embedded (dashboard, feedback, history)

### 5.5 .gitignore + Git Push ✅

- `.gitignore` covers: `venv/`, `output/*.db`, `*.csv`, Go binary, `.env`, `__pycache__/`
- Pushed to `git@github.com:Kabil777/quick-cart.git` (main branch)
- API key never committed — read via `os.getenv("NVIDIA_API_KEY")`

---

## Current Full Feature Matrix ✅

### Pipeline (Python)

| Feature | Status |
|---|---|
| 5-stage pipeline (ingest → clean → enrich → store → report) | ✅ |
| Parallel enrichment (ThreadPoolExecutor, 5 workers) | ✅ |
| Content-based dedup (not ID-based) | ✅ |
| Multi-format timestamp normalization (dateutil) | ✅ |
| Rating contradiction detection (keyword sets) | ✅ |
| AI retry with exponential backoff | ✅ |
| Enum validation on every AI response | ✅ |
| Token usage tracking (prompt/completion/total/retries) | ✅ |
| Run history in `runs.db` (separate from `feedback.db`) | ✅ |
| Configurable via `--input`, `--parallel`, `--workers`, `--skip-ai`, `--dry-run` | ✅ |
| Idempotent store (`INSERT OR REPLACE`) | ✅ |

### TUI (Go + Bubble Tea)

| Tab | Feature | Status |
|---|---|---|
| **[1] Dashboard** | Sentiment breakdown with bar charts | ✅ |
| | Category ranking | ✅ |
| | AI token utilization row (cumulative) | ✅ |
| **[2] Feedback** | Scrollable table with 6 columns | ✅ |
| | Regex / literal search across ALL 7 columns | ✅ |
| | Match highlight (amber) + "Match in" column | ✅ |
| | Sentiment / category filters | ✅ |
| | Detail popup overlay (enter on any row) | ✅ |
| **[3] Run Pipeline** | Live subprocess output streaming | ✅ |
| | Parallel / sequential mode toggle | ✅ |
| | **CSV file picker with validation** | ✅ |
| | **Cron auto-scheduler (6 intervals, live countdown)** | ✅ |
| **[4] Report** | Glamour-rendered `summary_report.md` | ✅ |
| | Toggle to `ai_usage_log.md` (`a` key) | ✅ |
| **[5] History** | Run list with status badges (✓/⚡/✗) | ✅ |
| | Per-run token breakdown (prompt/completion/total) | ✅ |
| | Per-run sentiment bars | ✅ |
| | Per-run efficiency (tokens/row) | ✅ |

### Submission Artifacts

| Artifact | Status |
|---|---|
| `README.md` | ✅ |
| `ai_usage_log.md` | ✅ |
| `plan.md` (versioned v1–v5) | ✅ |
| GitHub push (`Kabil777/quick-cart`) | ✅ |
| TUI screenshots in `docs/screenshots/` | ✅ |

---

## Phase 6 — Planned 🔄

### P6.1 Idempotent Re-run (B1) — High priority

Skip rows already in `feedback.db` when re-running — avoid re-charging tokens for data already enriched.

```python
# enrich.py — before batch loop
existing_ids = {row[0] for row in db.execute("SELECT id FROM feedback").fetchall()}
to_enrich = df[~df["id"].isin(existing_ids)]
already_done = df[df["id"].isin(existing_ids)]
print(f"[enrich] {len(to_enrich)} new rows to enrich, {len(already_done)} already in DB")
```

**Benefit:** Re-running with same CSV costs 0 tokens. Only processes genuinely new rows.

### P6.2 Export Filtered Table to CSV (A4)

In the Feedback tab, press `e` to export current filtered view (after regex + sentiment + category filters) to a timestamped CSV file.

```
output/export_2026-06-14_11-53.csv
```

### P6.3 Help Overlay (A6)

Press `?` anywhere to show a help overlay listing all keybindings for the current tab.

### P6.4 Dashboard Auto-Refresh

After a pipeline run completes (in any tab), automatically refresh the Dashboard stats so new data is reflected immediately.

---

## Open Questions for Phase 6

1. **Idempotent re-run**: Match by row `id` (fast) or by content hash (more robust)?
2. **Export path**: Always to `output/` or user-configurable?
3. **Auto-refresh**: Refresh all tabs on pipeline complete, or only Dashboard?
