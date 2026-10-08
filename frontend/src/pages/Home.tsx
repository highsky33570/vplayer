import { useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { listCategories, listVideosPage, type Category, type Video } from '../api';
import { DiscoveryNav } from '../components/DiscoveryNav';
import { FeaturedGrid } from '../components/FeaturedGrid';
import { VideoCard } from '../components/VideoCard';

const PAGE_SIZE = 30;

export function Home() {
  const [params, setParams] = useSearchParams();
  const [cats, setCats] = useState<Category[]>([]);
  const [videos, setVideos] = useState<Video[]>([]);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const active = params.get('ch') || 'home';
  const searchQ = (params.get('q') || '').trim();
  const offsetRef = useRef(0);

  const activeCat = cats.find((c) => c.slug === active);
  // 首页 is not a category filter — omit category_id entirely.
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
  const featured = !isSearch && active === 'home' ? videos.slice(0, 5) : [];
  const emptyCategory = !loading && videos.length === 0;
  const listTitle = isSearch
    ? `搜索：${searchQ}`
    : active === 'home'
      ? '最新上架'
      : activeCat?.name || '片库';

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

            <div className="section-head">
              <div className="section-title-row">
                <span className="section-accent" aria-hidden />
                <h2>{listTitle}</h2>
              </div>
              <p className="section-count">
                {emptyCategory
                  ? isSearch
                    ? '未找到相关内容'
                    : active === 'home'
                      ? '暂无内容'
                      : '该分类暂无内容'
                  : `共 ${total} 部 · 已加载 ${videos.length}`}
              </p>
            </div>

            {emptyCategory ? (
              <p className="section-empty">
                {isSearch
                  ? `没有匹配「${searchQ}」的视频，试试其他关键词。`
                  : active === 'home'
                    ? '还没有可播放的视频。'
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
