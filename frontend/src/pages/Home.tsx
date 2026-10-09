import { useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { listCategories, listVideosPage, type Category, type Video } from '../api';
import { ContentSection } from '../components/ContentSection';
import { DiscoveryNav, resolveNavCategories } from '../components/DiscoveryNav';
import { FeaturedGrid } from '../components/FeaturedGrid';
import { VideoCard } from '../components/VideoCard';

const PAGE_SIZE = 30;

/** Default desktop `.video-grid` is 6 columns → two rows = 12 cards per section. */
const GRID_COLS_DESKTOP = 6;
const HOME_SECTION_ROWS = 2;
const HOME_SECTION_LIMIT = GRID_COLS_DESKTOP * HOME_SECTION_ROWS;

const HOME_SECTIONS: { title: string; slug?: string }[] = [
  { title: '最新上架' },
  { title: '电影', slug: 'movie' },
  { title: '连续剧', slug: 'tv' },
  { title: '综艺', slug: 'variety' },
  { title: '动漫', slug: 'anime' },
  { title: '午夜影院', slug: 'midnight' },
  { title: 'VIP蓝光影院', slug: 'vip-bluray' },
];

type HomeSectionsData = {
  latest: Video[];
  bySlug: Record<string, Video[]>;
};

export function Home() {
  const [params, setParams] = useSearchParams();
  const [cats, setCats] = useState<Category[]>([]);
  const [videos, setVideos] = useState<Video[]>([]);
  const [homeSections, setHomeSections] = useState<HomeSectionsData | null>(null);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const active = params.get('ch') || 'home';
  const searchQ = (params.get('q') || '').trim();
  const offsetRef = useRef(0);

  const activeCat = cats.find((c) => c.slug === active);
  const categoryId = active === 'home' || searchQ ? undefined : activeCat?.id;

  const catById = useMemo(() => {
    const m = new Map<number, Category>();
    cats.forEach((c) => m.set(c.id, c));
    return m;
  }, [cats]);

  const isHomeLanding = !searchQ && active === 'home';

  useEffect(() => {
    let cancelled = false;
    listCategories()
      .then((c) => {
        if (!cancelled) setCats(c);
      })
      .catch((e: Error) => {
        if (!cancelled) setError(e.message);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;

    if (isHomeLanding) {
      if (cats.length === 0) {
        return;
      }
      setLoading(true);
      setError('');
      setVideos([]);
      setHomeSections(null);

      const resolved = resolveNavCategories(cats);
      const slugToCat = new Map(resolved.map((c) => [c.slug, c]));
      const categoryFetches = HOME_SECTIONS.filter((s) => s.slug).map((s) => {
        const cat = slugToCat.get(s.slug!);
        return cat
          ? listVideosPage({ categoryId: cat.id, limit: HOME_SECTION_LIMIT, offset: 0 })
          : Promise.resolve({ data: [] as Video[], total: 0, limit: HOME_SECTION_LIMIT, offset: 0, has_more: false });
      });

      Promise.all([listVideosPage({ limit: HOME_SECTION_LIMIT, offset: 0 }), ...categoryFetches])
        .then(([latestPage, ...catPages]) => {
          if (cancelled) return;
          const bySlug: Record<string, Video[]> = {};
          HOME_SECTIONS.filter((s) => s.slug).forEach((s, i) => {
            bySlug[s.slug!] = catPages[i]?.data ?? [];
          });
          setHomeSections({ latest: latestPage.data, bySlug });
        })
        .catch((e: Error) => {
          if (!cancelled) setError(e.message);
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });
      return () => {
        cancelled = true;
      };
    }

    if (!searchQ && active !== 'home' && cats.length > 0 && categoryId == null) {
      setVideos([]);
      setTotal(0);
      setHasMore(false);
      setHomeSections(null);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError('');
    setHomeSections(null);
    offsetRef.current = 0;
    listVideosPage({
      categoryId: searchQ ? undefined : categoryId,
      q: searchQ || undefined,
      limit: PAGE_SIZE,
      offset: 0,
    })
      .then((page) => {
        if (cancelled) return;
        setVideos(page.data);
        setTotal(page.total);
        setHasMore(page.has_more);
        offsetRef.current = page.data.length;
      })
      .catch((e: Error) => {
        if (!cancelled) setError(e.message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [active, categoryId, cats, searchQ, isHomeLanding]);

  async function loadMore() {
    setLoadingMore(true);
    try {
      const page = await listVideosPage({
        categoryId: searchQ ? undefined : categoryId,
        q: searchQ || undefined,
        limit: PAGE_SIZE,
        offset: offsetRef.current,
      });
      setVideos((prev) => [...prev, ...page.data]);
      setTotal(page.total);
      setHasMore(page.has_more);
      offsetRef.current += page.data.length;
    } catch (e) {
      setError(e instanceof Error ? e.message : '加载失败');
    } finally {
      setLoadingMore(false);
    }
  }

  function selectChannel(slug: string) {
    if (slug === 'home') setParams({});
    else setParams({ ch: slug });
  }

  const isSearch = Boolean(searchQ);
  const featured =
    isHomeLanding && homeSections ? homeSections.latest.slice(0, 5) : [];
  const emptyCategory = !loading && !isHomeLanding && videos.length === 0;
  const listTitle = isSearch
    ? `搜索：${searchQ}`
    : activeCat?.name || '片库';

  if (error && !loading && (isHomeLanding ? !homeSections : videos.length === 0)) {
    return (
      <div className="page page-error">
        加载失败：{error}
        <br />
        请确认 API 已在 :8080 运行
      </div>
    );
  }

  function renderHomeSkeleton() {
    return (
      <>
        <div className="skel skel-featured" />
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="content-section">
            <div className="skel skel-line" style={{ width: '8rem', marginBottom: 12 }} />
            <div className="video-grid">
              {Array.from({ length: HOME_SECTION_LIMIT }).map((__, j) => (
                <div key={j} className="skel-card">
                  <div className="skel skel-thumb" />
                  <div className="skel skel-line" />
                </div>
              ))}
            </div>
          </div>
        ))}
      </>
    );
  }

  function renderHomeLanding() {
    const latest = homeSections?.latest ?? [];
    return (
      <>
        {featured.length > 0 && <FeaturedGrid videos={featured} />}

        {HOME_SECTIONS.map((section) => {
          const items = section.slug
            ? (homeSections?.bySlug[section.slug] ?? [])
            : latest;
          const moreTo = section.slug ? `/?ch=${section.slug}` : undefined;

          return (
            <ContentSection
              key={section.title}
              title={section.title}
              moreTo={moreTo}
              videos={items}
              emptyText={
                section.slug
                  ? `${section.title}暂无内容`
                  : '还没有可播放的视频'
              }
            />
          );
        })}
      </>
    );
  }

  return (
    <>
      <DiscoveryNav categories={cats} active={active} onSelect={selectChannel} />

      <div className="page">
        {loading ? (
          isHomeLanding ? (
            renderHomeSkeleton()
          ) : (
            <>
              <div className="skel skel-featured" />
              <div className="video-grid">
                {Array.from({ length: 12 }).map((_, i) => (
                  <div key={i} className="skel-card">
                    <div className="skel skel-thumb" />
                    <div className="skel skel-line" />
                    <div className="skel skel-line short" />
                  </div>
                ))}
              </div>
            </>
          )
        ) : isHomeLanding ? (
          renderHomeLanding()
        ) : (
          <>
            <div className="section-head">
              <div className="section-title-row">
                <span className="section-accent" aria-hidden />
                <h2>{listTitle}</h2>
              </div>
              <p className="section-count">
                {emptyCategory
                  ? isSearch
                    ? '未找到相关内容'
                    : '该分类暂无内容'
                  : `共 ${total} 部 · 已加载 ${videos.length}`}
              </p>
            </div>

            {emptyCategory ? (
              <p className="section-empty">
                {isSearch
                  ? `没有匹配「${searchQ}」的视频，试试其他关键词。`
                  : '该分类下还没有视频。同步完成后会出现在这里。'}
              </p>
            ) : (
              <>
                <div className="video-grid">
                  {videos.map((v) => (
                    <VideoCard
                      key={`grid-${v.id}`}
                      video={v}
                      categoryName={catById.get(v.category_id)?.name}
                    />
                  ))}
                </div>
                {hasMore && (
                  <div className="load-more-wrap">
                    <button
                      type="button"
                      className="section-action"
                      disabled={loadingMore}
                      onClick={() => void loadMore()}
                    >
                      {loadingMore ? '加载中…' : '加载更多'}
                    </button>
                  </div>
                )}
              </>
            )}
          </>
        )}
      </div>

      <div className="dock">
        <button
          type="button"
          aria-label="回到顶部"
          onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}
        >
          ↑
        </button>
      </div>
    </>
  );
}
