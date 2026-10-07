import { Link } from 'react-router-dom';
import type { Video } from '../api';
import { FeaturedCarousel } from './FeaturedCarousel';
import { VideoCard } from './VideoCard';

type Props = {
  videos: Video[];
};

export function FeaturedGrid({ videos }: Props) {
  if (!videos.length) return null;
  const carousel = videos.slice(0, Math.min(5, videos.length));
  const side = videos.slice(0, 4);

  return (
    <section className="featured-grid" aria-label="精选内容">
      <FeaturedCarousel videos={carousel} />
      <div className="featured-side">
        {side.map((v) => (
          <VideoCard key={`feat-${v.id}`} video={v} compact />
        ))}
        {side.length === 0 && (
          <Link to="/" className="featured-empty">
            暂无精选
          </Link>
        )}
      </div>
    </section>
  );
}
