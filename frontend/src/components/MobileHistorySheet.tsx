import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { clearHistory, listHistoryPage, type HistoryVideo } from '../api';
import { useAuth } from '../auth/AuthContext';
import { clearGuestHistory, listGuestHistory } from '../guest/guestLibrary';
import { coverImgSrc, onCoverImgError } from '../media/coverFallback';
import { formatProgress } from './headerFlyoutData';

type Tab = 'local' | 'cloud';

export function MobileHistorySheet({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { token, authRequired, ready, user, openLogin } = useAuth();
  const [tab, setTab] = useState<Tab>('local');
  const [items, setItems] = useState<HistoryVideo[]>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose();
    }
    document.addEventListener('keydown', onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', onKey);
      document.body.style.overflow = prev;
    };
  }, [open, onClose]);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    if (tab === 'local') {
      setItems(listGuestHistory());
      setLoading(false);
      return;
    }
    if (!ready) return;
    if (authRequired && !token) {
      setItems([]);
      setLoading(false);
      return;
    }
    setLoading(true);
    listHistoryPage(token, { limit: 40 })
      .then((page) => {
        if (!cancelled) setItems(page.data);
      })
      .catch(() => {
        if (!cancelled) setItems([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open, tab, token, authRequired, ready]);

  async function onClear() {
    if (busy) return;
    setBusy(true);
    try {
      if (tab === 'local') {
        clearGuestHistory();
        setItems([]);
      } else if (token) {
        await clearHistory(token);
        setItems([]);
      } else if (authRequired) {
        openLogin('login');
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className={`mobile-history-sheet${open ? ' is-open' : ''}`} aria-hidden={!open}>
      <div className="mobile-history-panel" role="dialog" aria-modal="true" aria-label="播放记录">
        <header className="mobile-history-head">
          <button type="button" className="mobile-history-back" aria-label="返回" onClick={onClose}>
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" aria-hidden>
              <path d="M15 5 8 12l7 7" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </button>
          <h2 className="mobile-history-title">播放记录</h2>
          <span className="mobile-history-head-spacer" aria-hidden />
        </header>

        <div className="mobile-history-tabs" role="tablist" aria-label="记录来源">
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'local'}
            className={tab === 'local' ? 'is-active' : undefined}
            onClick={() => setTab('local')}
          >
            本地记录
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'cloud'}
            className={tab === 'cloud' ? 'is-active' : undefined}
            onClick={() => setTab('cloud')}
          >
            云端记录
          </button>
        </div>

        <div className="mobile-history-body">
          {tab === 'cloud' && authRequired && !user ? (
            <div className="mobile-history-empty">
              <EmptyBoxIcon />
              <p>登录后同步云端播放记录</p>
              <button type="button" className="mobile-history-login" onClick={() => openLogin('login')}>
                登录账号
              </button>
            </div>
          ) : loading ? (
            <p className="mobile-history-status">加载中…</p>
          ) : items.length === 0 ? (
            <div className="mobile-history-empty">
              <EmptyBoxIcon />
              <p>暂无播放记录</p>
            </div>
          ) : (
            <ul className="mobile-history-list">
              {items.map((item) => (
                <li key={item.id}>
                  <Link to={`/watch/${item.id}`} className="mobile-history-item" onClick={onClose}>
                    <img
                      src={coverImgSrc(item.cover_url)}
                      alt=""
                      loading="lazy"
                      onError={onCoverImgError}
                    />
                    <span className="mobile-history-item-meta">
                      <strong>{item.title}</strong>
                      <em>{formatProgress(item.progress_sec)}</em>
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="mobile-history-foot">
          <button type="button" className="mobile-history-clear" disabled={busy || items.length === 0} onClick={onClear}>
            清空记录
          </button>
        </div>
      </div>
    </div>
  );
}

function EmptyBoxIcon() {
  return (
    <svg className="mobile-history-empty-icon" width="88" height="72" viewBox="0 0 88 72" fill="none" aria-hidden>
      <path
        d="M10 28 44 12l34 16v28c0 2-1.5 4-4 4H14c-2.5 0-4-2-4-4V28Z"
        stroke="currentColor"
        strokeWidth="3"
        strokeLinejoin="round"
      />
      <path d="M10 28h68M44 12v48M28 36h8M52 36h8" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  );
}
