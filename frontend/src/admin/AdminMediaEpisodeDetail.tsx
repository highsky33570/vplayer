import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ApiError,
  fetchAdminMediaEpisode,
  type AdminMediaEpisode,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import { migrationStatusLabel, playbackStatusLabel, yesNo } from './adminLabels';

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
          <Link to="/admin/media">媒体管理</Link> / 分集详情
        </p>
        <h1>{data ? `分集 #${data.episode_id}` : '分集详情'}</h1>
        <p className="admin-muted">只读详情 · 不含播放地址、对象密钥与错误原文</p>
      </div>

      <div className="admin-alert is-info" role="note">
        应用尚未记录媒体迁移的运行历史。
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
            <dt>分集 ID</dt>
            <dd>{data.episode_id}</dd>
          </div>
          <div>
            <dt>视频 ID</dt>
            <dd>{data.video_id}</dd>
          </div>
          <div>
            <dt>视频标题</dt>
            <dd>{data.video_title || '—'}</dd>
          </div>
          <div>
            <dt>分集标题</dt>
            <dd>{data.episode_title || '—'}</dd>
          </div>
          <div>
            <dt>SID / NID</dt>
            <dd>
              {data.sid} / {data.nid}
            </dd>
          </div>
          <div>
            <dt>存储</dt>
            <dd>{data.storage_provider || '—'}</dd>
          </div>
          <div>
            <dt>迁移状态</dt>
            <dd>{migrationStatusLabel(data.migration_status)}</dd>
          </div>
          <div>
            <dt>播放状态</dt>
            <dd>{playbackStatusLabel(data.playback_status)}</dd>
          </div>
          <div>
            <dt>迁移时间</dt>
            <dd>{formatTime(data.migrated_at)}</dd>
          </div>
          <div>
            <dt>迁移错误</dt>
            <dd>
              <span className={data.has_error ? 'admin-badge is-off' : 'admin-badge is-ok'}>
                {yesNo(data.has_error)}
              </span>
            </dd>
          </div>
          <div>
            <dt>HLS 对象</dt>
            <dd>
              <span
                className={
                  data.has_hls_object ? 'admin-badge is-ok' : 'admin-badge is-off'
                }
              >
                {data.has_hls_object ? '已有' : '无'}
              </span>
            </dd>
          </div>
        </dl>
      ) : null}
    </section>
  );
}
