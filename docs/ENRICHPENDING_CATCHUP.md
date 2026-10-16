# Enrichpending catch-up (manual)

## Normal (unchanged)

Scheduled / steady-state enrichment stays bounded and gentle:

```bash
/opt/vplayer/backend/bin/enrichpending --limit 15 --concurrency 1 --trigger=timer
```

The systemd timer/service continue to use this form. Admin “Run Enrichment” remains `--limit 15`.

## Manual catch-up

For a large `pending_enrichment` backlog (e.g. after leaf import), use an **explicit** catch-up invocation. Do **not** put catch-up on an automatic drain loop or change the 3-hour timer.

Initial recommended batch:

```bash
cd /opt/vplayer
./backend/bin/enrichpending --catch-up --limit 40 --concurrency 1 --trigger=cli
```

Optional dry-run first:

```bash
./backend/bin/enrichpending --catch-up --limit 40 --concurrency 1 --dry-run
```

### Rules

- `--catch-up` is required for `--limit` above 20 (hard max **90**).
- Each run is a **single bounded batch** (no loop-until-empty).
- Same advisory lock, fairness (zero-playable first, ≤3 episodes/video), HTTP delay ≥1000ms, concurrency ≤2.
- Re-run manually only after the previous run finishes; verify `operational_runs` and spot-check `/play` before increasing limit (e.g. 40 → 60).
- Interrupt/re-run is safe; already-enriched rows are skipped.
