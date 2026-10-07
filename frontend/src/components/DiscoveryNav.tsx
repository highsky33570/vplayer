import type { Category } from '../api';

type Props = {
  categories: Category[];
  active: string;
  onSelect: (slug: string) => void;
};

/** OLEHDTV top-level categories in display order (source type IDs). */
const CANONICAL: { names: string[]; slug: string; id: number }[] = [
  { names: ['电影'], slug: 'movie', id: 1 },
  { names: ['连续剧', '剧集'], slug: 'tv', id: 2 },
  { names: ['综艺'], slug: 'variety', id: 3 },
  { names: ['动漫'], slug: 'anime', id: 4 },
  { names: ['午夜影院'], slug: 'midnight', id: 5 },
  { names: ['VIP蓝光影院'], slug: 'vip-bluray', id: 6 },
  { names: ['体育直播'], slug: 'ti-yu-zhi-bo', id: 13 },
  { names: ['短剧'], slug: 'duan-ju', id: 14 },
];

function resolveCategories(api: Category[]): Category[] {
  const usable = api.filter((c) => c.slug !== 'home' && c.name !== '推荐');
  const used = new Set<number>();
  const out: Category[] = [];

  for (const entry of CANONICAL) {
    const match =
      usable.find((c) => !used.has(c.id) && entry.names.includes(c.name)) ||
      usable.find((c) => !used.has(c.id) && c.slug === entry.slug) ||
      usable.find((c) => !used.has(c.id) && c.id === entry.id);

    if (match) {
      used.add(match.id);
      out.push({
        ...match,
        name: entry.names[0], // prefer OLEHDTV label (连续剧 over 剧集)
      });
    } else {
      out.push({
        id: entry.id,
        name: entry.names[0],
        slug: entry.slug,
        sort: entry.id * 10,
      });
    }
  }

  return out;
}

export function DiscoveryNav({ categories, active, onSelect }: Props) {
  const cats = resolveCategories(categories);

  return (
    <div className="discovery">
      <div className="category-grid" role="tablist" aria-label="分类导航">
        <button
          type="button"
          role="tab"
          aria-selected={active === 'home'}
          className={`category-btn entry-rec${active === 'home' ? ' is-active' : ''}`}
          onClick={() => onSelect('home')}
        >
          <span className="entry-icon" aria-hidden>
            <RecIcon />
          </span>
          <span className="category-label">推荐</span>
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={active === 'home'}
          className={`category-btn entry-hot${active === 'home' ? ' is-active' : ''}`}
          onClick={() => onSelect('home')}
        >
          <span className="entry-icon" aria-hidden>
            <HotIcon />
          </span>
          <span className="category-label">热门</span>
        </button>

        {cats.map((c) => (
          <button
            key={`${c.id}-${c.slug}`}
            type="button"
            role="tab"
            aria-selected={active === c.slug}
            className={`category-btn${active === c.slug ? ' is-active' : ''}`}
            onClick={() => onSelect(c.slug)}
          >
            <span className="category-label">{c.name}</span>
          </button>
        ))}
      </div>
    </div>
  );
}

function RecIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" aria-hidden>
      <path d="M12 2l2.4 7.2H22l-6 4.4 2.3 7.2L12 16.2 5.7 20.8 8 13.6 2 9.2h7.6L12 2z" />
    </svg>
  );
}

function HotIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" aria-hidden>
      <path d="M12 2c2 3.5 1.2 5.5 0 7 2.5-1 5 1 5 4.5A5.5 5.5 0 0 1 11.5 19 5.5 5.5 0 0 1 6 13.5C6 9 9 6.5 12 2z" />
    </svg>
  );
}
