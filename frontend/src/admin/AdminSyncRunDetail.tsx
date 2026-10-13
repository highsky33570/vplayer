import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ApiError,
  fetchAdminSyncRun,
  type AdminSyncRunSummary,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';

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
          <Link to="/admin/sync">Sync / Crawler</Link> / 记录详情
        </p>
        <h1>{data ? `Sync Run #${data.id}` : 'Sync Run Detail'}</h1>
        <p className="admin-muted">只读记录 · 不含错误原文与 summary_json</p>
      </div>

      <div className="admin-alert is-info" role="note">
        Worker/timer live status is not tracked by the application yet.
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
            <dt>Source</dt>
            <dd>{data.source_system}</dd>
          </div>
          <div>
            <dt>Trigger</dt>
            <dd>{data.trigger_type}</dd>
          </div>
          <div>
            <dt>Started</dt>
            <dd>{formatTime(data.started_at)}</dd>
          </div>
          <div>
            <dt>Finished</dt>
            <dd>{formatTime(data.finished_at)}</dd>
          </div>
          <div>
            <dt>Scanned</dt>
            <dd>{data.scanned}</dd>
          </div>
          <div>
            <dt>Created</dt>
            <dd>{data.created_count}</dd>
          </div>
          <div>
            <dt>Updated</dt>
            <dd>{data.updated_count}</dd>
          </div>
          <div>
            <dt>Unchanged</dt>
            <dd>{data.unchanged_count}</dd>
          </div>
          <div>
            <dt>Failed</dt>
            <dd>{data.failed_count}</dd>
          </div>
          <div>
            <dt>Deactivated</dt>
            <dd>{data.deactivated_count}</dd>
          </div>
          <div>
            <dt>Status</dt>
            <dd>
              <span
                className={
                  data.has_error || data.failed_count > 0 ? 'admin-badge is-off' : 'admin-badge is-ok'
                }
              >
                {data.has_error
                  ? 'recorded error'
                  : data.failed_count > 0
                    ? 'partial failures'
                    : 'recorded ok'}
              </span>
            </dd>
          </div>
        </dl>
      ) : null}
    </section>
  );
}
