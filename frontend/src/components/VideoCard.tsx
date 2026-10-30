import { type MouseEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { formatDuration, formatViews, type Video } from '../api';
import { useAuth } from '../auth/AuthContext';
import { videoEntryHref } from '../auth/paths';
import { HoverPreviewThumb } from './HoverPreviewThumb';

type Props = {
  video: Video;
  compact?: boolean;
  categoryName?: string;
};

export function VideoCard({ video, compact, categoryName }: Props) {
  const { user, ready } = useAuth();
  const navigate = useNavigate();
  const href = videoEntryHref(ready, Boolean(user), video.id) ?? `/watch/${video.id}`;
  const facts = [categoryName, video.year, video.area].filter(Boolean).join(' · ');
  const description = (video.description || '').trim();
  const showDuration = video.duration_sec > 0;
  const showViews = video.view_count > 0;

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
      data-testid={`video-card-${video.id}`}
    >
      <HoverPreviewThumb
        videoId={video.id}
        coverUrl={video.cover_url}
        durationSec={video.duration_sec}
        className="thumb"
      >
        {video.is_hot ? <span className="thumb-hot">热门推荐</span> : null}
        {showViews || showDuration ? (
          <div className="thumb-stats">
            {showViews ? (
              <span>
                <PlayIcon />
                {formatViews(video.view_count)}
              </span>
            ) : (
              <span />
            )}
            {showDuration ? <span>{formatDuration(video.duration_sec)}</span> : null}
          </div>
        ) : null}
      </HoverPreviewThumb>
      <div className="video-title">{video.title}</div>
      {!compact && facts ? <div className="video-meta video-meta-facts">{facts}</div> : null}
      {!compact ? <div className="video-meta video-meta-desc">{description}</div> : null}
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
