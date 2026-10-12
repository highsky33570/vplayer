import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  ApiError,
  fetchAdminDashboard,
  type AdminDashboardData,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';

function formatTime(value?: string) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

export function AdminDashboard() {
  const { token, logout } = useAuth();
  const navigate = useNavigate();
  const [data, setData] = useState<AdminDashboardData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      setError(null);
      setForbidden(false);
      try {
        const next = await fetchAdminDashboard(token);
        if (!cancelled) setData(next);
      } catch (e) {
        if (cancelled) return;
        if (e instanceof ApiError) {
          if (e.status === 401) {
            logout();
            navigate(loginWithNext('/admin'), { replace: true });
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
  }, [token, logout, navigate]);

  if (loading) {
    return (
      <section className="admin-page">
        <h1>Dashboard</h1>
        <p className="admin-muted">正在加载运营数据…</p>
      </section>
    );
  }

  if (forbidden) {
    return (
      <section className="admin-page">
        <h1>Dashboard</h1>
        <div className="admin-alert is-danger" role="alert">
          无权访问仪表盘（403）。后端已拒绝本次请求。
        </div>
      </section>
    );
  }

  if (error || !data) {
    return (
      <section className="admin-page">
        <h1>Dashboard</h1>
        <div className="admin-alert is-danger" role="alert">
          {error ?? '无法加载仪表盘'}
        </div>
      </section>
    );
  }

  const sync = data.latest_sync;

  return (
    <section className="admin-page">
      <div className="admin-page-head">
        <h1>Dashboard</h1>
        <p className="admin-muted">只读运营概览 · API {data.status === 'ok' ? '正常' : data.status}</p>
      </div>

      <div className="admin-stat-grid">
        <article className="admin-stat-card">
          <span>Users</span>
          <strong>{data.users_total}</strong>
        </article>
        <article className="admin-stat-card">
          <span>Videos</span>
          <strong>{data.videos_total}</strong>
        </article>
        <article className="admin-stat-card">
          <span>Active Videos</span>
          <strong>{data.videos_active}</strong>
        </article>
        <article className="admin-stat-card">
          <span>Categories</span>
          <strong>{data.categories_total}</strong>
        </article>
        <article className="admin-stat-card">
          <span>API Status</span>
          <strong className={data.status === 'ok' ? 'is-ok' : ''}>
            {data.status === 'ok' ? 'OK' : data.status}
          </strong>
        </article>
      </div>

      <article className="admin-panel">
        <div className="admin-panel-head">
          <h2>Sync Status</h2>
        </div>
        {!sync ? (
          <p className="admin-empty">暂无同步记录。尚未从后台触发同步。</p>
        ) : (
          <div className="admin-sync-grid">
            <div>
              <span>Source</span>
              <strong>{sync.source_system}</strong>
            </div>
            <div>
              <span>Trigger</span>
              <strong>{sync.trigger_type}</strong>
            </div>
            <div>
              <span>Started</span>
              <strong>{formatTime(sync.started_at)}</strong>
            </div>
            <div>
              <span>Finished</span>
              <strong>{formatTime(sync.finished_at)}</strong>
            </div>
            <div>
              <span>Scanned</span>
              <strong>{sync.scanned}</strong>
            </div>
            <div>
              <span>Created / Updated</span>
              <strong>
                {sync.created_count} / {sync.updated_count}
              </strong>
            </div>
            <div>
              <span>Failed / Deactivated</span>
              <strong>
                {sync.failed_count} / {sync.deactivated_count}
              </strong>
            </div>
            <div>
              <span>Result</span>
              <strong className={sync.has_error ? 'is-bad' : 'is-ok'}>
                {sync.has_error ? '有错误' : '完成'}
              </strong>
            </div>
          </div>
        )}
      </article>
    </section>
  );
}
