import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { formatDuration, listCategories, type Category, type HistoryVideo } from '../api';
import { HoverPreviewThumb } from '../components/HoverPreviewThumb';
import { formatProgress } from '../components/headerFlyoutData';
import { clearGuestHistory, listGuestHistory, removeGuestHistory } from './guestLibrary';
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
} from '../pages/historyPageLogic';

const INITIAL_FILTERS: HistoryFilterState = {
  query: '',
  mainSlug: 'all',
  time: 'all',
  status: 'all',
};

export function GuestHistoryPage() {
  const [items, setItems] = useState<HistoryVideo[]>(() => listGuestHistory());
  const [categories, setCategories] = useState<Category[]>([]);
  const [filters, setFilters] = useState<HistoryFilterState>(INITIAL_FILTERS);
  const [moreOpen, setMoreOpen] = useState(false);

  useEffect(() => {
    listCategories()
      .then(setCategories)
      .catch(() => setCategories([]));
  }, []);

  useEffect(() => {
    const refresh = () => setItems(listGuestHistory());
    window.addEventListener('vplayer-guest-library', refresh);
    return () => window.removeEventListener('vplayer-guest-library', refresh);
  }, []);

  const mainByCategoryId = useMemo(() => historyMainSlugByCategoryId(categories), [categories]);
  const filtered = useMemo(
    () => applyHistoryFilters(items, filters, mainByCategoryId),
    [items, filters, mainByCategoryId],
  );
  const groups = useMemo(() => groupHistoryItems(filtered), [filtered]);
  const tabs = useMemo(() => historyPrimaryTabs(items, categories), [items, categories]);
  const filtersOn = historyFiltersActive(filters);

  function patchFilters(patch: Partial<HistoryFilterState>) {
    setFilters((current) => ({ ...current, ...patch }));
  }

  return (
    <div className="hist-dest" data-testid="history-page">
      <div className="hist-workspace" data-testid="guest-history">
        <header className="hist-head">
          <h1 className="hist-title">历史记录</h1>
          <p className="hist-count">共 {filtered.length} 条 · 只保存在这台浏览器</p>
        </header>
        <div className="hist-toolbar">
          <nav className="hist-tabs" aria-label="历史分类">
            {tabs.map((tab) => (
              <button
                key={tab.id}
                type="button"
                className={filters.mainSlug === tab.id ? 'is-active' : undefined}
                onClick={() => patchFilters({ mainSlug: tab.id })}
              >
                {tab.label}
              </button>
            ))}
          </nav>
          <div className="hist-tools">
            <div className="hist-more">
              <button type="button" className="hist-tool-btn" onClick={() => setMoreOpen((v) => !v)}>
                更多筛选
              </button>
              {moreOpen ? (
                <div className="hist-more-panel" role="dialog" aria-label="更多筛选">
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
                </div>
              ) : null}
            </div>
            <label className="hist-search">
              <span className="sr-only">搜索历史记录</span>
              <input
                value={filters.query}
                onChange={(e) => patchFilters({ query: e.target.value })}
                placeholder="搜索标题"
                data-testid="guest-history-search"
              />
            </label>
            {items.length > 0 ? (
              <button
                type="button"
                className="hist-tool-btn hist-tool-btn--danger"
                onClick={() => {
                  if (!window.confirm('确认清空全部观看历史？')) return;
                  clearGuestHistory();
                  setItems([]);
                }}
              >
                清空历史
              </button>
            ) : null}
          </div>
        </div>
        {filtered.length === 0 ? (
          <div className="hist-empty">
            <p>{items.length === 0 ? '暂无观看历史' : '没有匹配的历史记录'}</p>
            {items.length === 0 ? (
              <Link to="/" className="me-btn me-btn--primary">
                去首页逛逛
              </Link>
            ) : filtersOn ? (
              <button type="button" className="me-btn" onClick={() => setFilters(INITIAL_FILTERS)}>
                清除筛选
              </button>
            ) : null}
          </div>
        ) : (
          <div className="hist-timeline">
            {groups.map((group) => (
              <section key={group.id} className="hist-group" aria-label={group.label}>
                <div className="hist-group-rail">
                  <h2 className="hist-group-label">{group.label}</h2>
                </div>
                <div className="hist-grid">
                  {group.items.map((video) => {
                    const ratio = historyProgressRatio(video.progress_sec, video.duration_sec);
                    const completed = isHistoryCompleted(video.progress_sec, video.duration_sec);
                    const clock = formatHistoryProgressClock(video.progress_sec, video.duration_sec);
                    return (
                      <article key={video.id} className="hist-card" data-testid={`guest-history-${video.id}`}>
                        <Link to={`/watch/${video.id}`} className="hist-card-link" title={video.title}>
                          <HoverPreviewThumb
                            videoId={video.id}
                            coverUrl={video.cover_url}
                            durationSec={video.duration_sec}
                            className="hist-card-thumb"
                          >
                            {clock ? <span className="hist-card-clock">{clock}</span> : null}
                            {completed ? <span className="hist-card-done">已看完</span> : null}
                            {!completed && ratio > 0 ? (
                              <span className="hist-progress" aria-hidden>
                                <span style={{ width: `${Math.round(ratio * 100)}%` }} />
                              </span>
                            ) : null}
                            {!completed && video.duration_sec > 0 && !clock ? (
                              <span className="hist-duration">{formatDuration(video.duration_sec)}</span>
                            ) : null}
                          </HoverPreviewThumb>
                          <span className="hist-card-title">{video.title}</span>
                          <span className="hist-card-sub">
                            {formatHistoryCardTime(video.last_watched_at)}
                            {formatProgress(video.progress_sec) && !completed
                              ? ` · ${formatProgress(video.progress_sec)}`
                              : ''}
                          </span>
                        </Link>
                        <button
                          type="button"
                          className="hist-card-remove"
                          aria-label={`删除历史：${video.title}`}
                          onClick={() => {
                            removeGuestHistory(video.id);
                            setItems(listGuestHistory());
                          }}
                        >
                          <span className="sr-only">删除</span>
                        </button>
                      </article>
                    );
                  })}
                </div>
              </section>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
