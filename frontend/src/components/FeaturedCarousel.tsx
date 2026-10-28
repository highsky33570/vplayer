import { type MouseEvent, type PointerEvent as ReactPointerEvent, useEffect, useRef, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { type Video } from '../api';
import { useAuth } from '../auth/AuthContext';
import { videoEntryHref } from '../auth/paths';
import { coverImgSrc, onCoverImgError } from '../media/coverFallback';
import { FEATURED_CAROUSEL_LIMIT } from './featuredLayout';

type Props = {
  videos: Video[];
};

const DRAG_THRESHOLD_PX = 42;

export function FeaturedCarousel({ videos }: Props) {
  const slides = videos.slice(0, FEATURED_CAROUSEL_LIMIT);
  const [index, setIndex] = useState(0);
  const { user, ready } = useAuth();
  const navigate = useNavigate();
  const dragRef = useRef<{
    pointerId: number;
    startX: number;
    startY: number;
    moved: boolean;
  } | null>(null);
  const suppressClickRef = useRef(false);

  useEffect(() => {
    setIndex(0);
  }, [slides.length]);

  useEffect(() => {
    if (slides.length <= 1) return;
    const timer = window.setInterval(() => {
      if (dragRef.current) return;
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
    if (suppressClickRef.current) {
      e.preventDefault();
      suppressClickRef.current = false;
      return;
    }
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

  function onPointerDown(e: ReactPointerEvent<HTMLDivElement>) {
    if (slides.length <= 1) return;
    if (e.pointerType === 'mouse' && e.button !== 0) return;
    dragRef.current = {
      pointerId: e.pointerId,
      startX: e.clientX,
      startY: e.clientY,
      moved: false,
    };
    e.currentTarget.setPointerCapture(e.pointerId);
  }

  function onPointerMove(e: ReactPointerEvent<HTMLDivElement>) {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== e.pointerId) return;
    const dx = e.clientX - drag.startX;
    const dy = e.clientY - drag.startY;
    if (!drag.moved && Math.abs(dx) > 8 && Math.abs(dx) > Math.abs(dy)) {
      drag.moved = true;
    }
  }

  function onPointerUp(e: ReactPointerEvent<HTMLDivElement>) {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== e.pointerId) return;
    const dx = e.clientX - drag.startX;
    dragRef.current = null;
    try {
      e.currentTarget.releasePointerCapture(e.pointerId);
    } catch {
      /* already released */
    }
    if (!drag.moved || Math.abs(dx) < DRAG_THRESHOLD_PX) return;
    suppressClickRef.current = true;
    go(dx < 0 ? 1 : -1);
  }

  function onPointerCancel(e: ReactPointerEvent<HTMLDivElement>) {
    if (dragRef.current?.pointerId === e.pointerId) dragRef.current = null;
  }

  return (
    <div
      className="carousel"
      aria-roledescription="carousel"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerCancel}
    >
      {slides.map((v, i) => {
        const href = videoEntryHref(ready, Boolean(user), v.id) ?? `/watch/${v.id}`;
        const desc = (v.description || '').trim();
        return (
          <Link
            key={v.id}
            to={href}
            onClick={(e) => onVideoClick(e, v.id)}
            className={`carousel-slide${i === index ? ' is-active' : ''}`}
            aria-hidden={i !== index}
            tabIndex={i === index ? 0 : -1}
            draggable={false}
          >
            <img
              src={coverImgSrc(v.cover_url)}
              alt=""
              loading={i === 0 ? 'eager' : 'lazy'}
              draggable={false}
              onError={onCoverImgError}
            />
            <div className="carousel-caption">
              <h3>{v.title}</h3>
              {desc ? <p>{desc}</p> : null}
            </div>
          </Link>
        );
      })}

      {slides.length > 1 ? (
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
      ) : null}

      <span className="sr-only">当前：{current?.title}</span>
    </div>
  );
}
