import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import type { Category } from '../api.ts';
import {
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
    assert.match(src, /\{expanded &&\s*kids\.map/);
    assert.equal(src.includes('category-grid'), false);
    assert.equal(src.includes('role="tablist"'), false);
    assert.equal(src.includes('role="tab"'), false);
    assert.equal(src.includes('aria-selected'), false);
  });
});

describe('Home browse still resolves Category from full API list', () => {
  it('passes active category id from complete cats list to list API', () => {
    const home = readFileSync(join(here, '../pages/Home.tsx'), 'utf8');
    assert.match(home, /const activeCat = cats\.find\(\(c\) => c\.slug === active\)/);
    assert.match(home, /categoryId = active === 'home' \|\| searchQ \? undefined : activeCat\?\.id/);
  });
});
