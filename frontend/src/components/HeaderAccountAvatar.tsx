import type { AuthUser } from '../api';
import { UserAvatar } from '../account/UserAvatar';

type Props = {
  user: AuthUser;
  active: boolean;
  flyoutOpen: boolean;
  onNavigate: () => void;
};

/**
 * Logged-in avatar trigger: hover preview is owned by HeaderFlyout;
 * click opens /account in a new browser tab (current tab unchanged).
 */
export function HeaderAccountAvatar({ user, active, flyoutOpen, onNavigate }: Props) {
  return (
    <a
      href="/account"
      target="_blank"
      rel="noopener noreferrer"
      className={`banner-user is-logged${active ? ' is-route-active' : ''}`}
      title={(user.nickname ?? '').trim() || '已登录'}
      aria-label="个人空间（新标签页）"
      aria-haspopup="dialog"
      aria-expanded={flyoutOpen}
      aria-current={active ? 'page' : undefined}
      data-testid="header-account-avatar"
      onClick={() => {
        onNavigate();
      }}
    >
      <UserAvatar user={user} />
    </a>
  );
}
