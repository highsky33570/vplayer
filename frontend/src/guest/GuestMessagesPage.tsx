import { useSearchParams } from 'react-router-dom';
import { MESSAGE_TABS, parseMessageFilter } from '../components/headerFlyoutData';
import { emptyDetailCopy, emptyListCopy } from '../pages/messagesPageLogic';

/** Guest messages have no users.id, so the shell stays empty and does not call the API. */
export function GuestMessagesPage() {
  const [params] = useSearchParams();
  const filter = parseMessageFilter(params.get('kind'));
  const listTitle = MESSAGE_TABS.find((tab) => tab.id === filter)?.label ?? '我的消息';

  return (
    <div className="msg-dest" data-testid="messages-page">
      <div className="msg-center-grid" data-testid="guest-messages">
        <aside className="msg-sidebar" aria-label="消息分类">
          <div className="msg-sidebar-head">
            <h1 className="msg-sidebar-title">消息中心</h1>
          </div>
          <nav className="msg-nav">
            {MESSAGE_TABS.map((tab) => (
              <button key={tab.id} type="button" className={filter === tab.id ? 'is-active' : undefined} disabled>
                <span>{tab.label}</span>
              </button>
            ))}
          </nav>
        </aside>
        <section className="msg-list-pane" aria-label="消息列表">
          <header className="msg-pane-head">
            <h2>{listTitle}</h2>
          </header>
          <div className="msg-list-empty" data-testid="messages-list-empty">
            <p>{emptyListCopy(filter)}</p>
            <p className="fav-empty-hint">访客没有账号消息。</p>
          </div>
        </section>
        <section className="msg-detail-pane" aria-label="消息详情" data-testid="messages-detail-empty">
          <div className="msg-detail-empty">
            <p>{emptyDetailCopy()}</p>
          </div>
        </section>
      </div>
    </div>
  );
}
