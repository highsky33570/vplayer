import { useEffect, useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ApiError,
  deleteAdminUser,
  fetchAdminMembershipPlans,
  fetchAdminUser,
  setAdminUserMembership,
  updateAdminUser,
  type AdminMembershipPlan,
  type AdminUser,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import { roleLabel } from './adminLabels';

function formatTime(value?: string | null) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

function toDatetimeLocal(value?: string | null) {
  if (!value) return '';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function AdminUserDetail() {
  const { id: idParam } = useParams();
  const id = Number(idParam);
  const { token, logout, user: me } = useAuth();
  const navigate = useNavigate();
  const [data, setData] = useState<AdminUser | null>(null);
  const [plans, setPlans] = useState<AdminMembershipPlan[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [okMsg, setOkMsg] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);

  const [nickname, setNickname] = useState('');
  const [isAdmin, setIsAdmin] = useState(false);
  const [isMember, setIsMember] = useState(false);
  const [expiresLocal, setExpiresLocal] = useState('');
  const [planSlug, setPlanSlug] = useState('');

  useEffect(() => {
    let cancelled = false;
    void fetchAdminMembershipPlans(token)
      .then((rows) => {
        if (!cancelled) setPlans(rows.filter((p) => p.is_active));
      })
      .catch(() => {
        if (!cancelled) setPlans([]);
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      if (!Number.isFinite(id) || id <= 0) {
        setError('无效的用户 ID');
        setLoading(false);
        return;
      }
      setLoading(true);
      setError(null);
      setForbidden(false);
      try {
        const next = await fetchAdminUser(id, token);
        if (cancelled) return;
        setData(next);
        setNickname(next.nickname || '');
        setIsAdmin(Boolean(next.is_admin));
        setIsMember(Boolean(next.is_member || next.is_admin));
        setExpiresLocal(toDatetimeLocal(next.membership_expires_at));
        setPlanSlug(next.latest_plan_slug || '');
      } catch (e) {
        if (cancelled) return;
        if (e instanceof ApiError) {
          if (e.status === 401) {
            logout();
            navigate(loginWithNext(`/admin/users/${id}`), { replace: true });
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

  async function onSaveProfile(e: FormEvent) {
    e.preventDefault();
    if (!data) return;
    setSaving(true);
    setError(null);
    setOkMsg(null);
    try {
      const updated = await updateAdminUser(token, data.id, {
        nickname: nickname.trim(),
        is_admin: isAdmin,
        is_member: isMember,
        membership_expires_at: isMember
          ? expiresLocal
            ? new Date(expiresLocal).toISOString()
            : null
          : null,
      });
      setData(updated);
      setNickname(updated.nickname || '');
      setIsAdmin(Boolean(updated.is_admin));
      setIsMember(Boolean(updated.is_member || updated.is_admin));
      setExpiresLocal(toDatetimeLocal(updated.membership_expires_at));
      setOkMsg('用户资料已保存');
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 401) {
          logout();
          navigate(loginWithNext(`/admin/users/${id}`), { replace: true });
          return;
        }
        setError(err.message);
      } else {
        setError(err instanceof Error ? err.message : '保存失败');
      }
    } finally {
      setSaving(false);
    }
  }

  async function onGrantPlan() {
    if (!data || !planSlug) return;
    setSaving(true);
    setError(null);
    setOkMsg(null);
    try {
      const updated = await setAdminUserMembership(token, data.id, true, {
        plan_slug: planSlug,
      });
      setData(updated);
      setIsMember(Boolean(updated.is_member));
      setExpiresLocal(toDatetimeLocal(updated.membership_expires_at));
      setOkMsg(`已按套餐 ${planSlug} 开通会员`);
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 401) {
          logout();
          navigate(loginWithNext(`/admin/users/${id}`), { replace: true });
          return;
        }
        setError(err.message);
      } else {
        setError(err instanceof Error ? err.message : '开通失败');
      }
    } finally {
      setSaving(false);
    }
  }

  async function onDelete() {
    if (!data) return;
    if (me?.id === data.id) {
      setError('不能删除当前登录账号');
      return;
    }
    if (!window.confirm(`确认删除用户 ${data.email}？此操作不可恢复。`)) return;
    setSaving(true);
    setError(null);
    try {
      await deleteAdminUser(token, data.id);
      navigate('/admin/users', { replace: true });
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 401) {
          logout();
          navigate(loginWithNext(`/admin/users/${id}`), { replace: true });
          return;
        }
        setError(err.message);
      } else {
        setError(err instanceof Error ? err.message : '删除失败');
      }
      setSaving(false);
    }
  }

  const vipLabel = data?.is_admin
    ? '管理员（默认可用）'
    : data?.is_member
      ? data.membership_expires_at
        ? `会员至 ${formatTime(data.membership_expires_at)}`
        : '终身会员'
      : '非会员';

  return (
    <section className="admin-page">
      <div className="admin-page-head">
        <p className="admin-breadcrumb">
          <Link to="/admin/users">用户管理</Link> / 详情
        </p>
        <h1>{data?.email ?? '用户详情'}</h1>
        <p className="admin-muted">可编辑昵称、角色与会员；可删除账号（不含密码与会话密钥）</p>
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
      {okMsg ? (
        <div className="admin-alert is-ok" role="status">
          {okMsg}
        </div>
      ) : null}

      {!loading && !forbidden && data ? (
        <>
          <div className="admin-stat-grid">
            <article className="admin-stat-card">
              <span>角色</span>
              <strong style={{ fontSize: '1.15rem' }}>{roleLabel(data.is_admin)}</strong>
            </article>
            <article className="admin-stat-card">
              <span>会员状态</span>
              <strong style={{ fontSize: '1.05rem' }}>{vipLabel}</strong>
            </article>
            <article className="admin-stat-card">
              <span>最近购买套餐</span>
              <strong style={{ fontSize: '1.05rem' }}>
                {data.latest_plan_name || data.latest_plan_slug || '—'}
              </strong>
            </article>
            <article className="admin-stat-card">
              <span>注册时间</span>
              <strong style={{ fontSize: '1.05rem' }}>{formatTime(data.created_at)}</strong>
            </article>
          </div>

          <form className="admin-panel" onSubmit={onSaveProfile}>
            <div className="admin-panel-head">
              <h2>编辑资料</h2>
            </div>
            <div className="admin-form-grid">
              <label className="admin-filter-field">
                <span>ID</span>
                <input className="admin-input" value={data.id} disabled />
              </label>
              <label className="admin-filter-field">
                <span>邮箱</span>
                <input className="admin-input" value={data.email} disabled />
              </label>
              <label className="admin-filter-field">
                <span>昵称</span>
                <input
                  className="admin-input"
                  value={nickname}
                  onChange={(e) => setNickname(e.target.value)}
                  required
                />
              </label>
              <label className="admin-filter-field">
                <span>管理员</span>
                <select
                  className="admin-input"
                  value={isAdmin ? '1' : '0'}
                  onChange={(e) => setIsAdmin(e.target.value === '1')}
                  disabled={me?.id === data.id}
                >
                  <option value="0">否</option>
                  <option value="1">是</option>
                </select>
              </label>
              <label className="admin-filter-field">
                <span>大会员</span>
                <select
                  className="admin-input"
                  value={isMember ? '1' : '0'}
                  onChange={(e) => setIsMember(e.target.value === '1')}
                  disabled={isAdmin}
                >
                  <option value="0">否</option>
                  <option value="1">是</option>
                </select>
              </label>
              <label className="admin-filter-field">
                <span>会员到期（空=终身）</span>
                <input
                  className="admin-input"
                  type="datetime-local"
                  value={expiresLocal}
                  onChange={(e) => setExpiresLocal(e.target.value)}
                  disabled={!isMember || isAdmin}
                />
              </label>
            </div>
            <div className="admin-form-actions">
              <button type="submit" className="admin-btn-primary" disabled={saving}>
                {saving ? '保存中…' : '保存资料'}
              </button>
              <button
                type="button"
                className="admin-btn-danger"
                disabled={saving || me?.id === data.id}
                onClick={() => void onDelete()}
              >
                删除用户
              </button>
            </div>
          </form>

          <article className="admin-panel">
            <div className="admin-panel-head">
              <h2>按套餐开通会员</h2>
            </div>
            <div className="admin-filters" style={{ marginBottom: 0 }}>
              <label className="admin-filter-field">
                <span>套餐</span>
                <select
                  className="admin-input"
                  value={planSlug}
                  onChange={(e) => setPlanSlug(e.target.value)}
                  disabled={isAdmin}
                >
                  <option value="">选择套餐</option>
                  {plans.map((p) => (
                    <option key={p.id} value={p.slug}>
                      {p.name} · ${p.price_usd} · {p.duration_days} 天
                    </option>
                  ))}
                </select>
              </label>
              <button
                type="button"
                className="admin-btn-primary"
                disabled={saving || !planSlug || isAdmin}
                onClick={() => void onGrantPlan()}
              >
                开通 / 续期
              </button>
            </div>
            <p className="admin-muted" style={{ marginTop: 12 }}>
              将按所选套餐时长延长会员到期时间。最近购买记录：
              {data.latest_plan_name || data.latest_plan_slug || '暂无'}
            </p>
          </article>
        </>
      ) : null}
    </section>
  );
}
