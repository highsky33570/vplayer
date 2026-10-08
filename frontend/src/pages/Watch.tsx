import { useEffect, useRef, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import Player from 'xgplayer';
import HlsPlugin from 'xgplayer-hls';
import 'xgplayer/dist/index.min.css';
import { formatViews, getPlayback, getVideo, type Video } from '../api';

export function Watch() {
  const { id } = useParams();
  const videoId = Number(id);
  const rootRef = useRef<HTMLDivElement>(null);
  const playerRef = useRef<Player | null>(null);
  const [video, setVideo] = useState<Video | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!videoId) return;
    let cancelled = false;

    (async () => {
      setLoading(true);
      setError('');
      try {
        const meta = await getVideo(videoId);
        if (cancelled) return;
        setVideo(meta);
        const playback = await getPlayback(videoId);
        if (cancelled || !rootRef.current) return;
        if (!playback.url) {
          throw new Error('无可用播放地址');
        }

        playerRef.current?.destroy();
        playerRef.current = null;
        const isHls = playback.type === 'hls' || playback.url.includes('.m3u8');
        const player = new Player({
          el: rootRef.current,
          url: playback.url,
          autoplay: true,
          playsinline: true,
          fluid: true,
          lang: 'zh-cn',
          isLive: false,
          plugins: isHls ? [HlsPlugin] : [],
        });
        player.on('error', () => {
          if (!cancelled) setError('播放失败，请稍后重试');
        });
        playerRef.current = player;
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : '播放失败');
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();

    return () => {
      cancelled = true;
      playerRef.current?.destroy();
      playerRef.current = null;
    };
  }, [videoId]);

  if (error) {
    return <div className="page page-error">{error}</div>;
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
