# Enrichpending catch-up and drain (manual)

## Normal scheduled mode (unchanged)

```bash
/opt/vplayer/backend/bin/enrichpending --limit 15 --concurrency 1 --trigger=timer
```

- Limit hard max **20** (default **15**).
- systemd timer/service stay on this form — **do not** point them at `--catch-up` or `--drain`.
- Admin “Run Enrichment” remains `--limit 15`.

## One-shot catch-up (manual)

Single bounded batch with a higher limit (max **90**):

```bash
cd /opt/vplayer
./backend/bin/enrichpending --catch-up --limit 40 --concurrency 1 --trigger=cli
```

Dry-run (single batch only):

```bash
./backend/bin/enrichpending --catch-up --limit 40 --concurrency 1 --dry-run
```

## Manual drain (explicit backlog clear)

Repeats catch-up batches under **one** advisory lock until the backlog is empty or a safety limit hits.

**Drain is NOT scheduled automatically.** Do not add a drain systemd timer.

Example:

```bash
cd /opt/vplayer
./backend/bin/enrichpending --catch-up --drain --limit 90 --concurrency 1 --max-batches 40 --max-runtime 60m --trigger=cli
```

### Safety limits

| Flag | Default | Hard bounds |
|------|---------|-------------|
| `--limit` (per batch) | 15 | 1–90 with `--catch-up` |
| `--concurrency` | 1 | 1–2 |
| request delay | from env | ≥ **1000ms** |
| `--max-batches` | 40 | 1–100 |
| `--max-runtime` | 60m | &gt;0 and ≤ **3h** |

Rules:

- `--drain` **requires** `--catch-up`.
- `--drain` **rejects** `--dry-run` (would re-select the same rows).
- Per-batch limit still capped at **90**; no parallel batches.
- Same fairness (zero-playable first, ≤3 episodes/video), source-restricted → unavailable, and `operational_runs` **per batch**.

### Stop conditions

Drain stops when any of these occurs:

1. A batch selects **0** rows → success (`stop_reason=empty`)
2. `--max-batches` reached (`max_batches`)
3. `--max-runtime` / context deadline (`max_runtime`)
4. SIGINT / SIGTERM (`interrupted`, non-zero exit)
5. Any real processing **failures** in a batch (`failures`, non-zero exit)
6. Selected rows but **zero resolved progress** (`enriched + unavailable + skipped_existing == 0`) → `no_progress`, non-zero exit

Unavailable / source_restricted counts as **progress**, not failure.

`skipped_existing` also counts as progress: it only occurs when `ApplyPlaybackEnrichment` updates 0 rows because the episode is already outside `pending_enrichment` + empty `playback_url`, so the next batch cannot select it again.

### Aggregate output

Stdout JSON includes `drain=true`, batch totals, `stop_reason`, and `elapsed` — not thousands of per-episode actions. Each batch still writes a normal `operational_runs` row (`trigger_type=cli`).
