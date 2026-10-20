import type { Category, HistoryVideo } from '../api';

export type HistoryGroupId = 'today' | 'yesterday' | 'week' | 'earlier';

/** Canonical mains for history primary tabs (matched from API categories). */
const HISTORY_MAINS: { names: string[]; slug: string; id: number }[] = [
  { names: ['电影'], slug: 'movie', id: 2 },
  { names: ['连续剧', '剧集'], slug: 'tv', id: 3 },
  { names: ['综艺'], slug: 'variety', id: 13 },
  { names: ['动漫'], slug: 'anime', id: 4 },
  { names: ['午夜影院'], slug: 'midnight', id: 16 },
  { names: ['VIP蓝光影院'], slug: 'vip-bluray', id: 17 },
];

function resolveHistoryMains(api: Category[]): Category[] {
  const usable = api.filter((c) => c.slug !== 'home' && c.name !== '推荐');
  const used = new Set<number>();
  const out: Category[] = [];
  for (const entry of HISTORY_MAINS) {
    const match =
      usable.find((c) => !used.has(c.id) && entry.names.includes(c.name)) ||
      usable.find((c) => !used.has(c.id) && c.slug === entry.slug) ||
      usable.find((c) => !used.has(c.id) && c.id === entry.id);
    if (match) {
      used.add(match.id);
      out.push({ ...match, name: entry.names[0] });
    }
  }
  return out;
}

export type HistoryGroup = {
  id: HistoryGroupId;
  label: string;
  items: HistoryVideo[];
};

export type HistoryTimeFilter = 'all' | HistoryGroupId;
export type HistoryStatusFilter = 'all' | 'unfinished' | 'completed';

/** Conservative completion: need both progress + duration; ≥90% counts as done. */
export const HISTORY_COMPLETE_RATIO = 0.9;

function startOfLocalDay(d: Date): number {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

function pad2(n: number): string {
  return String(n).padStart(2, '0');
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

/** Client-side title filter for history search (over loaded pages). */
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

export function isHistoryCompleted(progressSec: number, durationSec: number): boolean {
  return historyProgressRatio(progressSec, durationSec) >= HISTORY_COMPLETE_RATIO;
}

export function filterHistoryByTime(
  items: HistoryVideo[],
  filter: HistoryTimeFilter,
  now: Date = new Date(),
): HistoryVideo[] {
  if (filter === 'all') return items;
  return items.filter((item) => historyGroupId(item.last_watched_at, now) === filter);
}

export function filterHistoryByStatus(
  items: HistoryVideo[],
  filter: HistoryStatusFilter,
): HistoryVideo[] {
  if (filter === 'all') return items;
  return items.filter((item) => {
    const done = isHistoryCompleted(item.progress_sec, item.duration_sec);
    return filter === 'completed' ? done : !done;
  });
}

/**
 * Map each category id to its canonical main slug (self or parent).
 * Used for truthful primary filters — only when category data is loaded.
 */
export function historyMainSlugByCategoryId(categories: Category[]): Map<number, string> {
  const mains = resolveHistoryMains(categories);
  const map = new Map<number, string>();
  for (const main of mains) {
    map.set(main.id, main.slug);
    for (const child of categories) {
      if (child.parent_id != null && child.parent_id === main.id) {
        map.set(child.id, main.slug);
      }
    }
  }
  return map;
}

export function filterHistoryByMainSlug(
  items: HistoryVideo[],
  slug: string | 'all',
  mainByCategoryId: Map<number, string>,
): HistoryVideo[] {
  if (slug === 'all') return items;
  return items.filter((item) => mainByCategoryId.get(item.category_id) === slug);
}

/** Primary tabs: 全部 + mains that appear in the loaded history set. */
export function historyPrimaryTabs(
  items: HistoryVideo[],
  categories: Category[],
): { id: string; label: string }[] {
  const mains = resolveHistoryMains(categories);
  const mainById = historyMainSlugByCategoryId(categories);
  const present = new Set<string>();
  for (const item of items) {
    const slug = mainById.get(item.category_id);
    if (slug) present.add(slug);
  }
  const tabs: { id: string; label: string }[] = [{ id: 'all', label: '全部' }];
  for (const main of mains) {
    if (present.has(main.slug)) tabs.push({ id: main.slug, label: main.name });
  }
  return tabs;
}

export type HistoryFilterState = {
  query: string;
  mainSlug: string;
  time: HistoryTimeFilter;
  status: HistoryStatusFilter;
};

export function applyHistoryFilters(
  items: HistoryVideo[],
  state: HistoryFilterState,
  mainByCategoryId: Map<number, string>,
  now: Date = new Date(),
): HistoryVideo[] {
  let rows = filterHistoryByQuery(items, state.query);
  rows = filterHistoryByMainSlug(rows, state.mainSlug === 'all' ? 'all' : state.mainSlug, mainByCategoryId);
  rows = filterHistoryByTime(rows, state.time, now);
  rows = filterHistoryByStatus(rows, state.status);
  return rows;
}

export function historyFiltersActive(state: HistoryFilterState): boolean {
  return (
    state.query.trim() !== '' ||
    state.mainSlug !== 'all' ||
    state.time !== 'all' ||
    state.status !== 'all'
  );
}

/** Compact card timestamp: 今天 HH:MM / 昨天 HH:MM / M-D HH:MM / YYYY-M-D. */
export function formatHistoryCardTime(iso: string, now: Date = new Date()): string {
  const then = new Date(iso);
  if (Number.isNaN(then.getTime())) return '';
  const hm = `${pad2(then.getHours())}:${pad2(then.getMinutes())}`;
  const dayGap = Math.round((startOfLocalDay(now) - startOfLocalDay(then)) / 86400000);
  if (dayGap <= 0) return `今天 ${hm}`;
  if (dayGap === 1) return `昨天 ${hm}`;
  const md = `${then.getMonth() + 1}-${pad2(then.getDate())} ${hm}`;
  if (then.getFullYear() === now.getFullYear()) return md;
  return `${then.getFullYear()}-${md}`;
}

/** Thumbnail overlay clock when both progress and duration exist. */
export function formatHistoryProgressClock(progressSec: number, durationSec: number): string {
  if (!(progressSec > 0) || !(durationSec > 0)) return '';
  const fmt = (sec: number) => {
    const total = Math.max(0, Math.floor(sec));
    const h = Math.floor(total / 3600);
    const m = Math.floor((total % 3600) / 60);
    const s = total % 60;
    return h > 0
      ? `${h}:${pad2(m)}:${pad2(s)}`
      : `${pad2(m)}:${pad2(s)}`;
  };
  return `${fmt(progressSec)} / ${fmt(durationSec)}`;
}

export const HISTORY_TIME_FILTER_OPTIONS: { id: HistoryTimeFilter; label: string }[] = [
  { id: 'all', label: '全部时间' },
  { id: 'today', label: '今天' },
  { id: 'yesterday', label: '昨天' },
  { id: 'week', label: '近一周' },
  { id: 'earlier', label: '更早' },
];

export const HISTORY_STATUS_FILTER_OPTIONS: { id: HistoryStatusFilter; label: string }[] = [
  { id: 'all', label: '全部状态' },
  { id: 'unfinished', label: '未看完' },
  { id: 'completed', label: '已看完' },
];
