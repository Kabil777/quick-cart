"""
config.py — Central configuration for QuickCart Feedback Pipeline
"""

import os

# ─── NVIDIA / OpenAI-compatible API ────────────────────────────────────────
NVIDIA_API_KEY = os.getenv(
    "NVIDIA_API_KEY",
    "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"
)
NVIDIA_BASE_URL = "https://integrate.api.nvidia.com/v1"
AI_MODEL        = "openai/gpt-oss-20b"
AI_TEMPERATURE  = 0        # deterministic output
AI_MAX_TOKENS   = 256      # one JSON object per row is small
AI_BATCH_SIZE   = 10       # rows per API call
MAX_WORKERS     = 5        # concurrent threads for parallel enrichment

# ─── Paths ──────────────────────────────────────────────────────────────────
BASE_DIR    = os.path.dirname(os.path.abspath(__file__))
DATA_DIR    = os.path.join(BASE_DIR, "data")
OUTPUT_DIR  = os.path.join(BASE_DIR, "output")

RAW_CSV     = os.path.join(DATA_DIR,   "customer_feedback_raw.csv")
OUTPUT_CSV  = os.path.join(OUTPUT_DIR, "cleaned_enriched.csv")
DB_PATH     = os.path.join(OUTPUT_DIR, "feedback.db")
RUNS_DB     = os.path.join(OUTPUT_DIR, "runs.db")
REPORT_PATH = os.path.join(OUTPUT_DIR, "summary_report.md")

# ─── Domain constants ───────────────────────────────────────────────────────
VALID_SENTIMENTS = {"positive", "negative", "neutral"}
VALID_CATEGORIES = {"Billing", "App Bug", "Delivery", "Staff/Support", "Other"}
VALID_SOURCES    = {"support_ticket", "app_store_review", "survey_comment"}

MIN_TEXT_LENGTH  = 15     # drop feedback shorter than this
MAX_RETRY        = 2      # AI retry attempts on invalid output
