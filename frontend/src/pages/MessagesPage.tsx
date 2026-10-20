import { useEffect, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import {
  listNotificationsPage,
  markNotificationsRead,
  type UserNotification,
} from '../api';
import { appendUnique, countUnread, markRead } from '../account/accountLogic';
import { publishUnreadCount } from '../account/unreadEvents';
import { AuthGate } from '../auth/AuthGate';
import {
  MESSAGE_TABS,
  emptyMessagesText,
  formatHistoryTime,
  messageKindLabel,
  notificationHref,
  parseMessageFilter,
} from '../components/headerFlyoutData';

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
  const requestRef = useRef(0);

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
        setSelectedId(page.data[0]?.id ?? null);
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
  const replyUnread = countUnread(items.filter((item) => item.kind === 'comment_reply'));
  const systemUnread = countUnread(items.filter((item) => item.kind === 'system'));

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
    markNotificationsRead(token, { ids: [item.id] })
      .then(publishUnreadCount)
      .catch(() => {
        setItems((current) =>
          current.map((row) => (row.id === item.id ? { ...row, is_read: false } : row)),
        );
      });
  }

  async function onMarkAll() {
    try {
      const unread = await markNotificationsRead(token, { all: true });
      setItems((current) => markRead(current, 'all'));
      publishUnreadCount(unread);
    } catch (e) {
      setError(e instanceof Error ? e.message : '操作失败');
    }
  }

  function tabUnread(id: string): number {
    if (id === 'comment_reply') return replyUnread;
    if (id === 'system') return systemUnread;
    return unreadHere;
  }

  return (
    <div className="page me-page msg-center">
      <div className="msg-center-grid">
        <aside className="msg-sidebar" aria-label="消息分类">
          <h1 className="me-title">消息中心</h1>
          <nav className="msg-nav">
            {MESSAGE_TABS.map((tab) => {
              const badge = tabUnread(tab.id);
              return (
                <button
                  key={tab.id}
                  type="button"
                  className={filter === tab.id ? 'is-active' : undefined}
                  onClick={() => selectTab(tab.id)}
                >
                  <span>{tab.label}</span>
                  {badge > 0 ? <em>{badge > 99 ? '99+' : badge}</em> : null}
                </button>
              );
            })}
          </nav>
          <button
            type="button"
            className="me-btn"
            disabled={unreadHere === 0 && !loading}
            onClick={() => void onMarkAll()}
          >
            全部已读
          </button>
        </aside>

        <section className="msg-list-pane" aria-label="消息列表">
          {error ? (
            <p className="me-feedback is-error" role="alert">
              {error}
            </p>
          ) : null}
          {loading ? (
            <p className="me-status">加载中…</p>
          ) : items.length === 0 ? (
            <div className="me-empty">
              <p>{emptyMessagesText(filter)}</p>
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
                      onClick={() => void onSelect(item)}
                    >
                      <span className="msg-avatar" aria-hidden>
                        {(item.actor_name || messageKindLabel(item.kind)).slice(0, 1)}
                      </span>
                      <span className="msg-body">
                        <span className="msg-head">
                          <span className="msg-kind">{messageKindLabel(item.kind)}</span>
                          <strong>{item.title}</strong>
                        </span>
                        {item.body ? <span className="msg-text">{item.body}</span> : null}
                        <span className="msg-meta">
                          {item.actor_name ? `${item.actor_name} · ` : ''}
                          {formatHistoryTime(item.created_at)}
                        </span>
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

        <section className="msg-detail-pane" aria-label="消息详情">
          {!selected ? (
            <div className="me-empty msg-detail-empty">
              <p>选择一条消息查看详情</p>
            </div>
          ) : (
            <article className="msg-detail">
              <p className="msg-kind">{messageKindLabel(selected.kind)}</p>
              <h2>{selected.title}</h2>
              <p className="msg-meta">
                {selected.actor_name ? `${selected.actor_name} · ` : ''}
                {formatHistoryTime(selected.created_at)}
              </p>
              {selected.body ? <p className="msg-detail-body">{selected.body}</p> : null}
              {selected.kind === 'comment_reply' ? (
                <div className="msg-detail-actions">
                  {selected.actor_name ? <p>回复者：{selected.actor_name}</p> : null}
                  {notificationHref(selected) ? (
                    <Link className="me-btn me-btn--primary" to={notificationHref(selected)!}>
                      打开相关视频
                    </Link>
                  ) : (
                    <p className="me-hint">该回复没有关联视频</p>
                  )}
                </div>
              ) : (
                <div className="msg-detail-actions">
                  <p className="me-hint">系统通知</p>
                </div>
              )}
            </article>
          )}
        </section>
      </div>
    </div>
  );
}
