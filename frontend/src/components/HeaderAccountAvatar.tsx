import { Link } from 'react-router-dom';
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
 * click is a real React Router <Link> to /account (updates the address bar).
 */
export function HeaderAccountAvatar({ user, active, flyoutOpen, onNavigate }: Props) {
  return (
    <Link
      to="/account"
      className={`banner-user is-logged${active ? ' is-route-active' : ''}`}
      title={(user.nickname ?? '').trim() || '已登录'}
      aria-label="个人空间"
      aria-haspopup="dialog"
      aria-expanded={flyoutOpen}
      aria-current={active ? 'page' : undefined}
      data-testid="header-account-avatar"
      onClick={() => {
        onNavigate();
      }}
    >
      <UserAvatar user={user} />
    </Link>
  );
}
