import { useEffect, useState, type FormEvent } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { ApiError } from '../api';
import { useAuth } from '../auth/AuthContext';
import { resolveAdminPostLoginPath } from '../auth/paths';
import './admin.css';

/**
 * Standalone admin credential page — solid, minimal UI (not the public homepage login).
 * Unauthenticated /admin* visits redirect here; success goes straight into the admin shell.
 */
export function AdminLoginPage() {
  const { ready, user, token, login, logout } = useAuth();
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const nextTarget = resolveAdminPostLoginPath(params.get('next'));

  useEffect(() => {
    if (!ready || !user || !token) return;
    if (user.is_admin) {
      navigate(nextTarget, { replace: true });
    }
  }, [ready, user, token, nextTarget, navigate]);

  if (!ready) {
    return (
      <div className="admin-login-page">
        <div className="admin-login-card">
          <p className="admin-login-muted">加载中…</p>
        </div>
      </div>
    );
  }

  if (user && token && user.is_admin) {
    return (
      <div className="admin-login-page">
        <div className="admin-login-card">
          <p className="admin-login-muted">正在进入管理后台…</p>
        </div>
      </div>
    );
  }

  if (user && token && !user.is_admin) {
    return (
      <div className="admin-login-page">
        <div className="admin-login-card">
          <h1>无权访问</h1>
          <p className="admin-login-muted">当前账号没有管理后台权限。</p>
          <p className="admin-login-meta">{user.email}</p>
          <div className="admin-login-actions">
            <button
              type="button"
              className="admin-login-submit"
              onClick={() => {
                logout();
                setError('');
                setPassword('');
              }}
            >
              使用其他账号登录
            </button>
            <Link className="admin-login-link" to="/">
              返回站点首页
            </Link>
          </div>
        </div>
      </div>
    );
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (submitting) return;
    const em = email.trim();
    if (!em || !password) {
      setError('请输入邮箱和密码');
      return;
    }
    setSubmitting(true);
    setError('');
    try {
      await login(em, password);
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message || '登录失败');
      } else {
        setError(err instanceof Error ? err.message : '登录失败');
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="admin-login-page">
      <div className="admin-login-card">
        <p className="admin-login-kicker">管理后台</p>
        <h1>登录</h1>
        <p className="admin-login-muted">请使用管理员账号</p>
        <form className="admin-login-form" onSubmit={(e) => void onSubmit(e)}>
          <label>
            邮箱
            <input
              type="email"
              autoComplete="username"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              disabled={submitting}
              required
            />
          </label>
          <label>
            密码
            <div className="admin-login-password-row">
              <input
                type={showPassword ? 'text' : 'password'}
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                disabled={submitting}
                required
              />
              <button
                type="button"
                className="admin-login-eye"
                onClick={() => setShowPassword((v) => !v)}
                tabIndex={-1}
              >
                {showPassword ? '隐藏' : '显示'}
              </button>
            </div>
          </label>
          {error ? (
            <p className="admin-login-error" role="alert">
              {error}
            </p>
          ) : null}
          <button type="submit" className="admin-login-submit" disabled={submitting}>
            {submitting ? '登录中…' : '登录'}
          </button>
        </form>
      </div>
    </div>
  );
}
