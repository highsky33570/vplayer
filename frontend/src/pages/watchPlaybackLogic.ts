/**
 * Plan 7 v1 playback fallback helpers.
 *
 * xgplayer's generic `error` event does not reliably distinguish manifest vs
 * segment failures in our integration. We therefore allow at most one fallback
 * only for errors that occur BEFORE confirmed playback start (`playing`).
 *
 * Player callbacks are generation-scoped: replacing/destroying a player bumps
 * the generation so late events from the previous instance are ignored.
 */

export type PlaybackFallbackState = {
  fallbackUrl: string;
  fallbackAttempted: boolean;
  playbackStarted: boolean;
};

export function createPlaybackFallbackState(fallbackUrl?: string | null): PlaybackFallbackState {
  return {
    fallbackUrl: (fallbackUrl ?? '').trim(),
    fallbackAttempted: false,
    playbackStarted: false,
  };
}

/** Mark that media has begun playing successfully (blocks later automatic fallback). */
export function markPlaybackStarted(state: PlaybackFallbackState): PlaybackFallbackState {
  return { ...state, playbackStarted: true };
}

/**
 * Whether a player error should trigger a one-time source fallback.
 * Conservative: only before confirmed start, only once, only if URL present.
 */
export function shouldAttemptFallback(state: PlaybackFallbackState): boolean {
  if (state.playbackStarted) return false;
  if (state.fallbackAttempted) return false;
  if (!state.fallbackUrl) return false;
  return true;
}

/** Consume the single fallback attempt and return the URL to play (or ''). */
export function takeFallbackAttempt(state: PlaybackFallbackState): {
  next: PlaybackFallbackState;
  url: string;
} {
  if (!shouldAttemptFallback(state)) {
    return { next: state, url: '' };
  }
  return {
    next: { ...state, fallbackAttempted: true },
    url: state.fallbackUrl,
  };
}

export const PLAYBACK_GENERIC_ERROR = '播放失败，请稍后重试';
export const PLAYBACK_UNAVAILABLE_MESSAGE = 'This video is currently unavailable for playback.';

export type PlayerErrorAction =
  | { type: 'ignore' }
  | { type: 'fallback'; url: string }
  | { type: 'generic_error' };

/**
 * Session controller for one Watch mount / video load.
 * Bumps generation on each new player so stale callbacks cannot act.
 */
export class WatchPlaybackSession {
  private generation = 0;
  private cancelled = false;
  private fallback: PlaybackFallbackState = createPlaybackFallbackState();

  /** Invalidate all prior players/callbacks (unmount or video change). */
  cancel(): void {
    this.cancelled = true;
    this.generation += 1;
    this.fallback = createPlaybackFallbackState();
  }

  /** Reset fallback flags for a fresh /play response (same session). */
  beginPlayback(fallbackUrl?: string | null): void {
    this.fallback = createPlaybackFallbackState(fallbackUrl);
  }

  /**
   * Call immediately before constructing a player.
   * Invalidates any previous player's generation.
   */
  beginPlayer(): number {
    this.generation += 1;
    return this.generation;
  }

  currentGeneration(): number {
    return this.generation;
  }

  isCurrent(gen: number): boolean {
    return !this.cancelled && gen === this.generation;
  }

  handlePlaying(gen: number): boolean {
    if (!this.isCurrent(gen)) return false;
    this.fallback = markPlaybackStarted(this.fallback);
    return true;
  }

  handleError(gen: number): PlayerErrorAction {
    if (!this.isCurrent(gen)) {
      return { type: 'ignore' };
    }
    const { next, url } = takeFallbackAttempt(this.fallback);
    this.fallback = next;
    if (url) {
      return { type: 'fallback', url };
    }
    return { type: 'generic_error' };
  }

  /** Test/introspection helper. */
  debugState(): {
    generation: number;
    cancelled: boolean;
    fallback: PlaybackFallbackState;
  } {
    return {
      generation: this.generation,
      cancelled: this.cancelled,
      fallback: { ...this.fallback },
    };
  }
}
