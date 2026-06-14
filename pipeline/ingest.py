"""
pipeline/ingest.py — Stage 1: Load raw CSV
"""

import pandas as pd
from config import RAW_CSV


def load_raw(path: str = RAW_CSV) -> pd.DataFrame:
    """Load raw CSV, return DataFrame with consistent dtypes."""
    df = pd.read_csv(
        path,
        dtype=str,           # keep everything as string initially
        keep_default_na=False,
        encoding="utf-8",
        on_bad_lines="warn",
    )
    # Normalise column names
    df.columns = [c.strip().lower().replace(" ", "_") for c in df.columns]

    print(f"[ingest] Loaded {len(df):,} rows from {path}")
    return df
