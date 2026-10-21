/** Branded poster when cover_url is missing or fails to load. */
export const COVER_FALLBACK =
  'data:image/svg+xml,' +
  encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360" viewBox="0 0 640 360">
      <defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
        <stop stop-color="#00aeec"/><stop offset="1" stop-color="#fb7299"/>
      </linearGradient></defs>
      <rect width="640" height="360" fill="#f1f2f3"/>
      <rect width="640" height="360" fill="url(#g)" opacity=".35"/>
      <text x="320" y="188" text-anchor="middle" fill="#fff" font-size="36" font-family="sans-serif" font-weight="700">天涯欧乐</text>
    </svg>`,
  );

export function coverImgSrc(coverUrl: string | undefined | null): string {
  return coverUrl || COVER_FALLBACK;
}

export function onCoverImgError(e: { currentTarget: HTMLImageElement }): void {
  const el = e.currentTarget;
  if (el.src !== COVER_FALLBACK) {
    el.src = COVER_FALLBACK;
  }
}
