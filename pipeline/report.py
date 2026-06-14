"""
pipeline/report.py — Stage 5: Generate Markdown summary report
"""

import sqlite3
import pandas as pd
from datetime import datetime
from config import DB_PATH, REPORT_PATH


def _bar(value: int, total: int, width: int = 30) -> str:
    filled = int(width * value / total) if total else 0
    return "█" * filled + "░" * (width - filled)


def generate(db_path: str = DB_PATH, report_path: str = REPORT_PATH):
    conn = sqlite3.connect(db_path)
    df = pd.read_sql("SELECT * FROM feedback", conn)
    dropped = pd.read_sql("SELECT * FROM dropped_rows", conn)
    conn.close()

    if df.empty:
        print("[report] No data found in DB.")
        return

    total = len(df)
    lines = []

    # ── Header ────────────────────────────────────────────────────────────────
    lines += [
        "# QuickCart Customer Feedback — Intelligence Report",
        f"> Generated: {datetime.utcnow().strftime('%Y-%m-%d %H:%M UTC')}",
        "",
        "---",
        "",
    ]

    # ── 1. Dataset Summary ────────────────────────────────────────────────────
    lines += [
        "## 1. Dataset Summary",
        "",
        f"| Metric | Count |",
        f"|---|---|",
        f"| Raw rows loaded | {total + len(dropped):,} |",
        f"| Rows dropped (blank text) | {len(dropped[dropped.drop_reason=='blank_feedback_text'])} |",
        f"| Rows dropped (too short) | {len(dropped[dropped.drop_reason=='too_short_or_meaningless'])} |",
        f"| Rows dropped (duplicates) | {len(dropped[dropped.drop_reason=='duplicate_message'])} |",
        f"| **Rows processed by AI** | **{total:,}** |",
        f"| Missing timestamps (kept) | {df['timestamp_missing'].sum()} |",
        f"| Rating contradictions flagged | {df['rating_contradiction'].sum()} |",
        "",
    ]

    # ── 2. Sentiment Breakdown ────────────────────────────────────────────────
    sentiment_counts = df["sentiment"].value_counts()
    lines += [
        "## 2. Sentiment Breakdown",
        "",
        f"| Sentiment | Count | % | Bar |",
        f"|---|---|---|---|",
    ]
    for s in ["negative", "neutral", "positive"]:
        cnt = sentiment_counts.get(s, 0)
        pct = cnt / total * 100
        lines.append(f"| {s.capitalize()} | {cnt} | {pct:.1f}% | {_bar(cnt, total)} |")
    lines.append("")

    # ── 3. Top 5 Categories by Volume ─────────────────────────────────────────
    cat_counts = df["category"].value_counts().head(5)
    lines += [
        "## 3. Top 5 Complaint Categories",
        "",
        f"| Rank | Category | Count | % | Bar |",
        f"|---|---|---|---|---|",
    ]
    for rank, (cat, cnt) in enumerate(cat_counts.items(), 1):
        pct = cnt / total * 100
        lines.append(f"| {rank} | {cat} | {cnt} | {pct:.1f}% | {_bar(cnt, total)} |")
    lines.append("")

    # ── 4. Representative Examples (2-3 per top category) ─────────────────────
    lines += [
        "## 4. Representative Examples per Top Category",
        "",
    ]
    for cat in cat_counts.index:
        subset = df[(df["category"] == cat) & (df["sentiment"] == "negative")]
        if subset.empty:
            subset = df[df["category"] == cat]
        examples = subset.sample(min(3, len(subset)), random_state=42)
        lines += [f"### {cat}", ""]
        for _, row in examples.iterrows():
            lines += [
                f"- **Summary**: {row['summary']}",
                f"  > *\"{row['feedback_text'][:120]}{'...' if len(row['feedback_text'])>120 else ''}\"*",
                "",
            ]

    # ── 5. Rating Contradiction Alerts ────────────────────────────────────────
    contradictions = df[df["rating_contradiction"] == 1]
    lines += [
        "## 5. Rating Contradiction Alerts",
        "",
        f"**{len(contradictions)} rows** had ratings that contradict the message text.",
        "These were classified by text (not rating).",
        "",
    ]
    if not contradictions.empty:
        lines += ["| ID | Rating | Sentiment (AI) | Feedback (truncated) |", "|---|---|---|---|"]
        for _, row in contradictions.head(5).iterrows():
            txt = row["feedback_text"][:80].replace("|", "/")
            lines.append(f"| {row['id']} | ⭐{int(row['rating_int']) if pd.notna(row.get('rating_int')) else '?'} | {row['sentiment']} | {txt}... |")
    lines.append("")

    # ── Write file ─────────────────────────────────────────────────────────────
    with open(report_path, "w", encoding="utf-8") as f:
        f.write("\n".join(lines))

    print(f"[report] Summary report written to: {report_path}")
