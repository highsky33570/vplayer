import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  ApiError,
  fetchAdminDashboard,
  type AdminDashboardData,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import { apiStatusLabel, triggerLabel } from './adminLabels';

function formatTime(value?: string) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

function pct(part: number, total: number) {
  if (!total) return 0;
  return Math.round((part / total) * 100);
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

  const chartBars = useMemo(() => {
    if (!data) return [];
    const items = [
      { label: '用户', value: data.users_total },
      { label: '会员', value: data.members_total ?? 0 },
      { label: '视频', value: data.videos_total },
      { label: '启用', value: data.videos_active },
      { label: 'VIP片', value: data.videos_membership ?? 0 },
      { label: '分类', value: data.categories_total },
      { label: '已付订单', value: data.orders_paid_total ?? 0 },
    ];
    const max = Math.max(1, ...items.map((i) => i.value));
    return items.map((i) => ({ ...i, height: Math.max(8, Math.round((i.value / max) * 140)) }));
  }, [data]);

  if (loading) {
    return (
      <section className="admin-page">
        <div className="admin-page-head">
          <h1>控制台</h1>
        </div>
      </section>
    );
  }

  if (forbidden) {
    return (
      <section className="admin-page">
        <div className="admin-page-head">
          <h1>控制台</h1>
        </div>
        <div className="admin-alert is-danger" role="alert">
          无权访问控制台（403）。后端已拒绝本次请求。
        </div>
      </section>
    );
  }

  if (error || !data) {
    return (
      <section className="admin-page">
        <div className="admin-page-head">
          <h1>控制台</h1>
        </div>
        <div className="admin-alert is-danger" role="alert">
          {error ?? '无法加载控制台'}
        </div>
      </section>
    );
  }

  const sync = data.latest_sync;
  const activePct = pct(data.videos_active, data.videos_total);
  const memberPct = pct(data.members_total ?? 0, data.users_total);

  return (
    <section className="admin-page">
      <div className="admin-page-head">
        <h1>控制台</h1>
      </div>

      <div className="admin-stat-grid">
        <article className="admin-stat-card">
          <span>总收入 (USD)</span>
          <strong>${Number(data.revenue_paid_usd ?? 0).toFixed(2)}</strong>
        </article>
        <article className="admin-stat-card">
          <span>总收入 (USDT)</span>
          <strong>{Number(data.revenue_paid_usdt ?? 0).toFixed(2)}</strong>
        </article>
        <article className="admin-stat-card">
          <span>已付订单</span>
          <strong>{data.orders_paid_total ?? 0}</strong>
        </article>
        <article className="admin-stat-card">
          <span>用户总数</span>
          <strong>{data.users_total}</strong>
        </article>
        <article className="admin-stat-card">
          <span>有效会员</span>
          <strong>{data.members_total ?? 0}</strong>
        </article>
        <article className="admin-stat-card">
          <span>视频总数</span>
          <strong>{data.videos_total}</strong>
        </article>
        <article className="admin-stat-card">
          <span>启用视频</span>
          <strong>{data.videos_active}</strong>
        </article>
        <article className="admin-stat-card">
          <span>会员专属片</span>
          <strong>{data.videos_membership ?? 0}</strong>
        </article>
        <article className="admin-stat-card">
          <span>分类</span>
          <strong>{data.categories_total}</strong>
        </article>
        <article className="admin-stat-card">
          <span>API 状态</span>
          <strong className={data.status === 'ok' ? 'is-ok' : ''}>
            {apiStatusLabel(data.status)}
          </strong>
        </article>
      </div>

      <div className="admin-dash-grid">
        <article className="admin-panel">
          <div className="admin-panel-head">
            <h2>关键指标</h2>
          </div>
          <div className="admin-chart" aria-label="关键指标柱状图">
            {chartBars.map((bar) => (
              <div key={bar.label} className="admin-chart-bar">
                <strong>{bar.value}</strong>
                <div className="admin-chart-bar-fill" style={{ height: bar.height }} />
                <em>{bar.label}</em>
              </div>
            ))}
          </div>
        </article>

        <article className="admin-panel">
          <div className="admin-panel-head">
            <h2>结构占比</h2>
          </div>
          <div className="admin-sync-grid">
            <div>
              <span>视频启用率</span>
              <strong>{activePct}%</strong>
            </div>
            <div>
              <span>用户会员率</span>
              <strong>{memberPct}%</strong>
            </div>
            <div>
              <span>VIP 片占比</span>
              <strong>{pct(data.videos_membership ?? 0, data.videos_total)}%</strong>
            </div>
            <div>
              <span>停用视频</span>
              <strong>{Math.max(0, data.videos_total - data.videos_active)}</strong>
            </div>
          </div>
          <div className="admin-quick-links" style={{ marginTop: 16 }}>
            <Link className="admin-quick-link" to="/admin/videos">
              <strong>视频管理</strong>
              <span>筛选、编辑片库与会员专属</span>
            </Link>
            <Link className="admin-quick-link" to="/admin/users">
              <strong>用户管理</strong>
              <span>会员状态、套餐与账号维护</span>
            </Link>
            <Link className="admin-quick-link" to="/admin/billing">
              <strong>会员套餐</strong>
              <span>价格、时长与上下架</span>
            </Link>
            <Link className="admin-quick-link" to="/admin/sync">
              <strong>同步 / 采集</strong>
              <span>增量同步与运行记录</span>
            </Link>
          </div>
        </article>
      </div>

      <article className="admin-panel">
        <div className="admin-panel-head">
          <h2>最近同步</h2>
        </div>
        {!sync ? (
          <p className="admin-empty">暂无同步记录。尚未从后台触发同步。</p>
        ) : (
          <>
            <div className="admin-sync-grid">
              <div>
                <span>来源</span>
                <strong>{sync.source_system}</strong>
              </div>
              <div>
                <span>触发方式</span>
                <strong>{triggerLabel(sync.trigger_type)}</strong>
              </div>
              <div>
                <span>开始时间</span>
                <strong>{formatTime(sync.started_at)}</strong>
              </div>
              <div>
                <span>结束时间</span>
                <strong>{formatTime(sync.finished_at)}</strong>
              </div>
              <div>
                <span>扫描数</span>
                <strong>{sync.scanned}</strong>
              </div>
              <div>
                <span>新增 / 更新</span>
                <strong>
                  {sync.created_count} / {sync.updated_count}
                </strong>
              </div>
              <div>
                <span>失败 / 停用</span>
                <strong>
                  {sync.failed_count} / {sync.deactivated_count}
                </strong>
              </div>
              <div>
                <span>结果</span>
                <strong className={sync.has_error ? 'is-bad' : 'is-ok'}>
                  {sync.has_error ? '有错误' : '完成'}
                </strong>
              </div>
            </div>
            <div className="admin-form-actions">
              <Link className="admin-btn-ghost" to={`/admin/sync/runs/${sync.id}`}>
                查看同步详情
              </Link>
              <Link className="admin-btn-primary" to="/admin/sync" style={{ textDecoration: 'none' }}>
                打开同步面板
              </Link>
            </div>
          </>
        )}
      </article>
    </section>
  );
}
