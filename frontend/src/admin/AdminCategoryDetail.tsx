import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ApiError,
  fetchAdminCategory,
  type AdminCategory,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import { categoryOwnership } from './categoryOwnership';

function formatTime(value?: string) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

export function AdminCategoryDetail() {
  const { id: idParam } = useParams();
  const id = Number(idParam);
  const { token, logout } = useAuth();
  const navigate = useNavigate();
  const [data, setData] = useState<AdminCategory | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      if (!Number.isFinite(id) || id <= 0) {
        setError('无效的分类 ID');
        setLoading(false);
        return;
      }
      setLoading(true);
      setError(null);
      setForbidden(false);
      try {
        const next = await fetchAdminCategory(id, token);
        if (!cancelled) setData(next);
      } catch (e) {
        if (cancelled) return;
        if (e instanceof ApiError) {
          if (e.status === 401) {
            logout();
            navigate(loginWithNext(`/admin/categories/${id}`), { replace: true });
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

  const ownership = data ? categoryOwnership(data) : null;

  return (
    <section className="admin-page">
      <div className="admin-page-head">
        <p className="admin-breadcrumb">
          <Link to="/admin/categories">Categories</Link> / 详情
        </p>
        <h1>{data?.name ?? 'Category Detail'}</h1>
        <p className="admin-muted">只读详情 · 不含同步错误原文</p>
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
        <>
          {ownership === 'Synced' ? (
            <div className="admin-alert is-info" role="status">
              This category is managed by source synchronization.
            </div>
          ) : null}
          <dl className="admin-detail-fields admin-detail-panel">
            <div>
              <dt>ID</dt>
              <dd>{data.id}</dd>
            </div>
            <div>
              <dt>Name</dt>
              <dd>{data.name}</dd>
            </div>
            <div>
              <dt>Slug</dt>
              <dd>{data.slug}</dd>
            </div>
            <div>
              <dt>Videos</dt>
              <dd>{data.video_count}</dd>
            </div>
            <div>
              <dt>Sort</dt>
              <dd>{data.sort}</dd>
            </div>
            <div>
              <dt>Active</dt>
              <dd>{data.is_active ? 'active' : 'inactive'}</dd>
            </div>
            <div>
              <dt>Ownership</dt>
              <dd>{ownership}</dd>
            </div>
            <div>
              <dt>Source</dt>
              <dd>
                {ownership === 'Synced'
                  ? `${data.source_system} / ${data.source_id}`
                  : '—'}
              </dd>
            </div>
            <div>
              <dt>Sync status</dt>
              <dd>
                {data.sync_status || '—'}
                {data.has_error ? ' (has error)' : ''}
              </dd>
            </div>
            <div>
              <dt>Last synced</dt>
              <dd>{formatTime(data.last_synced_at)}</dd>
            </div>
            <div>
              <dt>Created</dt>
              <dd>{formatTime(data.created_at)}</dd>
            </div>
          </dl>
        </>
      ) : null}
    </section>
  );
}
