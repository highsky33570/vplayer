import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  clearHistory,
  deleteHistoryItem,
  listHistoryPage,
  type HistoryVideo,
} from '../api';
import { appendUnique, removeById } from '../account/accountLogic';
import { AuthGate } from '../auth/AuthGate';
import { coverImgSrc, onCoverImgError } from '../media/coverFallback';
import { formatHistoryTime, formatProgress } from '../components/headerFlyoutData';

const PAGE_SIZE = 24;

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
    <div className="page me-page">
      <div className="me-title-row">
        <h1 className="me-title">观看历史</h1>
        {items.length > 0 ? (
          <button type="button" className="me-btn me-btn--danger" onClick={() => void onClear()}>
            清空历史
          </button>
        ) : null}
      </div>

      {error ? (
        <p className="me-feedback is-error" role="alert">
          {error}
        </p>
      ) : null}

      {loading ? (
        <p className="me-status">加载中…</p>
      ) : items.length === 0 ? (
        <div className="me-empty">
          <p>暂无观看历史</p>
          <Link to="/" className="me-btn me-btn--primary">
            去首页逛逛
          </Link>
        </div>
      ) : (
        <>
          <p className="me-count">共 {total} 条记录</p>
          <ul className="history-list">
            {items.map((video) => {
              const progress = formatProgress(video.progress_sec);
              return (
                <li key={video.id} className="history-item">
                  <Link to={`/watch/${video.id}`} className="history-link" title={video.title}>
                    <img
                      className="history-cover"
                      src={coverImgSrc(video.cover_url)}
                      alt=""
                      loading="lazy"
                      onError={onCoverImgError}
                    />
                    <span className="history-text">
                      <span className="history-title">{video.title}</span>
                      <span className="history-sub">
                        {formatHistoryTime(video.last_watched_at)}
                        {progress ? ` · ${progress}` : ''}
                      </span>
                    </span>
                  </Link>
                  <button
                    type="button"
                    className="me-btn me-btn--small"
                    aria-label={`删除历史：${video.title}`}
                    onClick={() => void onRemove(video)}
                  >
                    删除
                  </button>
                </li>
              );
            })}
          </ul>
          {hasMore ? (
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
