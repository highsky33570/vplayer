import type { NotificationKind, UserNotification } from '../api';

export type MessageFilter = 'all' | 'search' | NotificationKind;

/** Truthful category tabs — list kinds + search/compose workspace. */
export const MESSAGE_CENTER_TABS = [
  { id: 'all' as const, label: '我的消息' },
  { id: 'search' as const, label: '查找用户' },
  { id: 'system' as const, label: '系统通知' },
] as const;

function kindLabel(kind: string): string {
  if (kind === 'comment_reply') return '评论回复';
  if (kind === 'system') return '系统通知';
  if (kind === 'direct_message') return '私信';
  return '消息';
}

export function messageListPrimary(item: UserNotification): string {
  if (item.kind === 'comment_reply' || item.kind === 'direct_message') {
    return (item.actor_name ?? '').trim() || kindLabel(item.kind);
  }
  return (item.title ?? '').trim() || kindLabel(item.kind);
}

export function messageListPreview(item: UserNotification): string {
  const body = (item.body ?? '').trim();
  if (body) return body;
  if (item.kind === 'comment_reply' || item.kind === 'direct_message') {
    return (item.title ?? '').trim();
  }
  return '';
}

export function messageDetailHeading(item: UserNotification): string {
  return kindLabel(item.kind);
}

export function messageWatchHref(item: Pick<UserNotification, 'video_id'>): string | null {
  return item.video_id != null && item.video_id > 0 ? `/watch/${item.video_id}` : null;
}

export function emptyDetailCopy(filter: MessageFilter = 'all'): string {
  if (filter === 'search') return '查找用户后，在此编辑并发送私信';
  return '选择一条消息查看详情';
}

/** Empty-list copy — never invent placeholder notifications. */
export function emptyListCopy(filter: MessageFilter): string {
  if (filter === 'search') return '输入用户 ID 查找并发送私信';
  if (filter === 'comment_reply') return '还没有人回复你的评论';
  if (filter === 'direct_message') return '暂无私信';
  if (filter === 'system') return '暂无系统通知';
  return '暂无消息';
}

/** Composer markers used by the search/DM workspace. */
export function messagesPageHasComposer(source: string): boolean {
  const markers = ['msg-composer', 'msg-search-pane'];
  return markers.some((m) => source.includes(m));
}
