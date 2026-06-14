"""
main.py — QuickCart Feedback Pipeline Entry Point

Usage:
    python main.py                   # full pipeline (clean + AI + store + report)
    python main.py --skip-ai         # clean only, skip AI (no API calls)
    python main.py --dry-run         # show what would be dropped, don't save
    python main.py --report-only     # regenerate report from existing DB
"""

import argparse
import os
import sys

# Ensure project root is on path
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from config import RAW_CSV, OUTPUT_DIR
from pipeline.ingest import load_raw
from pipeline.clean  import clean
from pipeline.enrich import enrich, enrich_parallel
from pipeline.store  import init_db, store_cleaned
from pipeline.report import generate
from pipeline.runs   import start_run, complete_run, fail_run


def main():
    parser = argparse.ArgumentParser(description="QuickCart Feedback Intelligence Pipeline")
    parser.add_argument("--input",       default=RAW_CSV, help="Path to raw CSV file")
    parser.add_argument("--skip-ai",     action="store_true", help="Skip AI enrichment (test mode)")
    parser.add_argument("--dry-run",     action="store_true", help="Show cleaning stats only, don't save")
    parser.add_argument("--report-only", action="store_true", help="Regenerate report from existing DB")
    parser.add_argument("--parallel",    action="store_true", help="Use parallel AI enrichment (faster)")
    parser.add_argument("--workers",     type=int, default=5,  help="Number of parallel workers (default: 5)")
    args = parser.parse_args()

    os.makedirs(OUTPUT_DIR, exist_ok=True)

    print("=" * 60)
    print("  QuickCart Feedback Intelligence Pipeline")
    print("=" * 60)

    # ── Report-only mode ─────────────────────────────────────────────────────
    if args.report_only:
        print("\n[main] Report-only mode")
        generate()
        print("[main] Done.")
        return

    # ── Stage 1: Ingest ───────────────────────────────────────────────────────
    print("\n[Stage 1] Ingesting raw data...")
    raw_df = load_raw(args.input)

    # ── Stage 2: Clean ────────────────────────────────────────────────────────
    print("\n[Stage 2] Cleaning and flagging...")
    cleaned_df, dropped_df = clean(raw_df)

    if args.dry_run:
        print("\n[main] --dry-run mode. No data saved. Exiting.")
        print(f"       Would process {len(cleaned_df):,} rows through AI.")
        return

    # ── Start run record ──────────────────────────────────────────────────────
    mode = "parallel" if args.parallel else "sequential"
    run_id = start_run(mode=mode)

    try:

        # ── Stage 3: AI Enrichment ────────────────────────────────────────────
        mode = "parallel" if args.parallel else "sequential"
        print(f"\n[Stage 3] AI Enrichment ({mode})...")
        if args.parallel:
            enriched_df, token_usage = enrich_parallel(cleaned_df, skip_ai=args.skip_ai, max_workers=args.workers)
        else:
            enriched_df, token_usage = enrich(cleaned_df, skip_ai=args.skip_ai)

        # ── Stage 4: Store ────────────────────────────────────────────────────
        print("\n[Stage 4] Storing results...")
        init_db()
        store_cleaned(enriched_df, dropped_df)

        # ── Stage 5: Report ───────────────────────────────────────────────────
        print("\n[Stage 5] Generating report...")
        generate()

        # ── Complete run record ───────────────────────────────────────────────
        if "sentiment" in enriched_df.columns and not enriched_df.empty:
            sent = enriched_df["sentiment"].value_counts().to_dict()
        else:
            # All rows were already in DB — read counts from DB
            import sqlite3
            conn = sqlite3.connect(args.db if hasattr(args, 'db') else "output/feedback.db")
            rows = conn.execute(
                "SELECT sentiment, COUNT(*) FROM feedback GROUP BY sentiment"
            ).fetchall()
            conn.close()
            sent = {r[0]: r[1] for r in rows}
        complete_run(
            run_id=run_id,
            rows_raw=len(raw_df),
            rows_cleaned=len(cleaned_df),
            rows_dropped=len(dropped_df),
            rows_enriched=len(enriched_df),
            negative=sent.get("negative", 0),
            positive=sent.get("positive", 0),
            neutral=sent.get("neutral", 0),
            tokens_prompt=token_usage.prompt_tokens,
            tokens_completion=token_usage.completion_tokens,
            tokens_total=token_usage.total_tokens,
            api_calls=token_usage.api_calls,
            retries=token_usage.retries,
        )

    except Exception as e:
        fail_run(run_id, str(e))
        raise

    print("\n" + "=" * 60)
    print("  Pipeline complete!")
    print(f"  DB      : output/feedback.db")
    print(f"  CSV     : output/cleaned_enriched.csv")
    print(f"  Report  : output/summary_report.md")
    print("=" * 60)


if __name__ == "__main__":
    main()
