import { useEffect, useState, type FormEvent } from 'react';
import {
  passwordForgot as apiPasswordForgot,
  passwordReset as apiPasswordReset,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import {
  authFormSubmitPlan,
  canSubmitRegistration,
  registrationBlockMessage,
  registerSubmitAction,
  showsVerificationStep,
  stateAfterRegisterStart,
} from '../auth/registerFlow';

type Props = {
  /** Optional: close parent after success (AuthContext already closes modal). */
  onSuccess?: () => void;
  /** Notifies parent so modal title can follow forgot-password steps. */
  onFlowChange?: (flow: 'auth' | 'forgot') => void;
};

type RegisterStep = 'form' | 'verify';
type ForgotStep = 'email' | 'reset';

function maskEmail(email: string): string {
  const e = email.trim();
  const at = e.indexOf('@');
  if (at <= 1) return e;
  const name = e.slice(0, at);
  const domain = e.slice(at);
  if (name.length <= 2) return `${name[0]}*${domain}`;
  return `${name[0]}${'*'.repeat(Math.min(name.length - 2, 4))}${name[name.length - 1]}${domain}`;
}

export function AuthForm({ onSuccess, onFlowChange }: Props) {
  const {
    authMode,
    setAuthMode,
    login,
    registrationConfig,
    retryRegistrationConfig,
    register,
    registerStart,
    registerVerify,
    registerResend,
  } = useAuth();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [nickname, setNickname] = useState('');
  const [code, setCode] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState('');
  const [info, setInfo] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [registerStep, setRegisterStep] = useState<RegisterStep>('form');
  const [forgotMode, setForgotMode] = useState(false);
  const [forgotStep, setForgotStep] = useState<ForgotStep>('email');
  const [expiresInSec, setExpiresInSec] = useState(600);
  const [resendAfterSec, setResendAfterSec] = useState(60);
  const [cooldownLeft, setCooldownLeft] = useState(0);

  const isRegister = authMode === 'register';
  const showVerify = !forgotMode && isRegister && showsVerificationStep(registrationConfig, registerStep);
  const registrationNotice =
    !forgotMode && isRegister && !showVerify ? registrationBlockMessage(registrationConfig) : null;
  const registrationReady = canSubmitRegistration(registrationConfig);

  useEffect(() => {
    onFlowChange?.(forgotMode ? 'forgot' : 'auth');
  }, [forgotMode, onFlowChange]);

  useEffect(() => {
    if (cooldownLeft <= 0) return;
    const t = window.setTimeout(() => setCooldownLeft((v) => Math.max(0, v - 1)), 1000);
    return () => window.clearTimeout(t);
  }, [cooldownLeft]);

  useEffect(() => {
    if (!isRegister) {
      setRegisterStep('form');
      setCode('');
      setError('');
    }
  }, [isRegister]);

  function enterForgot() {
    setForgotMode(true);
    setForgotStep('email');
    setPassword('');
    setConfirmPassword('');
    setCode('');
    setError('');
    setInfo('');
    setCooldownLeft(0);
    setAuthMode('login');
  }

  function leaveForgot() {
    setForgotMode(false);
    setForgotStep('email');
    setPassword('');
    setConfirmPassword('');
    setCode('');
    setError('');
    setInfo('');
    setCooldownLeft(0);
  }

  async function onForgotSend() {
    const account = email.trim();
    if (!account) {
      setError('请输入账号或邮箱');
      return;
    }
    setSubmitting(true);
    setError('');
    setInfo('');
    try {
      const meta = await apiPasswordForgot(account);
      setExpiresInSec(meta.expires_in_sec || 600);
      setResendAfterSec(meta.resend_after_sec || 60);
      setCooldownLeft(meta.resend_after_sec || 60);
      setForgotStep('reset');
      setInfo('若该邮箱已注册，验证码已发送，请查收邮件');
    } catch (err) {
      setError(err instanceof Error ? err.message : '发送失败，请稍后重试');
    } finally {
      setSubmitting(false);
    }
  }

  async function onForgotReset() {
    const account = email.trim();
    const trimmed = code.trim();
    if (!/^\d{6}$/.test(trimmed)) {
      setError('请输入 6 位数字验证码');
      return;
    }
    if (!password) {
      setError('请输入新密码');
      return;
    }
    if (password.length < 6) {
      setError('密码至少 6 位');
      return;
    }
    if (password !== confirmPassword) {
      setError('两次输入的密码不一致');
      return;
    }
    setSubmitting(true);
    setError('');
    setInfo('');
    try {
      await apiPasswordReset(account, trimmed, password);
      await login(account, password);
      onSuccess?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : '重置失败，请稍后重试');
    } finally {
      setSubmitting(false);
    }
  }

  async function onForgotResend() {
    const account = email.trim();
    if (!account) return;
    setSubmitting(true);
    setError('');
    try {
      const meta = await apiPasswordForgot(account);
      setExpiresInSec(meta.expires_in_sec || 600);
      setResendAfterSec(meta.resend_after_sec || 60);
      setCooldownLeft(meta.resend_after_sec || 60);
      setInfo('验证码已重新发送，请查收邮件');
    } catch (err) {
      setError(err instanceof Error ? err.message : '发送失败，请稍后重试');
    } finally {
      setSubmitting(false);
    }
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();

    if (forgotMode) {
      if (forgotStep === 'email') {
        await onForgotSend();
      } else {
        await onForgotReset();
      }
      return;
    }

    const account = email.trim();
    if (!account) {
      setError('请输入账号或邮箱');
      return;
    }

    if (showVerify) {
      const trimmed = code.trim();
      if (!/^\d{6}$/.test(trimmed)) {
        setError('请输入 6 位数字验证码');
        return;
      }
      setSubmitting(true);
      setError('');
      try {
        await registerVerify(account, trimmed);
        onSuccess?.();
      } catch (err) {
        setError(err instanceof Error ? err.message : '验证失败，请稍后重试');
      } finally {
        setSubmitting(false);
      }
      return;
    }

    if (isRegister && authFormSubmitPlan('register', registrationConfig) === 'blocked') {
      const blocked = registrationBlockMessage(registrationConfig);
      if (blocked) setError(blocked);
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
      if (isRegister && registerSubmitAction(registrationConfig) === 'start') {
        const meta = await registerStart(account, password, nickname.trim() || undefined);
        const next = stateAfterRegisterStart();
        setPassword(next.password);
        setRegisterStep(next.registerStep);
        setExpiresInSec(meta.expires_in_sec || 600);
        setResendAfterSec(meta.resend_after_sec || 60);
        setCooldownLeft(meta.resend_after_sec || 60);
      } else if (isRegister) {
        await register(account, password, nickname.trim() || undefined);
        onSuccess?.();
      } else {
        await login(account, password);
        onSuccess?.();
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '操作失败，请稍后重试');
    } finally {
      setSubmitting(false);
    }
  }

  async function onResend() {
    const account = email.trim();
    if (!account) return;
    setSubmitting(true);
    setError('');
    try {
      const meta = await registerResend(account);
      setExpiresInSec(meta.expires_in_sec || 600);
      setResendAfterSec(meta.resend_after_sec || 60);
      setCooldownLeft(meta.resend_after_sec || 60);
    } catch (err) {
      setError(err instanceof Error ? err.message : '发送失败，请稍后重试');
    } finally {
      setSubmitting(false);
    }
  }

  function backToRegisterForm() {
    setRegisterStep('form');
    setCode('');
    setError('');
    setCooldownLeft(0);
  }

  return (
    <form className="auth-form" onSubmit={(e) => void onSubmit(e)} noValidate>
      {!forgotMode && !showVerify && (
        <div className="auth-tabs" role="tablist" aria-label="登录方式">
          <button
            type="button"
            role="tab"
            aria-selected={!isRegister}
            className={!isRegister ? 'is-active' : undefined}
            onClick={() => {
              setAuthMode('login');
              setError('');
              setRegisterStep('form');
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
      )}

      {forgotMode ? (
        <>
          {forgotStep === 'reset' ? (
            <p className="auth-verify-hint">
              验证码已发送至 <strong>{maskEmail(email)}</strong>
              <br />
              请在 {Math.max(1, Math.round(expiresInSec / 60))} 分钟内完成验证并设置新密码
            </p>
          ) : (
            <p className="auth-verify-hint">输入注册邮箱，我们将发送找回密码验证码。</p>
          )}

          <label className="auth-field">
            <span>账号 / 邮箱</span>
            <input
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="请输入账号或邮箱"
              autoComplete="username"
              disabled={submitting || forgotStep === 'reset'}
            />
          </label>

          {forgotStep === 'reset' ? (
            <>
              <label className="auth-field">
                <span>邮箱验证码</span>
                <input
                  value={code}
                  onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                  placeholder="6 位数字验证码"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  disabled={submitting}
                  autoFocus
                />
              </label>
              <label className="auth-field">
                <span>新密码</span>
                <div className="auth-password">
                  <input
                    type={showPassword ? 'text' : 'password'}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    placeholder="至少 6 位新密码"
                    autoComplete="new-password"
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
              <label className="auth-field">
                <span>确认新密码</span>
                <input
                  type={showPassword ? 'text' : 'password'}
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  placeholder="再次输入新密码"
                  autoComplete="new-password"
                  disabled={submitting}
                />
              </label>
            </>
          ) : null}
        </>
      ) : showVerify ? (
        <>
          <p className="auth-verify-hint">
            验证码已发送至 <strong>{maskEmail(email)}</strong>
            <br />
            请在 {Math.max(1, Math.round(expiresInSec / 60))} 分钟内完成验证
          </p>
          <label className="auth-field">
            <span>邮箱验证码</span>
            <input
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
              placeholder="6 位数字验证码"
              inputMode="numeric"
              autoComplete="one-time-code"
              disabled={submitting}
              autoFocus
            />
          </label>
        </>
      ) : (
        <>
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
            <div className="auth-field-label-row">
              <span>密码</span>
              {!isRegister ? (
                <button type="button" className="auth-forgot-link" onClick={enterForgot}>
                  忘记密码？
                </button>
              ) : null}
            </div>
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
        </>
      )}

      {(registrationNotice || error || info) && (
        <p className={error || registrationNotice ? 'auth-error' : 'auth-info'} role="alert">
          {registrationNotice ?? error ?? info}
          {registrationNotice && (
            <>
              {' '}
              <button type="button" className="auth-link-btn" onClick={retryRegistrationConfig}>
                重试
              </button>
            </>
          )}
        </p>
      )}

      <div className="auth-actions">
        {forgotMode ? (
          <>
            <button type="button" className="auth-btn-secondary" disabled={submitting} onClick={leaveForgot}>
              返回登录
            </button>
            <button type="submit" className="auth-btn-primary" disabled={submitting}>
              {submitting
                ? forgotStep === 'email'
                  ? '发送中...'
                  : '重置中...'
                : forgotStep === 'email'
                  ? '发送验证码'
                  : '重置密码'}
            </button>
          </>
        ) : showVerify ? (
          <>
            <button type="button" className="auth-btn-secondary" disabled={submitting} onClick={backToRegisterForm}>
              返回修改
            </button>
            <button type="submit" className="auth-btn-primary" disabled={submitting}>
              {submitting ? '验证中...' : '验证并注册'}
            </button>
          </>
        ) : isRegister ? (
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
            <button type="submit" className="auth-btn-primary" disabled={submitting || !registrationReady}>
              {submitting ? (registrationConfig === 'enabled' ? '发送中...' : '注册中...') : '注册'}
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

      {(showVerify || (forgotMode && forgotStep === 'reset')) && (
        <div className="auth-resend-row">
          <button
            type="button"
            className="auth-link-btn auth-resend-btn"
            disabled={submitting || cooldownLeft > 0}
            onClick={() => void (forgotMode ? onForgotResend() : onResend())}
          >
            {cooldownLeft > 0 ? `重新发送（${cooldownLeft}s）` : '重新发送验证码'}
          </button>
          {cooldownLeft === 0 && resendAfterSec > 0 ? null : null}
        </div>
      )}
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
