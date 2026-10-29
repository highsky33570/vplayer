/**
 * Pure presentation helpers for the header flyouts and the account pages.
 * Only type imports from local modules so Node can run the tests directly.
 */
import type {
  FavoriteFolder,
  FavoriteVideo,
  HistoryVideo,
  MeSummary,
  NotificationKind,
  UserNotification,
} from '../api';

/** Header badge text: hidden at 0, capped at 99+. */
export function unreadBadgeLabel(count: number): string {
  if (!Number.isFinite(count) || count <= 0) return '';
  return count > 99 ? '99+' : String(Math.floor(count));
}

function pad2(n: number): string {
  return String(n).padStart(2, '0');
}

function startOfDay(d: Date): number {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

/**
 * Relative-then-absolute time label used by history and messages.
 * 刚刚 / N 分钟前 / N 小时前 / 昨天 HH:MM / M月D日 / YYYY年M月D日.
 */
export function formatHistoryTime(iso: string | null | undefined, now: Date = new Date()): string {
  if (!iso) return '';
  const then = new Date(iso);
  const ts = then.getTime();
  if (Number.isNaN(ts)) return '';
  const diffMs = now.getTime() - ts;
  if (diffMs < 0) return '刚刚';
  const mins = Math.floor(diffMs / 60000);
  if (mins < 1) return '刚刚';
  if (mins < 60) return `${mins} 分钟前`;
  const hours = Math.floor(mins / 60);
  if (hours < 24 && startOfDay(then) === startOfDay(now)) return `${hours} 小时前`;
  const dayGap = Math.round((startOfDay(now) - startOfDay(then)) / 86400000);
  if (dayGap === 1) return `昨天 ${pad2(then.getHours())}:${pad2(then.getMinutes())}`;
  if (then.getFullYear() === now.getFullYear()) return `${then.getMonth() + 1}月${then.getDate()}日`;
  return `${then.getFullYear()}年${then.getMonth() + 1}月${then.getDate()}日`;
}

/** mm:ss or h:mm:ss. */
export function formatClock(sec: number): string {
  const total = Math.max(0, Math.floor(Number.isFinite(sec) ? sec : 0));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  return h > 0 ? `${h}:${pad2(m)}:${pad2(s)}` : `${pad2(m)}:${pad2(s)}`;
}

/** "看到 12:30" when progress was recorded, otherwise an empty string. */
export function formatProgress(sec: number | null | undefined): string {
  if (sec == null || !Number.isFinite(sec) || sec <= 0) return '';
  return `看到 ${formatClock(sec)}`;
}

// ——— Favorites ———

export const DEFAULT_FOLDER_NAME = '默认收藏';

export type FolderItem = { id: number; name: string; count: number; isDefault: boolean };

/** Default folder first, then by creation order; guarantees at least 默认收藏. */
export function normalizeFolders(folders: FavoriteFolder[] | null | undefined): FolderItem[] {
  const rows = (folders ?? [])
    .slice()
    .sort((a, b) => Number(b.is_default) - Number(a.is_default) || a.id - b.id)
    .map((f) => ({
      id: f.id,
      name: (f.name ?? '').trim() || DEFAULT_FOLDER_NAME,
      count: Math.max(0, Number(f.count) || 0),
      isDefault: Boolean(f.is_default),
    }));
  if (rows.length === 0) return [{ id: 0, name: DEFAULT_FOLDER_NAME, count: 0, isDefault: true }];
  return rows;
}

export type PreviewItem = {
  id: number;
  title: string;
  cover: string;
  href: string;
  /** Secondary line (history: time and progress; favorites: favorited time). */
  sub: string;
};

export const PREVIEW_LIMIT = 8;

export function favoritePreviewItems(
  slice: FavoriteVideo[] | null | undefined,
  now: Date = new Date(),
  limit: number = PREVIEW_LIMIT,
): PreviewItem[] {
  return (slice ?? []).slice(0, limit).map((v) => ({
    id: v.id,
    title: v.title,
    cover: v.cover_url,
    href: `/watch/${v.id}`,
    sub: formatHistoryTime(v.favorited_at, now),
  }));
}

export function historyPreviewItems(
  slice: HistoryVideo[] | null | undefined,
  now: Date = new Date(),
  limit: number = PREVIEW_LIMIT,
): PreviewItem[] {
  return (slice ?? []).slice(0, limit).map((v) => ({
    id: v.id,
    title: v.title,
    cover: v.cover_url,
    href: `/watch/${v.id}`,
    sub: [formatHistoryTime(v.last_watched_at, now), formatProgress(v.progress_sec)]
      .filter(Boolean)
      .join(' · '),
  }));
}

export type PreviewState = 'loading' | 'error' | 'empty' | 'ready';

export function previewState(loading: boolean, error: boolean, count: number): PreviewState {
  if (loading && count === 0) return 'loading';
  if (error && count === 0) return 'error';
  return count === 0 ? 'empty' : 'ready';
}

// ——— Account stats ———

export function accountStats(summary: MeSummary | null | undefined): { label: string; value: string }[] {
  return [
    { label: '收藏', value: summary ? String(summary.favorites) : '—' },
    { label: '历史', value: summary ? String(summary.history) : '—' },
  ];
}

// ——— Messages ———

export type MessageFilter = 'all' | 'search' | NotificationKind;

export const MESSAGE_TABS: { id: MessageFilter; label: string; kind?: NotificationKind }[] = [
  { id: 'all', label: '我的消息' },
  { id: 'search', label: '查找用户' },
  { id: 'system', label: '系统通知', kind: 'system' },
];

export function parseMessageFilter(raw: string | null | undefined): MessageFilter {
  if (
    raw === 'search' ||
    raw === 'system' ||
    raw === 'direct_message' ||
    raw === 'direct_message_sent' ||
    raw === 'comment_reply'
  ) {
    return raw;
  }
  return 'all';
}

export function messageKindLabel(kind: string): string {
  if (kind === 'comment_reply') return '评论回复';
  if (kind === 'system') return '系统通知';
  if (kind === 'direct_message') return '私信';
  if (kind === 'direct_message_sent') return '已发送';
  return '消息';
}

/** Where a notification leads, or null when it has no destination. */
export function notificationHref(n: Pick<UserNotification, 'video_id'>): string | null {
  return n.video_id != null && n.video_id > 0 ? `/watch/${n.video_id}` : null;
}

export function emptyMessagesText(filter: MessageFilter): string {
  if (filter === 'search') return '输入用户 ID、邮箱或昵称查找并发送私信';
  if (filter === 'comment_reply') return '还没有人回复你的评论';
  if (filter === 'direct_message') return '暂无私信';
  if (filter === 'system') return '暂无系统通知';
  return '暂无消息';
}

/** Header 消息 flyout menu. */
export const MESSAGE_MENU = [
  { id: 'search', label: '查找用户', href: '/messages?kind=search' },
  { id: 'system', label: '系统通知', href: '/messages?kind=system' },
  { id: 'all', label: '我的消息', href: '/messages' },
] as const;
