import type { FavoriteVideo, HistoryVideo, Video } from '../api';

/** Max cards in 最近观看 on the personal home. */
export const ACCOUNT_HISTORY_PREVIEW_LIMIT = 8;

/** Max recent favorite videos under 收藏夹. */
export const ACCOUNT_FAVORITES_PREVIEW_LIMIT = 6;

/**
 * Catalog recommendation strip on /account (“猜你喜欢”).
 * Source: GET /api/v1/videos (same latest catalog as homepage Featured/最新上架),
 * excluding midnight. Not personalized ownership or AI ranking.
 */
export const ACCOUNT_RECOMMEND_LIMIT = 15;

/** VPlayer has no member upload ownership model yet. */
export const ACCOUNT_USER_UPLOADS_SUPPORTED = false;

/** Slice catalog videos for the account recommendation grid. */
export function accountRecommendVideos(
  videos: Video[] | null | undefined,
  limit: number = ACCOUNT_RECOMMEND_LIMIT,
): Video[] {
  const n = Math.max(0, Math.floor(limit));
  return (videos ?? []).slice(0, n);
}

/** Personal history preview — never mix with catalog recommendations. */
export function accountHistoryPreview(
  items: HistoryVideo[] | null | undefined,
  limit: number = ACCOUNT_HISTORY_PREVIEW_LIMIT,
): HistoryVideo[] {
  return (items ?? []).slice(0, Math.max(0, Math.floor(limit)));
}

/** Personal favorites preview — never mix with catalog recommendations. */
export function accountFavoritesPreview(
  items: FavoriteVideo[] | null | undefined,
  limit: number = ACCOUNT_FAVORITES_PREVIEW_LIMIT,
): FavoriteVideo[] {
  return (items ?? []).slice(0, Math.max(0, Math.floor(limit)));
}

/**
 * Owned-upload section content. Until uploads exist, always empty —
 * catalog recommendations must not be passed here.
 */
export function accountOwnedUploadVideos(_userId: number): Video[] {
  if (!ACCOUNT_USER_UPLOADS_SUPPORTED) return [];
  return [];
}

export function accountOwnedUploadsEmptyCopy(): { title: string; hint: string } {
  return {
    title: '暂无投稿视频',
    hint: '天涯欧乐 暂未开放用户投稿，此区域不会展示片库内容。',
  };
}

/** Compact meta line under recommendation cards. */
export function accountRecommendMeta(video: Pick<Video, 'year' | 'view_count' | 'area'>): string {
  const parts: string[] = [];
  if (video.year) parts.push(String(video.year));
  if (video.area) parts.push(video.area);
  if (video.view_count > 0) parts.push(`${video.view_count} 播放`);
  return parts.join(' · ');
}
