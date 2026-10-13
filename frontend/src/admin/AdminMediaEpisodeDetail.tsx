import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ApiError,
  fetchAdminMediaEpisode,
  type AdminMediaEpisode,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';

function formatTime(value?: string | null) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

export function AdminMediaEpisodeDetail() {
  const { id: idParam } = useParams();
  const id = Number(idParam);
  const { token, logout } = useAuth();
  const navigate = useNavigate();
  const [data, setData] = useState<AdminMediaEpisode | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      if (!Number.isFinite(id) || id <= 0) {
        setError('无效的分集 ID');
        setLoading(false);
        return;
      }
      setLoading(true);
      setError(null);
      setForbidden(false);
      try {
        const next = await fetchAdminMediaEpisode(id, token);
        if (!cancelled) setData(next);
      } catch (e) {
        if (cancelled) return;
        if (e instanceof ApiError) {
          if (e.status === 401) {
            logout();
            navigate(loginWithNext(`/admin/media/episodes/${id}`), { replace: true });
            return;
          }
          if (e.status === 403) {
            setForbidden(true);
            setError(e.message || '需要管理员权限');
            return;
          }
          setError(e.message);
          return;
        }
        setError(e instanceof Error ? e.message : '加载失败');
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [id, token, logout, navigate]);

  return (
    <section className="admin-page">
      <div className="admin-page-head">
        <p className="admin-breadcrumb">
          <Link to="/admin/media">Media</Link> / 分集详情
        </p>
        <h1>{data ? `Episode #${data.episode_id}` : 'Media Episode Detail'}</h1>
        <p className="admin-muted">只读详情 · 不含播放地址、对象密钥与错误原文</p>
      </div>

      <div className="admin-alert is-info" role="note">
        Media migration run history is not tracked by the application yet.
      </div>

      {loading ? <p className="admin-muted">加载中…</p> : null}
      {forbidden ? (
        <div className="admin-alert is-danger" role="alert">
          无权访问（403）。
        </div>
      ) : null}
      {!loading && !forbidden && error ? (
        <div className="admin-alert is-danger" role="alert">
          {error}
        </div>
      ) : null}

      {!loading && !forbidden && !error && data ? (
        <dl className="admin-detail-fields admin-detail-panel">
          <div>
            <dt>Episode ID</dt>
            <dd>{data.episode_id}</dd>
          </div>
          <div>
            <dt>Video ID</dt>
            <dd>{data.video_id}</dd>
          </div>
          <div>
            <dt>Video title</dt>
            <dd>{data.video_title || '—'}</dd>
          </div>
          <div>
            <dt>Episode title</dt>
            <dd>{data.episode_title || '—'}</dd>
          </div>
          <div>
            <dt>SID / NID</dt>
            <dd>
              {data.sid} / {data.nid}
            </dd>
          </div>
          <div>
            <dt>Storage provider</dt>
            <dd>{data.storage_provider || '—'}</dd>
          </div>
          <div>
            <dt>Migration status</dt>
            <dd>{data.migration_status || 'pending'}</dd>
          </div>
          <div>
            <dt>Playback status</dt>
            <dd>{data.playback_status || '—'}</dd>
          </div>
          <div>
            <dt>Migrated time</dt>
            <dd>{formatTime(data.migrated_at)}</dd>
          </div>
          <div>
            <dt>Has migration error</dt>
            <dd>
              <span className={data.has_error ? 'admin-badge is-off' : 'admin-badge is-ok'}>
                {data.has_error ? 'Yes' : 'No'}
              </span>
            </dd>
          </div>
          <div>
            <dt>HLS object available</dt>
            <dd>
              <span
                className={
                  data.has_hls_object ? 'admin-badge is-ok' : 'admin-badge is-off'
                }
              >
                {data.has_hls_object ? 'Yes' : 'No'}
              </span>
            </dd>
          </div>
        </dl>
      ) : null}
    </section>
  );
}
