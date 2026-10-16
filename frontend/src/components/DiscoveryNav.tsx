import { useEffect, useMemo, useRef, useState } from 'react';
import type { Category } from '../api';
import {
  activeCategoryScrollOptions,
  childrenOfParent,
  expandedParentSlugForActive,
  resolveNavCategories,
  syncExpandedParentSlug,
  toggleMainExpansion,
} from './discoveryNavLogic';

export {
  activeCategoryScrollOptions,
  childrenOfParent,
  expandedParentSlugForActive,
  findCategoryBySlug,
  resolveNavCategories,
  syncExpandedParentSlug,
  toggleMainExpansion,
} from './discoveryNavLogic';

type Props = {
  categories: Category[];
  active: string;
  onSelect: (slug: string) => void;
};

export function DiscoveryNav({ categories, active, onSelect }: Props) {
  const mains = useMemo(() => resolveNavCategories(categories), [categories]);
  const [expandedParentSlug, setExpandedParentSlug] = useState<string | null>(() =>
    expandedParentSlugForActive(categories, resolveNavCategories(categories), active),
  );

  const discoveryRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setExpandedParentSlug((current) =>
      syncExpandedParentSlug(categories, mains, active, current),
    );
  }, [active, categories, mains]);

  useEffect(() => {
    const rail = discoveryRef.current?.querySelector<HTMLElement>('nav.category-rail');
    if (!rail) return;
    const activeBtn = rail.querySelector<HTMLElement>(
      '.category-btn--main.is-active, .category-btn--main.is-parent-of-active',
    );
    if (!activeBtn) return;
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    activeBtn.scrollIntoView(activeCategoryScrollOptions(reduced));
  }, [active]);

  function handleHome() {
    setExpandedParentSlug(null);
    onSelect('home');
  }

  function handleMain(main: Category) {
    setExpandedParentSlug(toggleMainExpansion(expandedParentSlug, main.slug));
    onSelect(main.slug);
  }

  function handleChild(child: Category, parent: Category) {
    setExpandedParentSlug(parent.slug);
    onSelect(child.slug);
  }

  const expandedMain = mains.find((main) => main.slug === expandedParentSlug) ?? null;
  const expandedChildren = expandedMain ? childrenOfParent(categories, expandedMain.id) : [];

  return (
    <div className="discovery" ref={discoveryRef}>
      <nav className="category-rail" aria-label="分类导航">
        <button
          type="button"
          className={`category-btn category-btn--main${active === 'home' ? ' is-active' : ''}`}
          onClick={handleHome}
        >
          <span className="category-label">首页</span>
        </button>

        {mains.map((main) => {
          const kids = childrenOfParent(categories, main.id);
          const expanded = expandedParentSlug === main.slug;
          const childActive = kids.some((k) => k.slug === active);
          const mainActive = active === main.slug;

          return (
            <div key={`${main.id}-${main.slug}`} className="category-rail-group">
              <button
                type="button"
                aria-expanded={kids.length > 0 ? expanded : undefined}
                className={[
                  'category-btn',
                  'category-btn--main',
                  mainActive ? 'is-active' : '',
                  expanded && kids.length > 0 ? 'is-expanded' : '',
                  childActive ? 'is-parent-of-active' : '',
                ]
                  .filter(Boolean)
                  .join(' ')}
                onClick={() => handleMain(main)}
              >
                <span className="category-label">{main.name}</span>
              </button>

              {kids.length > 0 && (
                <div
                  className={`category-children${expanded ? ' is-open' : ''}`}
                  aria-hidden={!expanded}
                >
                  <div className="category-children-inner">
                    {kids.map((child) => (
                      <button
                        key={`${child.id}-${child.slug}`}
                        type="button"
                        tabIndex={expanded ? 0 : -1}
                        className={`category-btn category-btn--child${active === child.slug ? ' is-active' : ''}`}
                        onClick={() => handleChild(child, main)}
                      >
                        <span className="category-label">{child.name}</span>
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>
          );
        })}
      </nav>

      {/* Phone-only second row. Desktop keeps inline .category-children beside the parent. */}
      <div
        className={`category-subrow${expandedChildren.length > 0 ? ' is-open' : ''}`}
        aria-hidden={expandedChildren.length === 0}
      >
        <div
          className="category-subrow-scroll"
          role="group"
          aria-label={expandedMain ? `${expandedMain.name}子分类` : '子分类'}
        >
          {expandedMain &&
            expandedChildren.map((child) => (
              <button
                key={`sub-${child.id}-${child.slug}`}
                type="button"
                tabIndex={expandedChildren.length > 0 ? 0 : -1}
                className={`category-btn category-btn--child${active === child.slug ? ' is-active' : ''}`}
                onClick={() => handleChild(child, expandedMain)}
              >
                <span className="category-label">{child.name}</span>
              </button>
            ))}
        </div>
      </div>
    </div>
  );
}
