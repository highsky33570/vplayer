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
  const { user, ready, openLogin } = useAuth();
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
        const page = await listVideosPage({ categoryId: vip.id, limit: 48, offset: 0 });
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
  }, []);

  const vipCat = resolveVipCategory(categories);
  const hero = pickVipHero(videos);
  const featured = vipFeaturedStrip(videos, 8);
  const catalog = vipCatalogGrid(videos, featured.length > 0 ? 1 : 0, 24);
  const status = vipAccessStatus(Boolean(user));

  return (
    <div className="page vip-page">
      <header className="vip-page-head">
        <div>
          <p className="vip-eyebrow">VPlayer · {VIP_CHANNEL_NAME}</p>
          <h1 className="vip-page-title">大会员</h1>
          <p className="vip-page-lead">
            高质量片库落地页。站点没有付费订阅；本页展示现有 {VIP_CHANNEL_NAME} 频道内容。
          </p>
        </div>
        <Link className="me-btn" to={`/?ch=${VIP_CHANNEL_SLUG}`}>
          频道筛选浏览
        </Link>
      </header>

      {error ? (
        <p className="me-feedback is-error" role="alert">
          {error}
        </p>
      ) : null}

      <section className="vip-hero-wrap" aria-label="精选推荐">
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
              <h2 className="vip-hero-title">{hero.title}</h2>
              <p className="vip-hero-meta">
                {[hero.year, hero.area, hero.duration_sec > 0 ? `${Math.round(hero.duration_sec / 60)} 分钟` : '']
                  .filter(Boolean)
                  .join(' · ')}
              </p>
              {hero.description ? <p className="vip-hero-desc">{hero.description}</p> : null}
              <div className="vip-hero-actions">
                <Link className="me-btn me-btn--primary" to={`/watch/${hero.id}`}>
                  立即观看
                </Link>
                <Link className="me-btn" to={`/watch/${hero.id}`}>
                  查看详情
                </Link>
              </div>
            </div>
          </div>
        ) : (
          <div className="vip-hero vip-hero--empty">
            <p>暂无 VIP蓝光影院 内容</p>
            <Link className="me-btn me-btn--primary" to="/">
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
                <p>登录后可完整观看</p>
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
          {ready && !user ? (
            <button type="button" className="me-btn me-btn--primary" onClick={() => openLogin('login')}>
              登录观看
            </button>
          ) : (
            <Link className="me-btn me-btn--primary" to={hero ? `/watch/${hero.id}` : `/?ch=${VIP_CHANNEL_SLUG}`}>
              开始浏览
            </Link>
          )}
        </aside>
      </section>

      {featured.length > 0 ? (
        <section className="vip-featured" aria-label="精选横滑">
          <h2>精选推荐</h2>
          <div className="vip-featured-row">
            {featured.map((video) => (
              <Link key={video.id} to={`/watch/${video.id}`} className="vip-featured-card" title={video.title}>
                <img src={coverImgSrc(video.cover_url)} alt="" loading="lazy" onError={onCoverImgError} />
                <span>{video.title}</span>
              </Link>
            ))}
          </div>
        </section>
      ) : null}

      <section className="vip-features" aria-label="功能说明">
        {VIP_FEATURE_CARDS.map((card) => (
          <article key={card.id} className="vip-feature-card">
            <h3>{card.title}</h3>
            <p>{card.body}</p>
          </article>
        ))}
      </section>

      <section className="vip-catalog" aria-label={`${VIP_CHANNEL_NAME}片库`}>
        <div className="me-title-row">
          <h2>片库推荐</h2>
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
  );
}
