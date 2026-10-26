import { useState } from 'react';
import { useLocation } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';

const SESSION_KEY = 'vplayer_login_reminder_dismissed';

/** Inline homepage note. Not a floating card. */
export function LoginReminder() {
  const { pathname } = useLocation();
  const { user, ready, openLogin, authRequired } = useAuth();
  const [dismissed, setDismissed] = useState(
    () => typeof sessionStorage !== 'undefined' && sessionStorage.getItem(SESSION_KEY) === '1',
  );

  if (pathname !== '/' || !authRequired || !ready || user || dismissed) return null;

  return (
    <p className="login-reminder" role="note">
      <span>登录后可使用收藏、历史和消息。</span>
      <button type="button" className="login-reminder-cta" onClick={() => openLogin('login')}>
        登录
      </button>
      <button
        type="button"
        className="login-reminder-close"
        aria-label="关闭提示"
        onClick={() => {
          sessionStorage.setItem(SESSION_KEY, '1');
          setDismissed(true);
        }}
      >
        关闭
      </button>
    </p>
  );
}
