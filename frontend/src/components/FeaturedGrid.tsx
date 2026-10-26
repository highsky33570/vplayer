import type { Video } from '../api';
import { FeaturedCarousel } from './FeaturedCarousel';
import { splitFeaturedVideos } from './featuredLayout';
import { VideoCard } from './VideoCard';

type Props = {
  videos: Video[];
};

export function FeaturedGrid({ videos }: Props) {
  if (!videos.length) return null;
  const { carousel, side } = splitFeaturedVideos(videos);

  return (
    <section
      className={`featured-grid${side.length === 0 ? ' is-hero-only' : ''}`}
      aria-label="精选内容"
    >
      <FeaturedCarousel videos={carousel} />
      {side.length > 0 && (
        <div className="featured-side">
          {side.map((v) => (
            <VideoCard key={`feat-${v.id}`} video={v} />
          ))}
        </div>
      )}
    </section>
  );
}
