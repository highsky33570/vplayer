import { useState, type FormEvent } from 'react';
import { useAuth } from '../auth/AuthContext';

type Props = {
  /** Optional: close parent after success (AuthContext already closes modal). */
  onSuccess?: () => void;
};

export function AuthForm({ onSuccess }: Props) {
  const { authMode, setAuthMode, login, register } = useAuth();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [nickname, setNickname] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const isRegister = authMode === 'register';

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    const account = email.trim();
    if (!account) {
      setError('请输入账号或邮箱');
      return;
    }
    if (!password) {
      setError('请输入密码');
      return;
    }
    if (isRegister && password.length < 6) {
      setError('密码至少 6 位');
      return;
    }

    setSubmitting(true);
    setError('');
    try {
      if (isRegister) {
        await register(account, password, nickname.trim() || undefined);
      } else {
        await login(account, password);
      }
      onSuccess?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : '操作失败，请稍后重试');
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form className="auth-form" onSubmit={onSubmit} noValidate>
      <div className="auth-tabs" role="tablist" aria-label="登录方式">
        <button
          type="button"
          role="tab"
          aria-selected={!isRegister}
          className={!isRegister ? 'is-active' : undefined}
          onClick={() => {
            setAuthMode('login');
            setError('');
          }}
        >
          密码登录
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={isRegister}
          className={isRegister ? 'is-active' : undefined}
          onClick={() => {
            setAuthMode('register');
            setError('');
          }}
        >
          注册账号
        </button>
      </div>

      {isRegister && (
        <label className="auth-field">
          <span>昵称</span>
          <input
            value={nickname}
            onChange={(e) => setNickname(e.target.value)}
            placeholder="可选，展示名称"
            autoComplete="nickname"
            disabled={submitting}
          />
        </label>
      )}

      <label className="auth-field">
        <span>账号 / 邮箱</span>
        <input
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          placeholder="请输入账号或邮箱"
          autoComplete="username"
          disabled={submitting}
        />
      </label>

      <label className="auth-field">
        <span>密码</span>
        <div className="auth-password">
          <input
            type={showPassword ? 'text' : 'password'}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder={isRegister ? '至少 6 位密码' : '请输入密码'}
            autoComplete={isRegister ? 'new-password' : 'current-password'}
            disabled={submitting}
          />
          <button
            type="button"
            className="auth-eye"
            aria-label={showPassword ? '隐藏密码' : '显示密码'}
            onClick={() => setShowPassword((v) => !v)}
          >
            {showPassword ? <EyeOffIcon /> : <EyeIcon />}
          </button>
        </div>
      </label>

      {error && (
        <p className="auth-error" role="alert">
          {error}
        </p>
      )}

      <div className="auth-actions">
        {isRegister ? (
          <>
            <button
              type="button"
              className="auth-btn-secondary"
              disabled={submitting}
              onClick={() => {
                setAuthMode('login');
                setError('');
              }}
            >
              返回登录
            </button>
            <button type="submit" className="auth-btn-primary" disabled={submitting}>
              {submitting ? '注册中...' : '注册'}
            </button>
          </>
        ) : (
          <>
            <button
              type="button"
              className="auth-btn-secondary"
              disabled={submitting}
              onClick={() => {
                setAuthMode('register');
                setError('');
              }}
            >
              注册
            </button>
            <button type="submit" className="auth-btn-primary" disabled={submitting}>
              {submitting ? '登录中...' : '登录'}
            </button>
          </>
        )}
      </div>
    </form>
  );
}

function EyeIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"
        stroke="currentColor"
        strokeWidth="1.8"
      />
      <circle cx="12" cy="12" r="3" stroke="currentColor" strokeWidth="1.8" />
    </svg>
  );
}

function EyeOffIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path
        d="M3 3l18 18M10.5 10.7a2.5 2.5 0 0 0 3.5 3.5M6.2 6.5C4 8.1 2.7 10.3 2 12c0 0 3.5 7 10 7 1.8 0 3.4-.4 4.7-1M9.9 5.2A10.5 10.5 0 0 1 12 5c6.5 0 10 7 10 7a18.4 18.4 0 0 1-2.2 3.2"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
      />
    </svg>
  );
}
