import { useCallback, useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  listFavoriteFolders,
  listFavoritesPage,
  removeFavorite,
  type FavoriteVideo,
} from '../api';
import { appendUnique, removeById } from '../account/accountLogic';
import { AuthGate } from '../auth/AuthGate';
import { VideoCard } from '../components/VideoCard';
import { normalizeFolders, type FolderItem } from '../components/headerFlyoutData';

const PAGE_SIZE = 24;

export function FavoritesPage() {
  return <AuthGate>{({ token }) => <FavoritesContent token={token} />}</AuthGate>;
}

function FavoritesContent({ token }: { token: string }) {
  const [folders, setFolders] = useState<FolderItem[]>(() => normalizeFolders([]));
  const [folderId, setFolderId] = useState(0);
  const [items, setItems] = useState<FavoriteVideo[]>([]);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState('');
  const requestRef = useRef(0);

  const loadFolders = useCallback(() => {
    listFavoriteFolders(token)
      .then((rows) => setFolders(normalizeFolders(rows)))
      .catch(() => {
        /* keep placeholder folder */
      });
  }, [token]);

  useEffect(() => {
    loadFolders();
  }, [loadFolders]);

  useEffect(() => {
    const request = ++requestRef.current;
    setLoading(true);
    setError('');
    listFavoritesPage(token, { folderId: folderId || undefined, limit: PAGE_SIZE, offset: 0 })
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
  }, [token, folderId]);

  async function loadMore() {
    const request = requestRef.current;
    setLoadingMore(true);
    try {
      const page = await listFavoritesPage(token, {
        folderId: folderId || undefined,
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
      loadFolders();
    } catch (e) {
      setError(e instanceof Error ? e.message : '取消收藏失败');
    }
  }

  const activeName = folders.find((f) => f.id === folderId)?.name ?? '全部收藏';

  return (
    <div className="page me-page">
      <h1 className="me-title">我的收藏</h1>
      <div className="me-layout">
        <nav className="me-folders" aria-label="收藏夹">
          <button
            type="button"
            className={folderId === 0 ? 'is-active' : undefined}
            onClick={() => setFolderId(0)}
          >
            <span>全部收藏</span>
          </button>
          {folders
            .filter((folder) => folder.id > 0)
            .map((folder) => (
              <button
                key={folder.id}
                type="button"
                className={folderId === folder.id ? 'is-active' : undefined}
                onClick={() => setFolderId(folder.id)}
              >
                <span>{folder.name}</span>
                <em>{folder.count}</em>
              </button>
            ))}
        </nav>

        <section className="me-main" aria-label={activeName}>
          {error ? (
            <p className="me-feedback is-error" role="alert">
              {error}
            </p>
          ) : null}
          {loading ? (
            <p className="me-status">加载中…</p>
          ) : items.length === 0 ? (
            <div className="me-empty">
              <p>{error ? '收藏暂时无法加载' : '这里还没有收藏的视频'}</p>
              <Link to="/" className="me-btn me-btn--primary">
                去首页逛逛
              </Link>
            </div>
          ) : (
            <>
              <p className="me-count">
                {activeName} · 共 {total} 个
              </p>
              <div className="video-grid me-grid">
                {items.map((video) => (
                  <div key={video.id} className="me-grid-item">
                    <VideoCard video={video} />
                    <button
                      type="button"
                      className="me-remove"
                      aria-label={`取消收藏：${video.title}`}
                      title="取消收藏"
                      onClick={() => void onRemove(video)}
                    >
                      取消收藏
                    </button>
                  </div>
                ))}
              </div>
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
        </section>
      </div>
    </div>
  );
}
