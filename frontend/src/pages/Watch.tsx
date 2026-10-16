import { useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import Player from 'xgplayer';
import HlsPlugin from 'xgplayer-hls';
import 'xgplayer/dist/index.min.css';
import { ApiError, formatViews, getPlayback, getVideo, type Video } from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
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
): Player {
  const player = new Player({
    el,
    url,
    autoplay: true,
    playsinline: true,
    fluid: true,
    lang: 'zh-cn',
    isLive: false,
    plugins: isHls ? [HlsPlugin] : [],
  });
  player.on('playing', onPlaying);
  player.on('error', onError);
  return player;
}

export function Watch() {
  const { id } = useParams();
  const videoId = Number(id);
  const navigate = useNavigate();
  const { user, token, ready, logout } = useAuth();
  const rootRef = useRef<HTMLDivElement>(null);
  const playerRef = useRef<Player | null>(null);
  const sessionRef = useRef(new WatchPlaybackSession());
  const [video, setVideo] = useState<Video | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);

  // Gate: wait for auth hydration; guests go to login with return path.
  useEffect(() => {
    if (!videoId || !ready) return;
    if (!user || !token) {
      navigate(loginWithNext(`/watch/${videoId}`), { replace: true });
    }
  }, [ready, user, token, videoId, navigate]);

  useEffect(() => {
    if (!videoId || !ready || !user || !token) {
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
      // Bump generation first so any late events from the previous player are ignored.
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
          // Limitation: xgplayer error is not reliably manifest-vs-segment classified.
          // Only switch before confirmed `playing`, at most once, and only for current gen.
          const action = session.handleError(gen);
          if (action.type === 'ignore') return;
          if (action.type === 'fallback') {
            startPlayer(action.url, action.url.includes('.m3u8') ? 'hls' : 'url');
            return;
          }
          destroyPlayer();
          setError(PLAYBACK_GENERIC_ERROR);
        },
      );
      playerRef.current = player;
    };

    (async () => {
      setLoading(true);
      setError('');
      try {
        const meta = await getVideo(videoId);
        if (session.debugState().cancelled) return;
        setVideo(meta);
        const playback = await getPlayback(videoId, 0, 0, token);
        if (session.debugState().cancelled || !rootRef.current) return;
        if (!playback.url) {
          throw new Error('无可用播放地址');
        }

        session.beginPlayback(playback.fallback_url);
        startPlayer(playback.url, playback.type);
      } catch (e) {
        if (session.debugState().cancelled) return;
        if (e instanceof ApiError && e.status === 401) {
          logout();
          navigate(loginWithNext(`/watch/${videoId}`), { replace: true });
          return;
        }
        if (e instanceof ApiError && e.code === 'playback_unavailable') {
          setError(PLAYBACK_UNAVAILABLE_MESSAGE);
          return;
        }
        setError(e instanceof Error ? e.message : '播放失败');
      } finally {
        if (!session.debugState().cancelled) setLoading(false);
      }
    })();

    return () => {
      session.cancel();
      destroyPlayer();
    };
  }, [videoId, ready, user, token, navigate, logout]);

  if (!ready || !user || !token) {
    return (
      <div className="page watch">
        <p style={{ padding: 24, color: 'var(--muted)' }}>正在验证登录状态…</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="page watch">
        <aside className="watch-side" style={{ padding: 24 }}>
          <p style={{ margin: '0 0 8px', fontSize: 13 }}>
            <Link to="/" style={{ color: 'var(--brand)', fontWeight: 600 }}>
              ← 返回首页
            </Link>
          </p>
          <h2>{video?.title ?? '视频'}</h2>
          <p style={{ marginTop: 16, color: 'var(--muted)' }}>{error}</p>
        </aside>
      </div>
    );
  }

  return (
    <div className="page watch">
      <div className="player-panel">
        <div className="player-wrap" ref={rootRef} />
        {loading && <p style={{ padding: 12, color: 'var(--muted)' }}>加载中…</p>}
      </div>
      <aside className="watch-side">
        <p style={{ margin: '0 0 8px', fontSize: 13 }}>
          <Link to="/" style={{ color: 'var(--brand)', fontWeight: 600 }}>
            ← 返回首页
          </Link>
        </p>
        <h2>{video?.title ?? '加载中…'}</h2>
        <p>{video?.description || '精彩内容即将开始'}</p>
        {video && (
          <p style={{ marginTop: 10, color: 'var(--muted)', fontSize: 12 }}>
            {[video.year, video.area, video.director].filter(Boolean).join(' · ')}
            {video.view_count ? ` · ${formatViews(video.view_count)} 播放` : ''}
          </p>
        )}
        {video?.actors && (
          <p style={{ marginTop: 8, color: 'var(--muted)', fontSize: 12 }}>演员：{video.actors}</p>
        )}
        <span className="watch-chip">HLS · 西瓜播放器</span>
      </aside>
    </div>
  );
}
