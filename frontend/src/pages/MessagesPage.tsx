import { useEffect, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import {
  fetchUnreadCount,
  listNotificationsPage,
  markNotificationsRead,
  searchPublicUsers,
  sendDirectMessage,
  type PublicUser,
  type UserNotification,
} from '../api';
import { appendUnique, countUnread, markRead } from '../account/accountLogic';
import { publishUnreadCount } from '../account/unreadEvents';
import { useAuth } from '../auth/AuthContext';
import { AuthGate } from '../auth/AuthGate';
import { GuestMessagesPage } from '../guest/GuestMessagesPage';
import {
  MESSAGE_TABS,
  formatHistoryTime,
  messageKindLabel,
  parseMessageFilter,
} from '../components/headerFlyoutData';
import {
  emptyDetailCopy,
  emptyListCopy,
  messageDetailHeading,
  messageListPreview,
  messageListPrimary,
  messageWatchHref,
} from './messagesPageLogic';

const PAGE_SIZE = 30;
const DM_BODY_MAX = 500;
const DM_TITLE_MAX = 40;

export function MessagesPage() {
  const { authRequired, ready } = useAuth();
  if (!ready) {
    return (
      <div className="page me-page">
        <p className="me-status">正在验证登录状态…</p>
      </div>
    );
  }
  if (!authRequired) return <GuestMessagesPage />;
  return <AuthGate>{({ token }) => <MessagesContent token={token} />}</AuthGate>;
}

function MessagesContent({ token }: { token: string }) {
  const [params, setParams] = useSearchParams();
  const filter = parseMessageFilter(params.get('kind'));
  const isSearch = filter === 'search';
  const kind =
    MESSAGE_TABS.find((tab) => tab.id === filter)?.kind ??
    (filter === 'direct_message' || filter === 'direct_message_sent' || filter === 'comment_reply'
      ? filter
      : undefined);
  const [items, setItems] = useState<UserNotification[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(!isSearch);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState('');
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [totalUnread, setTotalUnread] = useState(0);
  const requestRef = useRef(0);

  const [searchId, setSearchId] = useState('');
  const [lookupBusy, setLookupBusy] = useState(false);
  const [lookupError, setLookupError] = useState('');
  const [foundUsers, setFoundUsers] = useState<PublicUser[]>([]);
  const [foundUser, setFoundUser] = useState<PublicUser | null>(null);
  const [draftTitle, setDraftTitle] = useState('私信');
  const [draftBody, setDraftBody] = useState('');
  const [sendBusy, setSendBusy] = useState(false);
  const [sendFeedback, setSendFeedback] = useState('');

  useEffect(() => {
    let cancelled = false;
    fetchUnreadCount(token)
      .then((n) => {
        if (!cancelled) {
          setTotalUnread(n);
          publishUnreadCount(n);
        }
      })
      .catch(() => {
        if (!cancelled) setTotalUnread(0);
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  useEffect(() => {
    if (isSearch) {
      setLoading(false);
      setItems([]);
      setHasMore(false);
      setSelectedId(null);
      setError('');
      return;
    }
    const request = ++requestRef.current;
    setLoading(true);
    setError('');
    setSelectedId(null);
    listNotificationsPage(token, { kind, limit: PAGE_SIZE, offset: 0 })
      .then((page) => {
        if (requestRef.current !== request) return;
        setItems(page.data);
        setHasMore(page.has_more);
      })
      .catch((e) => {
        if (requestRef.current !== request) return;
        setItems([]);
        setError(e instanceof Error ? e.message : '加载失败');
      })
      .finally(() => {
        if (requestRef.current === request) setLoading(false);
      });
  }, [token, kind, isSearch]);

  const selected = items.find((item) => item.id === selectedId) ?? null;
  const unreadHere = countUnread(items);
  const listTitle = MESSAGE_TABS.find((tab) => tab.id === filter)?.label ?? '我的消息';

  async function loadMore() {
    const request = requestRef.current;
    setLoadingMore(true);
    try {
      const page = await listNotificationsPage(token, { kind, limit: PAGE_SIZE, offset: items.length });
      if (requestRef.current !== request) return;
      setItems((current) => appendUnique(current, page.data));
      setHasMore(page.has_more);
    } catch (e) {
      if (requestRef.current === request) setError(e instanceof Error ? e.message : '加载失败');
    } finally {
      setLoadingMore(false);
    }
  }

  function selectTab(id: string) {
    const next = new URLSearchParams(params);
    if (id === 'all') next.delete('kind');
    else next.set('kind', id);
    setParams(next, { replace: true });
    setSendFeedback('');
    setLookupError('');
  }

  async function onSelect(item: UserNotification) {
    setSelectedId(item.id);
    if (item.is_read) return;
    setItems((current) => markRead(current, [item.id]));
    setTotalUnread((n) => Math.max(0, n - 1));
    markNotificationsRead(token, { ids: [item.id] })
      .then((unread) => {
        setTotalUnread(unread);
        publishUnreadCount(unread);
      })
      .catch(() => {
        setItems((current) =>
          current.map((row) => (row.id === item.id ? { ...row, is_read: false } : row)),
        );
        setTotalUnread((n) => n + 1);
      });
  }

  async function onMarkAll() {
    try {
      const unread = await markNotificationsRead(token, { all: true });
      setItems((current) => markRead(current, 'all'));
      setTotalUnread(unread);
      publishUnreadCount(unread);
    } catch (e) {
      setError(e instanceof Error ? e.message : '操作失败');
    }
  }

  function clearSelection() {
    setSelectedId(null);
  }

  function pickUser(user: PublicUser) {
    setFoundUser(user);
    setDraftTitle(`致 ${user.nickname}`);
    setDraftBody('');
    setSendFeedback('');
    setLookupError('');
  }

  async function onLookup() {
    const q = String(searchId).trim();
    if (!q) {
      setLookupError('请输入用户 ID、邮箱或昵称');
      setFoundUser(null);
      setFoundUsers([]);
      return;
    }
    setLookupBusy(true);
    setLookupError('');
    setSendFeedback('');
    setFoundUser(null);
    setFoundUsers([]);
    try {
      const users = await searchPublicUsers(token, q);
      if (users.length === 0) {
        setLookupError('未找到该用户，请确认 ID / 邮箱 / 昵称');
        return;
      }
      setFoundUsers(users);
      if (users.length === 1) {
        pickUser(users[0]);
      }
    } catch (e) {
      setLookupError(e instanceof Error ? e.message : '查找失败');
    } finally {
      setLookupBusy(false);
    }
  }

  async function onSend() {
    if (!foundUser) {
      setLookupError('请先查找接收用户');
      return;
    }
    const body = draftBody.trim();
    if (!body) {
      setSendFeedback('请输入私信内容');
      return;
    }
    if (body.length > DM_BODY_MAX) {
      setSendFeedback(`内容最多 ${DM_BODY_MAX} 字`);
      return;
    }
    setSendBusy(true);
    setSendFeedback('');
    try {
      await sendDirectMessage(token, {
        to_user_id: foundUser.id,
        title: draftTitle.trim() || undefined,
        body,
      });
      setSendFeedback('发送成功');
      setDraftBody('');
    } catch (e) {
      setSendFeedback(e instanceof Error ? e.message : '发送失败');
    } finally {
      setSendBusy(false);
    }
  }

  const watchHref = selected ? messageWatchHref(selected) : null;

  return (
    <div
      className={`msg-dest${selected && !isSearch ? ' msg-dest--detail' : ''}${isSearch ? ' msg-dest--compose' : ''}`}
      data-testid="messages-page"
    >
      <div className="msg-center-grid" data-testid="messages-workspace">
        <aside className="msg-sidebar" aria-label="消息分类">
          <div className="msg-sidebar-head">
            <h1 className="msg-sidebar-title">消息中心</h1>
            <p className="msg-sidebar-sub">系统通知、私信收发与查找用户都在这里</p>
          </div>
          <div className="msg-sidebar-tools">
            <nav className="msg-nav">
              {MESSAGE_TABS.map((tab) => {
                const showTotalBadge = tab.id === 'all' && totalUnread > 0;
                return (
                  <button
                    key={tab.id}
                    type="button"
                    className={filter === tab.id ? 'is-active' : undefined}
                    aria-current={filter === tab.id ? 'page' : undefined}
                    onClick={() => selectTab(tab.id)}
                  >
                    <span>{tab.label}</span>
                    {showTotalBadge ? <em>{totalUnread > 99 ? '99+' : totalUnread}</em> : null}
                  </button>
                );
              })}
            </nav>
            {!isSearch ? (
              <button
                type="button"
                className="me-btn msg-sidebar-action"
                disabled={totalUnread === 0 && unreadHere === 0 && !loading}
                onClick={() => void onMarkAll()}
              >
                全部已读
              </button>
            ) : null}
          </div>
        </aside>

        {isSearch ? (
          <section className="msg-list-pane msg-search-pane" aria-label="查找用户" data-testid="messages-search">
            <header className="msg-pane-head">
              <h2>查找用户</h2>
            </header>
            <div className="msg-search-form">
              <label className="msg-field">
                <span>用户 ID / 邮箱 / 昵称</span>
                <div className="msg-search-row">
                  <input
                    type="text"
                    placeholder="例如 12、name@mail.com、test2"
                    value={searchId}
                    onChange={(e) => setSearchId(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') void onLookup();
                    }}
                  />
                  <button type="button" className="me-btn me-btn--primary" disabled={lookupBusy} onClick={() => void onLookup()}>
                    {lookupBusy ? '查找中…' : '查找'}
                  </button>
                </div>
              </label>
              {lookupError ? (
                <p className="me-feedback is-error" role="alert">
                  {lookupError}
                </p>
              ) : null}
              {foundUsers.length > 1 ? (
                <ul className="msg-found-list" data-testid="messages-found-list">
                  {foundUsers.map((user) => (
                    <li key={user.id}>
                      <button
                        type="button"
                        className={`msg-found-user${foundUser?.id === user.id ? ' is-selected' : ''}`}
                        onClick={() => pickUser(user)}
                      >
                        <span
                          className="msg-avatar"
                          style={{ background: `hsl(${user.avatar_hue} 55% 42%)` }}
                          aria-hidden
                        >
                          {user.nickname.slice(0, 1)}
                        </span>
                        <span>
                          <strong>{user.nickname}</strong>
                          <p>ID · {user.id}</p>
                        </span>
                      </button>
                    </li>
                  ))}
                </ul>
              ) : foundUser ? (
                <div className="msg-found-user" data-testid="messages-found-user">
                  <span
                    className="msg-avatar"
                    style={{ background: `hsl(${foundUser.avatar_hue} 55% 42%)` }}
                    aria-hidden
                  >
                    {foundUser.nickname.slice(0, 1)}
                  </span>
                  <div>
                    <strong>{foundUser.nickname}</strong>
                    <p>ID · {foundUser.id}</p>
                  </div>
                </div>
              ) : (
                <p className="me-hint">输入用户 ID、邮箱或昵称查找并发送私信</p>
              )}
            </div>
          </section>
        ) : (
          <section className="msg-list-pane" aria-label="消息列表">
            <header className="msg-pane-head">
              <h2>{listTitle}</h2>
            </header>
            {error ? (
              <p className="me-feedback is-error" role="alert">
                {error}
              </p>
            ) : null}
            {loading ? (
              <p className="me-status msg-pane-status">加载中…</p>
            ) : items.length === 0 ? (
              <div className="msg-list-empty" data-testid="messages-list-empty">
                <div className="msg-empty-art" aria-hidden>
                  <MsgEmptyIcon />
                </div>
                <p className="msg-empty-title">{emptyListCopy(filter)}</p>
                <p className="msg-empty-desc">
                  新注册欢迎信、系统公告、好友私信都会出现在这里。也可以去「查找用户」按 ID 或昵称发私信。
                </p>
                {filter !== 'search' ? (
                  <button type="button" className="me-btn me-btn--primary" onClick={() => selectTab('search')}>
                    去查找用户
                  </button>
                ) : null}
              </div>
            ) : (
              <>
                <ul className="msg-list msg-list--center">
                  {items.map((item) => (
                    <li key={item.id}>
                      <button
                        type="button"
                        className={`msg-item${item.is_read ? '' : ' is-unread'}${
                          selectedId === item.id ? ' is-selected' : ''
                        }`}
                        data-testid={`message-item-${item.id}`}
                        onClick={() => void onSelect(item)}
                      >
                        <span className="msg-avatar" aria-hidden>
                          {(item.actor_name || messageKindLabel(item.kind)).slice(0, 1)}
                        </span>
                        <span className="msg-body">
                          <span className="msg-head">
                            <span className="msg-kind">{messageKindLabel(item.kind)}</span>
                            <strong>{messageListPrimary(item)}</strong>
                          </span>
                          {messageListPreview(item) ? (
                            <span className="msg-text">{messageListPreview(item)}</span>
                          ) : null}
                          <span className="msg-meta">{formatHistoryTime(item.created_at)}</span>
                        </span>
                        {!item.is_read ? <span className="msg-dot" aria-label="未读" /> : null}
                      </button>
                    </li>
                  ))}
                </ul>
                {hasMore ? (
                  <div className="load-more-wrap">
                    <button
                      type="button"
                      className="me-btn"
                      disabled={loadingMore}
                      onClick={() => void loadMore()}
                    >
                      {loadingMore ? '加载中…' : '加载更多'}
                    </button>
                  </div>
                ) : (
                  <div className="msg-list-foot">
                    <p>已显示全部消息 · 未读会在顶部消息图标显示红点</p>
                  </div>
                )}
              </>
            )}
          </section>
        )}

        <section className="msg-detail-pane" aria-label={isSearch ? '编辑私信' : '消息详情'} data-testid="messages-detail">
          {isSearch ? (
            <div className="msg-composer" data-testid="messages-composer">
              <h2>编写私信</h2>
              {!foundUser ? (
                <div className="msg-detail-empty">
                  <p>{emptyDetailCopy('search')}</p>
                </div>
              ) : (
                <>
                  <p className="msg-meta">
                    发送给 <strong>{foundUser.nickname}</strong>（ID {foundUser.id}）
                  </p>
                  <label className="msg-field">
                    <span>标题</span>
                    <input
                      type="text"
                      maxLength={DM_TITLE_MAX}
                      value={draftTitle}
                      onChange={(e) => setDraftTitle(e.target.value)}
                      placeholder="私信标题（可编辑）"
                    />
                  </label>
                  <label className="msg-field">
                    <span>
                      内容 <em>{draftBody.length}/{DM_BODY_MAX}</em>
                    </span>
                    <textarea
                      rows={8}
                      maxLength={DM_BODY_MAX}
                      value={draftBody}
                      onChange={(e) => setDraftBody(e.target.value)}
                      placeholder="在此编辑私信内容…"
                    />
                  </label>
                  {sendFeedback ? (
                    <p
                      className={`me-feedback${sendFeedback === '发送成功' ? ' is-success' : ' is-error'}`}
                      role="status"
                    >
                      {sendFeedback}
                    </p>
                  ) : null}
                  <div className="msg-detail-actions">
                    <button
                      type="button"
                      className="me-btn me-btn--primary"
                      disabled={sendBusy || !draftBody.trim()}
                      onClick={() => void onSend()}
                    >
                      {sendBusy ? '发送中…' : '发送私信'}
                    </button>
                  </div>
                </>
              )}
            </div>
          ) : !selected ? (
            <div className="msg-detail-empty" data-testid="messages-detail-empty">
              <div className="msg-detail-icon" aria-hidden>
                <MsgEmptyIcon />
              </div>
              <p>{emptyDetailCopy()}</p>
            </div>
          ) : (
            <article className="msg-detail" data-testid="messages-detail-body">
              <button type="button" className="msg-detail-back" onClick={clearSelection}>
                返回列表
              </button>
              <p className="msg-kind">{messageDetailHeading(selected)}</p>
              <h2>{selected.title}</h2>
              <p className="msg-meta">
                {selected.actor_name ? `${selected.actor_name} · ` : ''}
                {selected.actor_user_id ? `ID ${selected.actor_user_id} · ` : ''}
                {formatHistoryTime(selected.created_at)}
              </p>
              {selected.body ? <p className="msg-detail-body">{selected.body}</p> : null}
              {selected.kind === 'comment_reply' ? (
                <div className="msg-detail-actions">
                  {selected.actor_name ? <p>回复者：{selected.actor_name}</p> : null}
                  {watchHref ? (
                    <Link className="me-btn me-btn--primary" to={watchHref} data-testid="message-watch-link">
                      查看视频
                    </Link>
                  ) : (
                    <p className="me-hint">该回复没有关联视频</p>
                  )}
                </div>
              ) : selected.kind === 'direct_message' || selected.kind === 'direct_message_sent' ? (
                <div className="msg-detail-actions">
                  <p className="me-hint">
                    {selected.kind === 'direct_message_sent'
                      ? `已发送给 ${selected.actor_name || '用户'}`
                      : '来自用户的私信'}
                  </p>
                  {selected.actor_user_id ? (
                    <button
                      type="button"
                      className="me-btn me-btn--primary"
                      onClick={() => {
                        setSearchId(String(selected.actor_user_id));
                        selectTab('search');
                        setFoundUsers([]);
                        setFoundUser(
                          selected.actor_user_id
                            ? {
                                id: selected.actor_user_id,
                                nickname: selected.actor_name || `用户${selected.actor_user_id}`,
                                avatar_hue: 200,
                              }
                            : null,
                        );
                        setDraftTitle(
                          selected.kind === 'direct_message_sent'
                            ? `致 ${selected.actor_name || ''}`.trim()
                            : `回复 ${selected.actor_name || ''}`.trim(),
                        );
                        setDraftBody('');
                      }}
                    >
                      {selected.kind === 'direct_message_sent' ? '再次发送' : '回复私信'}
                    </button>
                  ) : null}
                </div>
              ) : (
                <div className="msg-detail-actions">
                  {watchHref ? (
                    <Link className="me-btn me-btn--primary" to={watchHref}>
                      查看相关内容
                    </Link>
                  ) : (
                    <p className="me-hint">系统通知</p>
                  )}
                </div>
              )}
            </article>
          )}
        </section>
      </div>
    </div>
  );
}

function MsgEmptyIcon() {
  return (
    <svg width="56" height="56" viewBox="0 0 56 56" fill="none" aria-hidden>
      <rect x="8" y="14" width="40" height="28" rx="6" stroke="currentColor" strokeWidth="2" opacity="0.55" />
      <path
        d="M12 18l16 12 16-12"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        opacity="0.55"
      />
    </svg>
  );
}
