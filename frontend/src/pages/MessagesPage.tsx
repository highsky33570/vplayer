import { useEffect, useRef, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
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

const PAGE_SIZE = 20;

export function MessagesPage() {
  return <AuthGate>{({ token }) => <MessagesContent token={token} />}</AuthGate>;
}

function MessagesContent({ token }: { token: string }) {
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const filter = parseMessageFilter(params.get('kind'));
  const kind = MESSAGE_TABS.find((tab) => tab.id === filter)?.kind;
  const [items, setItems] = useState<UserNotification[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState('');
  const requestRef = useRef(0);

  useEffect(() => {
    const request = ++requestRef.current;
    setLoading(true);
    setError('');
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

  async function onOpen(item: UserNotification) {
    const href = notificationHref(item);
    if (!item.is_read) {
      setItems((current) => markRead(current, [item.id]));
      markNotificationsRead(token, { ids: [item.id] })
        .then(publishUnreadCount)
        .catch(() => {
          setItems((current) =>
            current.map((row) => (row.id === item.id ? { ...row, is_read: false } : row)),
          );
        });
    }
    if (href) navigate(href);
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

  const unreadHere = countUnread(items);

  return (
    <div className="page me-page">
      <div className="me-title-row">
        <h1 className="me-title">我的消息</h1>
        <button
          type="button"
          className="me-btn"
          disabled={unreadHere === 0 && !loading}
          onClick={() => void onMarkAll()}
        >
          全部已读
        </button>
      </div>

      <div className="me-tabs" role="tablist" aria-label="消息类型">
        {MESSAGE_TABS.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={filter === tab.id}
            className={filter === tab.id ? 'is-active' : undefined}
            onClick={() => selectTab(tab.id)}
          >
            {tab.label}
          </button>
        ))}
      </div>

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
          <ul className="msg-list">
            {items.map((item) => {
              const href = notificationHref(item);
              return (
                <li key={item.id}>
                  <button
                    type="button"
                    className={`msg-item${item.is_read ? '' : ' is-unread'}`}
                    onClick={() => void onOpen(item)}
                    title={href ? '查看视频' : undefined}
                  >
                    <span className="msg-dot" aria-hidden />
                    <span className="msg-body">
                      <span className="msg-head">
                        <span className="msg-kind">{messageKindLabel(item.kind)}</span>
                        <strong>{item.title}</strong>
                        {item.is_read ? null : <span className="sr-only">未读</span>}
                      </span>
                      {item.body ? <span className="msg-text">{item.body}</span> : null}
                      <span className="msg-meta">
                        {item.actor_name ? `${item.actor_name} · ` : ''}
                        {formatHistoryTime(item.created_at)}
                      </span>
                    </span>
                  </button>
                </li>
              );
            })}
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
    </div>
  );
}
