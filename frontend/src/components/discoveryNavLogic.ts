import type { Category } from '../api';

export type DesktopTextLink = { label: string; href?: string; soon?: boolean; auth?: boolean };

/** Right-hand text links on the desktop discovery rail. */
export const DESKTOP_TEXT_LINKS: readonly DesktopTextLink[] = [
  { label: '历史', href: '/history', auth: true },
  { label: '收藏', href: '/favorites', auth: true },
  { label: '消息', href: '/messages', auth: true },
  { label: 'VIP', href: '/vip' },
  { label: '投稿', soon: true },
];

/** Homepage category buttons in display order (matched from API by name/slug/id). */
const CANONICAL: { names: string[]; slug: string; id: number }[] = [
  { names: ['电影'], slug: 'movie', id: 2 },
  { names: ['连续剧', '剧集'], slug: 'tv', id: 3 },
  { names: ['综艺'], slug: 'variety', id: 13 },
  { names: ['动漫'], slug: 'anime', id: 4 },
  { names: ['午夜影院'], slug: 'midnight', id: 16 },
  { names: ['VIP蓝光影院'], slug: 'vip-bluray', id: 17 },
];

/** Nav-order categories for Home sections and the discovery bar (API-matched). */
export function resolveNavCategories(api: Category[]): Category[] {
  const usable = api.filter((c) => c.slug !== 'home' && c.name !== '推荐');
  const used = new Set<number>();
  const out: Category[] = [];

  for (const entry of CANONICAL) {
    const match =
      usable.find((c) => !used.has(c.id) && entry.names.includes(c.name)) ||
      usable.find((c) => !used.has(c.id) && c.slug === entry.slug) ||
      usable.find((c) => !used.has(c.id) && c.id === entry.id);

    if (match) {
      used.add(match.id);
      out.push({
        ...match,
        name: entry.names[0], // prefer OLEHDTV label (连续剧 over 剧集)
      });
    }
  }

  return out;
}

export function findCategoryBySlug(api: Category[], slug: string): Category | undefined {
  return api.find((c) => c.slug === slug);
}

/** API-derived children of a parent, sorted by sort then id. */
export function childrenOfParent(api: Category[], parentId: number): Category[] {
  return api
    .filter((c) => c.parent_id != null && c.parent_id === parentId)
    .slice()
    .sort((a, b) => a.sort - b.sort || a.id - b.id);
}

export type DesktopSubcategory = Category & { parentName: string };

/**
 * Child categories under the canonical mains, in parent display order.
 * Top-level channels, blank names, and duplicate slugs are omitted.
 */
export function desktopSubcategories(api: Category[]): DesktopSubcategory[] {
  const seen = new Set<string>();
  const out: DesktopSubcategory[] = [];
  for (const main of resolveNavCategories(api)) {
    for (const child of childrenOfParent(api, main.id)) {
      const name = child.name.trim();
      const slug = child.slug.trim();
      if (!name || !slug || name === '推荐' || slug === 'home') continue;
      if (seen.has(slug)) continue;
      seen.add(slug);
      out.push({ ...child, name, slug, parentName: main.name });
    }
  }
  return out;
}

/**
 * Synchronize expanded parent with the active channel.
 * Preserves manual collapse when active is that same canonical main.
 */
export function syncExpandedParentSlug(
  api: Category[],
  mains: Category[],
  active: string,
  current: string | null,
): string | null {
  if (!active || active === 'home') return null;

  const activeCat = findCategoryBySlug(api, active);
  if (!activeCat) return null;

  if (activeCat.parent_id != null) {
    const parent = mains.find((m) => m.id === activeCat.parent_id);
    return parent?.slug ?? null;
  }

  if (mains.some((m) => m.slug === activeCat.slug)) {
    // Same main: keep whatever the user set (expanded or manually collapsed).
    // Different main still expanded: clear stale children.
    return current === activeCat.slug ? current : null;
  }

  return null;
}

/** Initial expansion for an active child; mains/home start collapsed. */
export function expandedParentSlugForActive(
  api: Category[],
  mains: Category[],
  active: string,
): string | null {
  return syncExpandedParentSlug(api, mains, active, null);
}

/** Expand/collapse when a main category button is clicked. */
export function toggleMainExpansion(current: string | null, mainSlug: string): string | null {
  return current === mainSlug ? null : mainSlug;
}

/** Scroll the active main chip inside the rail without moving the page vertically. */
export function activeCategoryScrollOptions(reducedMotion: boolean): ScrollIntoViewOptions {
  return {
    inline: 'center',
    block: 'nearest',
    behavior: reducedMotion ? 'auto' : 'smooth',
  };
}

/** Phone subcategory list is taller than the two-row cap. */
export function subcategoryListOverflows(
  scrollHeight: number,
  visibleHeight: number,
  tolerance = 1,
): boolean {
  if (visibleHeight <= 0) return false;
  return scrollHeight > visibleHeight + tolerance;
}

/** Flip the phone "show every child" state. */
export function toggleSubcategoryShowAll(showAll: boolean): boolean {
  return !showAll;
}
