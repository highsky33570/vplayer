import { useEffect, useState, type FormEvent } from 'react';
import { Link, NavLink, useNavigate } from 'react-router-dom';
import {
  fetchMeSummary,
  formatDuration,
  listFavoriteFolders,
  listFavoritesPage,
  listHistoryPage,
  listVideosPage,
  type FavoriteVideo,
  type HistoryVideo,
  type MeSummary,
  type Video,
} from '../api';
import { membershipBadgeLabel, nicknameValidationMessage, roleLabel } from '../account/accountLogic';
import { AVATAR_HUE_CHOICES, avatarHue, avatarStyle } from '../account/avatar';
import { UserAvatar } from '../account/UserAvatar';
import { useAuth } from '../auth/AuthContext';
import { AuthGate } from '../auth/AuthGate';
import { GuestAccountPage } from '../guest/GuestAccountPage';
import {
  formatHistoryTime,
  formatProgress,
  normalizeFolders,
  unreadBadgeLabel,
  type FolderItem,
} from '../components/headerFlyoutData';
import { HoverPreviewThumb } from '../components/HoverPreviewThumb';
import { coverImgSrc, onCoverImgError } from '../media/coverFallback';
import { useTheme } from '../theme/ThemeContext';
import {
  ACCOUNT_FAVORITES_PREVIEW_LIMIT,
  ACCOUNT_HISTORY_PREVIEW_LIMIT,
  ACCOUNT_RECOMMEND_LIMIT,
  accountFavoritesPreview,
  accountHistoryPreview,
  accountOwnedUploadVideos,
  accountOwnedUploadsEmptyCopy,
  accountRecommendMeta,
  accountRecommendVideos,
} from './accountPageLogic';
import { historyProgressRatio } from './historyPageLogic';

export function AccountPage() {
  const { authRequired, ready } = useAuth();
  if (!ready) {
    return (
      <div className="page me-page">
        <p className="me-status">正在验证登录状态…</p>
      </div>
    );
  }
  if (!authRequired) return <GuestAccountPage />;
  return <AuthGate>{({ token }) => <AccountSpace token={token} />}</AuthGate>;
}

function AccountSpace({ token }: { token: string }) {
  const navigate = useNavigate();
  const { user, logout, updateProfile } = useAuth();
  const { theme, toggleTheme } = useTheme();
  const [summary, setSummary] = useState<MeSummary | null>(null);
  const [folders, setFolders] = useState<FolderItem[]>(() => normalizeFolders([]));
  const [recentHistory, setRecentHistory] = useState<HistoryVideo[]>([]);
  const [recentFavorites, setRecentFavorites] = useState<FavoriteVideo[]>([]);
  const [recommended, setRecommended] = useState<Video[]>([]);
  const [nickname, setNickname] = useState(user?.nickname ?? '');
  const [nickSource, setNickSource] = useState(user?.nickname ?? '');
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const membership = membershipBadgeLabel(user);

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
    listHistoryPage(token, { limit: ACCOUNT_HISTORY_PREVIEW_LIMIT, offset: 0 })
      .then((page) => {
        if (!cancelled) setRecentHistory(accountHistoryPreview(page.data));
      })
      .catch(() => {
        if (!cancelled) setRecentHistory([]);
      });
    listFavoritesPage(token, { limit: ACCOUNT_FAVORITES_PREVIEW_LIMIT, offset: 0 })
      .then((page) => {
        if (!cancelled) setRecentFavorites(accountFavoritesPreview(page.data));
      })
      .catch(() => {
        if (!cancelled) setRecentFavorites([]);
      });
    // Catalog recommendations (not personal ownership) — same latest feed as homepage.
    listVideosPage({
      limit: ACCOUNT_RECOMMEND_LIMIT,
      offset: 0,
      excludeCategorySlug: 'midnight',
    })
      .then((page) => {
        if (!cancelled) setRecommended(accountRecommendVideos(page.data));
      })
      .catch(() => {
        if (!cancelled) setRecommended([]);
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  if (!user) return null;

  const currentHue = user.avatar_hue ?? -1;
  const dirty = nickname.trim() !== (user.nickname ?? '').trim();
  const unread = unreadBadgeLabel(summary?.unread_notifications ?? 0);
  const displayName = (user.nickname ?? '').trim() || '已登录';
  const ownedUploads = accountOwnedUploadVideos(user.id);
  const ownedEmpty = accountOwnedUploadsEmptyCopy();

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
    <div className="page space-page" data-testid="account-page">
      <header className="space-identity" aria-label="个人资料">
        <UserAvatar user={user} className="space-identity-avatar" />
        <div className="space-identity-meta">
          <h1 className="space-identity-name">{displayName}</h1>
          <p className="space-identity-role">
            <span className="me-role">{roleLabel(user.is_admin)}</span>
            {membership ? (
              <span className="me-member-badge" title={membership.detail}>
                {membership.badge}
              </span>
            ) : null}
          </p>
          {membership ? <p className="space-identity-member-detail">{membership.detail}</p> : null}
          <p className="space-identity-uid">用户 ID · {user.id}</p>
          <p className="space-identity-email-row">
            <span className="space-identity-email">{user.email}</span>
            <button
              type="button"
              className="space-identity-logout"
              onClick={() => {
                logout();
                navigate('/', { replace: true });
              }}
            >
              退出登录
            </button>
          </p>
        </div>
        <dl className="space-identity-stats" aria-label="我的数据">
          <div>
            <dd>{summary ? summary.favorites : '—'}</dd>
            <dt>收藏</dt>
          </div>
          <div>
            <dd>{summary ? summary.history : '—'}</dd>
            <dt>历史</dt>
          </div>
        </dl>
      </header>

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
              <p className="space-empty space-empty--compact">暂无观看历史 · 打开任意视频即可记录</p>
            ) : (
              <div className="space-video-grid" data-testid="account-history-grid">
                {recentHistory.map((video) => {
                  const progress = formatProgress(video.progress_sec);
                  const ratio = historyProgressRatio(video.progress_sec, video.duration_sec);
                  return (
                    <article key={video.id} className="space-video-card">
                      <Link to={`/watch/${video.id}`} title={video.title}>
                        <HoverPreviewThumb
                          videoId={video.id}
                          coverUrl={video.cover_url}
                          durationSec={video.duration_sec}
                          className="space-video-thumb"
                        >
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
                        </HoverPreviewThumb>
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

          <section className="space-section space-section--compact" aria-label="我的视频">
            <header className="space-section-head">
              <h2>
                我的视频<em>{ownedUploads.length}</em>
              </h2>
            </header>
            {ownedUploads.length === 0 ? (
              <div className="space-empty space-empty--compact" data-testid="account-uploads-empty">
                <p>{ownedEmpty.title}</p>
                <p className="space-empty-hint">{ownedEmpty.hint}</p>
              </div>
            ) : (
              <div className="space-video-grid">
                {ownedUploads.map((video) => (
                  <article key={video.id} className="space-video-card">
                    <Link to={`/watch/${video.id}`} title={video.title}>
                      <HoverPreviewThumb
                        videoId={video.id}
                        coverUrl={video.cover_url}
                        durationSec={video.duration_sec}
                        className="space-video-thumb"
                      />
                      <span className="space-video-title">{video.title}</span>
                    </Link>
                  </article>
                ))}
              </div>
            )}
          </section>

          <section className="space-section" aria-label="收藏夹">
            <header className="space-section-head">
              <h2>
                收藏夹
                <em>{summary ? summary.favorites : folders.reduce((n, f) => n + f.count, 0)}</em>
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
              <div className="space-video-grid space-video-grid--fav" data-testid="account-favorites-grid">
                {recentFavorites.map((video) => (
                  <article key={video.id} className="space-video-card">
                    <Link to={`/watch/${video.id}`} title={video.title}>
                      <HoverPreviewThumb
                        videoId={video.id}
                        coverUrl={video.cover_url}
                        durationSec={video.duration_sec}
                        className="space-video-thumb"
                      />
                      <span className="space-video-title">{video.title}</span>
                      <span className="space-video-sub">{formatHistoryTime(video.favorited_at)}</span>
                    </Link>
                  </article>
                ))}
              </div>
            ) : (
              <p className="space-empty space-empty--compact">这里还没有收藏的视频</p>
            )}
          </section>

          <section className="space-section" aria-label="猜你喜欢" data-testid="account-recommend">
            <header className="space-section-head">
              <h2>
                猜你喜欢
                {recommended.length > 0 ? <em>{recommended.length}</em> : null}
              </h2>
              <p className="space-section-note">站内片库推荐 · 非个人投稿</p>
            </header>
            {recommended.length === 0 ? (
              <p className="space-empty space-empty--compact">暂时没有可推荐的内容</p>
            ) : (
              <div className="space-video-grid" data-testid="account-recommend-grid">
                {recommended.map((video) => {
                  const meta = accountRecommendMeta(video);
                  return (
                    <article key={video.id} className="space-video-card">
                      <Link to={`/watch/${video.id}`} title={video.title}>
                        <HoverPreviewThumb
                          videoId={video.id}
                          coverUrl={video.cover_url}
                          durationSec={video.duration_sec}
                          className="space-video-thumb"
                        >
                          {video.duration_sec > 0 ? (
                            <span className="space-video-duration">
                              {formatDuration(video.duration_sec)}
                            </span>
                          ) : null}
                        </HoverPreviewThumb>
                        <span className="space-video-title">{video.title}</span>
                        {meta ? <span className="space-video-sub">{meta}</span> : null}
                      </Link>
                    </article>
                  );
                })}
              </div>
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
                <dt>大会员</dt>
                <dd>{membership ? membership.detail : '未开通'}</dd>
              </div>
              <div>
                <dt>状态</dt>
                <dd>已登录</dd>
              </div>
            </dl>
            {!membership ? (
              <Link className="me-btn" to="/vip">
                开通大会员
              </Link>
            ) : null}
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
