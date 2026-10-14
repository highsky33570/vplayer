import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { afterEach, describe, it, mock } from 'node:test';
import { fileURLToPath } from 'node:url';
import { listVideosPage } from './api.ts';

const here = dirname(fileURLToPath(import.meta.url));

describe('listVideosPage exclude_category_slug', () => {
  afterEach(() => {
    mock.restoreAll();
  });

  it('sends exclude_category_slug when provided', async () => {
    let seenURL = '';
    mock.method(globalThis, 'fetch', async (input: RequestInfo | URL) => {
      seenURL = String(input);
      return new Response(
        JSON.stringify({ ok: true, data: [], total: 0, limit: 12, offset: 0, has_more: false }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    });
    await listVideosPage({ limit: 12, offset: 0, excludeCategorySlug: 'midnight' });
    assert.match(seenURL, /exclude_category_slug=midnight/);
    assert.match(seenURL, /limit=12/);
  });

  it('omits exclude_category_slug by default', async () => {
    let seenURL = '';
    mock.method(globalThis, 'fetch', async (input: RequestInfo | URL) => {
      seenURL = String(input);
      return new Response(
        JSON.stringify({ ok: true, data: [], total: 0, limit: 12, offset: 0, has_more: false }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    });
    await listVideosPage({ limit: 12, offset: 0 });
    assert.equal(seenURL.includes('exclude_category_slug'), false);
  });
});

describe('homepage midnight exclusion wiring', () => {
  it('sends exclude only on shared latest/Featured request', () => {
    const home = readFileSync(join(here, 'pages/Home.tsx'), 'utf8');
    assert.match(home, /excludeCategorySlug:\s*['"]midnight['"]/);
    // Exactly one exclude wiring site on the landing latest fetch.
    const excludeHits = home.match(/excludeCategorySlug:\s*['"]midnight['"]/g) ?? [];
    assert.equal(excludeHits.length, 1);
    assert.match(home, /homeSections\.latest\.slice\(0,\s*5\)/);
    const latestCall = home.slice(home.indexOf('Promise.all'), home.indexOf('...categoryFetches'));
    assert.match(latestCall, /excludeCategorySlug:\s*['"]midnight['"]/);
    assert.match(latestCall, /limit:\s*HOME_SECTION_LIMIT/);
  });

  it('does not send exclusion on dedicated midnight category section', () => {
    const home = readFileSync(join(here, 'pages/Home.tsx'), 'utf8');
    // Category section fetches use categoryId only.
    assert.match(
      home,
      /listVideosPage\(\{\s*categoryId:\s*cat\.id,\s*limit:\s*HOME_SECTION_LIMIT,\s*offset:\s*0\s*\}\)/,
    );
    const categoryFetchBlock = home.slice(
      home.indexOf('const categoryFetches'),
      home.indexOf('Promise.all'),
    );
    assert.equal(categoryFetchBlock.includes('excludeCategorySlug'), false);
    assert.equal(categoryFetchBlock.includes('exclude_category_slug'), false);

    // Browse/search loadMore paths must not hardcode midnight exclusion.
    const browseIdx = home.indexOf('async function loadMore');
    const browse = home.slice(browseIdx);
    assert.equal(browse.includes('excludeCategorySlug'), false);
  });
});
