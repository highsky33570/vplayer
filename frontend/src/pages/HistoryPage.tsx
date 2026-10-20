import { useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  clearHistory,
  deleteHistoryItem,
  formatDuration,
  listCategories,
  listHistoryPage,
  type Category,
  type HistoryVideo,
} from '../api';
import { appendUnique, removeById } from '../account/accountLogic';
import { AuthGate } from '../auth/AuthGate';
import { coverImgSrc, onCoverImgError } from '../media/coverFallback';
import { formatProgress } from '../components/headerFlyoutData';
import {
  HISTORY_STATUS_FILTER_OPTIONS,
  HISTORY_TIME_FILTER_OPTIONS,
  applyHistoryFilters,
  formatHistoryCardTime,
  formatHistoryProgressClock,
  groupHistoryItems,
  historyFiltersActive,
  historyMainSlugByCategoryId,
  historyPrimaryTabs,
  historyProgressRatio,
  isHistoryCompleted,
  type HistoryFilterState,
  type HistoryStatusFilter,
  type HistoryTimeFilter,
} from './historyPageLogic';

const PAGE_SIZE = 36;

const INITIAL_FILTERS: HistoryFilterState = {
  query: '',
  mainSlug: 'all',
  time: 'all',
  status: 'all',
};

export function HistoryPage() {
  return <AuthGate>{({ token }) => <HistoryContent token={token} />}</AuthGate>;
}

function HistoryContent({ token }: { token: string }) {
  const [items, setItems] = useState<HistoryVideo[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState('');
  const [filters, setFilters] = useState<HistoryFilterState>(INITIAL_FILTERS);
  const [moreOpen, setMoreOpen] = useState(false);
  const [managing, setManaging] = useState(false);
  const [selected, setSelected] = useState<Set<number>>(() => new Set());
  const [busy, setBusy] = useState(false);
  const requestRef = useRef(0);
  const moreRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    listCategories()
      .then(setCategories)
      .catch(() => setCategories([]));
  }, []);

  useEffect(() => {
    const request = ++requestRef.current;
    listHistoryPage(token, { limit: PAGE_SIZE, offset: 0 })
      .then((page) => {
        if (requestRef.current !== request) return;
        setItems(page.data);
        setTotal(page.total);
        setHasMore(page.has_more);
      })
      .catch((e) => {
        if (requestRef.current === request) setError(e instanceof Error ? e.message : '加载失败');
      })
      .finally(() => {
        if (requestRef.current === request) setLoading(false);
      });
  }, [token]);

  useEffect(() => {
    if (!moreOpen) return;
    function onDoc(e: MouseEvent) {
      if (moreRef.current && !moreRef.current.contains(e.target as Node)) {
        setMoreOpen(false);
      }
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setMoreOpen(false);
    }
    document.addEventListener('mousedown', onDoc);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDoc);
      document.removeEventListener('keydown', onKey);
    };
  }, [moreOpen]);

  const mainByCategoryId = useMemo(() => historyMainSlugByCategoryId(categories), [categories]);
  const primaryTabs = useMemo(() => historyPrimaryTabs(items, categories), [items, categories]);
  const filtered = useMemo(
    () => applyHistoryFilters(items, filters, mainByCategoryId),
    [items, filters, mainByCategoryId],
  );
  const groups = useMemo(() => groupHistoryItems(filtered), [filtered]);
  const filtersOn = historyFiltersActive(filters);
  const allVisibleSelected =
    filtered.length > 0 && filtered.every((item) => selected.has(item.id));

  function patchFilters(patch: Partial<HistoryFilterState>) {
    setFilters((current) => ({ ...current, ...patch }));
  }

  async function loadMore() {
    setLoadingMore(true);
    try {
      const page = await listHistoryPage(token, { limit: PAGE_SIZE, offset: items.length });
      setItems((current) => appendUnique(current, page.data));
      setTotal(page.total);
      setHasMore(page.has_more);
    } catch (e) {
      setError(e instanceof Error ? e.message : '加载失败');
    } finally {
      setLoadingMore(false);
    }
  }

  async function onRemove(video: HistoryVideo) {
    try {
      await deleteHistoryItem(token, video.id);
      setItems((current) => removeById(current, video.id));
      setTotal((n) => Math.max(0, n - 1));
      setSelected((current) => {
        if (!current.has(video.id)) return current;
        const next = new Set(current);
        next.delete(video.id);
        return next;
      });
    } catch (e) {
      setError(e instanceof Error ? e.message : '删除失败');
    }
  }

  async function onClear() {
    if (!window.confirm('确认清空全部观看历史？此操作无法撤销。')) return;
    setBusy(true);
    try {
      await clearHistory(token);
      setItems([]);
      setTotal(0);
      setHasMore(false);
      setSelected(new Set());
      setManaging(false);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : '清空失败');
    } finally {
      setBusy(false);
    }
  }

  function enterManage() {
    setManaging(true);
    setSelected(new Set());
    setMoreOpen(false);
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
    setSelected(new Set(filtered.map((item) => item.id)));
  }

  async function onBatchDelete() {
    const ids = [...selected];
    if (ids.length === 0) return;
    if (!window.confirm(`确认删除选中的 ${ids.length} 条历史记录？`)) return;
    setBusy(true);
    setError('');
    try {
      for (const id of ids) {
        await deleteHistoryItem(token, id);
      }
      const gone = new Set(ids);
      setItems((current) => current.filter((row) => !gone.has(row.id)));
      setTotal((n) => Math.max(0, n - ids.length));
      setSelected(new Set());
      setManaging(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : '批量删除失败');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="hist-dest" data-testid="history-page">
      <div className="hist-workspace" data-testid="history-workspace">
        <header className="hist-head">
          <h1 className="hist-title">
            <span className="hist-title-icon" aria-hidden />
            历史记录
          </h1>
          <p className="hist-count" data-testid="history-count">
            共 {total} 条
          </p>
        </header>

        <div className="hist-toolbar" data-testid="history-toolbar">
          {!managing ? (
            <>
              <nav className="hist-tabs" aria-label="历史分类">
                {primaryTabs.map((tab) => (
                  <button
                    key={tab.id}
                    type="button"
                    className={filters.mainSlug === tab.id ? 'is-active' : undefined}
                    aria-current={filters.mainSlug === tab.id ? 'page' : undefined}
                    onClick={() => patchFilters({ mainSlug: tab.id })}
                  >
                    {tab.label}
                  </button>
                ))}
              </nav>

              <div className="hist-tools">
                <div className="hist-more" ref={moreRef}>
                  <button
                    type="button"
                    className={`hist-tool-btn${moreOpen || filters.time !== 'all' || filters.status !== 'all' ? ' is-active' : ''}`}
                    aria-expanded={moreOpen}
                    aria-haspopup="dialog"
                    onClick={() => setMoreOpen((v) => !v)}
                  >
                    更多筛选
                  </button>
                  {moreOpen ? (
                    <div className="hist-more-panel" role="dialog" aria-label="更多筛选" data-testid="history-more-filters">
                      <p className="hist-more-label">观看时间</p>
                      <div className="hist-more-options">
                        {HISTORY_TIME_FILTER_OPTIONS.map((opt) => (
                          <button
                            key={opt.id}
                            type="button"
                            className={filters.time === opt.id ? 'is-active' : undefined}
                            onClick={() => patchFilters({ time: opt.id as HistoryTimeFilter })}
                          >
                            {opt.label}
                          </button>
                        ))}
                      </div>
                      <p className="hist-more-label">观看状态</p>
                      <div className="hist-more-options">
                        {HISTORY_STATUS_FILTER_OPTIONS.map((opt) => (
                          <button
                            key={opt.id}
                            type="button"
                            className={filters.status === opt.id ? 'is-active' : undefined}
                            onClick={() => patchFilters({ status: opt.id as HistoryStatusFilter })}
                          >
                            {opt.label}
                          </button>
                        ))}
                      </div>
                      {filters.time !== 'all' || filters.status !== 'all' ? (
                        <button
                          type="button"
                          className="hist-tool-btn hist-more-reset"
                          onClick={() => patchFilters({ time: 'all', status: 'all' })}
                        >
                          重置筛选
                        </button>
                      ) : null}
                    </div>
                  ) : null}
                </div>

                <label className="hist-search">
                  <span className="sr-only">搜索历史记录</span>
                  <input
                    value={filters.query}
                    onChange={(e) => patchFilters({ query: e.target.value })}
                    placeholder="搜索标题"
                    autoComplete="off"
                    data-testid="history-search"
                  />
                </label>

                {items.length > 0 ? (
                  <button
                    type="button"
                    className="hist-tool-btn hist-tool-btn--danger"
                    disabled={busy}
                    onClick={() => void onClear()}
                    data-testid="history-clear"
                  >
                    清空历史
                  </button>
                ) : null}

                {items.length > 0 ? (
                  <button
                    type="button"
                    className="hist-tool-btn"
                    onClick={enterManage}
                    data-testid="history-batch"
                  >
                    批量管理
                  </button>
                ) : null}
              </div>
            </>
          ) : (
            <div className="hist-batch-bar" data-testid="history-batch-bar">
              <button type="button" className="hist-tool-btn" onClick={toggleSelectAll}>
                {allVisibleSelected ? '取消全选' : '全选'}
              </button>
              <button
                type="button"
                className="hist-tool-btn hist-tool-btn--danger"
                disabled={busy || selected.size === 0}
                onClick={() => void onBatchDelete()}
                data-testid="history-batch-delete"
              >
                删除所选（{selected.size}）
              </button>
              <button type="button" className="hist-tool-btn" onClick={cancelManage}>
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
          <p className="me-status hist-status">加载中…</p>
        ) : filtered.length === 0 ? (
          <div className="hist-empty" data-testid="history-empty">
            <div className="hist-empty-icon" aria-hidden />
            <p>{items.length === 0 ? '暂无观看历史' : '没有匹配的历史记录'}</p>
            <p className="hist-empty-hint">
              {items.length === 0 ? '可以去首页看看感兴趣的视频' : '试试调整搜索或筛选条件'}
            </p>
            {items.length === 0 ? (
              <Link to="/" className="me-btn me-btn--primary">
                去首页逛逛
              </Link>
            ) : filtersOn ? (
              <button
                type="button"
                className="me-btn"
                onClick={() => setFilters(INITIAL_FILTERS)}
              >
                清除筛选
              </button>
            ) : null}
          </div>
        ) : (
          <div className="hist-timeline" data-testid="history-timeline">
            {groups.map((group) => (
              <section key={group.id} className="hist-group" aria-label={group.label}>
                <div className="hist-group-rail">
                  <span className="hist-group-dot" aria-hidden />
                  <h2 className="hist-group-label">{group.label}</h2>
                </div>
                <div className="hist-grid">
                  {group.items.map((video) => {
                    const ratio = historyProgressRatio(video.progress_sec, video.duration_sec);
                    const completed = isHistoryCompleted(video.progress_sec, video.duration_sec);
                    const clock = formatHistoryProgressClock(video.progress_sec, video.duration_sec);
                    const progressLabel = formatProgress(video.progress_sec);
                    const checked = selected.has(video.id);
                    return (
                      <article
                        key={video.id}
                        className={`hist-card${managing && checked ? ' is-selected' : ''}`}
                        data-testid={`history-card-${video.id}`}
                      >
                        {managing ? (
                          <label className="hist-card-check">
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
                          className="hist-card-link"
                          title={video.title}
                          onClick={(e) => {
                            if (managing) {
                              e.preventDefault();
                              toggleSelect(video.id);
                            }
                          }}
                        >
                          <span className="hist-card-thumb">
                            <img
                              src={coverImgSrc(video.cover_url)}
                              alt=""
                              loading="lazy"
                              onError={onCoverImgError}
                            />
                            {clock ? <span className="hist-card-clock">{clock}</span> : null}
                            {completed ? (
                              <span className="hist-card-done">已看完</span>
                            ) : ratio > 0 ? (
                              <span className="hist-progress" aria-hidden>
                                <span style={{ width: `${Math.round(ratio * 100)}%` }} />
                              </span>
                            ) : null}
                            {!completed && video.duration_sec > 0 && !clock ? (
                              <span className="hist-duration">{formatDuration(video.duration_sec)}</span>
                            ) : null}
                          </span>
                          <span className="hist-card-title">{video.title}</span>
                          <span className="hist-card-sub">
                            {formatHistoryCardTime(video.last_watched_at)}
                            {progressLabel && !completed ? ` · ${progressLabel}` : ''}
                          </span>
                        </Link>
                        {!managing ? (
                          <button
                            type="button"
                            className="hist-card-remove"
                            aria-label={`删除历史：${video.title}`}
                            data-testid={`history-remove-${video.id}`}
                            onClick={(e) => {
                              e.preventDefault();
                              e.stopPropagation();
                              void onRemove(video);
                            }}
                          >
                            <span className="sr-only">删除</span>
                          </button>
                        ) : null}
                      </article>
                    );
                  })}
                </div>
              </section>
            ))}

            {hasMore && !filtersOn ? (
              <div className="load-more-wrap">
                <button
                  type="button"
                  className="me-btn"
                  disabled={loadingMore}
                  onClick={() => void loadMore()}
                  data-testid="history-load-more"
                >
                  {loadingMore ? '加载中…' : '加载更多'}
                </button>
              </div>
            ) : hasMore && filtersOn ? (
              <p className="hist-filter-note">
                筛选作用于已加载的历史；可先加载更多再筛选。
              </p>
            ) : null}
          </div>
        )}
      </div>
    </div>
  );
}
