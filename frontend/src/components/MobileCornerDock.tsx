import type { MouseEvent } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { useTheme } from '../theme/ThemeContext';

/** Phone-only stack at the lower right: history, theme, messages, back to top. */
export function MobileCornerDock() {
  const { pathname } = useLocation();
  const { user, ready, authRequired, openLogin } = useAuth();
  const { theme, toggleTheme } = useTheme();

  if (pathname.startsWith('/watch')) return null;

  function guardPersonal(event: MouseEvent<HTMLAnchorElement>) {
    if (!authRequired) return;
    if (!ready || !user) {
      event.preventDefault();
      if (ready) openLogin('login');
    }
  }

  return (
    <div className="mobile-corner-dock" aria-label="快捷操作">
      <Link to="/history" className="mobile-corner-btn" aria-label="观看历史" onClick={guardPersonal}>
        <ListIcon />
      </Link>
      <button
        type="button"
        className="mobile-corner-btn"
        aria-label={theme === 'dark' ? '切换到浅色模式' : '切换到深色模式'}
        onClick={toggleTheme}
      >
        <GearIcon />
      </button>
      <Link to="/messages" className="mobile-corner-btn is-light" aria-label="消息" onClick={guardPersonal}>
        <ChatIcon />
      </Link>
      <button
        type="button"
        className="mobile-corner-btn is-top"
        aria-label="回到顶部"
        onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}
      >
        <UpIcon />
      </button>
    </div>
  );
}

function ListIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path d="M8 7h11M8 12h11M8 17h11" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
      <path d="M4.5 7h.01M4.5 12h.01M4.5 17h.01" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" />
    </svg>
  );
}

function GearIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <circle cx="12" cy="12" r="3" stroke="currentColor" strokeWidth="1.8" />
      <path
        d="M12 3.5v2.2M12 18.3v2.2M3.5 12h2.2M18.3 12h2.2M6 6l1.6 1.6M16.4 16.4 18 18M18 6l-1.6 1.6M7.6 16.4 6 18"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
      />
    </svg>
  );
}

function ChatIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path d="M5 6.5h14v9H8.5L5 18.5v-12z" stroke="currentColor" strokeWidth="1.8" strokeLinejoin="round" />
    </svg>
  );
}

function UpIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
      <path d="M6 14.5 12 8.5l6 6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
