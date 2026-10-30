import { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import logoUrl from '../assets/logo.png';
import { useAuth } from '../auth/AuthContext';
import { AuthForm } from './AuthForm';

export function LoginModal() {
  const { loginOpen, closeLogin, authMode, authRequired } = useAuth();
  const [flow, setFlow] = useState<'auth' | 'forgot'>('auth');

  useEffect(() => {
    if (!loginOpen) setFlow('auth');
  }, [loginOpen]);

  useEffect(() => {
    if (!loginOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') closeLogin();
    };
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    window.addEventListener('keydown', onKey);
    return () => {
      document.body.style.overflow = prev;
      window.removeEventListener('keydown', onKey);
    };
  }, [loginOpen, closeLogin]);

  if (!authRequired || !loginOpen) return null;

  const title =
    flow === 'forgot' ? '找回密码' : authMode === 'register' ? '注册 天涯影视' : '登录 天涯影视';

  return createPortal(
    <div className="auth-overlay" role="presentation" onMouseDown={closeLogin}>
      <div
        className="auth-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="auth-modal-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <button type="button" className="auth-close" aria-label="关闭" onClick={closeLogin}>
          ×
        </button>

        <div className="auth-modal-brand">
          <img className="auth-modal-logo" src={logoUrl} alt="天涯影视" width={140} height={36} decoding="async" />
          <div>
            <h2 id="auth-modal-title">{title}</h2>
            <p>{flow === 'forgot' ? '通过邮箱验证码重置密码' : '高清点播 · 畅享精彩影视'}</p>
          </div>
        </div>

        <AuthForm onFlowChange={setFlow} />
      </div>
    </div>,
    document.body,
  );
}
