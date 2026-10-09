import { type MouseEvent, useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { formatViews, type Video } from '../api';
import { useAuth } from '../auth/AuthContext';
import { videoEntryHref } from '../auth/paths';

type Props = {
  videos: Video[];
};

export function FeaturedCarousel({ videos }: Props) {
  const slides = videos.slice(0, 5);
  const [index, setIndex] = useState(0);
  const { user, ready } = useAuth();
  const navigate = useNavigate();

  useEffect(() => {
    if (slides.length <= 1) return;
    const timer = window.setInterval(() => {
      setIndex((i) => (i + 1) % slides.length);
    }, 4800);
    return () => window.clearInterval(timer);
  }, [slides.length]);

  if (!slides.length) return null;

  const go = (dir: -1 | 1) => {
    setIndex((i) => (i + dir + slides.length) % slides.length);
  };

  const current = slides[index];

  function onVideoClick(e: MouseEvent<HTMLAnchorElement>, videoId: number) {
    const target = videoEntryHref(ready, Boolean(user), videoId);
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
    <div className="carousel" aria-roledescription="carousel">
      {slides.map((v, i) => {
        const href = videoEntryHref(ready, Boolean(user), v.id) ?? `/watch/${v.id}`;
        return (
          <Link
            key={v.id}
            to={href}
            onClick={(e) => onVideoClick(e, v.id)}
            className={`carousel-slide${i === index ? ' is-active' : ''}`}
            aria-hidden={i !== index}
            tabIndex={i === index ? 0 : -1}
          >
            <img src={v.cover_url} alt="" loading={i === 0 ? 'eager' : 'lazy'} />
            <div className="carousel-caption">
              <h3>{v.title}</h3>
              <p>
                {formatViews(v.view_count || 0)} 播放
                {v.year ? ` · ${v.year}` : ''}
                {v.area ? ` · ${v.area}` : ''}
              </p>
            </div>
          </Link>
        );
      })}

      <div className="carousel-dots">
        {slides.map((v, i) => (
          <button
            key={v.id}
            type="button"
            className={i === index ? 'is-active' : undefined}
            aria-label={`切换到 ${v.title}`}
            onClick={() => setIndex(i)}
          />
        ))}
      </div>

      {slides.length > 1 && (
        <div className="carousel-nav">
          <button type="button" aria-label="上一张" onClick={() => go(-1)}>
            ‹
          </button>
          <button type="button" aria-label="下一张" onClick={() => go(1)}>
            ›
          </button>
        </div>
      )}

      <span className="sr-only">当前：{current?.title}</span>
    </div>
  );
}
