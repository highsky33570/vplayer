import type { Video } from '../api';
import { FeaturedCarousel } from './FeaturedCarousel';
import { splitFeaturedVideos } from './featuredLayout';
import { VideoCard } from './VideoCard';

type Props = {
  videos: Video[];
};

export function FeaturedGrid({ videos }: Props) {
  if (!videos.length) return null;
  const { side } = splitFeaturedVideos(videos);

  return (
    <section
      className={`featured-grid${side.length === 0 ? ' is-hero-only' : ''}`}
      aria-label="精选内容"
    >
      {/* Full pool so phone can show up to 9 hero slides; desktop carousel still caps at 5. */}
      <FeaturedCarousel videos={videos} />
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
