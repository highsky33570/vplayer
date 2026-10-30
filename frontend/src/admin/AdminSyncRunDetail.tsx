import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ApiError,
  fetchAdminSyncRun,
  type AdminSyncRunSummary,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import { catalogSyncResult, triggerLabel } from './adminLabels';

function formatTime(value?: string) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

export function AdminSyncRunDetail() {
  const { id: idParam } = useParams();
  const id = Number(idParam);
  const { token, logout } = useAuth();
  const navigate = useNavigate();
  const [data, setData] = useState<AdminSyncRunSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      if (!Number.isFinite(id) || id <= 0) {
        setError('无效的同步记录 ID');
        setLoading(false);
        return;
      }
      setLoading(true);
      setError(null);
      setForbidden(false);
      try {
        const next = await fetchAdminSyncRun(id, token);
        if (!cancelled) setData(next);
      } catch (e) {
        if (cancelled) return;
        if (e instanceof ApiError) {
          if (e.status === 401) {
            logout();
            navigate(loginWithNext(`/admin/sync/runs/${id}`), { replace: true });
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
          <Link to="/admin/sync">同步 / 采集</Link> / 记录详情
        </p>
        <h1>{data ? `同步记录 #${data.id}` : '同步记录详情'}</h1>
      </div>

      <div className="admin-alert is-info" role="note">
        应用尚未跟踪工作进程或定时器的实时状态。
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
            <dt>ID</dt>
            <dd>{data.id}</dd>
          </div>
          <div>
            <dt>来源</dt>
            <dd>{data.source_system}</dd>
          </div>
          <div>
            <dt>触发方式</dt>
            <dd>{triggerLabel(data.trigger_type)}</dd>
          </div>
          <div>
            <dt>开始时间</dt>
            <dd>{formatTime(data.started_at)}</dd>
          </div>
          <div>
            <dt>结束时间</dt>
            <dd>{formatTime(data.finished_at)}</dd>
          </div>
          <div>
            <dt>扫描数</dt>
            <dd>{data.scanned}</dd>
          </div>
          <div>
            <dt>新增</dt>
            <dd>{data.created_count}</dd>
          </div>
          <div>
            <dt>更新</dt>
            <dd>{data.updated_count}</dd>
          </div>
          <div>
            <dt>未变化</dt>
            <dd>{data.unchanged_count}</dd>
          </div>
          <div>
            <dt>失败</dt>
            <dd>{data.failed_count}</dd>
          </div>
          <div>
            <dt>停用</dt>
            <dd>{data.deactivated_count}</dd>
          </div>
          <div>
            <dt>状态</dt>
            <dd>
              <span
                className={
                  data.has_error || data.failed_count > 0 ? 'admin-badge is-off' : 'admin-badge is-ok'
                }
              >
                {catalogSyncResult(data)}
              </span>
            </dd>
          </div>
        </dl>
      ) : null}
    </section>
  );
}
