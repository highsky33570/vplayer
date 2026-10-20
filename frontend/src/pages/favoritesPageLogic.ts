import type { Category, FavoriteVideo } from '../api';
import type { FolderItem } from '../components/headerFlyoutData';

export type FavoritesSort = 'recent' | 'oldest';

/** Canonical mains for optional category filter on favorites. */
const FAV_MAINS: { names: string[]; slug: string; id: number }[] = [
  { names: ['电影'], slug: 'movie', id: 2 },
  { names: ['连续剧', '剧集'], slug: 'tv', id: 3 },
  { names: ['综艺'], slug: 'variety', id: 13 },
  { names: ['动漫'], slug: 'anime', id: 4 },
  { names: ['午夜影院'], slug: 'midnight', id: 16 },
  { names: ['VIP蓝光影院'], slug: 'vip-bluray', id: 17 },
];

function resolveFavMains(api: Category[]): Category[] {
  const usable = api.filter((c) => c.slug !== 'home' && c.name !== '推荐');
  const used = new Set<number>();
  const out: Category[] = [];
  for (const entry of FAV_MAINS) {
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

export function favoritesMainSlugByCategoryId(categories: Category[]): Map<number, string> {
  const mains = resolveFavMains(categories);
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

/** Category options that appear in the loaded favorites set. */
export function favoritesCategoryOptions(
  items: FavoriteVideo[],
  categories: Category[],
): { id: string; label: string }[] {
  const mains = resolveFavMains(categories);
  const map = favoritesMainSlugByCategoryId(categories);
  const present = new Set<string>();
  for (const item of items) {
    const slug = map.get(item.category_id);
    if (slug) present.add(slug);
  }
  const out: { id: string; label: string }[] = [{ id: 'all', label: '全部' }];
  for (const main of mains) {
    if (present.has(main.slug)) out.push({ id: main.slug, label: main.name });
  }
  return out;
}

export function filterFavoritesByQuery(items: FavoriteVideo[], raw: string): FavoriteVideo[] {
  const q = raw.trim().toLowerCase();
  if (!q) return items;
  return items.filter((item) => item.title.toLowerCase().includes(q));
}

export function filterFavoritesByMainSlug(
  items: FavoriteVideo[],
  slug: string,
  mainByCategoryId: Map<number, string>,
): FavoriteVideo[] {
  if (slug === 'all') return items;
  return items.filter((item) => mainByCategoryId.get(item.category_id) === slug);
}

/** API returns newest first; oldest reverses the loaded page order by favorited_at. */
export function sortFavorites(items: FavoriteVideo[], sort: FavoritesSort): FavoriteVideo[] {
  if (sort === 'recent') return items;
  return items
    .slice()
    .sort((a, b) => {
      const ta = new Date(a.favorited_at).getTime();
      const tb = new Date(b.favorited_at).getTime();
      if (Number.isNaN(ta) || Number.isNaN(tb)) return 0;
      return ta - tb;
    });
}

export function applyFavoritesView(
  items: FavoriteVideo[],
  opts: { query: string; categorySlug: string; sort: FavoritesSort },
  mainByCategoryId: Map<number, string>,
): FavoriteVideo[] {
  let rows = filterFavoritesByQuery(items, opts.query);
  rows = filterFavoritesByMainSlug(rows, opts.categorySlug, mainByCategoryId);
  return sortFavorites(rows, opts.sort);
}

/** Prefer default folder id; fall back to first folder or 0 (all). */
export function initialFavoriteFolderId(folders: FolderItem[]): number {
  const def = folders.find((f) => f.isDefault && f.id > 0);
  if (def) return def.id;
  const first = folders.find((f) => f.id > 0);
  return first?.id ?? 0;
}

export function folderPreviewCover(items: FavoriteVideo[]): string {
  return items.find((v) => (v.cover_url ?? '').trim())?.cover_url ?? '';
}

export const FAVORITES_SORT_OPTIONS: { id: FavoritesSort; label: string }[] = [
  { id: 'recent', label: '最近收藏' },
  { id: 'oldest', label: '最早收藏' },
];

/** Personal nav for favorites — only real VPlayer destinations. */
export const FAVORITES_PERSONAL_NAV = [
  { id: 'account', label: '主页', href: '/account' },
  { id: 'favorites', label: '我的收藏', href: '/favorites' },
  { id: 'history', label: '观看历史', href: '/history' },
  { id: 'messages', label: '消息', href: '/messages' },
] as const;
