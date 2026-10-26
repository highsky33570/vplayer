import { useEffect, useRef, useState } from 'react';

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

type Props = {
  images: string[];
};

/** Wide catalog stills behind the header. Empty while covers are loading. */
export function BannerCarousel({ images }: Props) {
  const sources = images.map((src) => src.trim()).filter(Boolean).slice(0, 5);
  const reducedMotion = usePrefersReducedMotion();
  const [trackIndex, setTrackIndex] = useState(0);
  const [animate, setAnimate] = useState(true);
  const count = sources.length;
  const active = count ? trackIndex % count : 0;
  const transitioning = useRef(false);
  const sourceKey = sources.join('|');

  useEffect(() => {
    setTrackIndex(0);
    transitioning.current = false;
  }, [sourceKey]);

  useEffect(() => {
    if (reducedMotion || count < 2) return;
    const timer = window.setInterval(() => {
      if (transitioning.current) return;
      transitioning.current = true;
      setAnimate(true);
      setTrackIndex((i) => i + 1);
    }, HOLD_MS);
    return () => window.clearInterval(timer);
  }, [reducedMotion, count]);

  useEffect(() => {
    if (count === 0 || trackIndex < count) {
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
  }, [trackIndex, count]);

  const goTo = (i: number) => {
    if (count === 0) return;
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

  if (count === 0) {
    return <div className="banner-carousel banner-carousel--empty" aria-hidden />;
  }

  const slides = reducedMotion ? sources : [...sources, sources[0]];

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

      {count > 1 ? (
        <div className="banner-dots" role="tablist" aria-label="横幅切换">
          {sources.map((src, i) => (
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
      ) : null}
    </div>
  );
}
