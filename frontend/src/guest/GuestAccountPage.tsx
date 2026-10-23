import { useEffect, useState } from 'react';
import { Link, NavLink } from 'react-router-dom';
import { formatDuration, listVideosPage, type Video } from '../api';
import { formatHistoryTime } from '../components/headerFlyoutData';
import { HoverPreviewThumb } from '../components/HoverPreviewThumb';
import { coverImgSrc, onCoverImgError } from '../media/coverFallback';
import {
  ACCOUNT_FAVORITES_PREVIEW_LIMIT,
  ACCOUNT_HISTORY_PREVIEW_LIMIT,
  ACCOUNT_RECOMMEND_LIMIT,
  accountFavoritesPreview,
  accountHistoryPreview,
  accountRecommendMeta,
  accountRecommendVideos,
} from '../pages/accountPageLogic';
import { historyProgressRatio } from '../pages/historyPageLogic';
import { listGuestFavorites, listGuestHistory } from './guestLibrary';

export function GuestAccountPage() {
  const favorites = accountFavoritesPreview(listGuestFavorites(), ACCOUNT_FAVORITES_PREVIEW_LIMIT);
  const history = accountHistoryPreview(listGuestHistory(), ACCOUNT_HISTORY_PREVIEW_LIMIT);
  const [recommended, setRecommended] = useState<Video[]>([]);

  useEffect(() => {
    let cancelled = false;
    listVideosPage({ limit: ACCOUNT_RECOMMEND_LIMIT, offset: 0, excludeCategorySlug: 'midnight' })
      .then((page) => {
        if (!cancelled) setRecommended(accountRecommendVideos(page.data));
      })
      .catch(() => {
        if (!cancelled) setRecommended([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className="page space-page" data-testid="account-page">
      <header className="space-identity" aria-label="浏览空间">
        <div className="space-identity-meta">
          <h1 className="space-identity-name">浏览</h1>
          <p className="space-identity-role">收藏与历史只保存在这台浏览器</p>
        </div>
        <dl className="space-identity-stats" aria-label="本机数据">
          <div>
            <dd>{listGuestFavorites().length}</dd>
            <dt>收藏</dt>
          </div>
          <div>
            <dd>{listGuestHistory().length}</dd>
            <dt>历史</dt>
          </div>
        </dl>
      </header>
      <nav className="space-rail" aria-label="个人空间导航">
        <NavLink to="/account" end>
          主页
        </NavLink>
        <NavLink to="/favorites">我的收藏</NavLink>
        <NavLink to="/history">观看历史</NavLink>
        <NavLink to="/messages">消息</NavLink>
      </nav>
      <div className="space-body">
        <div className="space-main">
          <section className="space-section" aria-label="最近观看">
            <header className="space-section-head">
              <h2>最近观看</h2>
              <Link to="/history" className="space-more">
                查看更多
              </Link>
            </header>
            {history.length === 0 ? (
              <p className="space-empty space-empty--compact">暂无观看历史 · 打开任意视频即可记录</p>
            ) : (
              <div className="space-video-grid">
                {history.map((video) => {
                  const ratio = historyProgressRatio(video.progress_sec, video.duration_sec);
                  return (
                    <article key={video.id} className="space-video-card">
                      <Link to={`/watch/${video.id}`} title={video.title}>
                        <HoverPreviewThumb
                          videoId={video.id}
                          coverUrl={video.cover_url}
                          durationSec={video.duration_sec}
                          className="space-video-thumb"
                        >
                          {ratio > 0 ? (
                            <span className="space-video-progress" aria-hidden>
                              <span style={{ width: `${Math.round(ratio * 100)}%` }} />
                            </span>
                          ) : null}
                        </HoverPreviewThumb>
                        <span className="space-video-title">{video.title}</span>
                        <span className="space-video-sub">{formatHistoryTime(video.last_watched_at)}</span>
                      </Link>
                    </article>
                  );
                })}
              </div>
            )}
          </section>
          <section className="space-section" aria-label="收藏夹">
            <header className="space-section-head">
              <h2>
                收藏夹<em>{listGuestFavorites().length}</em>
              </h2>
              <Link to="/favorites" className="space-more">
                查看更多
              </Link>
            </header>
            {favorites.length === 0 ? (
              <p className="space-empty space-empty--compact">这里还没有收藏的视频</p>
            ) : (
              <div className="space-video-grid">
                {favorites.map((video) => (
                  <article key={video.id} className="space-video-card">
                    <Link to={`/watch/${video.id}`} title={video.title}>
                      <span className="space-video-thumb">
                        <img src={coverImgSrc(video.cover_url)} alt="" onError={onCoverImgError} />
                        {video.duration_sec > 0 ? (
                          <span className="space-video-duration">{formatDuration(video.duration_sec)}</span>
                        ) : null}
                      </span>
                      <span className="space-video-title">{video.title}</span>
                    </Link>
                  </article>
                ))}
              </div>
            )}
          </section>
          <section className="space-section" aria-label="猜你喜欢" data-testid="guest-account-recommend">
            <header className="space-section-head">
              <h2>猜你喜欢</h2>
              <p className="space-section-note">站内片库推荐 · 非个人投稿</p>
            </header>
            {recommended.length === 0 ? (
              <p className="space-empty space-empty--compact">暂时没有可推荐的内容</p>
            ) : (
              <div className="space-video-grid">
                {recommended.map((video) => {
                  const meta = accountRecommendMeta(video);
                  return (
                    <article key={video.id} className="space-video-card">
                      <Link to={`/watch/${video.id}`} title={video.title}>
                        <HoverPreviewThumb
                          videoId={video.id}
                          coverUrl={video.cover_url}
                          durationSec={video.duration_sec}
                          className="space-video-thumb"
                        />
                        <span className="space-video-title">{video.title}</span>
                        {meta ? <span className="space-video-sub">{meta}</span> : null}
                      </Link>
                    </article>
                  );
                })}
              </div>
            )}
          </section>
        </div>
      </div>
    </div>
  );
}
