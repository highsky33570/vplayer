import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { listCategories, listVideosPage, type Category, type Video } from '../api';
import { useAuth } from '../auth/AuthContext';
import { UserAvatar } from '../account/UserAvatar';
import { VideoCard } from '../components/VideoCard';
import { coverImgSrc, onCoverImgError } from '../media/coverFallback';
import {
  VIP_CHANNEL_NAME,
  VIP_CHANNEL_SLUG,
  VIP_FEATURE_CARDS,
  pickVipHero,
  resolveVipCategory,
  vipAccessStatus,
  vipCatalogGrid,
  vipFeaturedStrip,
} from './vipPageLogic';

export function VipPage() {
  const { user, token, ready, openLogin, authRequired, playbackAuthRequired } = useAuth();
  const playbackNeedsLogin = authRequired && playbackAuthRequired;
  const [categories, setCategories] = useState<Category[]>([]);
  const [videos, setVideos] = useState<Video[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError('');
    listCategories()
      .then(async (cats) => {
        if (cancelled) return;
        setCategories(cats);
        const vip = resolveVipCategory(cats);
        if (!vip) {
          setVideos([]);
          setTotal(0);
          return;
        }
        const page = await listVideosPage({ categoryId: vip.id, limit: 48, offset: 0, token });
        if (cancelled) return;
        setVideos(page.data);
        setTotal(page.total);
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : '加载失败');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  const vipCat = resolveVipCategory(categories);
  const hero = pickVipHero(videos);
  const featured = vipFeaturedStrip(videos, 8);
  const catalog = vipCatalogGrid(videos, featured.length > 0 ? 1 : 0, 24);
  const baseStatus = vipAccessStatus(Boolean(user), authRequired);
  const status =
    !user && !playbackNeedsLogin
      ? {
          label: '可浏览 VIP蓝光影院',
          detail:
            '天涯欧乐 没有付费会员等级。视频可直接播放。片源本身不可用时仍不可播放，并不表示会员限制已解除。收藏、历史和账号仍需要登录。',
        }
      : baseStatus;
  const featureCards = VIP_FEATURE_CARDS.map((card) => {
    if (card.id !== 'devices') return card;
    if (!authRequired) {
      return { ...card, body: '可在当前浏览器浏览与播放。片源本身不可播时仍会提示暂不可用。' };
    }
    if (!playbackNeedsLogin) {
      return { ...card, body: '视频可直接播放。登录后可在不同设备上同步收藏与观看记录。' };
    }
    return card;
  });

  return (
    <div className="vip-dest">
      <section className="vip-hero-stage" aria-label="大会员精选">
        {loading ? (
          <div className="vip-hero vip-hero--empty">加载中…</div>
        ) : hero ? (
          <div className="vip-hero">
            <img
              className="vip-hero-bg"
              src={coverImgSrc(hero.cover_url)}
              alt=""
              onError={onCoverImgError}
            />
            <div className="vip-hero-shade" aria-hidden />
            <div className="vip-hero-body">
              <p className="vip-hero-kicker">{VIP_CHANNEL_NAME}</p>
              <h1 className="vip-hero-title">{hero.title}</h1>
              <p className="vip-hero-meta">
                {[hero.year, hero.area, hero.duration_sec > 0 ? `${Math.round(hero.duration_sec / 60)} 分钟` : '']
                  .filter(Boolean)
                  .join(' · ')}
              </p>
              {hero.description ? <p className="vip-hero-desc">{hero.description}</p> : null}
              <div className="vip-hero-actions">
                <Link className="vip-cta" to={`/watch/${hero.id}`}>
                  立即观看
                </Link>
                <Link className="vip-cta vip-cta--ghost" to={`/?ch=${VIP_CHANNEL_SLUG}`}>
                  浏览频道
                </Link>
              </div>
            </div>
          </div>
        ) : (
          <div className="vip-hero vip-hero--empty">
            <p>暂无 VIP蓝光影院 内容</p>
            <Link className="vip-cta" to="/">
              返回首页
            </Link>
          </div>
        )}

        <aside className="vip-status-card" aria-label="访问状态">
          {user ? (
            <div className="vip-status-user">
              <UserAvatar user={user} className="me-avatar" />
              <div>
                <strong>{(user.nickname ?? '').trim() || '已登录'}</strong>
                <p>{user.email}</p>
              </div>
            </div>
          ) : (
            <div className="vip-status-user">
              <span className="banner-avatar vip-guest-avatar">客</span>
              <div>
                <strong>访客</strong>
                <p>
                  {playbackNeedsLogin ? '登录后可完整观看' : '可直接观看本频道'}
                </p>
              </div>
            </div>
          )}
          <p className="vip-status-label">{status.label}</p>
          <p className="vip-status-detail">{status.detail}</p>
          <dl className="vip-status-stats">
            <div>
              <dt>片库</dt>
              <dd>{VIP_CHANNEL_NAME}</dd>
            </div>
            <div>
              <dt>可浏览</dt>
              <dd>{loading ? '…' : `${total} 部`}</dd>
            </div>
          </dl>
          {playbackNeedsLogin && ready && !user ? (
            <button type="button" className="vip-cta" onClick={() => openLogin('login')}>
              登录观看
            </button>
          ) : (
            <Link
              className="vip-cta"
              to={hero ? `/watch/${hero.id}` : `/?ch=${VIP_CHANNEL_SLUG}`}
            >
              开始浏览
            </Link>
          )}
        </aside>

        {featured.length > 0 ? (
          <div className="vip-featured-overlap" aria-label="精选横滑">
            <div className="vip-featured-row">
              {featured.map((video) => (
                <Link
                  key={video.id}
                  to={`/watch/${video.id}`}
                  className="vip-featured-card"
                  title={video.title}
                >
                  <img
                    src={coverImgSrc(video.cover_url)}
                    alt=""
                    loading="lazy"
                    onError={onCoverImgError}
                  />
                  <span>{video.title}</span>
                </Link>
              ))}
            </div>
          </div>
        ) : null}
      </section>

      <div className="vip-dest-body">
        {error ? (
          <p className="me-feedback is-error" role="alert">
            {error}
          </p>
        ) : null}

        <section className="vip-features" aria-label="功能说明">
          {featureCards.map((card) => (
            <article key={card.id} className={`vip-feature-card vip-feature-card--${card.id}`}>
              <h2>{card.title}</h2>
              <p>{card.body}</p>
            </article>
          ))}
        </section>

        <section className="vip-catalog" aria-label={`${VIP_CHANNEL_NAME}片库`}>
          <div className="me-title-row">
            <h2>猜你喜欢</h2>
            {vipCat ? <p className="me-count">共 {total} 部</p> : null}
          </div>
          {loading ? (
            <p className="me-status">加载中…</p>
          ) : catalog.length === 0 ? (
            <div className="me-empty">
              <p>该频道暂时没有更多内容</p>
            </div>
          ) : (
            <div className="video-grid">
              {catalog.map((video) => (
                <VideoCard key={video.id} video={video} />
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
