import { useEffect, useRef, useState } from 'react';
import banner01 from '../assets/banners/banner-01.webp';
import banner02 from '../assets/banners/banner-02.webp';
import banner03 from '../assets/banners/banner-03.webp';
import banner04 from '../assets/banners/banner-04.webp';
import banner05 from '../assets/banners/banner-05.webp';
import banner06 from '../assets/banners/banner-06.webp';

const BANNERS = [banner01, banner02, banner03, banner04, banner05, banner06];

const HOLD_MS = 5000;
const TRANSITION_MS = 800;

function usePrefersReducedMotion() {
  const [reduced, setReduced] = useState(false);
  useEffect(() => {
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)');
    const update = () => setReduced(mq.matches);
    update();
    mq.addEventListener('change', update);
    return () => mq.removeEventListener('change', update);
  }, []);
  return reduced;
}

export function BannerCarousel() {
  const reducedMotion = usePrefersReducedMotion();
  const [trackIndex, setTrackIndex] = useState(0);
  const [animate, setAnimate] = useState(true);
  const active = trackIndex % BANNERS.length;
  const transitioning = useRef(false);

  useEffect(() => {
    BANNERS.forEach((src) => {
      const img = new Image();
      img.src = src;
    });
  }, []);

  useEffect(() => {
    if (reducedMotion) return;
    const timer = window.setInterval(() => {
      if (transitioning.current) return;
      transitioning.current = true;
      setAnimate(true);
      setTrackIndex((i) => i + 1);
    }, HOLD_MS);
    return () => window.clearInterval(timer);
  }, [reducedMotion]);

  useEffect(() => {
    if (trackIndex < BANNERS.length) {
      transitioning.current = false;
      return;
    }
    const t = window.setTimeout(() => {
      setAnimate(false);
      setTrackIndex(0);
      requestAnimationFrame(() => {
        transitioning.current = false;
      });
    }, TRANSITION_MS);
    return () => window.clearTimeout(t);
  }, [trackIndex]);

  const goTo = (i: number) => {
    if (reducedMotion) {
      setAnimate(false);
      setTrackIndex(i);
      return;
    }
    if (transitioning.current || i === active) return;
    transitioning.current = true;
    setAnimate(true);
    setTrackIndex(i);
    window.setTimeout(() => {
      transitioning.current = false;
    }, TRANSITION_MS);
  };

  const slides = reducedMotion ? BANNERS : [...BANNERS, BANNERS[0]];

  return (
    <div className="banner-carousel">
      <div className="banner-viewport" aria-hidden>
        <div
          className={`banner-track${animate && !reducedMotion ? ' is-animated' : ''}`}
          style={{ transform: `translate3d(-${(reducedMotion ? active : trackIndex) * 100}%, 0, 0)` }}
        >
          {slides.map((src, i) => (
            <div className="banner-slide" key={`${src}-${i}`}>
              <img src={src} alt="" draggable={false} />
            </div>
          ))}
        </div>
      </div>

      <div className="banner-shade" aria-hidden />

      <div className="banner-dots" role="tablist" aria-label="横幅切换">
        {BANNERS.map((src, i) => (
          <button
            key={src}
            type="button"
            role="tab"
            aria-selected={i === active}
            aria-label={`横幅 ${i + 1}`}
            className={i === active ? 'is-active' : undefined}
            onClick={() => goTo(i)}
          />
        ))}
      </div>
    </div>
  );
}
