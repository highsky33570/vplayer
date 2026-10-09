import type { Category } from '../api';

type Props = {
  categories: Category[];
  active: string;
  onSelect: (slug: string) => void;
};

/** Homepage category buttons in display order (matched from API by name/slug/id). */
const CANONICAL: { names: string[]; slug: string; id: number }[] = [
  { names: ['电影'], slug: 'movie', id: 2 },
  { names: ['连续剧', '剧集'], slug: 'tv', id: 3 },
  { names: ['综艺'], slug: 'variety', id: 13 },
  { names: ['动漫'], slug: 'anime', id: 4 },
  { names: ['午夜影院'], slug: 'midnight', id: 16 },
  { names: ['VIP蓝光影院'], slug: 'vip-bluray', id: 17 },
];

/** Nav-order categories for Home sections and the discovery bar (API-matched). */
export function resolveNavCategories(api: Category[]): Category[] {
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
    }
  }

  return out;
}

export function DiscoveryNav({ categories, active, onSelect }: Props) {
  const cats = resolveNavCategories(categories);

  return (
    <div className="discovery">
      <div className="category-grid" role="tablist" aria-label="分类导航">
        <button
          type="button"
          role="tab"
          aria-selected={active === 'home'}
          className={`category-btn${active === 'home' ? ' is-active' : ''}`}
          onClick={() => onSelect('home')}
        >
          <span className="category-label">首页</span>
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
