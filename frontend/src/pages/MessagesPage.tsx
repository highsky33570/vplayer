import { useEffect, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import {
  fetchUnreadCount,
  listNotificationsPage,
  markNotificationsRead,
  type UserNotification,
} from '../api';
import { appendUnique, countUnread, markRead } from '../account/accountLogic';
import { publishUnreadCount } from '../account/unreadEvents';
import { AuthGate } from '../auth/AuthGate';
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

export function MessagesPage() {
  return <AuthGate>{({ token }) => <MessagesContent token={token} />}</AuthGate>;
}

function MessagesContent({ token }: { token: string }) {
  const [params, setParams] = useSearchParams();
  const filter = parseMessageFilter(params.get('kind'));
  const kind = MESSAGE_TABS.find((tab) => tab.id === filter)?.kind;
  const [items, setItems] = useState<UserNotification[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState('');
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [totalUnread, setTotalUnread] = useState(0);
  const requestRef = useRef(0);

  useEffect(() => {
    let cancelled = false;
    fetchUnreadCount(token)
      .then((n) => {
        if (!cancelled) setTotalUnread(n);
      })
      .catch(() => {
        if (!cancelled) setTotalUnread(0);
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  useEffect(() => {
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
  }, [token, kind]);

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

  const watchHref = selected ? messageWatchHref(selected) : null;

  return (
    <div
      className={`msg-dest${selected ? ' msg-dest--detail' : ''}`}
      data-testid="messages-page"
    >
      <div className="msg-center-grid" data-testid="messages-workspace">
        <aside className="msg-sidebar" aria-label="消息分类">
          <div className="msg-sidebar-head">
            <h1 className="msg-sidebar-title">消息中心</h1>
          </div>
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
          <button
            type="button"
            className="me-btn msg-sidebar-action"
            disabled={totalUnread === 0 && unreadHere === 0 && !loading}
            onClick={() => void onMarkAll()}
          >
            全部已读
          </button>
        </aside>

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
              <p>{emptyListCopy(filter)}</p>
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
              ) : null}
            </>
          )}
        </section>

        <section className="msg-detail-pane" aria-label="消息详情" data-testid="messages-detail">
          {!selected ? (
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
