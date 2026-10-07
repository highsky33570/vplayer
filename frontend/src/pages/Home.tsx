import { useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { listCategories, listVideosPage, type Category, type Video } from '../api';
import { ContentSection } from '../components/ContentSection';
import { DiscoveryNav } from '../components/DiscoveryNav';
import { FeaturedGrid } from '../components/FeaturedGrid';
import { VideoCard } from '../components/VideoCard';

const PAGE_SIZE = 30;
const SECTION_SIZE = 6;

export function Home() {
  const [params, setParams] = useSearchParams();
  const [cats, setCats] = useState<Category[]>([]);
  const [videos, setVideos] = useState<Video[]>([]);
  const [homePool, setHomePool] = useState<Video[]>([]);
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

  // Prefetch a larger pool for home sections
  useEffect(() => {
    let cancelled = false;
    listVideosPage({ limit: 48, offset: 0 })
      .then((page) => {
        if (!cancelled) setHomePool(page.data);
      })
      .catch(() => {
        /* ignore — main load handles errors */
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    if (!searchQ && active !== 'home' && cats.length > 0 && categoryId == null) {
      setVideos([]);
      setTotal(0);
      setHasMore(false);
      setLoading(false);
      return;
    }
    setLoading(true);
    setError('');
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
  }, [active, categoryId, cats.length, searchQ]);

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
  const pool = !isSearch && active === 'home' ? (homePool.length ? homePool : videos) : videos;
  const featured = !isSearch ? pool.slice(0, 5) : [];
  const emptyCategory = !loading && videos.length === 0;

  const sections = useMemo(() => {
    if (isSearch || active !== 'home') return [];
    const source = homePool.length ? homePool : videos;
    const out: { title: string; slug: string; items: Video[] }[] = [
      { title: '热门推荐', slug: 'home', items: source.slice(0, SECTION_SIZE) },
    ];
    for (const c of cats.filter((x) => x.slug !== 'home')) {
      const items = source.filter((v) => v.category_id === c.id).slice(0, SECTION_SIZE);
      if (items.length) out.push({ title: c.name, slug: c.slug, items });
    }
    return out;
  }, [active, cats, homePool, videos, isSearch]);

  if (error && videos.length === 0 && !loading) {
    return (
      <div className="page page-error">
        加载失败：{error}
        <br />
        请确认 API 已在 :8080 运行
      </div>
    );
  }

  return (
    <>
      <DiscoveryNav categories={cats} active={active} onSelect={selectChannel} />

      <div className="page">
        {loading ? (
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
        ) : (
          <>
            {featured.length > 0 && <FeaturedGrid videos={featured} />}

            {isSearch ? (
              <>
                <div className="section-head">
                  <div className="section-title-row">
                    <span className="section-accent" aria-hidden />
                    <h2>搜索：{searchQ}</h2>
                  </div>
                  <p className="section-count">
                    {emptyCategory ? '未找到相关内容' : `共 ${total} 部 · 已加载 ${videos.length}`}
                  </p>
                </div>
                {emptyCategory ? (
                  <p className="section-empty">没有匹配「{searchQ}」的视频，试试其他关键词。</p>
                ) : (
                  <>
                    <div className="video-grid">
                      {videos.map((v) => (
                        <VideoCard
                          key={`search-${v.id}`}
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
            ) : active === 'home' ? (
              sections.map((s) => (
                <ContentSection
                  key={s.title}
                  title={s.title}
                  moreTo={s.slug === 'home' ? undefined : `/?ch=${s.slug}`}
                  videos={s.items}
                />
              ))
            ) : (
              <>
                <div className="section-head">
                  <div className="section-title-row">
                    <span className="section-accent" aria-hidden />
                    <h2>{activeCat?.name || '片库'}</h2>
                  </div>
                  <p className="section-count">
                    {emptyCategory ? '该分类暂无内容' : `共 ${total} 部 · 已加载 ${videos.length}`}
                  </p>
                </div>

                {emptyCategory ? (
                  <p className="section-empty">该分类下还没有视频。同步完成后会出现在这里。</p>
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
