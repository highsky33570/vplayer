import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  fetchMeSummary,
  formatDuration,
  listCategories,
  listFavoriteFolders,
  listFavoritesPage,
  removeFavorite,
  type Category,
  type FavoriteVideo,
  type MeSummary,
} from '../api';
import { appendUnique, removeById } from '../account/accountLogic';
import { AuthGate } from '../auth/AuthGate';
import { formatHistoryTime, normalizeFolders, type FolderItem } from '../components/headerFlyoutData';
import { HoverPreviewThumb } from '../components/HoverPreviewThumb';
import { coverImgSrc, onCoverImgError } from '../media/coverFallback';
import {
  FAVORITES_PERSONAL_NAV,
  FAVORITES_SORT_OPTIONS,
  applyFavoritesView,
  favoritesCategoryOptions,
  favoritesMainSlugByCategoryId,
  folderPreviewCover,
  initialFavoriteFolderId,
  type FavoritesSort,
} from './favoritesPageLogic';

const PAGE_SIZE = 24;

export function FavoritesPage() {
  return <AuthGate>{({ token }) => <FavoritesContent token={token} />}</AuthGate>;
}

function FavoritesContent({ token }: { token: string }) {
  const [summary, setSummary] = useState<MeSummary | null>(null);
  const [folders, setFolders] = useState<FolderItem[]>(() => normalizeFolders([]));
  const [folderId, setFolderId] = useState(0);
  const [folderReady, setFolderReady] = useState(false);
  const [categories, setCategories] = useState<Category[]>([]);
  const [items, setItems] = useState<FavoriteVideo[]>([]);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState('');
  const [query, setQuery] = useState('');
  const [sort, setSort] = useState<FavoritesSort>('recent');
  const [categorySlug, setCategorySlug] = useState('all');
  const [managing, setManaging] = useState(false);
  const [selected, setSelected] = useState<Set<number>>(() => new Set());
  const [busy, setBusy] = useState(false);
  const requestRef = useRef(0);

  const refreshSummary = useCallback(() => {
    fetchMeSummary(token)
      .then(setSummary)
      .catch(() => setSummary(null));
  }, [token]);

  const loadFolders = useCallback(() => {
    listFavoriteFolders(token)
      .then((rows) => {
        const next = normalizeFolders(rows);
        setFolders(next);
        setFolderId((current) => {
          if (!folderReady) return initialFavoriteFolderId(next);
          if (current === 0) return 0;
          if (next.some((f) => f.id === current)) return current;
          return initialFavoriteFolderId(next);
        });
        setFolderReady(true);
      })
      .catch(() => {
        setFolders(normalizeFolders([]));
        setFolderReady(true);
      });
  }, [token, folderReady]);

  useEffect(() => {
    loadFolders();
    refreshSummary();
    listCategories()
      .then(setCategories)
      .catch(() => setCategories([]));
  }, [token, loadFolders, refreshSummary]);

  useEffect(() => {
    if (!folderReady) return;
    const request = ++requestRef.current;
    setLoading(true);
    setError('');
    setSelected(new Set());
    listFavoritesPage(token, {
      folderId: folderId > 0 ? folderId : undefined,
      limit: PAGE_SIZE,
      offset: 0,
    })
      .then((page) => {
        if (requestRef.current !== request) return;
        setItems(page.data);
        setTotal(page.total);
        setHasMore(page.has_more);
      })
      .catch((e) => {
        if (requestRef.current !== request) return;
        setItems([]);
        setError(e instanceof Error ? e.message : '加载失败');
      })
      .finally(() => {
        if (requestRef.current === request) setLoading(false);
      });
  }, [token, folderId, folderReady]);

  const mainByCategoryId = useMemo(() => favoritesMainSlugByCategoryId(categories), [categories]);
  const categoryOptions = useMemo(
    () => favoritesCategoryOptions(items, categories),
    [items, categories],
  );
  const visible = useMemo(
    () => applyFavoritesView(items, { query, categorySlug, sort }, mainByCategoryId),
    [items, query, categorySlug, sort, mainByCategoryId],
  );

  const activeFolder = folders.find((f) => f.id === folderId);
  const activeName =
    folderId === 0 ? '全部收藏' : (activeFolder?.name ?? '默认收藏');
  const favCount = summary?.favorites ?? folders.reduce((n, f) => n + f.count, 0);
  const histCount = summary?.history;
  const previewCover = folderPreviewCover(items);
  const firstWatchHref = visible[0] ? `/watch/${visible[0].id}` : null;
  const filtersOn = query.trim() !== '' || categorySlug !== 'all' || sort !== 'recent';
  const allVisibleSelected =
    visible.length > 0 && visible.every((item) => selected.has(item.id));

  async function loadMore() {
    const request = requestRef.current;
    setLoadingMore(true);
    try {
      const page = await listFavoritesPage(token, {
        folderId: folderId > 0 ? folderId : undefined,
        limit: PAGE_SIZE,
        offset: items.length,
      });
      if (requestRef.current !== request) return;
      setItems((current) => appendUnique(current, page.data));
      setTotal(page.total);
      setHasMore(page.has_more);
    } catch (e) {
      if (requestRef.current === request) setError(e instanceof Error ? e.message : '加载失败');
    } finally {
      setLoadingMore(false);
    }
  }

  async function onRemove(video: FavoriteVideo) {
    try {
      await removeFavorite(token, video.id);
      setItems((current) => removeById(current, video.id));
      setTotal((n) => Math.max(0, n - 1));
      setSelected((current) => {
        if (!current.has(video.id)) return current;
        const next = new Set(current);
        next.delete(video.id);
        return next;
      });
      loadFolders();
      refreshSummary();
    } catch (e) {
      setError(e instanceof Error ? e.message : '取消收藏失败');
    }
  }

  function enterManage() {
    setManaging(true);
    setSelected(new Set());
  }

  function cancelManage() {
    setManaging(false);
    setSelected(new Set());
  }

  function toggleSelect(id: number) {
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function toggleSelectAll() {
    if (allVisibleSelected) {
      setSelected(new Set());
      return;
    }
    setSelected(new Set(visible.map((item) => item.id)));
  }

  async function onBatchRemove() {
    const ids = [...selected];
    if (ids.length === 0) return;
    if (!window.confirm(`确认将选中的 ${ids.length} 个视频移出收藏？`)) return;
    setBusy(true);
    setError('');
    try {
      for (const id of ids) {
        await removeFavorite(token, id);
      }
      const gone = new Set(ids);
      setItems((current) => current.filter((row) => !gone.has(row.id)));
      setTotal((n) => Math.max(0, n - ids.length));
      setSelected(new Set());
      setManaging(false);
      loadFolders();
      refreshSummary();
    } catch (e) {
      setError(e instanceof Error ? e.message : '批量取消收藏失败');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="fav-dest" data-testid="favorites-page">
      <div className="fav-workspace" data-testid="favorites-workspace">
        <nav className="fav-personal-nav" aria-label="个人空间导航">
          <div className="fav-personal-links">
            {FAVORITES_PERSONAL_NAV.map((item) => (
              <Link
                key={item.id}
                to={item.href}
                className={item.id === 'favorites' ? 'is-active' : undefined}
                aria-current={item.id === 'favorites' ? 'page' : undefined}
              >
                {item.label}
              </Link>
            ))}
          </div>
          <dl className="fav-personal-stats" aria-label="我的数据">
            <div>
              <dd>{favCount}</dd>
              <dt>收藏</dt>
            </div>
            <div>
              <dd>{histCount ?? '—'}</dd>
              <dt>历史</dt>
            </div>
          </dl>
        </nav>

        <div className="fav-layout">
          <aside className="fav-sidebar" aria-label="我的收藏夹">
            <p className="fav-sidebar-title">我创建的收藏夹</p>
            <button
              type="button"
              className={folderId === 0 ? 'is-active' : undefined}
              onClick={() => {
                setFolderId(0);
                setManaging(false);
                setSelected(new Set());
              }}
            >
              <span className="fav-folder-icon" aria-hidden />
              <span className="fav-folder-name">全部收藏</span>
              <em>{favCount}</em>
            </button>
            {folders
              .filter((folder) => folder.id > 0)
              .map((folder) => (
                <button
                  key={folder.id}
                  type="button"
                  className={folderId === folder.id ? 'is-active' : undefined}
                  onClick={() => {
                    setFolderId(folder.id);
                    setManaging(false);
                    setSelected(new Set());
                  }}
                >
                  <span className="fav-folder-icon" aria-hidden />
                  <span className="fav-folder-name">{folder.name}</span>
                  <em>{folder.count}</em>
                </button>
              ))}
          </aside>

          <section className="fav-main" aria-label={activeName}>
            <header className="fav-folder-head">
              <div className="fav-folder-preview" aria-hidden>
                {previewCover ? (
                  <img src={coverImgSrc(previewCover)} alt="" onError={onCoverImgError} />
                ) : (
                  <span className="fav-folder-placeholder" />
                )}
              </div>
              <div className="fav-folder-meta">
                <h1 className="fav-folder-title">{activeName}</h1>
                <p className="fav-folder-count" data-testid="favorites-folder-count">
                  视频数 {total}
                </p>
                <div className="fav-folder-actions">
                  {firstWatchHref ? (
                    <Link
                      to={firstWatchHref}
                      className="me-btn me-btn--primary"
                      data-testid="favorites-play-first"
                    >
                      播放第一个
                    </Link>
                  ) : null}
                  {!managing && items.length > 0 ? (
                    <button
                      type="button"
                      className="fav-tool-btn"
                      onClick={enterManage}
                      data-testid="favorites-batch"
                    >
                      批量管理
                    </button>
                  ) : null}
                </div>
              </div>
            </header>

            <div className="fav-toolbar" data-testid="favorites-toolbar">
              {!managing ? (
                <>
                  <nav className="fav-sort-tabs" aria-label="排序">
                    {FAVORITES_SORT_OPTIONS.map((opt) => (
                      <button
                        key={opt.id}
                        type="button"
                        className={sort === opt.id ? 'is-active' : undefined}
                        onClick={() => setSort(opt.id)}
                      >
                        {opt.label}
                      </button>
                    ))}
                  </nav>
                  <div className="fav-tools">
                    {categoryOptions.length > 1 ? (
                      <label className="fav-category">
                        <span className="sr-only">分类筛选</span>
                        <select
                          value={categorySlug}
                          onChange={(e) => setCategorySlug(e.target.value)}
                          data-testid="favorites-category"
                        >
                          {categoryOptions.map((opt) => (
                            <option key={opt.id} value={opt.id}>
                              {opt.label}
                            </option>
                          ))}
                        </select>
                      </label>
                    ) : null}
                    <label className="fav-search">
                      <span className="sr-only">搜索收藏视频</span>
                      <input
                        value={query}
                        onChange={(e) => setQuery(e.target.value)}
                        placeholder="搜索收藏视频"
                        autoComplete="off"
                        data-testid="favorites-search"
                      />
                    </label>
                  </div>
                </>
              ) : (
                <div className="fav-batch-bar" data-testid="favorites-batch-bar">
                  <button type="button" className="fav-tool-btn" onClick={toggleSelectAll}>
                    {allVisibleSelected ? '取消全选' : '全选'}
                  </button>
                  <button
                    type="button"
                    className="fav-tool-btn fav-tool-btn--danger"
                    disabled={busy || selected.size === 0}
                    onClick={() => void onBatchRemove()}
                    data-testid="favorites-batch-remove"
                  >
                    移出收藏（{selected.size}）
                  </button>
                  <button type="button" className="fav-tool-btn" onClick={cancelManage}>
                    取消管理
                  </button>
                </div>
              )}
            </div>

            {error ? (
              <p className="me-feedback is-error" role="alert">
                {error}
              </p>
            ) : null}

            {loading ? (
              <p className="me-status fav-status">加载中…</p>
            ) : visible.length === 0 ? (
              <div className="fav-empty" data-testid="favorites-empty">
                <div className="fav-empty-icon" aria-hidden />
                <p>{items.length === 0 ? '这里还没有收藏的视频' : '没有匹配的收藏标题'}</p>
                <p className="fav-empty-hint">
                  {items.length === 0 ? '去首页看看喜欢的内容吧' : '试试调整搜索或筛选'}
                </p>
                {items.length === 0 ? (
                  <Link to="/" className="me-btn me-btn--primary">
                    去首页逛逛
                  </Link>
                ) : (
                  <button
                    type="button"
                    className="me-btn"
                    onClick={() => {
                      setQuery('');
                      setCategorySlug('all');
                      setSort('recent');
                    }}
                  >
                    清除筛选
                  </button>
                )}
              </div>
            ) : (
              <>
                <div className="fav-grid" data-testid="favorites-grid">
                  {visible.map((video) => {
                    const checked = selected.has(video.id);
                    return (
                      <article
                        key={video.id}
                        className={`fav-card${managing && checked ? ' is-selected' : ''}`}
                        data-testid={`favorites-card-${video.id}`}
                      >
                        {managing ? (
                          <label className="fav-card-check">
                            <input
                              type="checkbox"
                              checked={checked}
                              onChange={() => toggleSelect(video.id)}
                              aria-label={`选择 ${video.title}`}
                            />
                          </label>
                        ) : null}
                        <Link
                          to={`/watch/${video.id}`}
                          className="fav-card-link"
                          title={video.title}
                          onClick={(e) => {
                            if (managing) {
                              e.preventDefault();
                              toggleSelect(video.id);
                            }
                          }}
                        >
                          <HoverPreviewThumb
                            videoId={video.id}
                            coverUrl={video.cover_url}
                            durationSec={video.duration_sec}
                            className="fav-card-thumb"
                          >
                            {video.duration_sec > 0 ? (
                              <span className="fav-card-duration">
                                {formatDuration(video.duration_sec)}
                              </span>
                            ) : null}
                          </HoverPreviewThumb>
                          <span className="fav-card-title">{video.title}</span>
                          <span className="fav-card-sub">
                            {formatHistoryTime(video.favorited_at)}
                          </span>
                        </Link>
                        {!managing ? (
                          <button
                            type="button"
                            className="fav-card-remove"
                            aria-label={`取消收藏：${video.title}`}
                            data-testid={`favorites-remove-${video.id}`}
                            onClick={(e) => {
                              e.preventDefault();
                              e.stopPropagation();
                              void onRemove(video);
                            }}
                          >
                            <span className="sr-only">取消收藏</span>
                          </button>
                        ) : null}
                      </article>
                    );
                  })}
                </div>
                {hasMore && !filtersOn ? (
                  <div className="load-more-wrap">
                    <button
                      type="button"
                      className="me-btn"
                      disabled={loadingMore}
                      onClick={() => void loadMore()}
                      data-testid="favorites-load-more"
                    >
                      {loadingMore ? '加载中…' : '加载更多'}
                    </button>
                  </div>
                ) : hasMore && filtersOn ? (
                  <p className="fav-filter-note">筛选作用于已加载的收藏；可先加载更多再筛选。</p>
                ) : null}
              </>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}
