import type { AuthUser } from '../api';

type Props = {
  user: AuthUser;
  active: boolean;
  flyoutOpen: boolean;
  onNavigate: () => void;
};

/**
 * Logged-in account trigger: hover preview is owned by HeaderFlyout;
 * click opens /account in a new browser tab (current tab unchanged).
 * The mark is an icon only — the nickname initial is not shown.
 */
export function HeaderAccountAvatar({ active, flyoutOpen, onNavigate }: Props) {
  return (
    <a
      href="/account"
      target="_blank"
      rel="noopener noreferrer"
      className={`banner-user is-logged${active ? ' is-route-active' : ''}`}
      title="个人空间"
      aria-label="个人空间（新标签页）"
      aria-haspopup="dialog"
      aria-expanded={flyoutOpen}
      aria-current={active ? 'page' : undefined}
      data-testid="header-account-avatar"
      onClick={() => {
        onNavigate();
      }}
    >
      <span className="banner-action-icon" aria-hidden>
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none">
          <circle cx="12" cy="8" r="3.2" stroke="currentColor" strokeWidth="1.8" />
          <path d="M5 19.2c1.4-3 3.8-4.4 7-4.4s5.6 1.4 7 4.4" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
        </svg>
      </span>
    </a>
  );
}
