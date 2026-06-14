"""
pipeline/enrich.py — Stage 3: AI enrichment via NVIDIA / OpenAI-compatible API
Supports sequential (default) and parallel (ThreadPoolExecutor) modes.

Every run always calls the AI for ALL rows with text — no DB-skip logic.
Rows with text_missing=True (rating-only) skip AI and derive sentiment from rating.
"""

import json
import time
import re
import threading
from dataclasses import dataclass, field
import pandas as pd
from openai import OpenAI
from concurrent.futures import ThreadPoolExecutor, as_completed
from config import (
    NVIDIA_API_KEY, NVIDIA_BASE_URL, AI_MODEL,
    AI_TEMPERATURE, AI_MAX_TOKENS, AI_BATCH_SIZE,
    VALID_SENTIMENTS, VALID_CATEGORIES, MAX_RETRY, MAX_WORKERS,
)

_client_local = threading.local()  # thread-safe client per thread


def _sentiment_from_rating(rating) -> str:
    """Derive sentiment from star rating when no feedback text is available."""
    import math
    if rating is None or (isinstance(rating, float) and math.isnan(rating)):
        return "neutral"
    r = float(rating)
    if r <= 2:
        return "negative"
    if r >= 4:
        return "positive"
    return "neutral"


@dataclass
class TokenUsage:
    """Accumulates token counts across all API calls in a run."""
    prompt_tokens:     int = 0
    completion_tokens: int = 0
    total_tokens:      int = 0
    api_calls:         int = 0
    retries:           int = 0
    _lock: object = field(default_factory=threading.Lock, repr=False, compare=False)

    def add(self, usage):
        """Thread-safe update from an API response usage object."""
        if usage is None:
            return
        with self._lock:
            self.prompt_tokens     += getattr(usage, 'prompt_tokens', 0) or 0
            self.completion_tokens += getattr(usage, 'completion_tokens', 0) or 0
            self.total_tokens      += getattr(usage, 'total_tokens', 0) or 0
            self.api_calls         += 1

    def summary(self) -> str:
        return (
            f"API calls: {self.api_calls} | "
            f"Prompt: {self.prompt_tokens:,} tkns | "
            f"Completion: {self.completion_tokens:,} tkns | "
            f"Total: {self.total_tokens:,} tkns | "
            f"Retries: {self.retries}"
        )


def _get_client() -> OpenAI:
    if not hasattr(_client_local, "client"):
        _client_local.client = OpenAI(base_url=NVIDIA_BASE_URL, api_key=NVIDIA_API_KEY)
    return _client_local.client


SYSTEM_PROMPT = """You are a customer feedback classifier for QuickCart, a food and grocery delivery app.

For each feedback message provided, return a JSON array where each element corresponds to one message.
Each element must have exactly these three fields:
  - "sentiment": one of ["positive", "negative", "neutral"]
  - "category":  one of ["Billing", "App Bug", "Delivery", "Staff/Support", "Other"]
  - "summary":   a single sentence (max 20 words) in plain English describing the core issue or praise

Rules:
1. Base sentiment ONLY on the message TEXT, never on the star rating.
2. Sarcasm and irony ("oh great, it crashed again") count as NEGATIVE.
3. If unsure between two categories, pick the more specific one.
4. If none of Billing/App Bug/Delivery/Staff/Support fits well, use "Other".
5. Return ONLY a valid JSON array. No explanations, no markdown, no extra text.
"""


def _build_user_prompt(batch: list[dict]) -> str:
    lines = []
    for i, row in enumerate(batch, 1):
        lines.append(f'{i}. "{row["feedback_text"]}"')
    return (
        "Classify the following feedback messages.\n"
        "Return a JSON array with one object per message, in the same order.\n\n"
        + "\n".join(lines)
    )


def _extract_json(text: str) -> list:
    text = re.sub(r"```(?:json)?", "", text).strip().rstrip("`").strip()
    return json.loads(text)


def _validate_result(result: dict) -> bool:
    return (
        isinstance(result, dict)
        and result.get("sentiment") in VALID_SENTIMENTS
        and result.get("category") in VALID_CATEGORIES
        and isinstance(result.get("summary"), str)
        and len(result.get("summary", "").strip()) > 0
    )


def _call_ai(batch: list[dict], attempt: int = 0, token_usage: TokenUsage = None) -> list[dict]:
    """Call AI for a batch; retry up to MAX_RETRY on bad output. Thread-safe."""
    client = _get_client()
    try:
        completion = client.chat.completions.create(
            model=AI_MODEL,
            messages=[
                {"role": "system", "content": SYSTEM_PROMPT},
                {"role": "user",   "content": _build_user_prompt(batch)},
            ],
            temperature=AI_TEMPERATURE,
            max_tokens=AI_MAX_TOKENS * len(batch),
            stream=False,
        )
        if token_usage is not None:
            token_usage.add(getattr(completion, 'usage', None))

        raw = completion.choices[0].message.content
        results = _extract_json(raw)

        if len(results) != len(batch):
            raise ValueError(f"Expected {len(batch)} results, got {len(results)}")
        for r in results:
            if not _validate_result(r):
                raise ValueError(f"Invalid result: {r}")
        return results

    except Exception as e:
        if attempt < MAX_RETRY:
            if token_usage is not None:
                with token_usage._lock:
                    token_usage.retries += 1
            time.sleep(2 ** attempt)
            return _call_ai(batch, attempt + 1, token_usage)
        else:
            return [
                {"sentiment": "neutral", "category": "Other", "summary": "Could not classify this message."}
                for _ in batch
            ]


def _split_text_and_rating_only(df: pd.DataFrame):
    """Split df into rows with text (go to AI) and rating-only rows (derive from star)."""
    mask_no_text = df.get("text_missing", pd.Series(False, index=df.index)).fillna(False).astype(bool)
    df_ai      = df[~mask_no_text].copy()
    df_no_text = df[mask_no_text].copy()
    if len(df_no_text) > 0:
        print(f"[enrich] Rating-only rows (no text): {len(df_no_text)} — deriving sentiment from star rating")
        df_no_text["sentiment"] = df_no_text["rating_int"].apply(_sentiment_from_rating)
        df_no_text["category"]  = "Other"
        df_no_text["summary"]   = "Rating-only feedback (no text provided)."
    return df_ai, df_no_text


def _attach_results(df: pd.DataFrame, sentiments: list, categories: list, summaries: list) -> pd.DataFrame:
    df = df.copy()
    df["sentiment"] = sentiments
    df["category"]  = categories
    df["summary"]   = summaries
    print(f"[enrich] Sentiment breakdown:")
    for s, cnt in df["sentiment"].value_counts().items():
        print(f"         {s}: {cnt}")
    return df


# ── Sequential mode (safe default) ──────────────────────────────────────────
def enrich(df: pd.DataFrame, skip_ai: bool = False) -> tuple[pd.DataFrame, TokenUsage]:
    """
    Process all rows sequentially through AI.
    Rows with text_missing=True skip AI — sentiment derived from star rating instead.
    Always calls AI fresh — does NOT use previously stored results.
    """
    token_usage = TokenUsage()

    if skip_ai:
        print("[enrich] --skip-ai mode: filling with placeholders")
        df = df.copy()
        df["sentiment"] = "neutral"
        df["category"]  = "Other"
        df["summary"]   = "AI enrichment skipped."
        return df, token_usage

    # Split text rows (go to AI) from rating-only rows (derive from star)
    df_ai, df_no_text = _split_text_and_rating_only(df)

    rows  = df_ai.to_dict("records")
    total = len(rows)

    if total == 0:
        print("[enrich] No text rows — all are rating-only.")
        return pd.concat([df_ai, df_no_text], ignore_index=True), token_usage

    print(f"[enrich] Sequential mode — {total:,} rows in batches of {AI_BATCH_SIZE}...")

    sentiments, categories, summaries = [], [], []
    for start in range(0, total, AI_BATCH_SIZE):
        batch = rows[start : start + AI_BATCH_SIZE]
        results = _call_ai(batch, token_usage=token_usage)
        for r in results:
            sentiments.append(r["sentiment"])
            categories.append(r["category"])
            summaries.append(r["summary"])
        done = min(start + AI_BATCH_SIZE, total)
        print(f"  ↳ {done}/{total} rows enriched  [{token_usage.total_tokens:,} tokens used]", end="\r")
        time.sleep(0.3)

    print()
    print(f"[enrich] Token summary: {token_usage.summary()}")

    enriched_ai = _attach_results(df_ai, sentiments, categories, summaries)
    return pd.concat([enriched_ai, df_no_text], ignore_index=True), token_usage


# ── Parallel mode (faster) ───────────────────────────────────────────────────
def enrich_parallel(df: pd.DataFrame, skip_ai: bool = False, max_workers: int = MAX_WORKERS) -> tuple[pd.DataFrame, TokenUsage]:
    """
    Process all rows concurrently through AI.
    Rows with text_missing=True skip AI — sentiment derived from star rating instead.
    Always calls AI fresh — does NOT use previously stored results.
    """
    token_usage = TokenUsage()

    if skip_ai:
        print("[enrich] --skip-ai mode: filling with placeholders")
        df = df.copy()
        df["sentiment"] = "neutral"
        df["category"]  = "Other"
        df["summary"]   = "AI enrichment skipped."
        return df, token_usage

    # Split text rows (go to AI) from rating-only rows (derive from star)
    df_ai, df_no_text = _split_text_and_rating_only(df)

    rows    = df_ai.to_dict("records")
    total   = len(rows)
    batches = [rows[i : i + AI_BATCH_SIZE] for i in range(0, total, AI_BATCH_SIZE)]

    if total == 0:
        print("[enrich] No text rows — all are rating-only.")
        return pd.concat([df_ai, df_no_text], ignore_index=True), token_usage

    print(f"[enrich] Parallel mode — {total:,} rows | {len(batches)} batches | {max_workers} workers")

    results_map = {}
    completed_count = 0
    lock = threading.Lock()

    with ThreadPoolExecutor(max_workers=max_workers) as executor:
        future_to_idx = {
            executor.submit(_call_ai, batch, 0, token_usage): idx
            for idx, batch in enumerate(batches)
        }
        for future in as_completed(future_to_idx):
            idx = future_to_idx[future]
            results_map[idx] = future.result()
            with lock:
                completed_count += len(results_map[idx])
                print(f"  ↳ {completed_count}/{total} rows enriched  [{token_usage.total_tokens:,} tokens]", end="\r")

    print()
    print(f"[enrich] Token summary: {token_usage.summary()}")

    sentiments, categories, summaries = [], [], []
    for idx in sorted(results_map.keys()):
        for r in results_map[idx]:
            sentiments.append(r["sentiment"])
            categories.append(r["category"])
            summaries.append(r["summary"])

    enriched_ai = _attach_results(df_ai, sentiments, categories, summaries)
    return pd.concat([enriched_ai, df_no_text], ignore_index=True), token_usage
