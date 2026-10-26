import { Link } from 'react-router-dom';
import type { Video } from '../api';
import { VideoCard } from './VideoCard';

type Props = {
  title: string;
  /** Phone-only heading. Desktop keeps `title`. */
  mobileTitle?: string;
  moreTo?: string;
  videos: Video[];
  emptyText?: string;
  /** Restrained editorial variation. Uses the same real videos. */
  layout?: 'plain' | 'lead' | 'compact';
};

export function ContentSection({ title, mobileTitle, moreTo, videos, emptyText, layout = 'plain' }: Props) {
  return (
    <section className={`content-section content-section--${layout}`}>
      <div className="section-head">
        <div className="section-title-row">
          <h2>
            <span className="section-title-long">{title}</span>
            {mobileTitle ? <span className="section-title-short">{mobileTitle}</span> : null}
          </h2>
        </div>
        {moreTo ? (
          <Link to={moreTo} className="section-more">
            更多 &gt;
          </Link>
        ) : null}
      </div>
      {videos.length === 0 ? (
        <p className="section-empty">{emptyText || '该分区暂无内容'}</p>
      ) : (
        <div className={`video-grid${layout === 'plain' ? '' : ` video-grid--${layout}`}`}>
          {videos.map((v) => (
            <VideoCard key={`${title}-${v.id}`} video={v} />
          ))}
        </div>
      )}
    </section>
  );
}
