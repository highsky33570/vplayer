import { useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  clearHistory,
  deleteHistoryItem,
  formatDuration,
  listHistoryPage,
  type HistoryVideo,
} from '../api';
import { appendUnique, removeById } from '../account/accountLogic';
import { AuthGate } from '../auth/AuthGate';
import { coverImgSrc, onCoverImgError } from '../media/coverFallback';
import { formatHistoryTime, formatProgress } from '../components/headerFlyoutData';
import {
  filterHistoryByQuery,
  groupHistoryItems,
  historyProgressRatio,
} from './historyPageLogic';

const PAGE_SIZE = 36;

export function HistoryPage() {
  return <AuthGate>{({ token }) => <HistoryContent token={token} />}</AuthGate>;
}

function HistoryContent({ token }: { token: string }) {
  const [items, setItems] = useState<HistoryVideo[]>([]);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState('');
  const [query, setQuery] = useState('');
  const requestRef = useRef(0);

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

  const filtered = useMemo(() => filterHistoryByQuery(items, query), [items, query]);
  const groups = useMemo(() => groupHistoryItems(filtered), [filtered]);

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
    } catch (e) {
      setError(e instanceof Error ? e.message : '删除失败');
    }
  }

  async function onClear() {
    if (!window.confirm('确认清空全部观看历史？此操作无法撤销。')) return;
    try {
      await clearHistory(token);
      setItems([]);
      setTotal(0);
      setHasMore(false);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : '清空失败');
    }
  }

  return (
    <div className="page me-page history-page">
      <div className="me-title-row">
        <div>
          <h1 className="me-title">历史记录</h1>
          <p className="me-count">视频 · 共 {total} 条</p>
        </div>
        <div className="history-toolbar">
          <label className="lib-search">
            <span className="sr-only">搜索历史</span>
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="搜索已加载的历史"
              autoComplete="off"
            />
          </label>
          {items.length > 0 ? (
            <button type="button" className="me-btn me-btn--danger" onClick={() => void onClear()}>
              清空历史
            </button>
          ) : null}
        </div>
      </div>

      {error ? (
        <p className="me-feedback is-error" role="alert">
          {error}
        </p>
      ) : null}

      {loading ? (
        <p className="me-status">加载中…</p>
      ) : filtered.length === 0 ? (
        <div className="me-empty">
          <p>{items.length === 0 ? '暂无观看历史' : '没有匹配的历史标题'}</p>
          <Link to="/" className="me-btn me-btn--primary">
            去首页逛逛
          </Link>
        </div>
      ) : (
        <>
          {groups.map((group) => (
            <section key={group.id} className="history-group" aria-label={group.label}>
              <h2 className="history-group-title">{group.label}</h2>
              <div className="history-grid">
                {group.items.map((video) => {
                  const progress = formatProgress(video.progress_sec);
                  const ratio = historyProgressRatio(video.progress_sec, video.duration_sec);
                  return (
                    <article key={video.id} className="history-card">
                      <Link to={`/watch/${video.id}`} className="history-card-link" title={video.title}>
                        <span className="history-card-thumb">
                          <img
                            src={coverImgSrc(video.cover_url)}
                            alt=""
                            loading="lazy"
                            onError={onCoverImgError}
                          />
                          {ratio > 0 ? (
                            <span className="history-progress" aria-hidden>
                              <span style={{ width: `${Math.round(ratio * 100)}%` }} />
                            </span>
                          ) : null}
                          {video.duration_sec > 0 ? (
                            <span className="history-duration">{formatDuration(video.duration_sec)}</span>
                          ) : null}
                        </span>
                        <span className="history-card-title">{video.title}</span>
                        <span className="history-card-sub">
                          {formatHistoryTime(video.last_watched_at)}
                          {progress ? ` · ${progress}` : ''}
                        </span>
                      </Link>
                      <button
                        type="button"
                        className="history-card-remove"
                        aria-label={`删除历史：${video.title}`}
                        onClick={() => void onRemove(video)}
                      >
                        删除
                      </button>
                    </article>
                  );
                })}
              </div>
            </section>
          ))}
          {hasMore && !query.trim() ? (
            <div className="load-more-wrap">
              <button
                type="button"
                className="me-btn"
                disabled={loadingMore}
                onClick={() => void loadMore()}
              >
                {loadingMore ? '加载中…' : '加载更多'}
              </button>
            </div>
          ) : null}
        </>
      )}
    </div>
  );
}
