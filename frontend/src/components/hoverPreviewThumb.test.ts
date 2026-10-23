/**
 * Hover preview behavior: delay, mute, one-at-a-time, cleanup, no history.
 */
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { afterEach, beforeEach, describe, it, mock } from 'node:test';
import { fileURLToPath } from 'node:url';
import { JSDOM } from 'jsdom';
import React, { createElement } from 'react';
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import { AuthTestProvider } from '../auth/AuthContext.tsx';
import {
  clearHoverPreviewCache,
} from '../media/hoverPreviewResolve.ts';
import {
  resetHoverPreviewCoordinator,
  getActiveHoverPreviewOwnerId,
} from '../media/hoverPreviewCoordinator.ts';
import { HOVER_PREVIEW_DELAY_MS } from '../media/hoverPreviewLogic.ts';
import { HoverPreviewThumb } from './HoverPreviewThumb.tsx';
import { VideoCard } from './VideoCard.tsx';

(globalThis as { React?: typeof React }).React = React;
(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const here = dirname(fileURLToPath(import.meta.url));
const TEST_USER = {
  id: 7,
  email: 'preview@vplayer.test',
  nickname: '预览',
  is_admin: false,
  avatar_hue: 120,
};

let dom: JSDOM;
let root: Root;
let playCalls = 0;
let fetchUrls: string[] = [];
let historyPosts = 0;

function installDom(hoverCapable = true) {
  dom = new JSDOM('<!DOCTYPE html><html><body><div id="root"></div></body></html>', {
    url: 'http://localhost/',
    pretendToBeVisual: true,
  });
  const win = dom.window;
  Object.defineProperty(globalThis, 'window', { value: win, configurable: true });
  Object.defineProperty(globalThis, 'document', { value: win.document, configurable: true });
  Object.defineProperty(globalThis, 'navigator', { value: win.navigator, configurable: true });
  Object.defineProperty(globalThis, 'HTMLElement', { value: win.HTMLElement, configurable: true });
  Object.defineProperty(globalThis, 'HTMLVideoElement', {
    value: win.HTMLVideoElement,
    configurable: true,
  });
  Object.defineProperty(globalThis, 'Node', { value: win.Node, configurable: true });
  Object.defineProperty(globalThis, 'MutationObserver', {
    value: win.MutationObserver,
    configurable: true,
  });
  Object.defineProperty(globalThis, 'getComputedStyle', {
    value: win.getComputedStyle.bind(win),
    configurable: true,
  });
  Object.defineProperty(globalThis, 'localStorage', {
    value: win.localStorage,
    configurable: true,
  });
  win.matchMedia = ((query: string) => ({
    matches: hoverCapable
      ? String(query).includes('hover: hover') && String(query).includes('pointer: fine')
      : false,
    media: query,
    onchange: null,
    addListener() {},
    removeListener() {},
    addEventListener() {},
    removeEventListener() {},
    dispatchEvent() {
      return false;
    },
  })) as typeof win.matchMedia;

  win.HTMLVideoElement.prototype.play = function play() {
    playCalls += 1;
    queueMicrotask(() => {
      this.dispatchEvent(new win.Event('loadedmetadata'));
      this.dispatchEvent(new win.Event('playing'));
    });
    return Promise.resolve();
  };
  win.HTMLVideoElement.prototype.pause = function pause() {
    /* no-op */
  };
  win.HTMLVideoElement.prototype.load = function load() {
    /* jsdom stub */
  };
  Object.defineProperty(win.HTMLVideoElement.prototype, 'duration', {
    configurable: true,
    get() {
      return 600;
    },
  });
  Object.defineProperty(win.HTMLVideoElement.prototype, 'readyState', {
    configurable: true,
    get() {
      return 1;
    },
  });
}

function mountThumbs(ids: number[], token: string | null = 'tok-preview', authRequired = true) {
  const mount = document.getElementById('root')!;
  root = createRoot(mount);
  act(() => {
    root.render(
      createElement(
        MemoryRouter,
        null,
        createElement(
          AuthTestProvider,
          {
            user: TEST_USER,
            token: token ?? '',
            authRequired,
            children: createElement(
              'div',
              null,
              ...ids.map((id) =>
                createElement(HoverPreviewThumb, {
                  key: id,
                  videoId: id,
                  coverUrl: `/c${id}.jpg`,
                  durationSec: 600,
                  'data-testid': `thumb-${id}`,
                }),
              ),
            ),
          },
        ),
      ),
    );
  });
}

/** React synthesizes enter/leave from over/out — dispatch those. */
function mouseEnter(el: Element) {
  el.dispatchEvent(
    new dom.window.MouseEvent('mouseover', { bubbles: true, relatedTarget: document.body }),
  );
}

function mouseLeave(el: Element) {
  el.dispatchEvent(
    new dom.window.MouseEvent('mouseout', { bubbles: true, relatedTarget: document.body }),
  );
}

describe('HoverPreviewThumb', () => {
  beforeEach(() => {
    playCalls = 0;
    fetchUrls = [];
    historyPosts = 0;
    clearHoverPreviewCache();
    resetHoverPreviewCoordinator();
    installDom(true);
    mock.method(globalThis, 'fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      fetchUrls.push(url);
      if (url.includes('/me/history')) {
        historyPosts += 1;
        return new Response(JSON.stringify({ ok: true, data: {} }), { status: 200 });
      }
      if (url.includes('/play')) {
        const idMatch = url.match(/videos\/(\d+)\/play/);
        const id = idMatch?.[1] ?? '0';
        if (id === '99') {
          return new Response(
            JSON.stringify({ ok: false, error: 'playback_unavailable', message: 'no' }),
            { status: 200 },
          );
        }
        return new Response(
          JSON.stringify({
            ok: true,
            data: {
              type: 'url',
              url: `https://cdn.example/${id}.mp4`,
              source: 'demo',
              sid: 1,
              nid: 1,
            },
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        );
      }
      return new Response(JSON.stringify({ ok: true, data: {} }), { status: 200 });
    });
  });

  afterEach(() => {
    act(() => root?.unmount());
    mock.restoreAll();
    resetHoverPreviewCoordinator();
    clearHoverPreviewCache();
    dom.window.close();
  });

  it('shows poster initially', () => {
    mountThumbs([1]);
    const thumb = document.querySelector('[data-testid="thumb-1"]')!;
    assert.equal(thumb.getAttribute('data-preview-phase'), 'idle');
    const poster = thumb.querySelector('.hover-preview-poster') as HTMLImageElement;
    assert.ok(poster);
    assert.ok(!poster.classList.contains('is-faded'));
    const video = thumb.querySelector('video') as HTMLVideoElement;
    assert.equal(video.muted, true);
    assert.equal(video.playsInline, true);
    assert.equal(video.controls, false);
  });

  it('short hover does not resolve or play preview', async () => {
    mountThumbs([1]);
    const thumb = document.querySelector('[data-testid="thumb-1"]')!;
    await act(async () => {
      mouseEnter(thumb);
    });
    assert.equal(thumb.getAttribute('data-preview-phase'), 'pending');
    await act(async () => {
      mouseLeave(thumb);
    });
    assert.equal(fetchUrls.filter((u) => u.includes('/play')).length, 0);
    assert.equal(playCalls, 0);
    assert.equal(thumb.getAttribute('data-preview-phase'), 'idle');
  });

  it('sustained hover activates muted playsInline preview', async () => {
    mountThumbs([1]);
    const thumb = document.querySelector('[data-testid="thumb-1"]')!;
    await act(async () => {
      mouseEnter(thumb);
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, HOVER_PREVIEW_DELAY_MS + 30));
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, 40));
    });
    assert.ok(fetchUrls.some((u) => u.includes('/videos/1/play')));
    assert.ok(playCalls >= 1);
    const video = thumb.querySelector('video') as HTMLVideoElement;
    assert.equal(video.muted, true);
    assert.equal(video.playsInline, true);
    assert.equal(thumb.getAttribute('data-preview-phase'), 'playing');
    assert.ok(thumb.querySelector('.hover-preview-poster')!.classList.contains('is-faded'));
  });

  it('mouse leave stops preview and restores poster', async () => {
    mountThumbs([1]);
    const thumb = document.querySelector('[data-testid="thumb-1"]')!;
    await act(async () => {
      mouseEnter(thumb);
      await new Promise((r) => setTimeout(r, HOVER_PREVIEW_DELAY_MS + 40));
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, 40));
    });
    assert.equal(thumb.getAttribute('data-preview-phase'), 'playing');
    await act(async () => {
      mouseLeave(thumb);
    });
    assert.equal(thumb.getAttribute('data-preview-phase'), 'idle');
    assert.ok(!thumb.querySelector('.hover-preview-poster')!.classList.contains('is-faded'));
    assert.equal(getActiveHoverPreviewOwnerId(), null);
  });

  it('switching A -> B stops A; only one preview active', async () => {
    mountThumbs([1, 2]);
    const a = document.querySelector('[data-testid="thumb-1"]')!;
    const b = document.querySelector('[data-testid="thumb-2"]')!;
    await act(async () => {
      mouseEnter(a);
      await new Promise((r) => setTimeout(r, HOVER_PREVIEW_DELAY_MS + 40));
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, 40));
    });
    assert.equal(a.getAttribute('data-preview-phase'), 'playing');
    await act(async () => {
      mouseLeave(a);
      mouseEnter(b);
      await new Promise((r) => setTimeout(r, HOVER_PREVIEW_DELAY_MS + 40));
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, 40));
    });
    assert.equal(a.getAttribute('data-preview-phase'), 'idle');
    assert.equal(b.getAttribute('data-preview-phase'), 'playing');
  });

  it('failed media stays poster-only without retry spam', async () => {
    mountThumbs([99]);
    const thumb = document.querySelector('[data-testid="thumb-99"]')!;
    await act(async () => {
      mouseEnter(thumb);
      await new Promise((r) => setTimeout(r, HOVER_PREVIEW_DELAY_MS + 40));
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, 40));
    });
    assert.equal(thumb.getAttribute('data-preview-phase'), 'failed');
    assert.ok(!thumb.querySelector('.hover-preview-poster')!.classList.contains('is-faded'));
    const playFetches = fetchUrls.filter((u) => u.includes('/play')).length;
    await act(async () => {
      mouseLeave(thumb);
      mouseEnter(thumb);
      await new Promise((r) => setTimeout(r, HOVER_PREVIEW_DELAY_MS + 40));
    });
    assert.equal(fetchUrls.filter((u) => u.includes('/play')).length, playFetches);
  });

  it('preview does not record history or modify progress', async () => {
    mountThumbs([1]);
    const thumb = document.querySelector('[data-testid="thumb-1"]')!;
    await act(async () => {
      mouseEnter(thumb);
      await new Promise((r) => setTimeout(r, HOVER_PREVIEW_DELAY_MS + 40));
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, 40));
    });
    assert.equal(historyPosts, 0);
    assert.equal(fetchUrls.some((u) => u.includes('/me/history')), false);
  });

  it('touch / non-hover devices do not activate preview', async () => {
    act(() => root?.unmount());
    installDom(false);
    mountThumbs([1]);
    const thumb = document.querySelector('[data-testid="thumb-1"]')!;
    await act(async () => {
      mouseEnter(thumb);
      await new Promise((r) => setTimeout(r, HOVER_PREVIEW_DELAY_MS + 40));
    });
    assert.equal(fetchUrls.filter((u) => u.includes('/play')).length, 0);
    assert.equal(thumb.getAttribute('data-preview-phase'), 'idle');
  });

  it('guest without token stays poster-only (auth preserved)', async () => {
    mountThumbs([1], '');
    const thumb = document.querySelector('[data-testid="thumb-1"]')!;
    await act(async () => {
      mouseEnter(thumb);
      await new Promise((r) => setTimeout(r, HOVER_PREVIEW_DELAY_MS + 40));
    });
    assert.equal(fetchUrls.filter((u) => u.includes('/play')).length, 0);
    assert.equal(thumb.getAttribute('data-preview-phase'), 'idle');
  });

  it('guest mode resolves preview without a token and does not record history', async () => {
    mountThumbs([1], '', false);
    const thumb = document.querySelector('[data-testid="thumb-1"]')!;
    await act(async () => {
      mouseEnter(thumb);
      await new Promise((r) => setTimeout(r, HOVER_PREVIEW_DELAY_MS + 40));
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, 40));
    });
    assert.equal(fetchUrls.filter((u) => u.includes('/play')).length, 1);
    assert.equal(historyPosts, 0);
    assert.equal(fetchUrls.some((u) => u.includes('/me/history')), false);
  });

  it('unmount cleans up timer and active preview', async () => {
    mountThumbs([1]);
    const thumb = document.querySelector('[data-testid="thumb-1"]')!;
    await act(async () => {
      mouseEnter(thumb);
      await new Promise((r) => setTimeout(r, HOVER_PREVIEW_DELAY_MS + 40));
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, 40));
    });
    assert.equal(thumb.getAttribute('data-preview-phase'), 'playing');
    await act(async () => {
      root.unmount();
    });
    assert.equal(getActiveHoverPreviewOwnerId(), null);
  });

  it('hidden browser tab stops active preview', async () => {
    mountThumbs([1]);
    const thumb = document.querySelector('[data-testid="thumb-1"]')!;
    await act(async () => {
      mouseEnter(thumb);
      await new Promise((r) => setTimeout(r, HOVER_PREVIEW_DELAY_MS + 40));
    });
    await act(async () => {
      await new Promise((r) => setTimeout(r, 40));
    });
    assert.equal(thumb.getAttribute('data-preview-phase'), 'playing');
    await act(async () => {
      Object.defineProperty(document, 'visibilityState', {
        configurable: true,
        get: () => 'hidden',
      });
      document.dispatchEvent(new dom.window.Event('visibilitychange'));
    });
    assert.equal(getActiveHoverPreviewOwnerId(), null);
    assert.equal(thumb.getAttribute('data-preview-phase'), 'idle');
  });

  it('VideoCard click still navigates to /watch/:id and video does not steal clicks', () => {
    const cardSrc = readFileSync(join(here, 'VideoCard.tsx'), 'utf8');
    assert.match(cardSrc, /HoverPreviewThumb/);
    assert.match(cardSrc, /\/watch\/\$\{video\.id\}|videoEntryHref/);
    const thumbSrc = readFileSync(join(here, 'HoverPreviewThumb.tsx'), 'utf8');
    assert.match(thumbSrc, /pointerEvents:\s*['"]none['"]/);
    assert.match(thumbSrc, /muted/);
    assert.match(thumbSrc, /playsInline/);

    const mount = document.getElementById('root')!;
    root = createRoot(mount);
    act(() => {
      root.render(
        createElement(
          MemoryRouter,
          null,
          createElement(
            AuthTestProvider,
            {
              user: TEST_USER,
              token: 'tok',
              children: createElement(VideoCard, {
                video: {
                  id: 55,
                  category_id: 1,
                  title: '测试片',
                  description: '',
                  cover_url: '/c.jpg',
                  duration_sec: 100,
                  view_count: 1,
                  status: 'ready',
                },
              }),
            },
          ),
        ),
      );
    });
    const link = document.querySelector('[data-testid="video-card-55"]') as HTMLAnchorElement;
    assert.ok(link);
    assert.match(link.getAttribute('href') ?? '', /\/watch\/55/);
    const video = link.querySelector('video') as HTMLVideoElement;
    assert.equal(video.style.pointerEvents, 'none');
  });
});

describe('hover preview wiring (source)', () => {
  it('is applied on catalog pages and excluded from flyouts / recs sidebar', () => {
    const account = readFileSync(join(here, '../pages/AccountPage.tsx'), 'utf8');
    const fav = readFileSync(join(here, '../pages/FavoritesPage.tsx'), 'utf8');
    const hist = readFileSync(join(here, '../pages/HistoryPage.tsx'), 'utf8');
    const recs = readFileSync(join(here, 'RecommendationList.tsx'), 'utf8');
    const flyout = readFileSync(join(here, 'HeaderFlyoutPanels.tsx'), 'utf8');
    assert.match(account, /HoverPreviewThumb/);
    assert.match(fav, /HoverPreviewThumb/);
    assert.match(hist, /HoverPreviewThumb/);
    assert.equal(recs.includes('HoverPreviewThumb'), false);
    assert.equal(flyout.includes('HoverPreviewThumb'), false);
  });

  it('does not touch watch playback core or resolver', () => {
    const watchLogic = readFileSync(join(here, '../pages/watchPlaybackLogic.ts'), 'utf8');
    assert.ok(watchLogic.length > 0);
    // Presence check only — file must remain the Watch session helper.
    assert.match(watchLogic, /WatchPlaybackSession/);
  });
});
