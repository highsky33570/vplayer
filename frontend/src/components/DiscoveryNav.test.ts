import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import type { Category } from '../api.ts';
import {
  activeCategoryScrollOptions,
  childrenOfParent,
  desktopSubcategories,
  expandedParentSlugForActive,
  resolveNavCategories,
  subcategoryListOverflows,
  syncExpandedParentSlug,
  toggleMainExpansion,
  toggleSubcategoryShowAll,
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

describe('desktopSubcategories', () => {
  it('lists children of canonical mains in parent order and skips unrelated top-levels', () => {
    const subs = desktopSubcategories(API);
    assert.deepEqual(
      subs.map((c) => c.slug),
      ['type-102', 'type-103', 'type-101', 'type-201'],
    );
    assert.deepEqual(
      subs.map((c) => c.parentName),
      ['电影', '电影', '电影', '连续剧'],
    );
    assert.equal(
      subs.some((c) => c.slug === 'movie' || c.slug === 'type-8' || c.slug === 'type-14'),
      false,
    );
  });

  it('drops blank names and duplicate slugs', () => {
    const api = [
      ...API,
      cat({ id: 104, name: '  ', slug: 'type-blank', parent_id: 2 }),
      cat({ id: 105, name: '推荐', slug: 'home', parent_id: 2, sort: 0 }),
      cat({ id: 106, name: '喜剧片', slug: 'type-102', parent_id: 2, sort: 1 }),
    ];
    const slugs = desktopSubcategories(api).map((c) => c.slug);
    assert.equal(slugs.filter((slug) => slug === 'type-102').length, 1);
    assert.equal(slugs.includes('type-blank'), false);
    assert.equal(slugs.includes('home'), false);
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
    assert.match(src, /max-width: 767px/);
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

  it('wraps phone categories without a horizontal scroller and keeps desktop inline children', () => {
    const css = readFileSync(join(here, '../styles.css'), 'utf8');
    const src = readFileSync(join(here, 'DiscoveryNav.tsx'), 'utf8');
    const phone = css.slice(css.indexOf('/* bp:phone'), css.indexOf('/* bp:phone-narrow'));
    const railBlock = phone.match(/\.category-rail\s*\{[^}]*\}/)?.[0] ?? '';
    const subBlock = phone.match(/\.category-subrow-scroll\s*\{[^}]*\}/)?.[0] ?? '';
    assert.match(railBlock, /flex-wrap:\s*wrap/);
    assert.equal(/overflow-x:\s*auto/.test(railBlock), false);
    assert.match(railBlock, /overflow-x:\s*visible/);
    assert.match(subBlock, /flex-wrap:\s*wrap/);
    assert.equal(/overflow-x:\s*auto/.test(subBlock), false);
    assert.match(subBlock, /max-height:\s*var\(--subrow-cap\)/);
    assert.match(phone, /\.category-children\s*\{[^}]*display:\s*none/);
    assert.match(phone, /\.category-subrow\s*\{[^}]*display:\s*grid/);
    assert.match(phone, /\.category-btn--main\s*\{[^}]*height:\s*44px/);
    assert.match(phone, /\.category-btn--main\s*\{[^}]*max-width:\s*none/);
    assert.match(phone, /\.brand-text\s*\{[^}]*display:\s*inline/);
    assert.match(phone, /repeat\(2,\s*minmax\(0,\s*1fr\)\)/);
    assert.equal(/\.brand-text\s*\{[^}]*display:\s*none/.test(css), false);
    assert.match(css, /\.category-subrow\s*\{[^}]*display:\s*none/);
    assert.match(src, /resolveNavCategories\(categories\)/);
    assert.match(src, /<span className="category-label">首页<\/span>/);
    assert.match(src, /mains\.map\(\(main\)/);
    const tablet = css.slice(css.indexOf('/* bp:tablet'), css.indexOf('/* bp:phone'));
    assert.match(tablet, /repeat\(3,\s*minmax\(0,\s*1fr\)\)/);
    assert.match(tablet, /--banner-h:\s*76px/);
    assert.match(tablet, /\.category-rail\s*\{[^}]*flex-wrap:\s*nowrap/);
    assert.match(tablet, /\.category-rail\s*\{[^}]*overflow-x:\s*auto/);
    assert.match(phone, /--banner-h:\s*96px/);
    const desktop = css.slice(0, css.indexOf('/* bp:compact'));
    assert.match(desktop, /\.category-rail\s*\{[^}]*flex-wrap:\s*nowrap/);
    assert.match(desktop, /\.category-children\s*\{[^}]*display:\s*grid/);
    assert.equal(src.includes('上一组分类'), false);
    assert.equal(src.includes('下一组分类'), false);
    assert.equal(src.includes('scrollBy'), false);
    assert.equal(src.includes('category-rail-arrow'), false);
    assert.equal(src.includes('canScrollLeft'), false);
    assert.equal(css.includes('category-rail-arrow'), false);
    assert.equal(css.includes('category-nav-mobile'), false);
  });
});

describe('phone subcategory show-all', () => {
  it('toggles between the two-row cap and the full child list', () => {
    assert.equal(toggleSubcategoryShowAll(false), true);
    assert.equal(toggleSubcategoryShowAll(true), false);
  });

  it('treats a list as overflowing only when it is taller than the visible cap', () => {
    assert.equal(subcategoryListOverflows(200, 86), true);
    assert.equal(subcategoryListOverflows(86, 86), false);
    assert.equal(subcategoryListOverflows(87, 86), false);
    assert.equal(subcategoryListOverflows(40, 0), false);
  });

  it('resets show-all when the parent changes and keeps aria-expanded', () => {
    const src = readFileSync(join(here, 'DiscoveryNav.tsx'), 'utf8');
    assert.match(src, /showAllParent !== expandedParentSlug/);
    assert.match(src, /setShowAllChildren\(false\)/);
    assert.match(src, /toggleSubcategoryShowAll\(current\)/);
    assert.match(src, /更多 ▾/);
    assert.match(src, /收起 ▴/);
    assert.match(src, /aria-expanded=\{showAllChildren\}/);
    assert.match(src, /aria-expanded=\{kids\.length > 0 \? expanded : undefined\}/);
    assert.match(src, /category-children\$\{expanded \? ' is-open' : ''\}/);
    assert.match(src, /subcategoryListOverflows\(row\.scrollHeight, visible\)/);
  });
});

describe('Home browse still resolves Category from full API list', () => {
  it('passes active category id from complete cats list to list API', () => {
    const home = readFileSync(join(here, '../pages/Home.tsx'), 'utf8');
    assert.match(home, /const activeCat = cats\.find\(\(c\) => c\.slug === active\)/);
    assert.match(home, /categoryId = active === 'home' \|\| searchQ \? undefined : activeCat\?\.id/);
  });
});
