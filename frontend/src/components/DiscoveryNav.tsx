import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { listCategories, type Category } from '../api';
import {
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

export function DiscoveryNav({ channelHost = null }: { channelHost?: HTMLElement | null }) {
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const [params] = useSearchParams();
  const [categories, setCategories] = useState<Category[]>([]);
  const routeChannel = params.get('ch') || 'home';
  const active = pathname === '/' ? routeChannel : '';

  function onSelect(slug: string) {
    if (slug === 'home') navigate('/');
    else navigate(`/?ch=${encodeURIComponent(slug)}`);
  }

  useEffect(() => {
    let cancelled = false;
    listCategories()
      .then((rows) => {
        if (!cancelled) setCategories(rows);
      })
      .catch(() => {
        if (!cancelled) setCategories([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const mains = useMemo(() => resolveNavCategories(categories), [categories]);
  const subs = useMemo(() => desktopSubcategories(categories), [categories]);
  const selectedMain = useMemo(() => {
    return (
      mains.find((main) => main.slug === active) ??
      mains.find((main) => childrenOfParent(categories, main.id).some((child) => child.slug === active)) ??
      null
    );
  }, [mains, categories, active]);
  const catalogSubs = useMemo(
    () => (selectedMain ? subs.filter((child) => child.parent_id === selectedMain.id) : subs),
    [subs, selectedMain],
  );
  const [expandedParentSlug, setExpandedParentSlug] = useState<string | null>(() =>
    expandedParentSlugForActive(categories, resolveNavCategories(categories), active),
  );
  const [showAllChildren, setShowAllChildren] = useState(false);
  const [showAllParent, setShowAllParent] = useState(expandedParentSlug);
  const [childrenOverflow, setChildrenOverflow] = useState(false);
  const [catalogOpen, setCatalogOpen] = useState(false);
  const [catalogOverflow, setCatalogOverflow] = useState(false);
  const [wide, setWide] = useState(
    () => typeof window !== 'undefined' && window.matchMedia('(min-width: 768px)').matches,
  );
  if (showAllParent !== expandedParentSlug) {
    setShowAllParent(expandedParentSlug);
    setShowAllChildren(false);
  }

  const discoveryRef = useRef<HTMLDivElement>(null);
  const subrowRef = useRef<HTMLDivElement>(null);
  const catalogRef = useRef<HTMLElement>(null);

  useEffect(() => {
    const mq = window.matchMedia('(min-width: 768px)');
    const apply = () => setWide(mq.matches);
    apply();
    mq.addEventListener('change', apply);
    return () => mq.removeEventListener('change', apply);
  }, []);

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
    const row = catalogRef.current;
    if (!row) return;
    const measure = () => {
      const next = subcategoryListOverflows(row.scrollHeight, row.clientHeight);
      setCatalogOverflow((prev) => (prev === next ? prev : next));
    };
    measure();
    window.addEventListener('resize', measure);
    const observer = new ResizeObserver(measure);
    observer.observe(row);
    return () => {
      window.removeEventListener('resize', measure);
      observer.disconnect();
    };
  }, [subs, catalogOpen, selectedMain]);

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

  const channelRail = (
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
  );

  return (
    <div className="discovery" ref={discoveryRef}>
      {wide && channelHost ? createPortal(channelRail, channelHost) : channelRail}

      {catalogSubs.length > 0 ? (
        <div className="discovery-catalog-wrap">
          <nav
            ref={catalogRef}
            className={`discovery-catalog${catalogOpen ? ' is-all' : ''}`}
            aria-label={selectedMain ? `${selectedMain.name}子分类` : '子分类目录'}
          >
            {catalogSubs.map((child) => (
              <button
                key={`catalog-${child.id}-${child.slug}`}
                type="button"
                className={`discovery-cell${active === child.slug ? ' is-active' : ''}`}
                title={`${child.parentName} · ${child.name}`}
                onClick={() => {
                  const parent = mains.find((main) => main.id === child.parent_id);
                  if (parent) handleChild(child, parent);
                }}
              >
                {child.name}
              </button>
            ))}
          </nav>
          {catalogOpen || catalogOverflow ? (
            <button
              type="button"
              className="discovery-catalog-more"
              aria-expanded={catalogOpen}
              onClick={() => setCatalogOpen((current) => toggleSubcategoryShowAll(current))}
            >
              {catalogOpen ? '收起' : '更多'}
            </button>
          ) : null}
        </div>
      ) : null}

      {/* Second text row. Shown only while a channel with children is open. */}
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

function expandedChildrenKey(categories: Category[], parentSlug: string | null): string {
  if (!parentSlug) return '';
  const parent = resolveNavCategories(categories).find((main) => main.slug === parentSlug);
  if (!parent) return parentSlug;
  return childrenOfParent(categories, parent.id)
    .map((child) => child.slug)
    .join(',');
}
