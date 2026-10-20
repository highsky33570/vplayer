import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  fetchMeSummary,
  listFavoriteFolders,
  listFavoritesPage,
  removeFavorite,
  type FavoriteVideo,
  type MeSummary,
} from '../api';
import { appendUnique, removeById, roleLabel } from '../account/accountLogic';
import { UserAvatar } from '../account/UserAvatar';
import { useAuth } from '../auth/AuthContext';
import { AuthGate } from '../auth/AuthGate';
import { VideoCard } from '../components/VideoCard';
import { normalizeFolders, type FolderItem } from '../components/headerFlyoutData';

const PAGE_SIZE = 24;

export function FavoritesPage() {
  return <AuthGate>{({ token }) => <FavoritesContent token={token} />}</AuthGate>;
}

function FavoritesContent({ token }: { token: string }) {
  const { user } = useAuth();
  const [summary, setSummary] = useState<MeSummary | null>(null);
  const [folders, setFolders] = useState<FolderItem[]>(() => normalizeFolders([]));
  const [folderId, setFolderId] = useState(0);
  const [items, setItems] = useState<FavoriteVideo[]>([]);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState('');
  const [query, setQuery] = useState('');
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
    fetchMeSummary(token)
      .then(setSummary)
      .catch(() => setSummary(null));
  }, [token, loadFolders]);

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

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return items;
    return items.filter((item) => item.title.toLowerCase().includes(q));
  }, [items, query]);

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
      fetchMeSummary(token)
        .then(setSummary)
        .catch(() => undefined);
    } catch (e) {
      setError(e instanceof Error ? e.message : '取消收藏失败');
    }
  }

  if (!user) return null;
  const activeFolder = folders.find((f) => f.id === folderId);
  const activeName = folderId === 0 ? '全部收藏' : (activeFolder?.name ?? '默认收藏夹');
  const favCount = summary?.favorites ?? folders.reduce((n, f) => n + f.count, 0);

  return (
    <div className="page me-page lib-page lib-dest">
      <header className="lib-banner" aria-label="我的收藏">
        <UserAvatar user={user} className="me-avatar lib-banner-avatar" />
        <div className="lib-banner-main">
          <h1 className="me-title">{(user.nickname ?? '').trim() || '已登录'}</h1>
          <p className="me-profile-meta">
            {roleLabel(user.is_admin)} · 收藏 {favCount}
          </p>
        </div>
        <nav className="lib-rail" aria-label="个人内容">
          <Link to="/favorites" className="is-active" aria-current="page">
            收藏
          </Link>
          <Link to="/history">历史</Link>
          <Link to="/account">个人中心</Link>
        </nav>
      </header>

      <div className="me-layout lib-layout">
        <nav className="me-folders" aria-label="我的收藏夹">
          <p className="me-folders-title">我创建的收藏夹</p>
          <button
            type="button"
            className={folderId === 0 ? 'is-active' : undefined}
            onClick={() => setFolderId(0)}
          >
            <span>全部收藏</span>
            <em>{favCount}</em>
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

        <section className="me-main lib-main" aria-label={activeName}>
          <div className="lib-folder-head">
            <div>
              <h2 className="lib-section-title">{activeName}</h2>
              <p className="me-count">{total} 个视频</p>
            </div>
            <label className="lib-search">
              <span className="sr-only">搜索当前已加载收藏</span>
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="搜索收藏夹内视频"
                autoComplete="off"
              />
            </label>
          </div>

          {error ? (
            <p className="me-feedback is-error" role="alert">
              {error}
            </p>
          ) : null}

          {loading ? (
            <p className="me-status">加载中…</p>
          ) : visible.length === 0 ? (
            <div className="me-empty">
              <p>{items.length === 0 ? '这里还没有收藏的视频' : '没有匹配的收藏标题'}</p>
              <Link to="/" className="me-btn me-btn--primary">
                去首页逛逛
              </Link>
            </div>
          ) : (
            <>
              <div className="video-grid me-grid">
                {visible.map((video) => (
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
        </section>
      </div>
    </div>
  );
}
