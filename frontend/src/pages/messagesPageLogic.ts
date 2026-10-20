import type { NotificationKind, UserNotification } from '../api';

export type MessageFilter = 'all' | NotificationKind;

/** Truthful category tabs — only kinds VPlayer actually stores. */
export const MESSAGE_CENTER_TABS = [
  { id: 'all' as const, label: '我的消息' },
  { id: 'comment_reply' as const, label: '回复我的' },
  { id: 'system' as const, label: '系统通知' },
] as const;

function kindLabel(kind: string): string {
  if (kind === 'comment_reply') return '回复我的';
  if (kind === 'system') return '系统通知';
  return '消息';
}

export function messageListPrimary(item: UserNotification): string {
  if (item.kind === 'comment_reply') {
    return (item.actor_name ?? '').trim() || kindLabel(item.kind);
  }
  return (item.title ?? '').trim() || kindLabel(item.kind);
}

export function messageListPreview(item: UserNotification): string {
  const body = (item.body ?? '').trim();
  if (body) return body;
  if (item.kind === 'comment_reply') return (item.title ?? '').trim();
  return '';
}

export function messageDetailHeading(item: UserNotification): string {
  return kindLabel(item.kind);
}

export function messageWatchHref(item: Pick<UserNotification, 'video_id'>): string | null {
  return item.video_id != null && item.video_id > 0 ? `/watch/${item.video_id}` : null;
}

export function emptyDetailCopy(): string {
  return '选择一条消息查看详情';
}

/** Empty-list copy — never invent placeholder notifications. */
export function emptyListCopy(filter: MessageFilter): string {
  if (filter === 'comment_reply') return '还没有人回复你的评论';
  if (filter === 'system') return '暂无系统通知';
  return '暂无消息';
}

/** Messages workspace must not ship a send/DM composer. */
export function messagesPageHasComposer(source: string): boolean {
  const markers = ['msg-composer', 'textarea', 'contenteditable', 'fake-dm'];
  return markers.some((m) => source.includes(m));
}
