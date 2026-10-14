import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ApiError,
  fetchAdminOperationalRun,
  type AdminOperationalRun,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';

function formatTime(value?: string | null) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

function formatNullableCount(value?: number | null) {
  if (value === null || value === undefined) return '—';
  return String(value);
}

function formatDuration(seconds?: number | null) {
  if (seconds === null || seconds === undefined) return '—';
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return s ? `${m}m ${s}s` : `${m}m`;
}

function jobLabel(jobType: string) {
  switch (jobType) {
    case 'incremental_sync':
      return 'Incremental Sync';
    case 'enrichment':
      return 'Pending Enrichment';
    case 'media_migration':
      return 'Media Migration';
    default:
      return jobType || '—';
  }
}

function triggerLabel(triggerType: string) {
  switch (triggerType) {
    case 'cli':
      return 'CLI';
    case 'timer':
      return 'Timer';
    default:
      return triggerType || '—';
  }
}

function statusLabel(status: string) {
  switch (status) {
    case 'running':
      return 'Running';
    case 'succeeded':
      return 'Succeeded';
    case 'failed':
      return 'Failed';
    case 'rejected':
      return 'Rejected';
    default:
      return status || '—';
  }
}

export function AdminOperationalRunDetail() {
  const { id: idParam } = useParams();
  const id = Number(idParam);
  const { token, logout } = useAuth();
  const navigate = useNavigate();
  const [data, setData] = useState<AdminOperationalRun | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      if (!Number.isFinite(id) || id <= 0) {
        setError('无效的运行记录 ID');
        setLoading(false);
        return;
      }
      setLoading(true);
      setError(null);
      setForbidden(false);
      try {
        const next = await fetchAdminOperationalRun(id, token);
        if (!cancelled) setData(next);
      } catch (e) {
        if (cancelled) return;
        if (e instanceof ApiError) {
          if (e.status === 401) {
            logout();
            navigate(loginWithNext(`/admin/sync/operations/${id}`), { replace: true });
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

  const isEnrichment = data?.job_type === 'enrichment';
  const isMediaMigration = data?.job_type === 'media_migration';
  const useSelectedCounters = isEnrichment || isMediaMigration;

  return (
    <section className="admin-page">
      <div className="admin-page-head">
        <p className="admin-breadcrumb">
          <Link to="/admin/sync">Sync / Crawler</Link> / Operational Run
        </p>
        <h1>{data ? `Operational Run #${data.id}` : 'Operational Run Detail'}</h1>
        <p className="admin-muted">只读 worker 审计记录 · 不含错误原文与内部诊断</p>
      </div>

      <div className="admin-alert is-info" role="note">
        Live systemd/timer status is not queried. This page shows a recorded{' '}
        <span className="admin-mono">operational_runs</span> row only.
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
            <dt>Job</dt>
            <dd>
              {jobLabel(data.job_type)}{' '}
              <span className="admin-mono admin-muted">({data.job_type})</span>
            </dd>
          </div>
          <div>
            <dt>Trigger</dt>
            <dd>
              {triggerLabel(data.trigger_type)}{' '}
              <span className="admin-mono admin-muted">({data.trigger_type})</span>
            </dd>
          </div>
          <div>
            <dt>Status</dt>
            <dd>
              <span
                className={
                  data.stale || data.status === 'failed' || data.status === 'rejected' || data.has_error
                    ? 'admin-badge is-off'
                    : data.status === 'succeeded'
                      ? 'admin-badge is-ok'
                      : 'admin-badge'
                }
              >
                {statusLabel(data.status)}
              </span>
              {data.stale ? (
                <span className="admin-badge is-off" style={{ marginLeft: 6 }}>
                  stale
                </span>
              ) : null}
            </dd>
          </div>
          <div>
            <dt>Requested</dt>
            <dd>{formatTime(data.requested_at)}</dd>
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
            <dt>Duration</dt>
            <dd>{formatDuration(data.duration_seconds)}</dd>
          </div>
          {useSelectedCounters ? (
            <>
              <div>
                <dt>Selected</dt>
                <dd>{formatNullableCount(data.selected_count)}</dd>
              </div>
              <div>
                <dt>Succeeded</dt>
                <dd>{formatNullableCount(data.succeeded_count)}</dd>
              </div>
              <div>
                <dt>Skipped</dt>
                <dd>{formatNullableCount(data.skipped_count)}</dd>
              </div>
              {isEnrichment ? (
                <div>
                  <dt>Unavailable</dt>
                  <dd>{formatNullableCount(data.unavailable_count)}</dd>
                </div>
              ) : null}
              <div>
                <dt>Failed</dt>
                <dd>{formatNullableCount(data.failed_count)}</dd>
              </div>
            </>
          ) : (
            <>
              <div>
                <dt>Scanned</dt>
                <dd>{formatNullableCount(data.scanned)}</dd>
              </div>
              <div>
                <dt>Created</dt>
                <dd>{formatNullableCount(data.created_count)}</dd>
              </div>
              <div>
                <dt>Updated</dt>
                <dd>{formatNullableCount(data.updated_count)}</dd>
              </div>
              <div>
                <dt>Unchanged</dt>
                <dd>{formatNullableCount(data.unchanged_count)}</dd>
              </div>
              <div>
                <dt>Failed</dt>
                <dd>{formatNullableCount(data.failed_count)}</dd>
              </div>
            </>
          )}
          <div>
            <dt>Error</dt>
            <dd>
              {data.has_error ? (
                <span className="admin-mono">{data.error_code || '—'}</span>
              ) : (
                '—'
              )}
            </dd>
          </div>
          <div>
            <dt>Requested by user ID</dt>
            <dd>
              {data.requested_by_user_id === null || data.requested_by_user_id === undefined
                ? '—'
                : data.requested_by_user_id}
            </dd>
          </div>
        </dl>
      ) : null}
    </section>
  );
}
