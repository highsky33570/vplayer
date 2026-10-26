import { type MouseEvent, useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { type Video } from '../api';
import { useAuth } from '../auth/AuthContext';
import { videoEntryHref } from '../auth/paths';
import { coverImgSrc, onCoverImgError } from '../media/coverFallback';

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

  const current = slides[index];

  const go = (dir: -1 | 1) => {
    setIndex((i) => (i + dir + slides.length) % slides.length);
  };

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
            <img
              src={coverImgSrc(v.cover_url)}
              alt=""
              loading={i === 0 ? 'eager' : 'lazy'}
              onError={onCoverImgError}
            />
            <div className="carousel-caption">
              <h3>{v.title}</h3>
            </div>
          </Link>
        );
      })}

      {slides.length > 1 ? (
        <>
          <div className="carousel-dots">
            {slides.map((v, i) => (
              <button
                key={v.id}
                type="button"
                className={i === index ? 'is-active' : ''}
                aria-label={`第 ${i + 1} 张`}
                aria-current={i === index ? 'true' : undefined}
                onClick={() => setIndex(i)}
              />
            ))}
          </div>
          <div className="carousel-nav">
            <button type="button" aria-label="上一张" onClick={() => go(-1)}>
              <Chevron dir="left" />
            </button>
            <button type="button" aria-label="下一张" onClick={() => go(1)}>
              <Chevron dir="right" />
            </button>
          </div>
        </>
      ) : null}

      <span className="sr-only">当前：{current?.title}</span>
    </div>
  );
}

function Chevron({ dir }: { dir: 'left' | 'right' }) {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d={dir === 'left' ? 'M14.5 6.5 9 12l5.5 5.5' : 'M9.5 6.5 15 12l-5.5 5.5'}
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
