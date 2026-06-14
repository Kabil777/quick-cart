# AI Usage Log — QuickCart Feedback Intelligence System

> Required submission artifact. Documents how AI was used, where it went wrong, and how it was corrected.

---

## 1. Pipeline — AI Enrichment (NVIDIA API / gpt-oss-20b)

### What I asked the AI to do
For each of the 958 cleaned feedback rows, classify:
- **Sentiment**: positive / negative / neutral
- **Category**: Billing / App Bug / Delivery / Staff/Support / Other
- **Summary**: one-line plain English description

Processed in batches of 10 rows per API call using a structured JSON prompt.

### System prompt used
```
You are a customer feedback classifier for QuickCart...
Base sentiment ONLY on the message TEXT, never on the star rating.
Sarcasm and irony count as NEGATIVE.
Return ONLY a valid JSON array.
```

### Where AI output was wrong / incomplete

| Issue | Example | How I caught it | Fix |
|---|---|---|---|
| **Wrong JSON format** | Returned markdown fences ` ```json ``` ` around output | Output validation failed (`json.loads` error) | Stripped markdown fences before parsing |
| **Sarcasm misclassified** | "Oh great, crashed again right at checkout, just love it" → AI sometimes returned `positive` | Cross-checked rating=5 rows with negative text | Explicit prompt rule: "Sarcasm = NEGATIVE" |
| **Batch length mismatch** | Returned 9 items for a batch of 10 | `len(results) != len(batch)` check | Retry with exponential backoff, fallback to neutral/Other |
| **Category hallucination** | Returned `"Payment"` instead of `"Billing"` | Enum validation against fixed set | Retry + fallback to "Other" on invalid enum |
| **Summary too long** | Returned 40+ word summaries | Manual review of output CSV | Prompt updated: "max 20 words" |

### Corrections made to pipeline code
- Added `_extract_json()` to strip markdown fences before parsing
- Added enum validation: `assert sentiment in VALID_SENTIMENTS`
- Added `MAX_RETRY = 2` with exponential backoff (`2^attempt` seconds)
- Added fallback: on repeated failure → `"neutral"` / `"Other"` (never leaves row unenriched)
- Set `temperature=0` for deterministic, consistent output across runs

---

## 2. Data Cleaning — Hidden Traps Found

### What I asked AI to help with
Asked AI assistant to help identify hidden data quality issues in the raw CSV before building the cleaning pipeline.

### AI found correctly
- Blank `feedback_text` rows (25 rows)
- Multiple timestamp formats needing normalization

### What AI missed — caught manually by inspecting the data
| Trap | How I found it | Action taken |
|---|---|---|
| **713 duplicate messages** (same text, different ID) | Ran `Counter()` on normalized feedback text | Dedup on content hash, not ID |
| **Sarcastic high-rating rows** (e.g. rating=5, text="crashed again") | Sampled rating=5 rows manually | Flag `rating_contradiction`, trust text over rating |
| **Short meaningless messages** (94+ rows: `"meh"`, `"👍"`, `"...."`") | Checked length distribution | Drop if <15 chars AND no letters |
| **Blank timestamps should be KEPT** | AI initially suggested dropping them | Stored as NULL — feedback text is still valid |

---

## 3. TUI — Go + Bubble Tea (AI-assisted development)

### What I asked AI to do
Build a terminal UI using Go + Charm's Bubble Tea framework with:
- 4-tab navigation (Dashboard, Feedback Table, Run Pipeline, Report)
- Real-time pipeline subprocess streaming
- SQLite integration

### Where AI output was wrong / I caught mistakes

| Issue | What went wrong | How I fixed it |
|---|---|---|
| **Wrong DB path** | Binary resolved project root as `exeDir/..` (one level too high → `/home/kabil` instead of `/home/kabil/quickcart-feedback`) | Fixed to use `exeDir` directly + cwd fallback |
| **Python stdout buffering** | Pipeline ran but showed "0 lines captured" forever | Added `python -u` flag + `PYTHONUNBUFFERED=1` env var |
| **Duplicate Go method** | `runPipelineStreaming()` declared twice after refactor | Caught by compiler error, removed duplicate |
| **Missing go.sum entries** | `go build` failed with missing checksum entries | Ran `go mod tidy` to regenerate |
| **CGO dependency** | Initially used `mattn/go-sqlite3` (requires GCC/CGO) | Swapped to `modernc.org/sqlite` (pure Go, zero build issues) |
| **Popup scroll broke table keys** | `↑↓` keys scrolled table while popup was open | Added popup visibility check — intercepts all keys when popup is active |

---

## 4. Overall Assessment

### Where AI was most useful
- Writing boilerplate Go Bubble Tea code (model/update/view pattern)
- SQL query generation for filtering feedback
- Structuring the JSON prompt for consistent AI output
- Generating the project scaffolding quickly

### Where human judgment was essential
- Deciding NOT to drop blank-timestamp rows (kept as NULL instead)
- Catching sarcasm/rating contradiction edge cases
- Noticing 713 content-duplicate rows that ID-dedup would miss
- Testing that Python subprocess output was actually streaming vs appearing to stream
- Validating AI sentiment on known sarcastic examples manually

### Key principle applied
> "Never trust AI output without validation."
Every AI-enriched field is checked against a fixed enum before being stored. Invalid output triggers retry, then fallback — the pipeline never silently stores garbage.
