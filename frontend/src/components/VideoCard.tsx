import { type MouseEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { formatDuration, formatViews, type Video } from '../api';
import { useAuth } from '../auth/AuthContext';
import { videoEntryHref } from '../auth/paths';

type Props = {
  video: Video;
  compact?: boolean;
  categoryName?: string;
};

const FALLBACK =
  'data:image/svg+xml,' +
  encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360" viewBox="0 0 640 360">
      <defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
        <stop stop-color="#00aeec"/><stop offset="1" stop-color="#fb7299"/>
      </linearGradient></defs>
      <rect width="640" height="360" fill="#f1f2f3"/>
      <rect width="640" height="360" fill="url(#g)" opacity=".35"/>
      <text x="320" y="188" text-anchor="middle" fill="#fff" font-size="36" font-family="sans-serif" font-weight="700">VPlayer</text>
    </svg>`,
  );

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
          src={video.cover_url || FALLBACK}
          alt=""
          loading="lazy"
          onError={(e) => {
            const el = e.currentTarget;
            if (el.src !== FALLBACK) el.src = FALLBACK;
          }}
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
