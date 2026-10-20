import { useEffect, useState, type FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { fetchMeSummary, type MeSummary } from '../api';
import { nicknameValidationMessage, roleLabel } from '../account/accountLogic';
import { AVATAR_HUE_CHOICES, avatarHue, avatarStyle } from '../account/avatar';
import { UserAvatar } from '../account/UserAvatar';
import { useAuth } from '../auth/AuthContext';
import { AuthGate } from '../auth/AuthGate';
import { unreadBadgeLabel } from '../components/headerFlyoutData';
import { useTheme } from '../theme/ThemeContext';

export function AccountPage() {
  return <AuthGate>{() => <AccountContent />}</AuthGate>;
}

function AccountContent() {
  const { user, token, logout, updateProfile } = useAuth();
  const { theme, toggleTheme } = useTheme();
  const [summary, setSummary] = useState<MeSummary | null>(null);
  const [nickname, setNickname] = useState(user?.nickname ?? '');
  const [nickSource, setNickSource] = useState(user?.nickname ?? '');
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);

  // Keep the field in sync if the profile is refreshed elsewhere.
  if ((user?.nickname ?? '') !== nickSource) {
    setNickSource(user?.nickname ?? '');
    setNickname(user?.nickname ?? '');
  }

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    fetchMeSummary(token)
      .then((s) => {
        if (!cancelled) setSummary(s);
      })
      .catch(() => {
        if (!cancelled) setSummary(null);
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  if (!user) return null;
  const currentHue = user.avatar_hue ?? -1;
  const dirty = nickname.trim() !== (user.nickname ?? '').trim();

  async function save(patch: { nickname?: string; avatar_hue?: number }, done: string) {
    setSaving(true);
    setError('');
    setMessage('');
    try {
      await updateProfile(patch);
      setMessage(done);
    } catch (e) {
      setError(e instanceof Error ? e.message : '保存失败');
    } finally {
      setSaving(false);
    }
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const problem = nicknameValidationMessage(nickname);
    if (problem) {
      setError(problem);
      setMessage('');
      return;
    }
    void save({ nickname: nickname.trim() }, '昵称已更新');
  }

  const unread = unreadBadgeLabel(summary?.unread_notifications ?? 0);

  return (
    <div className="page me-page">
      <h1 className="me-title">个人中心</h1>

      <section className="me-card me-profile" aria-label="账号信息">
        <UserAvatar user={user} className="me-avatar" />
        <div className="me-profile-main">
          <p className="me-profile-name">{(user.nickname ?? '').trim() || '已登录'}</p>
          <p className="me-profile-meta">{user.email}</p>
          <p className="me-profile-meta">
            <span className="me-role">{roleLabel(user.is_admin)}</span>
          </p>
        </div>
        <dl className="me-stats">
          <div>
            <dt>收藏</dt>
            <dd>{summary ? summary.favorites : '—'}</dd>
          </div>
          <div>
            <dt>历史</dt>
            <dd>{summary ? summary.history : '—'}</dd>
          </div>
        </dl>
      </section>

      <section className="me-card" aria-label="编辑资料">
        <h2>编辑资料</h2>
        <form className="me-form" onSubmit={onSubmit}>
          <label htmlFor="me-nickname">昵称</label>
          <div className="me-form-row">
            <input
              id="me-nickname"
              value={nickname}
              maxLength={64}
              autoComplete="nickname"
              onChange={(e) => setNickname(e.target.value)}
            />
            <button type="submit" className="me-btn me-btn--primary" disabled={saving || !dirty}>
              {saving ? '保存中…' : '保存'}
            </button>
          </div>
        </form>

        <p className="me-label" id="me-hue-label">
          头像颜色
        </p>
        <div className="me-hues" role="radiogroup" aria-labelledby="me-hue-label">
          <button
            type="button"
            role="radio"
            aria-checked={currentHue < 0}
            className={`me-hue me-hue--default${currentHue < 0 ? ' is-active' : ''}`}
            disabled={saving}
            title="默认"
            onClick={() => void save({ avatar_hue: -1 }, '头像颜色已恢复默认')}
          >
            默认
          </button>
          {AVATAR_HUE_CHOICES.map((hue) => (
            <button
              key={hue}
              type="button"
              role="radio"
              aria-checked={currentHue === hue}
              aria-label={`色相 ${hue}`}
              className={`me-hue${currentHue === hue ? ' is-active' : ''}`}
              style={{ background: avatarStyle(hue) }}
              disabled={saving}
              onClick={() => void save({ avatar_hue: hue }, '头像颜色已更新')}
            />
          ))}
        </div>
        <p className="me-hint">当前：{currentHue < 0 ? `默认（${avatarHue(user)}°）` : `${currentHue}°`}</p>

        {message ? (
          <p className="me-feedback" role="status">
            {message}
          </p>
        ) : null}
        {error ? (
          <p className="me-feedback is-error" role="alert">
            {error}
          </p>
        ) : null}
      </section>

      <section className="me-card" aria-label="我的内容">
        <h2>我的内容</h2>
        <div className="me-links">
          <Link to="/favorites">我的收藏{summary ? `（${summary.favorites}）` : ''}</Link>
          <Link to="/history">观看历史{summary ? `（${summary.history}）` : ''}</Link>
          <Link to="/messages">我的消息{unread ? `（${unread} 未读）` : ''}</Link>
          {user.is_admin ? <Link to="/admin">管理后台</Link> : null}
        </div>
        <p className="me-disabled-note" aria-disabled="true">
          投稿：即将推出
        </p>
      </section>

      <section className="me-card" aria-label="设置">
        <h2>设置</h2>
        <div className="me-actions">
          <button type="button" className="me-btn" onClick={toggleTheme}>
            {theme === 'dark' ? '切换到浅色模式' : '切换到深色模式'}
          </button>
          <button type="button" className="me-btn me-btn--danger" onClick={logout}>
            退出登录
          </button>
        </div>
      </section>
    </div>
  );
}
