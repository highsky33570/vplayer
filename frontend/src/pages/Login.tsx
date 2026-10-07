import { Link } from 'react-router-dom';
import { AuthForm } from '../components/AuthForm';
import { useAuth } from '../auth/AuthContext';

export function LoginPage() {
  const { user } = useAuth();

  if (user) {
    return (
      <div className="page auth-page">
        <div className="auth-page-card">
          <h1>已登录</h1>
          <p>
            当前账号：{user.nickname || user.email}
          </p>
          <Link to="/" className="auth-btn-primary auth-link-btn">
            返回首页
          </Link>
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
