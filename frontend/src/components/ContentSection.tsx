import { Link } from 'react-router-dom';
import type { Video } from '../api';
import { VideoCard } from './VideoCard';

type Props = {
  title: string;
  moreTo?: string;
  videos: Video[];
  emptyText?: string;
};

export function ContentSection({ title, moreTo, videos, emptyText }: Props) {
  return (
    <section className="content-section">
      <div className="section-head">
        <div className="section-title-row">
          <span className="section-accent" aria-hidden />
          <h2>{title}</h2>
        </div>
        {moreTo ? (
          <Link to={moreTo} className="section-more">
            查看更多 ›
          </Link>
        ) : null}
      </div>
      {videos.length === 0 ? (
        <p className="section-empty">{emptyText || '该分区暂无内容'}</p>
      ) : (
        <div className="video-grid">
          {videos.map((v) => (
            <VideoCard key={`${title}-${v.id}`} video={v} />
          ))}
        </div>
      )}
    </section>
  );
}
