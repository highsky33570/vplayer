/** Hero slides on tablet/desktop. FeaturedCarousel also caps at this count off-phone. */
export const FEATURED_CAROUSEL_LIMIT = 5;

/** Phone hero slides (line dots). Desktop side grid still uses FEATURED_CAROUSEL_LIMIT. */
export const FEATURED_CAROUSEL_MOBILE_LIMIT = 9;

/** Secondary cards beside the hero on large desktop (3×2). */
export const FEATURED_SIDE_LIMIT = 6;

/**
 * Latest fetch covers desktop carousel+side (5+6). Phone hero reuses the same
 * pool and shows up to FEATURED_CAROUSEL_MOBILE_LIMIT slides.
 */
export const FEATURED_POOL_LIMIT = FEATURED_CAROUSEL_LIMIT + FEATURED_SIDE_LIMIT;

type IdItem = { id: number };

/**
 * Carousel takes the first videos. Side cards are the next distinct videos,
 * up to six. Shorter lists stay short — nothing is repeated to fill the grid.
 */
export function splitFeaturedVideos<T extends IdItem>(videos: T[]): { carousel: T[]; side: T[] } {
  const carousel = videos.slice(0, FEATURED_CAROUSEL_LIMIT);
  const used = new Set(carousel.map((video) => video.id));
  const side: T[] = [];
  for (const video of videos) {
    if (used.has(video.id)) continue;
    side.push(video);
    if (side.length >= FEATURED_SIDE_LIMIT) break;
  }
  return { carousel, side };
}
