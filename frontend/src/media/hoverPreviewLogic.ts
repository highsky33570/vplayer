/** Desktop hover-preview timing and seek helpers (pure; no media I/O). */

export const HOVER_PREVIEW_DELAY_MS = 600;
export const HOVER_PREVIEW_START_SEC = 10;
export const HOVER_PREVIEW_LOOP_SEC = 10;

/** CSS media query: real hover + fine pointer (desktop). */
export const HOVER_PREVIEW_MEDIA_QUERY = '(hover: hover) and (pointer: fine)';

export function supportsHoverPreview(
  matchMedia?: (query: string) => { matches: boolean } | null | undefined,
): boolean {
  if (typeof matchMedia !== 'function') {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
      return false;
    }
    return window.matchMedia(HOVER_PREVIEW_MEDIA_QUERY).matches;
  }
  try {
    return Boolean(matchMedia(HOVER_PREVIEW_MEDIA_QUERY)?.matches);
  } catch {
    return false;
  }
}

/**
 * Prefer ~10s in, but never past a sensible early point of a short clip.
 * Duration unknown (0) → still start at PREVIEW_START (player clamps).
 */
export function previewStartSec(durationSec?: number | null): number {
  const d = Number(durationSec);
  if (!Number.isFinite(d) || d <= 0) return HOVER_PREVIEW_START_SEC;
  if (d <= HOVER_PREVIEW_START_SEC + 2) {
    return Math.max(0, Math.min(2, d * 0.15));
  }
  return Math.min(HOVER_PREVIEW_START_SEC, Math.max(0, d * 0.08));
}

export function previewLoopEndSec(startSec: number, durationSec?: number | null): number {
  const end = startSec + HOVER_PREVIEW_LOOP_SEC;
  const d = Number(durationSec);
  if (Number.isFinite(d) && d > 0) return Math.min(end, d);
  return end;
}

export function isHlsPlayback(url: string, type?: string): boolean {
  if ((type ?? '').toLowerCase() === 'hls') return true;
  return /\.m3u8(\?|$)/i.test(url);
}

/** Guests / missing token must not resolve play URLs. */
export function canResolveHoverPreview(token: string | null | undefined): boolean {
  return Boolean(token && token.trim());
}
