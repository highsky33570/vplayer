import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { formatDuration, listCategories, type Category, type FavoriteVideo } from '../api';
import { formatHistoryTime } from '../components/headerFlyoutData';
import { HoverPreviewThumb } from '../components/HoverPreviewThumb';
import { coverImgSrc, onCoverImgError } from '../media/coverFallback';
import {
  FAVORITES_PERSONAL_NAV,
  FAVORITES_SORT_OPTIONS,
  applyFavoritesView,
  favoritesCategoryOptions,
  favoritesMainSlugByCategoryId,
  folderPreviewCover,
  type FavoritesSort,
} from '../pages/favoritesPageLogic';
import { listGuestFavorites, listGuestHistory, removeGuestFavorite } from './guestLibrary';

export function GuestFavoritesPage() {
  const [items, setItems] = useState<FavoriteVideo[]>(() => listGuestFavorites());
  const [categories, setCategories] = useState<Category[]>([]);
  const [query, setQuery] = useState('');
  const [sort, setSort] = useState<FavoritesSort>('recent');
  const [categorySlug, setCategorySlug] = useState('all');
  const historyCount = listGuestHistory().length;

  useEffect(() => {
    listCategories()
      .then(setCategories)
      .catch(() => setCategories([]));
  }, []);

  useEffect(() => {
    const refresh = () => setItems(listGuestFavorites());
    window.addEventListener('vplayer-guest-library', refresh);
    return () => window.removeEventListener('vplayer-guest-library', refresh);
  }, []);

  const mainByCategoryId = useMemo(() => favoritesMainSlugByCategoryId(categories), [categories]);
  const visible = useMemo(
    () => applyFavoritesView(items, { query, categorySlug, sort }, mainByCategoryId),
    [items, query, categorySlug, sort, mainByCategoryId],
  );
  const categoryOptions = useMemo(() => favoritesCategoryOptions(items, categories), [items, categories]);
  const previewCover = folderPreviewCover(visible.length > 0 ? visible : items);

  function onRemove(video: FavoriteVideo) {
    removeGuestFavorite(video.id);
    setItems(listGuestFavorites());
  }

  return (
    <div className="fav-dest" data-testid="favorites-page">
      <div className="fav-workspace" data-testid="guest-favorites">
        <nav className="fav-personal-nav" aria-label="个人空间导航">
          <div className="fav-personal-links">
            {FAVORITES_PERSONAL_NAV.map((item) => (
              <Link
                key={item.id}
                to={item.href}
                className={item.id === 'favorites' ? 'is-active' : undefined}
              >
                {item.label}
              </Link>
            ))}
          </div>
          <dl className="fav-personal-stats" aria-label="本机数据">
            <div>
              <dd>{items.length}</dd>
              <dt>收藏</dt>
            </div>
            <div>
              <dd>{historyCount}</dd>
              <dt>历史</dt>
            </div>
          </dl>
        </nav>
        <div className="fav-layout">
          <aside className="fav-sidebar" aria-label="我的收藏夹">
            <p className="fav-sidebar-title">本机收藏</p>
            <button type="button" className="is-active">
              <span className="fav-folder-name">全部收藏</span>
              <em>{items.length}</em>
            </button>
          </aside>
          <section className="fav-main" aria-label="全部收藏">
            <header className="fav-folder-head">
              <div className="fav-folder-preview" aria-hidden>
                {previewCover ? (
                  <img src={coverImgSrc(previewCover)} alt="" onError={onCoverImgError} />
                ) : (
                  <span className="fav-folder-placeholder" />
                )}
              </div>
              <div className="fav-folder-meta">
                <h1 className="fav-folder-title">全部收藏</h1>
                <p className="fav-folder-count">视频数 {visible.length}</p>
                <p className="fav-empty-hint">只保存在这台浏览器，不会写入账号。</p>
              </div>
            </header>
            <div className="fav-toolbar">
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
                    <select value={categorySlug} onChange={(e) => setCategorySlug(e.target.value)}>
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
                    data-testid="guest-favorites-search"
                  />
                </label>
              </div>
            </div>
            {visible.length === 0 ? (
              <div className="fav-empty">
                <p>{items.length === 0 ? '这里还没有收藏的视频' : '没有匹配的收藏标题'}</p>
                {items.length === 0 ? (
                  <Link to="/" className="me-btn me-btn--primary">
                    去首页逛逛
                  </Link>
                ) : null}
              </div>
            ) : (
              <div className="fav-grid">
                {visible.map((video) => (
                  <article key={video.id} className="fav-card" data-testid={`guest-favorite-${video.id}`}>
                    <Link to={`/watch/${video.id}`} className="fav-card-link" title={video.title}>
                      <HoverPreviewThumb
                        videoId={video.id}
                        coverUrl={video.cover_url}
                        durationSec={video.duration_sec}
                        className="fav-card-thumb"
                      >
                        {video.duration_sec > 0 ? (
                          <span className="fav-card-duration">{formatDuration(video.duration_sec)}</span>
                        ) : null}
                      </HoverPreviewThumb>
                      <span className="fav-card-title">{video.title}</span>
                      <span className="fav-card-sub">{formatHistoryTime(video.favorited_at)}</span>
                    </Link>
                    <button
                      type="button"
                      className="fav-card-remove"
                      aria-label={`取消收藏：${video.title}`}
                      onClick={() => onRemove(video)}
                    >
                      <span className="sr-only">取消收藏</span>
                    </button>
                  </article>
                ))}
              </div>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}
