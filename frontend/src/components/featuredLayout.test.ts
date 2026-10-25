import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import {
  FEATURED_CAROUSEL_LIMIT,
  FEATURED_POOL_LIMIT,
  FEATURED_SIDE_LIMIT,
  splitFeaturedVideos,
} from './featuredLayout.ts';

const here = dirname(fileURLToPath(import.meta.url));

function videos(count: number) {
  return Array.from({ length: count }, (_, i) => ({ id: i + 1 }));
}

describe('splitFeaturedVideos', () => {
  it('keeps five hero slides and six distinct side cards', () => {
    assert.equal(FEATURED_CAROUSEL_LIMIT, 5);
    assert.equal(FEATURED_SIDE_LIMIT, 6);
    assert.equal(FEATURED_POOL_LIMIT, 11);
    const { carousel, side } = splitFeaturedVideos(videos(11));
    assert.deepEqual(carousel.map((v) => v.id), [1, 2, 3, 4, 5]);
    assert.deepEqual(side.map((v) => v.id), [6, 7, 8, 9, 10, 11]);
    const overlap = carousel.filter((v) => side.some((s) => s.id === v.id));
    assert.equal(overlap.length, 0);
  });

  it('does not duplicate videos when fewer than six side items exist', () => {
    const { carousel, side } = splitFeaturedVideos(videos(8));
    assert.equal(carousel.length, 5);
    assert.deepEqual(side.map((v) => v.id), [6, 7, 8]);
    const short = splitFeaturedVideos(videos(4));
    assert.equal(short.carousel.length, 4);
    assert.deepEqual(short.side, []);
  });
});

describe('desktop featured and catalog density', () => {
  it('wires six secondary cards without changing the hero carousel cap', () => {
    const grid = readFileSync(join(here, 'FeaturedGrid.tsx'), 'utf8');
    const carousel = readFileSync(join(here, 'FeaturedCarousel.tsx'), 'utf8');
    const home = readFileSync(join(here, '../pages/Home.tsx'), 'utf8');
    assert.match(grid, /splitFeaturedVideos\(videos\)/);
    assert.equal(grid.includes('slice(0, 4)'), false);
    assert.match(carousel, /videos\.slice\(0,\s*5\)/);
    assert.match(carousel, /setInterval/);
    assert.match(home, /GRID_COLS_DESKTOP = 5/);
    assert.match(home, /latest\.slice\(0,\s*HOME_SECTION_LIMIT\)/);
    assert.match(home, /latest\.slice\(0,\s*FEATURED_POOL_LIMIT\)/);
  });

  it('uses a 3×2 featured side and 5 catalog columns from 1280px up', () => {
    const css = readFileSync(join(here, '../styles.css'), 'utf8');
    const desktop = css.slice(css.indexOf('/* ——— Featured grid'), css.indexOf('/* ——— Responsive'));
    assert.match(
      desktop,
      /\.featured-grid\s*\{[^}]*grid-template-columns:\s*minmax\(0,\s*5fr\)\s*minmax\(0,\s*7fr\)/,
    );
    assert.match(desktop, /\.featured-side\s*\{[^}]*repeat\(3,\s*minmax\(0,\s*1fr\)\)/);
    assert.match(desktop, /\.featured-side \.video-card \.thumb\s*\{[^}]*aspect-ratio:\s*16\s*\/\s*9/);
    assert.match(desktop, /\.video-grid\s*\{[^}]*repeat\(5,\s*minmax\(0,\s*1fr\)\)/);
    assert.match(desktop, /\.carousel\s*\{[^}]*width:\s*100%/);
    assert.match(desktop, /\.carousel\s*\{[^}]*min-width:\s*0/);
    assert.match(desktop, /\.carousel\s*\{[^}]*max-height:\s*none/);
    const mid = css.slice(css.indexOf('/* bp:desk-mid'), css.indexOf('/* bp:compact'));
    assert.match(mid, /\.video-grid\s*\{[^}]*repeat\(4,\s*minmax\(0,\s*1fr\)\)/);
    assert.match(mid, /\.featured-side\s*\{[^}]*repeat\(2,\s*minmax\(0,\s*1fr\)\)/);
    assert.match(mid, /\.carousel\s*\{[^}]*aspect-ratio:\s*16\s*\/\s*10/);
    assert.equal(/max-height:\s*\d+px/.test(mid.match(/\.carousel\s*\{[^}]*\}/)?.[0] ?? ''), false);
    const tablet = css.slice(css.indexOf('/* bp:tablet'), css.indexOf('/* bp:phone'));
    assert.match(tablet, /\.video-grid\s*\{[^}]*repeat\(3,\s*minmax\(0,\s*1fr\)\)/);
    const phone = css.slice(css.indexOf('/* bp:phone'), css.indexOf('/* bp:phone-narrow'));
    assert.match(phone, /\.video-grid\s*\{[^}]*repeat\(2,\s*minmax\(0,\s*1fr\)\)/);
    assert.match(phone, /\.category-rail\s*\{[^}]*flex-wrap:\s*nowrap/);
    assert.match(phone, /\.category-rail\s*\{[^}]*overflow-x:\s*auto/);
  });
});
