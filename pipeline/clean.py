"""
pipeline/clean.py — Stage 2: Clean, flag, and deduplicate

Drop policy (revised):
  - Empty/invalid RATING  → DROP immediately (no signal at all)
  - Empty feedback_text   → KEEP, flag as text_missing=True, derive sentiment from rating
  - Too short/meaningless → KEEP if rating exists, DROP only if rating is also missing
  - Duplicates            → DROP (content-based, not ID-based)
"""

import re
import pandas as pd
from dateutil import parser as dateparser
from config import MIN_TEXT_LENGTH, VALID_SOURCES

# ─── Keyword sets for contradiction detection ────────────────────────────────
_NEGATIVE_WORDS = {
    "crash", "crashed", "crashing", "wrong", "fail", "failed", "charged twice",
    "terrible", "horrible", "awful", "useless", "scam", "fraud", "refund",
    "broken", "error", "bug", "glitch", "slow", "freeze", "freezing",
    "not working", "doesn't work", "does not work", "never", "worst",
    "ridiculous", "disappointed", "unhappy", "angry", "upset", "frustrated",
    "hold", "disconnected", "ignored", "unacceptable", "overcharged", "missing",
    "late", "delayed", "spilled", "cancelled", "unauthorized", "no response",
}
_POSITIVE_WORDS = {
    "love", "loved", "great", "fantastic", "excellent", "wonderful",
    "amazing", "perfect", "awesome", "brilliant", "outstanding",
    "superb", "best", "happy", "satisfied", "thank you", "thanks",
    "helpful", "friendly", "fast", "quick", "easy", "smooth",
}


def _normalize_timestamp(ts: str) -> str | None:
    """Try to parse any timestamp format → ISO 8601 string. Returns None on failure."""
    ts = ts.strip()
    if not ts:
        return None
    try:
        return dateparser.parse(ts, dayfirst=False).strftime("%Y-%m-%d %H:%M:%S")
    except Exception:
        return None


def _has_negative_signal(text: str) -> bool:
    t = text.lower()
    return any(w in t for w in _NEGATIVE_WORDS)


def _has_positive_signal(text: str) -> bool:
    t = text.lower()
    return any(w in t for w in _POSITIVE_WORDS)


def _is_meaningful(text: str) -> bool:
    """True if text is long enough and has at least one letter."""
    stripped = text.strip()
    return len(stripped) >= MIN_TEXT_LENGTH and bool(re.search(r"[a-zA-Z]", stripped))


def _sentiment_from_rating(rating: float) -> str:
    """Derive sentiment purely from star rating when no text is available."""
    if pd.isna(rating):
        return "neutral"
    if rating <= 2:
        return "negative"
    if rating >= 4:
        return "positive"
    return "neutral"


def clean(df: pd.DataFrame) -> tuple[pd.DataFrame, pd.DataFrame]:
    """
    Clean raw DataFrame.

    New drop policy:
      - Missing rating  → DROP (no signal whatsoever)
      - Missing/short feedback_text but valid rating → KEEP, flag text_missing=True
      - Duplicate content → DROP

    Returns:
        cleaned_df  — rows ready for enrichment (some may have text_missing=True)
        dropped_df  — rows removed, with drop_reason
    """
    dropped_rows = []

    # ── 2.1  Strip whitespace ────────────────────────────────────────────────
    for col in df.columns:
        df[col] = df[col].str.strip()

    # ── 2.2  Parse rating first (we need it for the new drop rule) ───────────
    df["rating_int"] = pd.to_numeric(df["rating"], errors="coerce")

    # ── 2.3  DROP rows with missing/invalid rating ───────────────────────────
    # Rows with no rating have zero signal — text alone without rating is
    # insufficient to validate the record for business use.
    mask_no_rating = df["rating_int"].isna()
    dropped_rows.extend(
        df[mask_no_rating].assign(drop_reason="missing_rating").to_dict("records")
    )
    df = df[~mask_no_rating].copy()
    print(f"        ↳ Missing rating (dropped): {mask_no_rating.sum()}")

    # ── 2.4  Flag rows with empty/short feedback_text (KEEP them) ────────────
    df["text_missing"] = df["feedback_text"].apply(
        lambda t: t.strip() == "" or not _is_meaningful(t)
    )
    text_missing_count = df["text_missing"].sum()

    # ── 2.5  Dedup only on rows that HAVE text ───────────────────────────────
    # Rating-only rows don't participate in text dedup (their key is empty)
    has_text = ~df["text_missing"]
    df_with_text = df[has_text].copy()
    df_no_text   = df[~has_text].copy()

    df_with_text["_text_key"] = (
        df_with_text["feedback_text"].str.lower().str.strip()
        .apply(lambda t: re.sub(r"\s+", " ", t))
        .apply(lambda t: re.sub(r"[^\w\s]", "", t))
    )
    mask_dup = df_with_text.duplicated(subset=["_text_key"], keep="first")
    dropped_rows.extend(
        df_with_text[mask_dup].assign(drop_reason="duplicate_message").to_dict("records")
    )
    df_with_text = df_with_text[~mask_dup].drop(columns=["_text_key"])

    # Rejoin: text rows (deduped) + rating-only rows
    df = pd.concat([df_with_text, df_no_text], ignore_index=True)

    # ── 2.6  Normalize timestamps ────────────────────────────────────────────
    df["timestamp_normalized"] = df["timestamp"].apply(_normalize_timestamp)

    # ── 2.7  Flag missing timestamp ──────────────────────────────────────────
    df["timestamp_missing"] = df["timestamp_normalized"].isna()

    # ── 2.8  Validate source ─────────────────────────────────────────────────
    df["source"] = df["source"].str.lower().str.strip()
    df.loc[~df["source"].isin(VALID_SOURCES), "source"] = "unknown"

    # ── 2.9  Flag rating contradictions (only for rows WITH text) ────────────
    def _is_contradiction(row):
        r   = row["rating_int"]
        txt = row["feedback_text"]
        if row["text_missing"] or pd.isna(r):
            return False   # can't contradict when there's no text
        if r >= 4 and _has_negative_signal(txt) and not _has_positive_signal(txt):
            return True
        if r <= 2 and _has_positive_signal(txt) and not _has_negative_signal(txt):
            return True
        return False

    df["rating_contradiction"] = df.apply(_is_contradiction, axis=1)

    # ── Final reset ──────────────────────────────────────────────────────────
    df = df.reset_index(drop=True)
    dropped_df = pd.DataFrame(dropped_rows)

    dup_count = sum(1 for r in dropped_rows if r.get("drop_reason") == "duplicate_message")

    print(f"[clean] {len(df):,} rows kept | {len(dropped_df):,} rows dropped")
    print(f"        ↳ Missing rating (dropped) : {mask_no_rating.sum()}")
    print(f"        ↳ Duplicates (dropped)     : {dup_count}")
    print(f"        ↳ Text missing (kept/flagged): {text_missing_count}")
    print(f"        ↳ Timestamp NULL (kept)    : {df['timestamp_missing'].sum()}")
    print(f"        ↳ Rating conflicts flagged : {df['rating_contradiction'].sum()}")

    return df, dropped_df
