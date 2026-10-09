import assert from 'node:assert/strict';
import { afterEach, describe, it, mock } from 'node:test';
import { bearerAuthHeader, getPlayback } from './api.ts';

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
});
