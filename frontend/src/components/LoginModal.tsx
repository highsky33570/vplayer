import { useEffect } from 'react';
import { createPortal } from 'react-dom';
import { useAuth } from '../auth/AuthContext';
import { AuthForm } from './AuthForm';

export function LoginModal() {
  const { loginOpen, closeLogin, authMode } = useAuth();

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

  if (!loginOpen) return null;

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
          <span className="brand-mark">V</span>
          <div>
            <h2 id="auth-modal-title">{authMode === 'register' ? '注册 VPlayer' : '登录 VPlayer'}</h2>
            <p>高清点播 · 畅享精彩影视</p>
          </div>
        </div>

        <AuthForm />
      </div>
    </div>,
    document.body,
  );
}
