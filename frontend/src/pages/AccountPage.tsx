import { useEffect, useState, type CSSProperties, type FormEvent } from 'react';
import { Link, NavLink } from 'react-router-dom';
import {
  fetchMeSummary,
  formatDuration,
  listFavoriteFolders,
  listFavoritesPage,
  listHistoryPage,
  type FavoriteVideo,
  type HistoryVideo,
  type MeSummary,
} from '../api';
import { nicknameValidationMessage, roleLabel } from '../account/accountLogic';
import { AVATAR_HUE_CHOICES, avatarHue, avatarStyle } from '../account/avatar';
import { UserAvatar } from '../account/UserAvatar';
import { useAuth } from '../auth/AuthContext';
import { AuthGate } from '../auth/AuthGate';
import {
  formatHistoryTime,
  formatProgress,
  normalizeFolders,
  unreadBadgeLabel,
  type FolderItem,
} from '../components/headerFlyoutData';
import { coverImgSrc, onCoverImgError } from '../media/coverFallback';
import { useTheme } from '../theme/ThemeContext';
import { historyProgressRatio } from './historyPageLogic';

const RECENT_HISTORY_LIMIT = 8;
const RECENT_FAVORITES_LIMIT = 6;

export function AccountPage() {
  return <AuthGate>{({ token }) => <AccountSpace token={token} />}</AuthGate>;
}

function AccountSpace({ token }: { token: string }) {
  const { user, logout, updateProfile } = useAuth();
  const { theme, toggleTheme } = useTheme();
  const [summary, setSummary] = useState<MeSummary | null>(null);
  const [folders, setFolders] = useState<FolderItem[]>(() => normalizeFolders([]));
  const [recentHistory, setRecentHistory] = useState<HistoryVideo[]>([]);
  const [recentFavorites, setRecentFavorites] = useState<FavoriteVideo[]>([]);
  const [nickname, setNickname] = useState(user?.nickname ?? '');
  const [nickSource, setNickSource] = useState(user?.nickname ?? '');
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const [editOpen, setEditOpen] = useState(false);

  if ((user?.nickname ?? '') !== nickSource) {
    setNickSource(user?.nickname ?? '');
    setNickname(user?.nickname ?? '');
  }

  useEffect(() => {
    let cancelled = false;
    fetchMeSummary(token)
      .then((s) => {
        if (!cancelled) setSummary(s);
      })
      .catch(() => {
        if (!cancelled) setSummary(null);
      });
    listFavoriteFolders(token)
      .then((rows) => {
        if (!cancelled) setFolders(normalizeFolders(rows));
      })
      .catch(() => {
        if (!cancelled) setFolders(normalizeFolders([]));
      });
    listHistoryPage(token, { limit: RECENT_HISTORY_LIMIT, offset: 0 })
      .then((page) => {
        if (!cancelled) setRecentHistory(page.data);
      })
      .catch(() => {
        if (!cancelled) setRecentHistory([]);
      });
    listFavoritesPage(token, { limit: RECENT_FAVORITES_LIMIT, offset: 0 })
      .then((page) => {
        if (!cancelled) setRecentFavorites(page.data);
      })
      .catch(() => {
        if (!cancelled) setRecentFavorites([]);
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  if (!user) return null;

  const currentHue = user.avatar_hue ?? -1;
  const dirty = nickname.trim() !== (user.nickname ?? '').trim();
  const unread = unreadBadgeLabel(summary?.unread_notifications ?? 0);
  const hue = avatarHue(user);
  const displayName = (user.nickname ?? '').trim() || '已登录';

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

  return (
    <div className="space-page" data-testid="account-page">
      <section
        className="space-cover"
        aria-label="个人空间封面"
        style={{ '--space-hue': String(hue) } as CSSProperties}
      >
        <div className="space-cover-shade" aria-hidden />
        <div className="space-cover-inner">
          <UserAvatar user={user} className="space-cover-avatar" />
          <div className="space-cover-meta">
            <h1 className="space-cover-name">{displayName}</h1>
            <p className="space-cover-role">
              <span className="me-role">{roleLabel(user.is_admin)}</span>
              <span className="space-cover-email">{user.email}</span>
            </p>
          </div>
          <dl className="space-cover-stats" aria-label="我的数据">
            <div>
              <dd>{summary ? summary.favorites : '—'}</dd>
              <dt>收藏</dt>
            </div>
            <div>
              <dd>{summary ? summary.history : '—'}</dd>
              <dt>历史</dt>
            </div>
          </dl>
        </div>
      </section>

      <nav className="space-rail" aria-label="个人空间导航">
        <NavLink to="/account" end className={({ isActive }) => (isActive ? 'is-active' : undefined)}>
          主页
        </NavLink>
        <NavLink to="/favorites" className={({ isActive }) => (isActive ? 'is-active' : undefined)}>
          我的收藏{summary ? ` ${summary.favorites}` : ''}
        </NavLink>
        <NavLink to="/history" className={({ isActive }) => (isActive ? 'is-active' : undefined)}>
          观看历史{summary ? ` ${summary.history}` : ''}
        </NavLink>
        <a
          href="#space-edit"
          className={editOpen ? 'is-active' : undefined}
          onClick={(e) => {
            e.preventDefault();
            setEditOpen(true);
            document.getElementById('space-edit')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
          }}
        >
          编辑资料
        </a>
      </nav>

      <div className="space-body">
        <div className="space-main">
          <section className="space-section" aria-label="最近观看">
            <header className="space-section-head">
              <h2>
                最近观看
                {summary ? <em>{summary.history}</em> : null}
              </h2>
              <Link to="/history" className="space-more">
                查看更多
              </Link>
            </header>
            {recentHistory.length === 0 ? (
              <p className="space-empty">暂无观看历史</p>
            ) : (
              <div className="space-video-grid">
                {recentHistory.map((video) => {
                  const progress = formatProgress(video.progress_sec);
                  const ratio = historyProgressRatio(video.progress_sec, video.duration_sec);
                  return (
                    <article key={video.id} className="space-video-card">
                      <Link to={`/watch/${video.id}`} title={video.title}>
                        <span className="space-video-thumb">
                          <img
                            src={coverImgSrc(video.cover_url)}
                            alt=""
                            loading="lazy"
                            onError={onCoverImgError}
                          />
                          {ratio > 0 ? (
                            <span className="space-video-progress" aria-hidden>
                              <span style={{ width: `${Math.round(ratio * 100)}%` }} />
                            </span>
                          ) : null}
                          {video.duration_sec > 0 ? (
                            <span className="space-video-duration">
                              {formatDuration(video.duration_sec)}
                            </span>
                          ) : null}
                        </span>
                        <span className="space-video-title">{video.title}</span>
                        <span className="space-video-sub">
                          {formatHistoryTime(video.last_watched_at)}
                          {progress ? ` · ${progress}` : ''}
                        </span>
                      </Link>
                    </article>
                  );
                })}
              </div>
            )}
          </section>

          <section className="space-section" aria-label="我的视频">
            <header className="space-section-head">
              <h2>
                我的视频<em>0</em>
              </h2>
            </header>
            <div className="space-empty space-empty--panel">
              <p>暂无投稿视频</p>
              <p className="space-empty-hint">VPlayer 暂未开放用户投稿，此区域不会展示片库内容。</p>
            </div>
          </section>

          <section className="space-section" aria-label="收藏夹">
            <header className="space-section-head">
              <h2>
                收藏夹
                <em>{folders.filter((f) => f.id > 0).length || folders.length}</em>
              </h2>
              <Link to="/favorites" className="space-more">
                查看更多
              </Link>
            </header>
            <div className="space-folder-row">
              {folders.map((folder) => {
                const cover =
                  (folder.id > 0
                    ? recentFavorites.find((v) => v.folder_id === folder.id)?.cover_url
                    : undefined) ??
                  recentFavorites[0]?.cover_url ??
                  '';
                return (
                  <Link
                    key={folder.id || 'default'}
                    to="/favorites"
                    className="space-folder-card"
                    title={folder.name}
                  >
                    <span className="space-folder-thumb">
                      {cover ? (
                        <img src={coverImgSrc(cover)} alt="" loading="lazy" onError={onCoverImgError} />
                      ) : (
                        <span className="space-folder-placeholder" aria-hidden />
                      )}
                      <span className="space-folder-count">{folder.count} 个视频</span>
                    </span>
                    <span className="space-folder-name">{folder.name}</span>
                    <span className="space-folder-meta">{folder.isDefault ? '默认' : '收藏夹'}</span>
                  </Link>
                );
              })}
            </div>
            {recentFavorites.length > 0 ? (
              <div className="space-video-grid space-video-grid--fav">
                {recentFavorites.map((video) => (
                  <article key={video.id} className="space-video-card">
                    <Link to={`/watch/${video.id}`} title={video.title}>
                      <span className="space-video-thumb">
                        <img
                          src={coverImgSrc(video.cover_url)}
                          alt=""
                          loading="lazy"
                          onError={onCoverImgError}
                        />
                      </span>
                      <span className="space-video-title">{video.title}</span>
                      <span className="space-video-sub">{formatHistoryTime(video.favorited_at)}</span>
                    </Link>
                  </article>
                ))}
              </div>
            ) : (
              <p className="space-empty">这里还没有收藏的视频</p>
            )}
          </section>
        </div>

        <aside className="space-side" aria-label="个人资料与快捷入口">
          <section className="space-side-card">
            <h2>个人资料</h2>
            <dl className="space-side-dl">
              <div>
                <dt>邮箱</dt>
                <dd>{user.email}</dd>
              </div>
              <div>
                <dt>身份</dt>
                <dd>{roleLabel(user.is_admin)}</dd>
              </div>
              <div>
                <dt>状态</dt>
                <dd>已登录</dd>
              </div>
            </dl>
            <button
              type="button"
              className="me-btn"
              onClick={() => {
                setEditOpen(true);
                document.getElementById('space-edit')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
              }}
            >
              编辑资料
            </button>
          </section>

          <section className="space-side-card">
            <h2>快捷入口</h2>
            <div className="space-side-links">
              <Link to="/favorites">我的收藏{summary ? `（${summary.favorites}）` : ''}</Link>
              <Link to="/history">观看历史{summary ? `（${summary.history}）` : ''}</Link>
              <Link to="/messages">我的消息{unread ? `（${unread} 未读）` : ''}</Link>
              {user.is_admin ? <Link to="/admin">管理后台</Link> : null}
            </div>
          </section>

          <section className="space-side-card" id="space-edit" aria-label="编辑资料">
            <h2>编辑资料</h2>
            {!editOpen ? (
              <p className="space-empty-hint">点击上方按钮展开昵称与头像颜色设置。</p>
            ) : (
              <>
                <form className="me-form" onSubmit={onSubmit}>
                  <label htmlFor="space-nickname">昵称</label>
                  <div className="me-form-row">
                    <input
                      id="space-nickname"
                      value={nickname}
                      maxLength={64}
                      autoComplete="nickname"
                      onChange={(e) => setNickname(e.target.value)}
                    />
                    <button
                      type="submit"
                      className="me-btn me-btn--primary"
                      disabled={saving || !dirty}
                    >
                      {saving ? '保存中…' : '保存'}
                    </button>
                  </div>
                </form>

                <p className="me-label" id="space-hue-label">
                  头像颜色
                </p>
                <div className="me-hues" role="radiogroup" aria-labelledby="space-hue-label">
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
                  {AVATAR_HUE_CHOICES.map((h) => (
                    <button
                      key={h}
                      type="button"
                      role="radio"
                      aria-checked={currentHue === h}
                      aria-label={`色相 ${h}`}
                      className={`me-hue${currentHue === h ? ' is-active' : ''}`}
                      style={{ background: avatarStyle(h) }}
                      disabled={saving}
                      onClick={() => void save({ avatar_hue: h }, '头像颜色已更新')}
                    />
                  ))}
                </div>
                <p className="me-hint">
                  当前：{currentHue < 0 ? `默认（${avatarHue(user)}°）` : `${currentHue}°`}
                </p>
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
              </>
            )}
          </section>

          <section className="space-side-card">
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
        </aside>
      </div>
    </div>
  );
}
