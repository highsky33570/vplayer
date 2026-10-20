import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { afterEach, describe, it, mock } from 'node:test';
import { fileURLToPath } from 'node:url';
import {
  clearHoverPreviewCache,
  resolveHoverPreviewMedia,
} from './hoverPreviewResolve.ts';

const here = dirname(fileURLToPath(import.meta.url));

describe('resolveHoverPreviewMedia', () => {
  afterEach(() => {
    mock.restoreAll();
    clearHoverPreviewCache();
  });

  it('returns null without a token (fail closed)', async () => {
    let called = 0;
    mock.method(globalThis, 'fetch', async () => {
      called += 1;
      return new Response('{}', { status: 200 });
    });
    assert.equal(await resolveHoverPreviewMedia(1, null), null);
    assert.equal(called, 0);
  });

  it('uses GET /videos/:id/play with default episode (sid/nid omitted)', async () => {
    let url = '';
    let auth = '';
    mock.method(globalThis, 'fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
      url = String(input);
      auth = new Headers(init?.headers).get('Authorization') ?? '';
      return new Response(
        JSON.stringify({
          ok: true,
          data: { type: 'hls', url: 'https://cdn.example/p.m3u8', source: 'r2', sid: 1, nid: 1 },
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    });
    const media = await resolveHoverPreviewMedia(42, 'tok-abc');
    assert.ok(media);
    assert.equal(media!.url, 'https://cdn.example/p.m3u8');
    assert.match(url, /\/api\/v1\/videos\/42\/play$/);
    assert.equal(auth, 'Bearer tok-abc');
  });

  it('caches successful resolution and does not re-fetch', async () => {
    let calls = 0;
    mock.method(globalThis, 'fetch', async () => {
      calls += 1;
      return new Response(
        JSON.stringify({
          ok: true,
          data: { type: 'url', url: 'https://cdn.example/a.mp4', source: 'x', sid: 1, nid: 1 },
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    });
    await resolveHoverPreviewMedia(7, 'tok');
    await resolveHoverPreviewMedia(7, 'tok');
    assert.equal(calls, 1);
  });

  it('caches failures so the same card does not retry while hovering', async () => {
    let calls = 0;
    mock.method(globalThis, 'fetch', async () => {
      calls += 1;
      return new Response(
        JSON.stringify({ ok: false, error: 'playback_unavailable', message: 'unavailable' }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    });
    assert.equal(await resolveHoverPreviewMedia(9, 'tok'), null);
    assert.equal(await resolveHoverPreviewMedia(9, 'tok'), null);
    assert.equal(calls, 1);
  });

  it('never imports history or favorite side-effect APIs', () => {
    const src = readFileSync(join(here, 'hoverPreviewResolve.ts'), 'utf8');
    const attach = readFileSync(join(here, 'attachHoverPreview.ts'), 'utf8');
    const logic = readFileSync(join(here, 'hoverPreviewLogic.ts'), 'utf8');
    const coord = readFileSync(join(here, 'hoverPreviewCoordinator.ts'), 'utf8');
    const thumb = readFileSync(join(here, '../components/HoverPreviewThumb.tsx'), 'utf8');
    const bundle = [src, attach, logic, coord, thumb].join('\n');
    assert.equal(bundle.includes('upsertHistory'), false);
    assert.equal(bundle.includes('addFavorite'), false);
    assert.equal(bundle.includes('removeFavorite'), false);
    assert.equal(bundle.includes('/me/history'), false);
    assert.equal(bundle.includes('progress_sec'), false);
  });
});
