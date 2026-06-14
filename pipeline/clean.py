"""
pipeline/clean.py — Stage 2: Clean, flag, and deduplicate
"""

import re
import pandas as pd
from dateutil import parser as dateparser
from config import MIN_TEXT_LENGTH, VALID_SOURCES

# ─── Strongly negative/positive keyword sets for contradiction detection ────
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


def clean(df: pd.DataFrame) -> tuple[pd.DataFrame, pd.DataFrame]:
    """
    Clean raw DataFrame.
    Returns:
        cleaned_df  — rows ready for AI enrichment
        dropped_df  — rows that were removed, with reason
    """
    dropped_rows = []

    # ── 2.1  Strip whitespace ────────────────────────────────────────────────
    for col in df.columns:
        df[col] = df[col].str.strip()

    # ── 2.2  Drop rows with blank feedback_text ─────────────────────────────
    mask_blank = df["feedback_text"] == ""
    dropped_rows.extend(
        df[mask_blank].assign(drop_reason="blank_feedback_text").to_dict("records")
    )
    df = df[~mask_blank].copy()

    # ── 2.3  Drop very short / meaningless messages ─────────────────────────
    mask_short = ~df["feedback_text"].apply(_is_meaningful)
    dropped_rows.extend(
        df[mask_short].assign(drop_reason="too_short_or_meaningless").to_dict("records")
    )
    df = df[~mask_short].copy()

    # ── 2.4  Normalize text key for dedup ───────────────────────────────────
    df["_text_key"] = df["feedback_text"].str.lower().str.strip()
    df["_text_key"] = df["_text_key"].apply(
        lambda t: re.sub(r"\s+", " ", t)          # collapse whitespace
    )
    df["_text_key"] = df["_text_key"].apply(
        lambda t: re.sub(r"[^\w\s]", "", t)       # remove punctuation for matching
    )

    # ── 2.5  Dedup on normalized text (keep first occurrence) ───────────────
    mask_dup = df.duplicated(subset=["_text_key"], keep="first")
    dropped_rows.extend(
        df[mask_dup].assign(drop_reason="duplicate_message").to_dict("records")
    )
    df = df[~mask_dup].copy()
    df = df.drop(columns=["_text_key"])

    # ── 2.6  Normalize timestamps ────────────────────────────────────────────
    df["timestamp_normalized"] = df["timestamp"].apply(_normalize_timestamp)

    # ── 2.7  Flag missing timestamp ──────────────────────────────────────────
    df["timestamp_missing"] = df["timestamp_normalized"].isna()

    # ── 2.8  Validate source ─────────────────────────────────────────────────
    df["source"] = df["source"].str.lower().str.strip()
    df.loc[~df["source"].isin(VALID_SOURCES), "source"] = "unknown"

    # ── 2.9  Parse rating safely ─────────────────────────────────────────────
    df["rating_int"] = pd.to_numeric(df["rating"], errors="coerce")

    # ── 2.10 Flag rating contradictions ─────────────────────────────────────
    def _is_contradiction(row):
        r = row["rating_int"]
        txt = row["feedback_text"]
        if pd.isna(r):
            return False
        # High rating but clearly negative text
        if r >= 4 and _has_negative_signal(txt) and not _has_positive_signal(txt):
            return True
        # Low rating but clearly positive text
        if r <= 2 and _has_positive_signal(txt) and not _has_negative_signal(txt):
            return True
        return False

    df["rating_contradiction"] = df.apply(_is_contradiction, axis=1)

    # ── Final reset ──────────────────────────────────────────────────────────
    df = df.reset_index(drop=True)
    dropped_df = pd.DataFrame(dropped_rows)

    print(f"[clean] {len(df):,} rows kept | {len(dropped_df):,} rows dropped")
    print(f"        ↳ Blank text     : {sum(1 for r in dropped_rows if r.get('drop_reason')=='blank_feedback_text')}")
    print(f"        ↳ Too short      : {sum(1 for r in dropped_rows if r.get('drop_reason')=='too_short_or_meaningless')}")
    print(f"        ↳ Duplicates     : {sum(1 for r in dropped_rows if r.get('drop_reason')=='duplicate_message')}")
    print(f"        ↳ Timestamp NULL : {df['timestamp_missing'].sum()}")
    print(f"        ↳ Rating conflicts: {df['rating_contradiction'].sum()}")

    return df, dropped_df
