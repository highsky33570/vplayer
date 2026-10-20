import { useEffect, type ReactNode } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import type { AuthUser } from '../api';
import { useAuth } from './AuthContext';
import { loginWithNext } from './paths';

type Props = {
  /** Render-prop receives the authenticated user and token. */
  children: (auth: { user: AuthUser; token: string }) => ReactNode;
};

/**
 * Renders children only for a signed-in user. Once auth is ready and there is no
 * user, sends the visitor to /login?next=<current path> (same as the Login page flow).
 */
export function AuthGate({ children }: Props) {
  const { user, token, ready } = useAuth();
  const { pathname, search } = useLocation();
  const navigate = useNavigate();
  const here = `${pathname}${search}`;

  useEffect(() => {
    if (ready && !user) navigate(loginWithNext(here), { replace: true });
  }, [ready, user, here, navigate]);

  if (!ready) {
    return (
      <div className="page me-page">
        <p className="me-status">正在验证登录状态…</p>
      </div>
    );
  }
  if (!user || !token) {
    return (
      <div className="page me-page">
        <p className="me-status">正在跳转登录…</p>
      </div>
    );
  }
  return <>{children({ user, token })}</>;
}
