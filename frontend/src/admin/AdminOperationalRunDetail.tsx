import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ApiError,
  fetchAdminOperationalRun,
  type AdminOperationalRun,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import {
  formatDuration,
  formatNullableCount,
  jobLabel,
  runStatusLabel,
  triggerLabel,
} from './adminLabels';

function formatTime(value?: string | null) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
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
          <Link to="/admin/sync">同步 / 采集</Link> / 运行记录
        </p>
        <h1>{data ? `运行记录 #${data.id}` : '运行记录详情'}</h1>
      </div>

      <div className="admin-alert is-info" role="note">
        这里不查询 systemd 或定时器的实时状态。本页只展示{' '}
        <span className="admin-mono">operational_runs</span> 中已记录的一行。
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
            <dt>任务</dt>
            <dd>
              {jobLabel(data.job_type)}{' '}
              <span className="admin-mono admin-muted">({data.job_type})</span>
            </dd>
          </div>
          <div>
            <dt>触发方式</dt>
            <dd>
              {triggerLabel(data.trigger_type)}{' '}
              <span className="admin-mono admin-muted">({data.trigger_type})</span>
            </dd>
          </div>
          <div>
            <dt>状态</dt>
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
                {runStatusLabel(data.status)}
              </span>
              {data.stale ? (
                <span className="admin-badge is-off" style={{ marginLeft: 6 }}>
                  超时
                </span>
              ) : null}
            </dd>
          </div>
          <div>
            <dt>请求时间</dt>
            <dd>{formatTime(data.requested_at)}</dd>
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
            <dt>耗时</dt>
            <dd>{formatDuration(data.duration_seconds)}</dd>
          </div>
          {useSelectedCounters ? (
            <>
              <div>
                <dt>已选</dt>
                <dd>{formatNullableCount(data.selected_count)}</dd>
              </div>
              <div>
                <dt>成功</dt>
                <dd>{formatNullableCount(data.succeeded_count)}</dd>
              </div>
              <div>
                <dt>跳过</dt>
                <dd>{formatNullableCount(data.skipped_count)}</dd>
              </div>
              {isEnrichment ? (
                <div>
                  <dt>不可用</dt>
                  <dd>{formatNullableCount(data.unavailable_count)}</dd>
                </div>
              ) : null}
              <div>
                <dt>失败</dt>
                <dd>{formatNullableCount(data.failed_count)}</dd>
              </div>
            </>
          ) : (
            <>
              <div>
                <dt>扫描数</dt>
                <dd>{formatNullableCount(data.scanned)}</dd>
              </div>
              <div>
                <dt>新增</dt>
                <dd>{formatNullableCount(data.created_count)}</dd>
              </div>
              <div>
                <dt>更新</dt>
                <dd>{formatNullableCount(data.updated_count)}</dd>
              </div>
              <div>
                <dt>未变化</dt>
                <dd>{formatNullableCount(data.unchanged_count)}</dd>
              </div>
              <div>
                <dt>失败</dt>
                <dd>{formatNullableCount(data.failed_count)}</dd>
              </div>
            </>
          )}
          <div>
            <dt>错误</dt>
            <dd>
              {data.has_error ? (
                <span className="admin-mono">{data.error_code || '—'}</span>
              ) : (
                '—'
              )}
            </dd>
          </div>
          <div>
            <dt>发起用户 ID</dt>
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
