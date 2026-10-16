import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import {
  PLAYBACK_GENERIC_ERROR,
  PLAYBACK_UNAVAILABLE_MESSAGE,
  WatchPlaybackSession,
  createPlaybackFallbackState,
  markPlaybackStarted,
  shouldAttemptFallback,
  takeFallbackAttempt,
} from './watchPlaybackLogic.ts';

const here = dirname(fileURLToPath(import.meta.url));

describe('watchPlaybackLogic', () => {
  it('primary success path never needs fallback', () => {
    let state = createPlaybackFallbackState('https://source.example/a.m3u8');
    state = markPlaybackStarted(state);
    assert.equal(shouldAttemptFallback(state), false);
    const taken = takeFallbackAttempt(state);
    assert.equal(taken.url, '');
    assert.equal(taken.next.fallbackAttempted, false);
  });

  it('qualifying pre-start error uses fallback exactly once', () => {
    let state = createPlaybackFallbackState('https://source.example/a.m3u8');
    assert.equal(shouldAttemptFallback(state), true);
    const first = takeFallbackAttempt(state);
    assert.equal(first.url, 'https://source.example/a.m3u8');
    assert.equal(first.next.fallbackAttempted, true);
    state = first.next;
    assert.equal(shouldAttemptFallback(state), false);
    const second = takeFallbackAttempt(state);
    assert.equal(second.url, '');
  });

  it('primary error without fallback does not attempt', () => {
    const state = createPlaybackFallbackState('');
    assert.equal(shouldAttemptFallback(state), false);
  });

  it('post-start error does not automatic fallback', () => {
    let state = createPlaybackFallbackState('https://source.example/a.m3u8');
    state = markPlaybackStarted(state);
    assert.equal(shouldAttemptFallback(state), false);
  });

  it('fresh state resets on new playback session', () => {
    let state = createPlaybackFallbackState('https://source.example/old.m3u8');
    state = takeFallbackAttempt(state).next;
    state = markPlaybackStarted(state);
    state = createPlaybackFallbackState('https://source.example/new.m3u8');
    assert.equal(state.fallbackAttempted, false);
    assert.equal(state.playbackStarted, false);
    assert.equal(state.fallbackUrl, 'https://source.example/new.m3u8');
    assert.equal(shouldAttemptFallback(state), true);
  });

  it('keeps exact unavailable and generic error copy', () => {
    assert.equal(PLAYBACK_UNAVAILABLE_MESSAGE, 'This video is currently unavailable for playback.');
    assert.equal(PLAYBACK_GENERIC_ERROR, '播放失败，请稍后重试');
  });
});

describe('WatchPlaybackSession stale player race', () => {
  it('ignores late primary error after fallback player is current', () => {
    const session = new WatchPlaybackSession();
    session.beginPlayback('https://source.example/fallback.m3u8');

    const primaryGen = session.beginPlayer();
    assert.equal(primaryGen, 1);

    // Primary pre-start error → fallback once.
    const first = session.handleError(primaryGen);
    assert.deepEqual(first, { type: 'fallback', url: 'https://source.example/fallback.m3u8' });

    // Creating fallback invalidates primary generation.
    const fallbackGen = session.beginPlayer();
    assert.equal(fallbackGen, 2);
    assert.equal(session.isCurrent(primaryGen), false);
    assert.equal(session.isCurrent(fallbackGen), true);

    // Late/stale primary error must not destroy fallback / show generic error.
    const stale = session.handleError(primaryGen);
    assert.deepEqual(stale, { type: 'ignore' });
    assert.equal(session.isCurrent(fallbackGen), true);
    assert.equal(session.debugState().fallback.fallbackAttempted, true);
    assert.equal(session.debugState().generation, 2);
  });

  it('ignores stale playing from destroyed primary after fallback starts', () => {
    const session = new WatchPlaybackSession();
    session.beginPlayback('https://source.example/fallback.m3u8');

    const primaryGen = session.beginPlayer();
    assert.equal(session.handleError(primaryGen).type, 'fallback');

    const fallbackGen = session.beginPlayer();
    assert.equal(session.handlePlaying(primaryGen), false);
    assert.equal(session.debugState().fallback.playbackStarted, false);

    assert.equal(session.handlePlaying(fallbackGen), true);
    assert.equal(session.debugState().fallback.playbackStarted, true);
  });

  it('cancel invalidates all generations', () => {
    const session = new WatchPlaybackSession();
    session.beginPlayback('https://source.example/fallback.m3u8');
    const gen = session.beginPlayer();
    session.cancel();
    assert.deepEqual(session.handleError(gen), { type: 'ignore' });
    assert.equal(session.handlePlaying(gen), false);
    assert.equal(session.debugState().cancelled, true);
  });

  it('fallback error after switch yields generic_error once, not another fallback', () => {
    const session = new WatchPlaybackSession();
    session.beginPlayback('https://source.example/fallback.m3u8');
    const primaryGen = session.beginPlayer();
    assert.equal(session.handleError(primaryGen).type, 'fallback');
    const fallbackGen = session.beginPlayer();
    assert.deepEqual(session.handleError(fallbackGen), { type: 'generic_error' });
  });
});

describe('Watch wiring', () => {
  it('uses generation-scoped session for player events', () => {
    const src = readFileSync(join(here, 'Watch.tsx'), 'utf8');
    assert.match(src, /WatchPlaybackSession/);
    assert.match(src, /beginPlayer\(/);
    assert.match(src, /handleError\(gen\)/);
    assert.match(src, /handlePlaying\(gen\)/);
    assert.match(src, /session\.cancel\(\)/);
  });
});
