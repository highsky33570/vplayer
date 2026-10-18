/** Pure watch-page helpers. Playback session behavior lives in watchPlaybackLogic.ts. */

export const COMMENT_MAX_RUNES = 1000;

export function commentValidationMessage(raw: string): string | null {
  const body = raw.trim();
  if (!body) return '评论不能为空';
  if ([...body].length > COMMENT_MAX_RUNES) return '评论不能超过 1000 字';
  return null;
}

export function showEpisodeSelector(count: number): boolean {
  return count > 1;
}

export function recommendationMeta(parts: Array<string | number | null | undefined>): string {
  return parts
    .map((part) => (part == null ? '' : String(part).trim()))
    .filter(Boolean)
    .join(' · ');
}

export function formatCommentTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}
