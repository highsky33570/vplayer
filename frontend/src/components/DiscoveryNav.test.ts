import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import type { Category } from '../api.ts';
import {
  activeCategoryScrollOptions,
  CATEGORY_RAIL_SCROLL_FRACTION,
  categoryRailScrollByOptions,
  categoryRailScrollEdges,
  childrenOfParent,
  expandedParentSlugForActive,
  resolveNavCategories,
  syncExpandedParentSlug,
  toggleMainExpansion,
} from './discoveryNavLogic.ts';

const here = dirname(fileURLToPath(import.meta.url));

function cat(partial: Partial<Category> & Pick<Category, 'id' | 'name' | 'slug'>): Category {
  return {
    sort: 0,
    parent_id: null,
    source_id: null,
    ...partial,
  };
}

/** Representative OLEHDTV-shaped API payload (ids are illustrative; hierarchy is parent_id). */
const API: Category[] = [
  cat({ id: 2, name: '电影', slug: 'movie', sort: 2, source_id: '1' }),
  cat({ id: 3, name: '连续剧', slug: 'tv', sort: 3, source_id: '2' }),
  cat({ id: 13, name: '综艺', slug: 'variety', sort: 4, source_id: '3' }),
  cat({ id: 4, name: '动漫', slug: 'anime', sort: 5, source_id: '4' }),
  cat({ id: 16, name: '午夜影院', slug: 'midnight', sort: 6, source_id: '16' }),
  cat({ id: 17, name: 'VIP蓝光影院', slug: 'vip-bluray', sort: 7, source_id: '17' }),
  // Unrelated source top-levels must never enter canonical main nav
  cat({ id: 8, name: '伦理片', slug: 'type-8', sort: 8, source_id: '8' }),
  cat({ id: 14, name: '纪录片', slug: 'type-14', sort: 14, source_id: '14' }),
  // Movie children (parent_id → 电影)
  cat({ id: 101, name: '动作片', slug: 'type-101', sort: 20, parent_id: 2, source_id: '101' }),
  cat({ id: 102, name: '喜剧片', slug: 'type-102', sort: 10, parent_id: 2, source_id: '102' }),
  cat({ id: 103, name: '爱情片', slug: 'type-103', sort: 10, parent_id: 2, source_id: '103' }),
  // TV child
  cat({ id: 201, name: '国产剧', slug: 'type-201', sort: 1, parent_id: 3, source_id: '201' }),
];

describe('resolveNavCategories', () => {
  it('keeps only the six canonical mains in display order', () => {
    const mains = resolveNavCategories(API);
    assert.deepEqual(
      mains.map((c) => c.name),
      ['电影', '连续剧', '综艺', '动漫', '午夜影院', 'VIP蓝光影院'],
    );
  });

  it('does not admit unrelated source top-level categories', () => {
    const mains = resolveNavCategories(API);
    const ids = new Set(mains.map((c) => c.id));
    assert.equal(ids.has(8), false);
    assert.equal(ids.has(14), false);
    assert.equal(
      mains.some((c) => c.slug === 'type-8' || c.slug === 'type-14'),
      false,
    );
  });
});

describe('childrenOfParent', () => {
  it('derives movie children from API parent_id and sorts by sort then id', () => {
    const kids = childrenOfParent(API, 2);
    assert.deepEqual(
      kids.map((c) => c.slug),
      ['type-102', 'type-103', 'type-101'],
    );
    assert.ok(kids.every((c) => c.parent_id === 2));
  });

  it('returns empty when parent has no children', () => {
    assert.deepEqual(childrenOfParent(API, 16), []);
  });
});

describe('toggleMainExpansion', () => {
  it('expands a closed main and collapses the same main', () => {
    assert.equal(toggleMainExpansion(null, 'movie'), 'movie');
    assert.equal(toggleMainExpansion('movie', 'movie'), null);
  });

  it('switching parent replaces the expanded slug', () => {
    assert.equal(toggleMainExpansion('movie', 'tv'), 'tv');
  });
});

describe('syncExpandedParentSlug', () => {
  const mains = resolveNavCategories(API);

  it('clears expansion on home', () => {
    assert.equal(syncExpandedParentSlug(API, mains, 'home', 'tv'), null);
  });

  it('expands the correct parent for an active child', () => {
    assert.equal(syncExpandedParentSlug(API, mains, 'type-101', null), 'movie');
    assert.equal(syncExpandedParentSlug(API, mains, 'type-201', 'movie'), 'tv');
  });

  it('history/navigation from TV child to movie main clears TV expansion', () => {
    assert.equal(syncExpandedParentSlug(API, mains, 'movie', 'tv'), null);
  });

  it('switching child under one parent to child under another expands the new parent', () => {
    assert.equal(syncExpandedParentSlug(API, mains, 'type-201', 'movie'), 'tv');
    assert.equal(syncExpandedParentSlug(API, mains, 'type-101', 'tv'), 'movie');
  });

  it('manual collapse of the currently active main stays collapsed', () => {
    assert.equal(syncExpandedParentSlug(API, mains, 'movie', null), null);
  });

  it('preserves expansion when active main is the currently expanded parent', () => {
    assert.equal(syncExpandedParentSlug(API, mains, 'movie', 'movie'), 'movie');
  });

  it('clears expansion for unknown or non-canonical active slugs', () => {
    assert.equal(syncExpandedParentSlug(API, mains, 'no-such-channel', 'movie'), null);
    assert.equal(syncExpandedParentSlug(API, mains, 'type-8', 'movie'), null);
  });
});

describe('expandedParentSlugForActive', () => {
  it('expands the parent when reloading a child channel', () => {
    const mains = resolveNavCategories(API);
    assert.equal(expandedParentSlugForActive(API, mains, 'type-101'), 'movie');
    assert.equal(expandedParentSlugForActive(API, mains, 'type-201'), 'tv');
  });

  it('does not force-expand on home or main-only channels', () => {
    const mains = resolveNavCategories(API);
    assert.equal(expandedParentSlugForActive(API, mains, 'home'), null);
    assert.equal(expandedParentSlugForActive(API, mains, 'movie'), null);
  });
});

describe('DiscoveryNav interaction wiring', () => {
  it('child click keeps parent expanded and selects child slug', () => {
    const src = readFileSync(join(here, 'DiscoveryNav.tsx'), 'utf8');
    assert.match(src, /function handleChild\(child: Category, parent: Category\)/);
    assert.match(src, /setExpandedParentSlug\(parent\.slug\)/);
    assert.match(src, /onSelect\(child\.slug\)/);
  });

  it('main click uses toggle + sync path without forcing reopen', () => {
    const src = readFileSync(join(here, 'DiscoveryNav.tsx'), 'utf8');
    assert.match(src, /setExpandedParentSlug\(toggleMainExpansion\(expandedParentSlug, main\.slug\)\)/);
    assert.match(src, /onSelect\(main\.slug\)/);
    assert.match(src, /syncExpandedParentSlug\(categories, mains, active, current\)/);
    assert.match(src, /aria-expanded=\{kids\.length > 0 \? expanded : undefined\}/);
  });

  it('renders a semantic nav rail without WAI-ARIA tab roles', () => {
    const src = readFileSync(join(here, 'DiscoveryNav.tsx'), 'utf8');
    assert.match(src, /<nav className="category-rail" aria-label="分类导航">/);
    assert.match(src, /category-children\$\{expanded \? ' is-open' : ''\}/);
    assert.match(src, /kids\.map/);
    assert.match(src, /tabIndex=\{expanded \? 0 : -1\}/);
    assert.match(src, /aria-hidden=\{!expanded\}/);
    assert.equal(src.includes('category-grid'), false);
    assert.equal(src.includes('role="tablist"'), false);
    assert.equal(src.includes('role="tab"'), false);
    assert.equal(src.includes('aria-selected'), false);
  });
});

describe('active category scroll', () => {
  it('centers the chip horizontally and does not jump the page', () => {
    const options = activeCategoryScrollOptions(false);
    assert.equal(options.inline, 'center');
    assert.equal(options.block, 'nearest');
    assert.equal(options.behavior, 'smooth');
  });

  it('uses auto behavior when reduced motion is requested', () => {
    assert.equal(activeCategoryScrollOptions(true).behavior, 'auto');
  });

  it('DiscoveryNav scrolls only the active main chip with that helper', () => {
    const src = readFileSync(join(here, 'DiscoveryNav.tsx'), 'utf8');
    assert.match(src, /activeCategoryScrollOptions\(reduced\)/);
    assert.match(src, /scrollIntoView\(activeCategoryScrollOptions\(reduced\)\)/);
    assert.match(src, /prefers-reduced-motion: reduce/);
    assert.match(src, /\.category-btn--main\.is-active, \.category-btn--main\.is-parent-of-active/);
    assert.equal(src.match(/const \[expandedParentSlug, setExpandedParentSlug\] = useState/g)?.length, 1);
    assert.equal(src.match(/toggleMainExpansion\(/g)?.length, 1);
  });
});

describe('mobile and desktop category presentation', () => {
  it('keeps one expanded-parent source and a separate phone subcategory row', () => {
    const src = readFileSync(join(here, 'DiscoveryNav.tsx'), 'utf8');
    assert.match(src, /const \[expandedParentSlug, setExpandedParentSlug\]/);
    assert.match(src, /category-subrow/);
    assert.match(src, /category-children\$\{expanded \? ' is-open' : ''\}/);
    assert.match(src, /aria-expanded=\{kids\.length > 0 \? expanded : undefined\}/);
    assert.match(src, /handleChild\(child, expandedMain\)/);
    assert.match(src, /handleChild\(child, main\)/);
    assert.equal(src.includes('function toggleMainExpansion'), false);
  });

  it('phone rail does not wrap, keeps the wordmark, and uses a 2-column grid', () => {
    const css = readFileSync(join(here, '../styles.css'), 'utf8');
    const phone = css.slice(css.indexOf('/* bp:phone'), css.indexOf('/* bp:phone-narrow'));
    assert.match(phone, /\.category-rail\s*\{[^}]*flex-wrap:\s*nowrap/);
    assert.match(phone, /\.category-children\s*\{[^}]*display:\s*none/);
    assert.match(phone, /\.category-subrow\s*\{[^}]*display:\s*grid/);
    assert.match(phone, /\.category-btn--main\s*\{[^}]*height:\s*44px/);
    assert.match(phone, /\.brand-text\s*\{[^}]*display:\s*inline/);
    assert.match(phone, /repeat\(2,\s*minmax\(0,\s*1fr\)\)/);
    assert.equal(/\.brand-text\s*\{[^}]*display:\s*none/.test(css), false);
    assert.match(css, /\.category-subrow\s*\{[^}]*display:\s*none/);
    const tablet = css.slice(css.indexOf('/* bp:tablet'), css.indexOf('/* bp:phone'));
    assert.match(tablet, /repeat\(3,\s*minmax\(0,\s*1fr\)\)/);
    assert.match(tablet, /--banner-h:\s*76px/);
    assert.match(phone, /--banner-h:\s*96px/);
  });
});

describe('main category rail arrows', () => {
  it('steps about 70% of the visible rail and honors reduced motion', () => {
    assert.ok(CATEGORY_RAIL_SCROLL_FRACTION >= 0.6 && CATEGORY_RAIL_SCROLL_FRACTION <= 0.75);
    const forward = categoryRailScrollByOptions(200, 1, false);
    const back = categoryRailScrollByOptions(200, -1, true);
    assert.equal(forward.left, 140);
    assert.equal(forward.behavior, 'smooth');
    assert.equal(back.left, -140);
    assert.equal(back.behavior, 'auto');
    assert.equal(categoryRailScrollByOptions(200, 1, true).behavior, 'auto');
    assert.equal(categoryRailScrollByOptions(200, -1, false).behavior, 'smooth');
  });

  it('hides the left control at the start and the right control at the end', () => {
    assert.deepEqual(categoryRailScrollEdges({ scrollLeft: 0, clientWidth: 300, scrollWidth: 900 }), {
      canScrollLeft: false,
      canScrollRight: true,
    });
    assert.deepEqual(categoryRailScrollEdges({ scrollLeft: 2, clientWidth: 300, scrollWidth: 900 }), {
      canScrollLeft: false,
      canScrollRight: true,
    });
    assert.deepEqual(categoryRailScrollEdges({ scrollLeft: 600, clientWidth: 300, scrollWidth: 900 }), {
      canScrollLeft: true,
      canScrollRight: false,
    });
    assert.deepEqual(categoryRailScrollEdges({ scrollLeft: 598, clientWidth: 300, scrollWidth: 900 }), {
      canScrollLeft: true,
      canScrollRight: false,
    });
    assert.deepEqual(categoryRailScrollEdges({ scrollLeft: 0, clientWidth: 400, scrollWidth: 400 }), {
      canScrollLeft: false,
      canScrollRight: false,
    });
  });

  it('wires previous/next controls outside the scrolling rail', () => {
    const src = readFileSync(join(here, 'DiscoveryNav.tsx'), 'utf8');
    const prev = src.indexOf('aria-label="上一组分类"');
    const nav = src.indexOf('<nav className="category-rail" aria-label="分类导航">');
    const next = src.indexOf('aria-label="下一组分类"');
    assert.ok(prev !== -1 && nav !== -1 && next !== -1);
    assert.ok(prev < nav && nav < next);
    assert.match(src, /className="category-nav-mobile"/);
    assert.match(src, /hidden=\{!railEdges\.canScrollLeft\}/);
    assert.match(src, /disabled=\{!railEdges\.canScrollLeft\}/);
    assert.match(src, /hidden=\{!railEdges\.canScrollRight\}/);
    assert.match(src, /disabled=\{!railEdges\.canScrollRight\}/);
    assert.match(src, /onClick=\{\(\) => scrollRail\(-1\)\}/);
    assert.match(src, /onClick=\{\(\) => scrollRail\(1\)\}/);
    assert.match(src, /railRef\.current/);
    assert.match(src, /rail\.scrollBy\(categoryRailScrollByOptions\(rail\.clientWidth, direction, reduced\)\)/);
    assert.match(src, /scrollIntoView\(activeCategoryScrollOptions\(reduced\)\)/);
    assert.match(src, /addEventListener\('scroll', measure/);
    assert.match(src, /addEventListener\('resize', measure\)/);
    assert.match(src, /new ResizeObserver\(measure\)/);

    const css = readFileSync(join(here, '../styles.css'), 'utf8');
    const beforePhone = css.slice(0, css.indexOf('/* bp:phone'));
    const phone = css.slice(css.indexOf('/* bp:phone'), css.indexOf('/* bp:phone-narrow'));
    assert.match(beforePhone, /\.category-rail-arrow\s*\{[^}]*display:\s*none/);
    assert.match(beforePhone, /\.category-nav-mobile\s*\{[^}]*display:\s*contents/);
    assert.match(phone, /\.category-nav-mobile\s*\{[^}]*display:\s*flex/);
    assert.match(phone, /\.category-nav-mobile\s*\{[^}]*align-items:\s*center/);
    assert.match(phone, /\.category-rail-arrow\s*\{[^}]*width:\s*40px/);
    assert.match(phone, /\.category-rail-arrow\s*\{[^}]*height:\s*44px/);
    assert.match(phone, /\.category-rail-arrow\[hidden\]\s*\{[^}]*display:\s*none/);
    const railBlock = phone.match(/\.category-rail\s*\{[^}]*\}/)?.[0] ?? '';
    assert.match(railBlock, /flex-wrap:\s*nowrap/);
    assert.equal(railBlock.includes('mask-image'), false);
    assert.match(phone, /\.banner-actions\s*\{[^}]*display:\s*flex/);
    assert.match(phone, /\.banner-actions\s*\{[^}]*align-items:\s*center/);
    assert.match(phone, /\.banner-actions\s*\{[^}]*height:\s*52px/);
    assert.match(phone, /\.banner-actions \.banner-user,\s*\.banner-actions \.theme-toggle\s*\{[^}]*justify-content:\s*center/);
  });
});

describe('Home browse still resolves Category from full API list', () => {
  it('passes active category id from complete cats list to list API', () => {
    const home = readFileSync(join(here, '../pages/Home.tsx'), 'utf8');
    assert.match(home, /const activeCat = cats\.find\(\(c\) => c\.slug === active\)/);
    assert.match(home, /categoryId = active === 'home' \|\| searchQ \? undefined : activeCat\?\.id/);
  });
});
