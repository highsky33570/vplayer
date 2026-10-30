import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { ApiError, fetchAdminPayments, type AdminPayment } from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';

type StatusFilter = 'all' | 'pending' | 'paid' | 'expired' | 'cancelled';

function formatTime(value?: string | null) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

function formatMoney(usd: number, usdt: number) {
  const parts: string[] = [];
  if (usd > 0) parts.push(`$${usd.toFixed(2)}`);
  if (usdt > 0) parts.push(`${usdt} USDT`);
  return parts.length ? parts.join(' / ') : '—';
}

function statusLabel(status: string) {
  switch (status) {
    case 'paid':
      return '已支付';
    case 'pending':
      return '待支付';
    case 'expired':
      return '已过期';
    case 'cancelled':
      return '已取消';
    default:
      return status || '—';
  }
}

function statusClass(status: string) {
  if (status === 'paid') return 'admin-badge is-ok';
  if (status === 'pending') return 'admin-badge';
  return 'admin-badge is-off';
}

function deliveryLabel(status: string) {
  switch (status) {
    case 'success':
      return '开通成功';
    case 'fail':
      return '开通失败';
    case 'pending':
      return '待开通';
    case 'none':
      return '—';
    default:
      return status || '—';
  }
}

function deliveryClass(status: string) {
  if (status === 'success') return 'admin-badge is-ok';
  if (status === 'fail') return 'admin-badge is-off';
  if (status === 'pending') return 'admin-badge';
  return 'admin-muted';
}

function parsePaymentsSearch(params: URLSearchParams) {
  const pageSize = 24;
  const q = (params.get('q') ?? '').trim();
  const statusRaw = (params.get('status') ?? 'all') as StatusFilter;
  const status: StatusFilter = ['all', 'pending', 'paid', 'expired', 'cancelled'].includes(statusRaw)
    ? statusRaw
    : 'all';
  const pageRaw = Number(params.get('page') ?? '1');
  const page = Number.isFinite(pageRaw) && pageRaw > 0 ? Math.floor(pageRaw) : 1;
  return { q, status, page, pageSize };
}

function buildPaymentsSearch(input: { q: string; status: StatusFilter; page: number }) {
  const params = new URLSearchParams();
  if (input.q.trim()) params.set('q', input.q.trim());
  if (input.status !== 'all') params.set('status', input.status);
  if (input.page > 1) params.set('page', String(input.page));
  const s = params.toString();
  return s ? `?${s}` : '';
}

export function AdminPayments() {
  const { token, logout } = useAuth();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const parsed = useMemo(() => parsePaymentsSearch(params), [params]);
  const [qInput, setQInput] = useState(parsed.q);
  const [rows, setRows] = useState<AdminPayment[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);

  useEffect(() => {
    setQInput(parsed.q);
  }, [parsed.q]);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      setError(null);
      setForbidden(false);
      try {
        const page = await fetchAdminPayments(token, {
          limit: parsed.pageSize,
          offset: (parsed.page - 1) * parsed.pageSize,
          q: parsed.q,
          status: parsed.status,
        });
        if (!cancelled) {
          setRows(page.data);
          setTotal(page.total);
        }
      } catch (e) {
        if (cancelled) return;
        if (e instanceof ApiError) {
          if (e.status === 401) {
            logout();
            navigate(loginWithNext('/admin/payments'), { replace: true });
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
  }, [token, logout, navigate, parsed]);

  const totalPages = Math.max(1, Math.ceil(total / parsed.pageSize));
  const paidCount = rows.filter((r) => r.status === 'paid').length;
  const pendingCount = rows.filter((r) => r.status === 'pending').length;
  const paidAmount = rows
    .filter((r) => r.status === 'paid')
    .reduce((sum, r) => sum + (r.amount_usd || 0), 0);

  function applyFilters(next: { q?: string; status?: StatusFilter; page?: number }) {
    const qs = buildPaymentsSearch({
      q: next.q ?? parsed.q,
      status: next.status ?? parsed.status,
      page: next.page ?? 1,
    });
    setParams(qs ? new URLSearchParams(qs.slice(1)) : new URLSearchParams(), { replace: false });
  }

  function onSearchSubmit(e: FormEvent) {
    e.preventDefault();
    applyFilters({ q: qInput, page: 1 });
  }

  return (
    <section className="admin-page">
      <div className="admin-page-head">
        <h1>支付记录</h1>
        <p className="admin-muted">
          大会员订单与链上支付流水 · 支持按订单号 / 用户邮箱搜索与状态筛选
        </p>
      </div>

      <div className="admin-pay-summary" aria-label="本页摘要">
        <div className="admin-pay-stat">
          <span className="admin-muted">本页订单</span>
          <strong>{rows.length}</strong>
        </div>
        <div className="admin-pay-stat">
          <span className="admin-muted">已支付</span>
          <strong>{paidCount}</strong>
        </div>
        <div className="admin-pay-stat">
          <span className="admin-muted">待支付</span>
          <strong>{pendingCount}</strong>
        </div>
        <div className="admin-pay-stat">
          <span className="admin-muted">本页已付金额</span>
          <strong>${paidAmount.toFixed(2)}</strong>
        </div>
        <div className="admin-pay-stat">
          <span className="admin-muted">全部匹配</span>
          <strong>{total}</strong>
        </div>
      </div>

      <form className="admin-filters admin-panel" onSubmit={onSearchSubmit}>
        <label className="admin-filter-field admin-filter-grow">
          <span>搜索</span>
          <input
            value={qInput}
            onChange={(e) => setQInput(e.target.value)}
            placeholder="订单号 / 邮箱 / 昵称"
          />
        </label>
        <label className="admin-filter-field">
          <span>状态</span>
          <select
            value={parsed.status}
            onChange={(e) =>
              applyFilters({ status: e.target.value as StatusFilter, page: 1 })
            }
          >
            <option value="all">全部</option>
            <option value="paid">已支付</option>
            <option value="pending">待支付</option>
            <option value="expired">已过期</option>
            <option value="cancelled">已取消</option>
          </select>
        </label>
        <button type="submit" className="admin-btn-primary">
          搜索
        </button>
      </form>

      {loading ? <p className="admin-muted">加载中…</p> : null}
      {forbidden ? (
        <div className="admin-alert is-danger" role="alert">
          无权访问支付记录（403）。
        </div>
      ) : null}
      {!loading && !forbidden && error ? (
        <div className="admin-alert is-danger" role="alert">
          {error}
        </div>
      ) : null}
      {!loading && !forbidden && !error && rows.length === 0 ? (
        <p className="admin-empty">暂无支付记录。</p>
      ) : null}

      {!loading && !forbidden && !error && rows.length > 0 ? (
        <>
          <div className="admin-table-wrap">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>订单号</th>
                  <th>用户</th>
                  <th>套餐</th>
                  <th>金额</th>
                  <th>支付状态</th>
                  <th>开通结果</th>
                  <th>网关 / 网络</th>
                  <th>交易号</th>
                  <th>支付时间</th>
                  <th>开通至</th>
                  <th>创建时间</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.id}>
                    <td>{row.id}</td>
                    <td>
                      <span className="admin-mono">{row.order_no}</span>
                    </td>
                    <td>
                      <Link className="admin-title-link" to={`/admin/users/${row.user_id}`}>
                        #{row.user_id} {row.user_email || row.user_nickname || '—'}
                      </Link>
                    </td>
                    <td>
                      {row.plan_name || row.plan_slug}
                      <div className="admin-muted">{row.duration_days} 天</div>
                    </td>
                    <td>{formatMoney(row.amount_usd, row.amount_usdt)}</td>
                    <td>
                      <span className={statusClass(row.status)}>{statusLabel(row.status)}</span>
                    </td>
                    <td>
                      <span className={deliveryClass(row.delivery_status)}>
                        {deliveryLabel(row.delivery_status)}
                      </span>
                    </td>
                    <td>
                      <span className="admin-mono">
                        {row.gateway_provider || '—'}
                        {row.gateway_network ? ` / ${row.gateway_network}` : ''}
                      </span>
                    </td>
                    <td>
                      <span className="admin-mono">
                        {row.block_txid || row.gateway_trade_id || '—'}
                      </span>
                    </td>
                    <td>{formatTime(row.paid_at)}</td>
                    <td>{formatTime(row.membership_granted_until)}</td>
                    <td>{formatTime(row.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="admin-card-list" aria-label="支付记录（移动端）">
            {rows.map((row) => (
              <div key={row.id} className="admin-card-item">
                <div>
                  <strong>
                    #{row.id} · {row.order_no}
                  </strong>
                  <p>
                    {row.user_email || `用户 #${row.user_id}`} · {row.plan_name || row.plan_slug} ·{' '}
                    {formatMoney(row.amount_usd, row.amount_usdt)}
                  </p>
                  <p>
                    <span className={statusClass(row.status)}>{statusLabel(row.status)}</span>
                    {' · '}
                    <span className={deliveryClass(row.delivery_status)}>
                      {deliveryLabel(row.delivery_status)}
                    </span>
                    <span className="admin-muted"> · {formatTime(row.created_at)}</span>
                  </p>
                </div>
              </div>
            ))}
          </div>

          <div className="admin-pager">
            <span>
              共 {total} 条 · 第 {parsed.page}/{totalPages} 页
            </span>
            <div className="admin-pager-actions">
              <button
                type="button"
                className="admin-btn-ghost"
                disabled={parsed.page <= 1}
                onClick={() => applyFilters({ page: parsed.page - 1 })}
              >
                上一页
              </button>
              <button
                type="button"
                className="admin-btn-ghost"
                disabled={parsed.page >= totalPages}
                onClick={() => applyFilters({ page: parsed.page + 1 })}
              >
                下一页
              </button>
            </div>
          </div>
        </>
      ) : null}
    </section>
  );
}
