import { useEffect, useRef, useState } from 'react';
import banner01 from '../assets/banners/banner-01.webp';
import banner02 from '../assets/banners/banner-02.webp';
import banner03 from '../assets/banners/banner-03.webp';
import banner04 from '../assets/banners/banner-04.webp';
import banner05 from '../assets/banners/banner-05.webp';
import banner06 from '../assets/banners/banner-06.webp';

type BannerTheme = 'blue' | 'red' | 'sakura' | 'gold' | 'teal' | 'purple';
type BannerPosition = 'bl' | 'bcl' | 'br' | 'blc' | 'brc' | 'bc';

type BannerConfig = {
  image: string;
  headline: string;
  subtitle: string;
  highlight: string;
  theme: BannerTheme;
  position: BannerPosition;
};

const BANNERS: BannerConfig[] = [
  {
    image: banner01,
    headline: '海量影视 · 高清畅看',
    subtitle: '热门电影、电视剧、动漫、综艺精彩不断',
    highlight: '精彩内容 随心观看',
    theme: 'blue',
    position: 'bc',
  },
  {
    image: banner02,
    headline: '热门影视 · 每日更新',
    subtitle: '发现正在热播的人气电影与精彩剧集',
    highlight: '热门推荐',
    theme: 'red',
    position: 'bc',
  },
  {
    image: banner03,
    headline: '精彩世界 · 随心探索',
    subtitle: '发现更多好剧，让每一次观看都有新惊喜',
    highlight: '发现精彩',
    theme: 'sakura',
    position: 'bc',
  },
  {
    image: banner04,
    headline: '经典大片 · 不容错过',
    subtitle: '精选热门佳作，开启沉浸式观影体验',
    highlight: '精选推荐',
    theme: 'gold',
    position: 'bc',
  },
  {
    image: banner05,
    headline: '高清点播 · 畅享精彩',
    subtitle: '电影、剧集、动漫、综艺，一站尽享',
    highlight: '高清流畅',
    theme: 'teal',
    position: 'bc',
  },
  {
    image: banner06,
    headline: '精彩动漫 · 热血来袭',
    subtitle: '热门新番、经典动漫、精彩内容持续更新',
    highlight: '动漫精选',
    theme: 'purple',
    position: 'bc',
  },
];

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
  const ad = BANNERS[active];

  useEffect(() => {
    BANNERS.forEach((b) => {
      const img = new Image();
      img.src = b.image;
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
          {slides.map((b, i) => (
            <div className="banner-slide" key={`${b.image}-${i}`}>
              <img src={b.image} alt="" draggable={false} />
            </div>
          ))}
        </div>
      </div>

      <div className="banner-shade" aria-hidden />

      <div
        key={active}
        className={`banner-ad banner-ad--${ad.theme} banner-ad--${ad.position}${reducedMotion ? ' is-static' : ''}`}
        aria-hidden
      >
        <div className="banner-ad-accent">
          <span className="banner-ad-line" />
          <span className="banner-ad-pill">{ad.highlight}</span>
        </div>
        <h2 className="banner-ad-headline">{ad.headline}</h2>
        <p className="banner-ad-subtitle">{ad.subtitle}</p>
      </div>

      <div className="banner-dots" role="tablist" aria-label="横幅切换">
        {BANNERS.map((b, i) => (
          <button
            key={b.image}
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
