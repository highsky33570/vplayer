import { useState } from 'react';
import { useAuth } from '../auth/AuthContext';

const SESSION_KEY = 'vplayer_login_reminder_dismissed';

export function LoginReminder() {
  const { user, ready, openLogin } = useAuth();
  const [dismissed, setDismissed] = useState(
    () => typeof sessionStorage !== 'undefined' && sessionStorage.getItem(SESSION_KEY) === '1',
  );

  if (!ready || user || dismissed) return null;

  return (
    <aside className="login-reminder" aria-label="登录提示">
      <button
        type="button"
        className="login-reminder-close"
        aria-label="关闭提示"
        onClick={() => {
          sessionStorage.setItem(SESSION_KEY, '1');
          setDismissed(true);
        }}
      >
        ×
      </button>
      <div className="login-reminder-body">
        <span className="login-reminder-mark" aria-hidden>
          V
        </span>
        <div>
          <p className="login-reminder-title">登录 VPlayer，畅享高清影视！</p>
          <p className="login-reminder-sub">登录后可使用收藏、历史记录等功能</p>
        </div>
      </div>
      <button type="button" className="login-reminder-cta" onClick={() => openLogin('login')}>
        立即登录
      </button>
    </aside>
  );
}
