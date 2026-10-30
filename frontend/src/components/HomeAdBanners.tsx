import ad1 from '../assets/ads/home-ad-1.jpg';
import ad2 from '../assets/ads/home-ad-2.jpg';
import ad3 from '../assets/ads/home-ad-3.jpg';

const HOME_ADS = [
  { id: 'ad1', src: ad1, alt: '广告', href: '#' },
  { id: 'ad2', src: ad2, alt: '广告', href: '#' },
  { id: 'ad3', src: ad3, alt: '广告', href: '#' },
] as const;

type Placement = 'nav' | 'page';

/** Stacked home ad strip between hero and category UI (desktop nav / mobile page). */
export function HomeAdBanners({ placement }: { placement: Placement }) {
  return (
    <aside className={`home-ad-stack home-ad-stack--${placement}`} aria-label="广告">
      {HOME_ADS.map((ad) => (
        <a
          key={ad.id}
          className="home-ad-stack__item"
          href={ad.href}
          target="_blank"
          rel="noopener noreferrer"
        >
          <img src={ad.src} alt={ad.alt} loading="lazy" decoding="async" draggable={false} />
        </a>
      ))}
    </aside>
  );
}
