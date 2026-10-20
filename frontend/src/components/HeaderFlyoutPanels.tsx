import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  fetchMeSummary,
  listFavoriteFolders,
  listFavoritesPage,
  listHistoryPage,
  type MeSummary,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { coverImgSrc, onCoverImgError } from '../media/coverFallback';
import {
  ACCOUNT_UPLOAD_NOTE,
  VIP_PANEL,
  accountActions,
  accountDisplayName,
  accountStatusLabel,
} from './headerFlyoutLogic';
import {
  MESSAGE_MENU,
  accountStats,
  favoritePreviewItems,
  historyPreviewItems,
  normalizeFolders,
  previewState,
  unreadBadgeLabel,
  type FolderItem,
  type PreviewItem,
} from './headerFlyoutData';

type NavProps = { onNavigate: () => void };

export function AccountPanel({
  nickname,
  isAdmin,
  email,
  theme,
  onTheme,
  onLogout,
  onNavigate,
}: NavProps & {
  nickname: string;
  isAdmin: boolean | undefined;
  email: string;
  theme: 'light' | 'dark';
  onTheme: () => void;
  onLogout: () => void;
}) {
  const { token } = useAuth();
  const [summary, setSummary] = useState<MeSummary | null>(null);

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

  const stats = accountStats(summary);
  const actions = accountActions(isAdmin, theme);

  return (
    <div className="account-card">
      <div className="account-card-id">
        <span className="account-card-name">{accountDisplayName(nickname)}</span>
        <span className="account-card-status">{accountStatusLabel(isAdmin)}</span>
        {email ? <span className="account-card-email">{email}</span> : null}
      </div>
      <div className="account-card-stats" aria-label="我的数据">
        <Link to="/favorites" onClick={onNavigate}>
          <strong>{stats[0].value}</strong>
          <span>{stats[0].label}</span>
        </Link>
        <Link to="/history" onClick={onNavigate}>
          <strong>{stats[1].value}</strong>
          <span>{stats[1].label}</span>
        </Link>
      </div>
      <div className="account-card-actions">
        {actions.map((action) => {
          if (action.kind === 'link') {
            return (
              <Link key={action.id} to={action.href} onClick={onNavigate}>
                {action.label}
              </Link>
            );
          }
          if (action.kind === 'theme') {
            return (
              <button key={action.id} type="button" onClick={onTheme}>
                {action.label}
              </button>
            );
          }
          return (
            <button key={action.id} type="button" onClick={onLogout}>
              {action.label}
            </button>
          );
        })}
        <p className="account-card-note" aria-disabled="true">
          {ACCOUNT_UPLOAD_NOTE}
        </p>
      </div>
    </div>
  );
}

export function VipPanel({ onNavigate }: NavProps) {
  return (
    <div className="vip-panel">
      <div className="vip-panel-head">
        <strong>{VIP_PANEL.title}</strong>
        <span className="vip-panel-tag">{VIP_PANEL.subtitle}</span>
      </div>
      <p className="vip-panel-body">{VIP_PANEL.body}</p>
      <ul className="vip-benefits">
        {VIP_PANEL.bullets.map((bullet) => (
          <li key={bullet}>{bullet}</li>
        ))}
      </ul>
      <Link className="vip-panel-cta" to={VIP_PANEL.href} onClick={onNavigate}>
        {VIP_PANEL.cta}
      </Link>
    </div>
  );
}

export function MessagesMenu({ unread, onNavigate }: NavProps & { unread: number }) {
  const badge = unreadBadgeLabel(unread);
  return (
    <div className="message-menu">
      {MESSAGE_MENU.map((item) => (
        <Link key={item.id} to={item.href} onClick={onNavigate}>
          <span>{item.label}</span>
          {item.id === 'all' && badge ? <span className="message-menu-badge">{badge}</span> : null}
        </Link>
      ))}
    </div>
  );
}

function PreviewList({
  items,
  state,
  emptyText,
  errorText,
  onNavigate,
}: NavProps & {
  items: PreviewItem[];
  state: ReturnType<typeof previewState>;
  emptyText: string;
  errorText: string;
  onNavigate: () => void;
}) {
  if (state === 'loading') return <p className="flyout-empty">加载中…</p>;
  if (state === 'error') return <p className="flyout-empty">{errorText}</p>;
  if (state === 'empty') return <p className="flyout-empty">{emptyText}</p>;
  return (
    <ul className="flyout-list">
      {items.map((item) => (
        <li key={item.id}>
          <Link to={item.href} onClick={onNavigate} title={item.title}>
            <img
              className="flyout-thumb"
              src={coverImgSrc(item.cover)}
              alt=""
              loading="lazy"
              onError={onCoverImgError}
            />
            <span className="flyout-item-text">
              <span className="flyout-item-title">{item.title}</span>
              {item.sub ? <span className="flyout-item-sub">{item.sub}</span> : null}
            </span>
          </Link>
        </li>
      ))}
    </ul>
  );
}

export function FavoritesPanel({ onNavigate }: NavProps) {
  const { token } = useAuth();
  const [folders, setFolders] = useState<FolderItem[]>(() => normalizeFolders([]));
  const [folderId, setFolderId] = useState(0);
  const [items, setItems] = useState<PreviewItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    listFavoriteFolders(token)
      .then((rows) => {
        if (!cancelled) setFolders(normalizeFolders(rows));
      })
      .catch(() => {
        /* keep default folder placeholder */
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    setLoading(true);
    setError(false);
    listFavoritesPage(token, { folderId: folderId || undefined, limit: 8 })
      .then((page) => {
        if (!cancelled) setItems(favoritePreviewItems(page.data));
      })
      .catch(() => {
        if (!cancelled) {
          setItems([]);
          setError(true);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [token, folderId]);

  return (
    <div className="favorites-flyout">
      <div className="favorites-flyout-split">
        <ul className="favorites-flyout-folders" aria-label="收藏夹">
          <li>
            <button
              type="button"
              className={folderId === 0 ? 'is-active' : undefined}
              onClick={() => setFolderId(0)}
            >
              <span>全部收藏</span>
            </button>
          </li>
          {folders
            .filter((folder) => folder.id > 0)
            .map((folder) => (
              <li key={folder.id}>
                <button
                  type="button"
                  className={folderId === folder.id ? 'is-active' : undefined}
                  onClick={() => setFolderId(folder.id)}
                >
                  <span>{folder.name}</span>
                  <em>{folder.count}</em>
                </button>
              </li>
            ))}
        </ul>
        <div className="favorites-flyout-items">
          <PreviewList
            items={items}
            state={previewState(loading, error, items.length)}
            emptyText="还没有收藏，去看看喜欢的视频吧"
            errorText="收藏加载失败"
            onNavigate={onNavigate}
          />
        </div>
      </div>
      <Link className="flyout-footer" to="/favorites" onClick={onNavigate}>
        查看全部 →
      </Link>
    </div>
  );
}

export function HistoryPanel({ onNavigate }: NavProps) {
  const { token } = useAuth();
  const [items, setItems] = useState<PreviewItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    listHistoryPage(token, { limit: 8 })
      .then((page) => {
        if (!cancelled) setItems(historyPreviewItems(page.data));
      })
      .catch(() => {
        if (!cancelled) {
          setItems([]);
          setError(true);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  return (
    <div className="history-flyout">
      <div className="history-flyout-scroll">
        <PreviewList
          items={items}
          state={previewState(loading, error, items.length)}
          emptyText="暂无观看历史"
          errorText="历史加载失败"
          onNavigate={onNavigate}
        />
      </div>
      <Link className="flyout-footer" to="/history" onClick={onNavigate}>
        查看全部 →
      </Link>
    </div>
  );
}
