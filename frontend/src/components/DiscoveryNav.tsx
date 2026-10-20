import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import type { Category } from '../api';
import { useAuth } from '../auth/AuthContext';
import {
  DESKTOP_TEXT_LINKS,
  activeCategoryScrollOptions,
  childrenOfParent,
  desktopSubcategories,
  expandedParentSlugForActive,
  resolveNavCategories,
  subcategoryListOverflows,
  syncExpandedParentSlug,
  toggleMainExpansion,
  toggleSubcategoryShowAll,
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
  const { user, openLogin } = useAuth();
  const navigate = useNavigate();
  const mains = useMemo(() => resolveNavCategories(categories), [categories]);
  const subs = useMemo(() => desktopSubcategories(categories), [categories]);
  const [expandedParentSlug, setExpandedParentSlug] = useState<string | null>(() =>
    expandedParentSlugForActive(categories, resolveNavCategories(categories), active),
  );
  const [showAllChildren, setShowAllChildren] = useState(false);
  const [showAllParent, setShowAllParent] = useState(expandedParentSlug);
  const [childrenOverflow, setChildrenOverflow] = useState(false);
  const [desktopShowAll, setDesktopShowAll] = useState(false);
  const [desktopOverflow, setDesktopOverflow] = useState(false);
  if (showAllParent !== expandedParentSlug) {
    setShowAllParent(expandedParentSlug);
    setShowAllChildren(false);
  }

  const discoveryRef = useRef<HTMLDivElement>(null);
  const subrowRef = useRef<HTMLDivElement>(null);
  const desktopSubsRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setExpandedParentSlug((current) =>
      syncExpandedParentSlug(categories, mains, active, current),
    );
  }, [active, categories, mains]);

  useEffect(() => {
    if (window.matchMedia('(max-width: 767px)').matches) return;
    const rail = discoveryRef.current?.querySelector<HTMLElement>('nav.category-rail');
    if (!rail || rail.getClientRects().length === 0) return;
    const activeBtn = rail.querySelector<HTMLElement>(
      '.category-btn--main.is-active, .category-btn--main.is-parent-of-active',
    );
    if (!activeBtn) return;
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    activeBtn.scrollIntoView(activeCategoryScrollOptions(reduced));
  }, [active]);

  useLayoutEffect(() => {
    const row = subrowRef.current;
    if (!row) return;

    const measure = () => {
      const cap = Number.parseFloat(getComputedStyle(row).getPropertyValue('--subrow-cap'));
      const visible = row.classList.contains('is-all') && Number.isFinite(cap) ? cap : row.clientHeight;
      const next = subcategoryListOverflows(row.scrollHeight, visible);
      setChildrenOverflow((prev) => (prev === next ? prev : next));
    };

    measure();
    window.addEventListener('resize', measure);
    const observer = new ResizeObserver(measure);
    observer.observe(row);
    return () => {
      window.removeEventListener('resize', measure);
      observer.disconnect();
    };
  }, [expandedParentSlug, expandedChildrenKey(categories, expandedParentSlug), showAllChildren]);

  useLayoutEffect(() => {
    const grid = desktopSubsRef.current;
    if (!grid) return;

    const measure = () => {
      const cap = Number.parseFloat(getComputedStyle(grid).getPropertyValue('--desktop-sub-cap'));
      const visible =
        grid.classList.contains('is-all') && Number.isFinite(cap) ? cap : grid.clientHeight;
      const next = subcategoryListOverflows(grid.scrollHeight, visible);
      setDesktopOverflow((prev) => (prev === next ? prev : next));
    };

    measure();
    window.addEventListener('resize', measure);
    const observer = new ResizeObserver(measure);
    observer.observe(grid);
    return () => {
      window.removeEventListener('resize', measure);
      observer.disconnect();
    };
  }, [subs, desktopShowAll]);

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

      <nav className="desktop-rail" aria-label="子分类">
        <div className="desktop-rail-primary">
          <button
            type="button"
            className={`desktop-primary desktop-primary--hot${active === 'home' ? ' is-active' : ''}`}
            onClick={handleHome}
          >
            <HotMark />
            <span>热门</span>
          </button>
        </div>

        <div className="desktop-rail-subs">
          <div
            ref={desktopSubsRef}
            className={`desktop-sub-grid${desktopShowAll ? ' is-all' : ''}`}
          >
            {subs.map((child) => (
              <button
                key={`${child.id}-${child.slug}`}
                type="button"
                title={`${child.parentName} · ${child.name}`}
                className={`desktop-sub-btn${active === child.slug ? ' is-active' : ''}`}
                onClick={() => {
                  const parent = mains.find((main) => main.id === child.parent_id);
                  if (parent) handleChild(child, parent);
                }}
              >
                {child.name}
              </button>
            ))}
          </div>
          {desktopOverflow && (
            <button
              type="button"
              className="desktop-sub-more"
              aria-expanded={desktopShowAll}
              onClick={() => setDesktopShowAll((current) => toggleSubcategoryShowAll(current))}
            >
              {desktopShowAll ? '收起 ▴' : '更多 ▾'}
            </button>
          )}
        </div>

        <div className="desktop-rail-links">
          {DESKTOP_TEXT_LINKS.map((link) => (
            <button
              key={link.label}
              type="button"
              className="desktop-text-link"
              title={link.soon ? '即将推出' : undefined}
              aria-disabled={link.soon && user ? true : undefined}
              onClick={() => {
                if (link.soon) {
                  if (!user) openLogin('login');
                  return;
                }
                if (link.auth && !user) {
                  openLogin('login');
                  return;
                }
                if (link.href) navigate(link.href);
              }}
            >
              {link.label}
            </button>
          ))}
        </div>
      </nav>

      {/* Phone-only wrapped children. Desktop keeps inline .category-children beside the parent. */}
      <div
        className={`category-subrow${expandedChildren.length > 0 ? ' is-open' : ''}`}
        aria-hidden={expandedChildren.length === 0}
      >
        <div className="category-subrow-inner">
          <div
            ref={subrowRef}
            className={`category-subrow-scroll${showAllChildren ? ' is-all' : ''}`}
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
          {childrenOverflow && (
            <button
              type="button"
              className="category-subrow-more"
              aria-expanded={showAllChildren}
              onClick={() => setShowAllChildren((current) => toggleSubcategoryShowAll(current))}
            >
              {showAllChildren ? '收起 ▴' : '更多 ▾'}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

function HotMark() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M12 3s1 3 1 5a3 3 0 0 1-3 3c0 4 2 6 4 7 4 0 7-3.5 7-8 0-5-5-8-9-7z"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function expandedChildrenKey(categories: Category[], parentSlug: string | null): string {
  if (!parentSlug) return '';
  const parent = resolveNavCategories(categories).find((main) => main.slug === parentSlug);
  if (!parent) return parentSlug;
  return childrenOfParent(categories, parent.id)
    .map((child) => child.slug)
    .join(',');
}
