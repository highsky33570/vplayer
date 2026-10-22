import assert from 'node:assert/strict';
import { afterEach, describe, it, mock } from 'node:test';
import { ApiError, bearerAuthHeader, getPlayback, listEpisodes, listRelated } from './api.ts';

describe('bearerAuthHeader', () => {
  it('sets Bearer token', () => {
    assert.deepEqual(bearerAuthHeader('abc'), { Authorization: 'Bearer abc' });
    assert.deepEqual(bearerAuthHeader(null), {});
  });
});

describe('getPlayback auth header', () => {
  afterEach(() => {
    mock.restoreAll();
  });

  it('sends Authorization Bearer when token is provided', async () => {
    let seenAuth = '';
    mock.method(globalThis, 'fetch', async (_input: RequestInfo | URL, init?: RequestInit) => {
      const headers = new Headers(init?.headers);
      seenAuth = headers.get('Authorization') ?? '';
      return new Response(
        JSON.stringify({
          ok: true,
          data: { type: 'hls', url: 'https://cdn.example/a.m3u8', source: 'demo', sid: 1, nid: 1 },
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    });

    const info = await getPlayback(9, 0, 0, 'tok-123');
    assert.equal(seenAuth, 'Bearer tok-123');
    assert.equal(info.url, 'https://cdn.example/a.m3u8');
  });

  it('surfaces playback_unavailable with safe message', async () => {
    mock.method(globalThis, 'fetch', async () => {
      return new Response(
        JSON.stringify({
          ok: false,
          error: 'playback_unavailable',
          message: 'This video is currently unavailable for playback.',
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    });
    await assert.rejects(
      () => getPlayback(9870, 0, 0, 'tok'),
      (e: unknown) => {
        assert.ok(e instanceof ApiError);
        assert.equal(e.code, 'playback_unavailable');
        assert.equal(e.message, 'This video is currently unavailable for playback.');
        return true;
      },
    );
  });
});

describe('catalog list null payloads (video 10056 regression)', () => {
  afterEach(() => {
    mock.restoreAll();
  });

  it('listEpisodes normalizes data:null to []', async () => {
    mock.method(globalThis, 'fetch', async () => {
      return new Response(JSON.stringify({ ok: true, data: null }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    });
    const rows = await listEpisodes(10056);
    assert.deepEqual(rows, []);
    assert.equal(rows.length, 0);
  });

  it('listRelated normalizes data:null to []', async () => {
    mock.method(globalThis, 'fetch', async () => {
      return new Response(JSON.stringify({ ok: true, data: null }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    });
    assert.deepEqual(await listRelated(10056), []);
  });

  it('listEpisodes keeps a real episode array', async () => {
    mock.method(globalThis, 'fetch', async () => {
      return new Response(
        JSON.stringify({ ok: true, data: [{ sid: 1, nid: 1, title: '第1集' }] }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    });
    const rows = await listEpisodes(10077);
    assert.equal(rows.length, 1);
    assert.equal(rows[0]?.title, '第1集');
  });
});
