import { type MouseEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { formatDuration, formatViews, type Video } from '../api';
import { useAuth } from '../auth/AuthContext';
import { videoEntryHref } from '../auth/paths';
import { coverImgSrc, onCoverImgError } from '../media/coverFallback';

type Props = {
  video: Video;
  compact?: boolean;
  categoryName?: string;
};

export function VideoCard({ video, compact, categoryName }: Props) {
  const { user, ready } = useAuth();
  const navigate = useNavigate();
  const href = videoEntryHref(ready, Boolean(user), video.id) ?? `/watch/${video.id}`;
  const metaBits = [categoryName, video.year, video.area].filter(Boolean);
  const showDuration = video.duration_sec > 0;

  function onClick(e: MouseEvent<HTMLAnchorElement>) {
    const target = videoEntryHref(ready, Boolean(user), video.id);
    if (target == null) {
      e.preventDefault();
      return;
    }
    if (target.startsWith('/login')) {
      e.preventDefault();
      navigate(target);
    }
  }

  return (
    <Link
      to={href}
      onClick={onClick}
      className={`video-card${compact ? ' is-compact' : ''}`}
      title={video.title}
    >
      <div className="thumb">
        <img
          src={coverImgSrc(video.cover_url)}
          alt=""
          loading="lazy"
          onError={onCoverImgError}
        />
        <div className="thumb-stats">
          <span>
            <PlayIcon />
            {formatViews(video.view_count || 0)}
          </span>
          {showDuration ? <span>{formatDuration(video.duration_sec)}</span> : null}
        </div>
      </div>
      <div className="video-title">{video.title}</div>
      {!compact && (
        <div className="video-meta">
          {metaBits.length ? metaBits.join(' · ') : video.description || '精彩内容'}
        </div>
      )}
    </Link>
  );
}

function PlayIcon() {
  return (
    <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" aria-hidden>
      <path d="M8 5.5v13l11-6.5L8 5.5z" />
    </svg>
  );
}
