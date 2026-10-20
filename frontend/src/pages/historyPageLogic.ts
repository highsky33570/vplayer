import type { HistoryVideo } from '../api';

export type HistoryGroupId = 'today' | 'yesterday' | 'week' | 'earlier';

export type HistoryGroup = {
  id: HistoryGroupId;
  label: string;
  items: HistoryVideo[];
};

function startOfLocalDay(d: Date): number {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

export function historyGroupId(iso: string, now: Date = new Date()): HistoryGroupId {
  const then = new Date(iso);
  if (Number.isNaN(then.getTime())) return 'earlier';
  const dayGap = Math.round((startOfLocalDay(now) - startOfLocalDay(then)) / 86400000);
  if (dayGap <= 0) return 'today';
  if (dayGap === 1) return 'yesterday';
  if (dayGap < 7) return 'week';
  return 'earlier';
}

const GROUP_ORDER: { id: HistoryGroupId; label: string }[] = [
  { id: 'today', label: '今天' },
  { id: 'yesterday', label: '昨天' },
  { id: 'week', label: '近一周' },
  { id: 'earlier', label: '更早' },
];

/** Group history rows by relative day buckets while preserving list order within each group. */
export function groupHistoryItems(items: HistoryVideo[], now: Date = new Date()): HistoryGroup[] {
  const buckets = new Map<HistoryGroupId, HistoryVideo[]>();
  for (const item of items) {
    const id = historyGroupId(item.last_watched_at, now);
    const list = buckets.get(id) ?? [];
    list.push(item);
    buckets.set(id, list);
  }
  return GROUP_ORDER.filter((g) => (buckets.get(g.id)?.length ?? 0) > 0).map((g) => ({
    id: g.id,
    label: g.label,
    items: buckets.get(g.id) ?? [],
  }));
}

/** Client-side title filter for history search. */
export function filterHistoryByQuery(items: HistoryVideo[], raw: string): HistoryVideo[] {
  const q = raw.trim().toLowerCase();
  if (!q) return items;
  return items.filter((item) => item.title.toLowerCase().includes(q));
}

/** Progress ratio 0..1 for the thumbnail bar. */
export function historyProgressRatio(progressSec: number, durationSec: number): number {
  if (!(progressSec > 0) || !(durationSec > 0)) return 0;
  return Math.min(1, progressSec / durationSec);
}
