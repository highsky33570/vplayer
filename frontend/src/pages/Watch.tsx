import { useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import Player from 'xgplayer';
import HlsPlugin from 'xgplayer-hls';
import 'xgplayer/dist/index.min.css';
import {
  ApiError,
  addFavorite,
  formatDuration,
  getFavoriteStatus,
  removeFavorite,
  upsertHistory,
  formatViews,
  getPlayback,
  getVideo,
  listCategories,
  listEpisodes,
  listRelated,
  type Category,
  type PublicEpisode,
  type RelatedVideo,
  type Video,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import {
  addGuestFavorite,
  isGuestFavorite,
  removeGuestFavorite,
  upsertGuestHistory,
} from '../guest/guestLibrary';
import { CommentSection } from '../components/CommentSection';
import { EpisodeStrip } from '../components/EpisodeStrip';
import { RecommendationList } from '../components/RecommendationList';
import {
  PLAYBACK_GENERIC_ERROR,
  PLAYBACK_UNAVAILABLE_MESSAGE,
  WatchPlaybackSession,
} from './watchPlaybackLogic';

function attachPlayer(
  el: HTMLDivElement,
  url: string,
  isHls: boolean,
  onPlaying: () => void,
  onError: () => void,
  authToken?: string | null,
): Player {
  const needsAuthHeader = Boolean(authToken && /[?&]uid=\d+/.test(url));
  const hlsOpts = needsAuthHeader
    ? {
        xhrSetup: (xhr: XMLHttpRequest) => {
          xhr.setRequestHeader('Authorization', `Bearer ${authToken}`);
        },
      }
    : undefined;
  const player = new Player({
    el,
    url,
    autoplay: true,
    playsinline: true,
    fluid: true,
    lang: 'zh-cn',
    isLive: false,
    plugins: isHls ? [HlsPlugin] : [],
    ...(hlsOpts ? { hls: hlsOpts } : {}),
  });
  player.on('playing', onPlaying);
  player.on('error', onError);
  return player;
}

export function Watch() {
  const { id } = useParams();
  const videoId = Number(id);
  const navigate = useNavigate();
  const { user, token, ready, authRequired, playbackAuthRequired, logout, openLogin } = useAuth();
  const playbackNeedsLogin = authRequired && playbackAuthRequired;
  const userId = user?.id ?? 0;
  const canPlayMembership = Boolean(user?.is_member || user?.is_admin);
  const [favorited, setFavorited] = useState(false);
  const [favBusy, setFavBusy] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const videoRef = useRef<Video | null>(null);
  const playerRef = useRef<Player | null>(null);
  const sessionRef = useRef(new WatchPlaybackSession());
  const [video, setVideo] = useState<Video | null>(null);
  const [related, setRelated] = useState<RelatedVideo[]>([]);
  const [episodes, setEpisodes] = useState<PublicEpisode[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [metaError, setMetaError] = useState('');
  const [metaLoading, setMetaLoading] = useState(true);
  const [playError, setPlayError] = useState('');
  const [playLoading, setPlayLoading] = useState(false);
  const [playFor, setPlayFor] = useState(id ?? '');
  const [sid, setSid] = useState(0);
  const [nid, setNid] = useState(0);

  videoRef.current = video;

  if ((id ?? '') !== playFor) {
    setPlayFor(id ?? '');
    setSid(0);
    setNid(0);
  }

  useEffect(() => {
    window.scrollTo({ top: 0, left: 0 });
  }, [videoId]);

  useEffect(() => {
    if (!Number.isFinite(videoId) || videoId <= 0) return;
    let cancelled = false;
    setMetaLoading(true);
    setMetaError('');
    setVideo(null);
    setRelated([]);
    setEpisodes([]);
    getVideo(videoId, token)
      .then((meta) => {
        if (!cancelled) setVideo(meta);
      })
      .catch((e) => {
        if (!cancelled) setMetaError(e instanceof Error ? e.message : '加载失败');
      })
      .finally(() => {
        if (!cancelled) setMetaLoading(false);
      });
    listRelated(videoId)
      .then((rows) => {
        if (!cancelled) setRelated(Array.isArray(rows) ? rows : []);
      })
      .catch(() => {
        if (!cancelled) setRelated([]);
      });
    listEpisodes(videoId)
      .then((rows) => {
        // Production may return data:null for videos with no episode rows (e.g. id 10056).
        if (!cancelled) setEpisodes(Array.isArray(rows) ? rows : []);
      })
      .catch(() => {
        if (!cancelled) setEpisodes([]);
      });
    listCategories()
      .then((rows) => {
        if (!cancelled) setCategories(Array.isArray(rows) ? rows : []);
      })
      .catch(() => {
        if (!cancelled) setCategories([]);
      });
    return () => {
      cancelled = true;
    };
  }, [videoId, token]);

  useEffect(() => {
    if (!videoId || !ready || metaLoading) return;
    if (!video) return;
    if (playbackNeedsLogin && (!user || !token)) return;
    if (video.membership_required && !canPlayMembership) {
      setPlayLoading(false);
      setPlayError('需要升级大会员后才能观看');
      return;
    }
    const session = new WatchPlaybackSession();
    sessionRef.current = session;

    const destroyPlayer = () => {
      playerRef.current?.destroy();
      playerRef.current = null;
    };

    const startPlayer = (url: string, type: string) => {
      if (session.debugState().cancelled || !rootRef.current) return;
      const gen = session.beginPlayer();
      destroyPlayer();
      const isHls = type === 'hls' || url.includes('.m3u8');
      const player = attachPlayer(
        rootRef.current,
        url,
        isHls,
        () => {
          session.handlePlaying(gen);
        },
        () => {
          const action = session.handleError(gen);
          if (action.type === 'ignore') return;
          if (action.type === 'fallback') {
            startPlayer(action.url, action.url.includes('.m3u8') ? 'hls' : 'url');
            return;
          }
          destroyPlayer();
          setPlayError(PLAYBACK_GENERIC_ERROR);
        },
        token,
      );
      playerRef.current = player;
    };

    (async () => {
      setPlayLoading(true);
      setPlayError('');
      try {
        const playback = await getPlayback(videoId, sid, nid, playbackNeedsLogin ? token : null);
        if (session.debugState().cancelled || !rootRef.current) return;
        if (!playback.url) {
          throw new Error('无可用播放地址');
        }
        session.beginPlayback(playback.fallback_url);
        startPlayer(playback.url, playback.type);
      } catch (e) {
        if (session.debugState().cancelled) return;
        if (playbackNeedsLogin && e instanceof ApiError && e.status === 401) {
          logout();
          navigate(loginWithNext(`/watch/${videoId}`), { replace: true });
          return;
        }
        if (e instanceof ApiError && e.code === 'playback_unavailable') {
          setPlayError(PLAYBACK_UNAVAILABLE_MESSAGE);
          return;
        }
        if (e instanceof ApiError && (e.code === 'membership_required' || e.status === 403)) {
          setPlayError(e.message || '需要升级大会员后才能观看');
          return;
        }
        setPlayError(e instanceof Error ? e.message : '播放失败');
      } finally {
        if (!session.debugState().cancelled) setPlayLoading(false);
      }
    })();

    return () => {
      if (!authRequired) {
        const current = videoRef.current;
        if (current && current.id === videoId) {
          const played = (playerRef.current as { currentTime?: number } | null)?.currentTime;
          const progress = typeof played === 'number' && Number.isFinite(played) ? Math.floor(played) : 0;
          upsertGuestHistory(current, progress);
        }
      }
      session.cancel();
      destroyPlayer();
    };
  }, [videoId, sid, nid, ready, metaLoading, video, playbackNeedsLogin, user, token, navigate, logout, canPlayMembership]);

  // Favorite status for the signed-in user.
  useEffect(() => {
    setFavorited(false);
    if (!ready || !Number.isFinite(videoId) || videoId <= 0) return;
    if (!authRequired) {
      setFavorited(isGuestFavorite(videoId));
      return;
    }
    if (!userId || !token) return;
    let cancelled = false;
    getFavoriteStatus(token, videoId)
      .then((value) => {
        if (!cancelled) setFavorited(value);
      })
      .catch(() => {
        if (!cancelled) setFavorited(false);
      });
    return () => {
      cancelled = true;
    };
  }, [ready, authRequired, userId, token, videoId]);

  useEffect(() => {
    if (!ready || authRequired || !video || video.id !== videoId) return;
    upsertGuestHistory(video, 0);
  }, [ready, authRequired, video, videoId]);

  // Record watch history once per visit (fire-and-forget; independent from playback).
  useEffect(() => {
    if (!ready || !authRequired || !userId || !token || !Number.isFinite(videoId) || videoId <= 0) return;
    void upsertHistory(token, videoId).catch(() => {
      /* history is best-effort */
    });
  }, [ready, authRequired, userId, token, videoId]);

  async function toggleFavorite() {
    if (!authRequired) {
      if (!video) return;
      if (favorited) removeGuestFavorite(videoId);
      else addGuestFavorite(video);
      setFavorited(isGuestFavorite(videoId));
      return;
    }
    if (!user || !token) {
      openLogin('login');
      return;
    }
    if (favBusy) return;
    const next = !favorited;
    setFavBusy(true);
    setFavorited(next);
    try {
      const result = next ? await addFavorite(token, videoId) : await removeFavorite(token, videoId);
      setFavorited(result);
    } catch {
      setFavorited(!next);
    } finally {
      setFavBusy(false);
    }
  }

  if (!ready) {
    return (
      <div className="page watch">
        <p style={{ padding: 24, color: 'var(--muted)' }}>正在验证登录状态…</p>
      </div>
    );
  }

  if (!Number.isFinite(videoId) || videoId <= 0) {
    return (
      <div className="page watch">
        <p>无效的视频</p>
      </div>
    );
  }

  const categoryName = (categoryId: number) => categories.find((category) => category.id === categoryId)?.name ?? '';
  const activeSid = sid > 0 ? sid : (video?.play_sid ?? 0);
  const activeNid = nid > 0 ? nid : (video?.play_nid ?? 0);
  const meta = video
    ? [video.view_count != null ? `${formatViews(video.view_count)} 播放` : '', video.year, categoryName(video.category_id), video.area]
        .map((part) => (part ?? '').trim())
        .filter(Boolean)
        .join(' · ')
    : '';
  const facts = video
    ? [
        video.year ? ['年份', video.year] : null,
        video.area ? ['地区', video.area] : null,
        categoryName(video.category_id) ? ['分类', categoryName(video.category_id)] : null,
        video.director ? ['导演', video.director] : null,
        video.actors ? ['演员', video.actors] : null,
        video.duration_sec > 0 ? ['时长', formatDuration(video.duration_sec)] : null,
        video.rating ? ['评分', String(video.rating)] : null,
      ].filter((row): row is [string, string] => row != null)
    : [];

  const membershipLocked = Boolean(video?.membership_required && !canPlayMembership);

  return (
    <div className="page watch">
      <header className="watch-head">
        <p style={{ margin: '0 0 8px', fontSize: 13 }}>
          <Link to="/" style={{ color: 'var(--brand)', fontWeight: 600 }}>
            ← 返回首页
          </Link>
        </p>
        <div className="watch-title-row">
          <h1>{video?.title ?? (metaLoading ? '加载中…' : metaError ? '视频' : '视频')}</h1>
          <button
            type="button"
            className={`watch-fav${favorited ? ' is-active' : ''}`}
            aria-pressed={favorited}
            disabled={favBusy}
            onClick={() => void toggleFavorite()}
          >
            <span aria-hidden>{favorited ? '★' : '☆'}</span>
            <span>{favorited ? '已收藏' : '收藏'}</span>
          </button>
        </div>
        {meta && <p className="watch-meta">{meta}</p>}
        {membershipLocked ? (
          <div className="watch-membership-bar" role="status">
            <span>本片为大会员专属内容，升级后即可观看</span>
            <Link className="watch-membership-cta" to="/vip">
              升级大会员
            </Link>
          </div>
        ) : null}
        {metaError && <p className="watch-comments-error">{metaError}</p>}
      </header>
      <div className={`watch-main-grid${related.length > 0 ? ' has-recs' : ''}`}>
        <div className="player-panel">
          {membershipLocked ? (
            <div className="player-gate">
              <p>需要升级大会员后才能观看</p>
              <button type="button" onClick={() => navigate('/vip')}>
                去升级大会员
              </button>
            </div>
          ) : !playbackNeedsLogin || (user && token) ? (
            <>
              <div className="player-wrap" ref={rootRef} />
              {playLoading && <p style={{ padding: 12, color: 'var(--muted)' }}>加载中…</p>}
              {playError && <p style={{ padding: 12, color: 'var(--muted)' }}>{playError}</p>}
            </>
          ) : (
            <div className="player-gate">
              <p>登录后观看完整视频</p>
              <button type="button" onClick={() => navigate(loginWithNext(`/watch/${videoId}`))}>
                立即登录
              </button>
            </div>
          )}
        </div>
        <EpisodeStrip
          episodes={episodes}
          activeSid={activeSid}
          activeNid={activeNid}
          onSelect={(episode) => {
            if (playbackNeedsLogin && (!user || !token)) {
              navigate(loginWithNext(`/watch/${videoId}`));
              return;
            }
            setSid(episode.sid);
            setNid(episode.nid);
          }}
        />
        <section className="watch-info" aria-label="视频信息">
          {video?.description?.trim() && <p className="watch-desc">{video.description}</p>}
          {facts.length > 0 && (
            <dl className="watch-facts">
              {facts.map(([label, value]) => (
                <div key={label}>
                  <dt>{label}</dt>
                  <dd>{value}</dd>
                </div>
              ))}
            </dl>
          )}
        </section>
        <RecommendationList items={related} categoryName={categoryName} />
      </div>
      <CommentSection
        key={videoId}
        videoId={videoId}
        token={token}
        nickname={user?.nickname ?? ''}
        onLogin={() => {
          if (authRequired) navigate(loginWithNext(`/watch/${videoId}`));
        }}
      />
    </div>
  );
}
