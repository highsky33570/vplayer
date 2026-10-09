import { useEffect } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { AuthForm } from '../components/AuthForm';
import { useAuth } from '../auth/AuthContext';
import { resolvePostLoginPath } from '../auth/paths';

export function LoginPage() {
  const { user, ready } = useAuth();
  const [params] = useSearchParams();
  const navigate = useNavigate();

  useEffect(() => {
    if (!ready || !user) return;
    navigate(resolvePostLoginPath(params.get('next')), { replace: true });
  }, [ready, user, params, navigate]);

  if (!ready) {
    return (
      <div className="page auth-page">
        <div className="auth-page-card">
          <p>加载中…</p>
        </div>
      </div>
    );
  }

  if (user) {
    return (
      <div className="page auth-page">
        <div className="auth-page-card">
          <h1>登录成功</h1>
          <p>正在跳转…</p>
        </div>
      </div>
    );
  }

  return (
    <div className="page auth-page">
      <div className="auth-page-card">
        <div className="auth-modal-brand">
          <span className="brand-mark">V</span>
          <div>
            <h2>登录 VPlayer</h2>
            <p>高清点播 · 畅享精彩影视</p>
          </div>
        </div>
        <AuthForm />
        <p className="auth-page-foot">
          <Link to="/">返回首页</Link>
        </p>
      </div>
    </div>
  );
}
