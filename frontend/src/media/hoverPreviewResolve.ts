import { ApiError, getPlayback, type PlaybackInfo } from '../api';
import { canResolveHoverPreview } from './hoverPreviewLogic';

export type HoverPreviewMedia = {
  url: string;
  type: string;
};

type CacheEntry = {
  media: HoverPreviewMedia | null;
  /** null media means permanently failed for this session key (no retry on same hover). */
};

const cache = new Map<string, CacheEntry>();

function cacheKey(videoId: number, token: string): string {
  // Token fragment keeps guest/user isolation; sid/nid fixed to default episode.
  return `${videoId}:default:${token.slice(0, 12)}`;
}

/** Session cache clear (route change / tests). */
export function clearHoverPreviewCache() {
  cache.clear();
}

/**
 * Resolve a playable preview URL via the existing authenticated play API.
 * Uses sid=0,nid=0 so the backend picks the first/default episode.
 * Never writes history, favorites, or notifications.
 */
export async function resolveHoverPreviewMedia(
  videoId: number,
  token: string | null | undefined,
  authRequired = true,
): Promise<HoverPreviewMedia | null> {
  if (!canResolveHoverPreview(token, authRequired) || !Number.isFinite(videoId) || videoId <= 0) {
    return null;
  }
  const key = cacheKey(videoId, authRequired ? token || '' : 'guest');
  const hit = cache.get(key);
  if (hit) return hit.media;

  try {
    const info: PlaybackInfo = await getPlayback(videoId, 0, 0, token);
    const url = (info.url || info.m3u8_url || '').trim();
    if (!url) {
      cache.set(key, { media: null });
      return null;
    }
    const media: HoverPreviewMedia = {
      url,
      type: info.type || (url.toLowerCase().includes('.m3u8') ? 'hls' : 'url'),
    };
    cache.set(key, { media });
    return media;
  } catch (e) {
    // Auth / unavailable / network — fail closed to poster; cache null to avoid retry spam.
    if (e instanceof ApiError && (e.status === 401 || e.status === 403)) {
      cache.set(key, { media: null });
      return null;
    }
    cache.set(key, { media: null });
    return null;
  }
}

