/** Desktop header flyouts that VPlayer can support with existing data. */
export const HEADER_FLYOUTS = ['account', 'vip'] as const;

export type HeaderFlyoutId = (typeof HEADER_FLYOUTS)[number];

export const FLYOUT_OPEN_DELAY_MS = 80;
export const FLYOUT_CLOSE_DELAY_MS = 180;

/**
 * Screenshot previews with no VPlayer page, API, or stored data.
 * These header buttons stay direct actions and do not grow a flyout.
 */
export const OMITTED_HEADER_PREVIEWS = [
  { id: 'messages', reason: 'no message API, destinations, or unread count' },
  { id: 'favorites', reason: 'no favorites API, folders, or saved videos' },
  { id: 'history', reason: 'no watch-history API, progress, or history page' },
  { id: 'feed', reason: 'no feed page or preview items' },
  { id: 'creator', reason: 'no creator dashboard' },
  { id: 'upload', reason: 'no member upload manager' },
] as const;

export const VIP_PANEL = {
  title: 'VIP蓝光影院',
  body: 'VPlayer 没有会员等级或付费权益。此入口只打开现有的 VIP 蓝光分类。',
  href: '/?ch=vip-bluray',
} as const;

export type AccountAction =
  | { id: 'admin'; label: '管理后台'; href: '/admin' }
  | { id: 'theme'; label: string }
  | { id: 'logout'; label: '退出登录' };

export function accountDisplayName(nickname: string | null | undefined): string {
  const name = (nickname ?? '').trim();
  return name || '已登录';
}

export function accountStatusLabel(isAdmin: boolean | undefined): string {
  return isAdmin ? '管理员' : '普通用户';
}

export function accountActions(isAdmin: boolean | undefined, theme: 'light' | 'dark'): AccountAction[] {
  const actions: AccountAction[] = [];
  if (isAdmin) actions.push({ id: 'admin', label: '管理后台', href: '/admin' });
  actions.push({ id: 'theme', label: theme === 'dark' ? '切换到浅色' : '切换到深色' });
  actions.push({ id: 'logout', label: '退出登录' });
  return actions;
}

export function canUseFlyout(desktop: boolean, authenticated: boolean, id: string): boolean {
  if (!desktop || !authenticated) return false;
  return (HEADER_FLYOUTS as readonly string[]).includes(id);
}

/** Opening one flyout replaces any other. */
export function openHeaderFlyout(_current: HeaderFlyoutId | null, next: HeaderFlyoutId): HeaderFlyoutId {
  return next;
}

/** A delayed close must not dismiss a flyout the pointer has since moved into. */
export function closeHeaderFlyoutIfCurrent(
  current: HeaderFlyoutId | null,
  id: HeaderFlyoutId,
): HeaderFlyoutId | null {
  return current === id ? null : current;
}

export type HoverZone = 'trigger' | 'panel' | 'outside';

/** Moving from the trigger into its panel stays inside the same anchor. */
export function flyoutAfterPointer(
  current: HeaderFlyoutId | null,
  id: HeaderFlyoutId,
  zone: HoverZone,
): HeaderFlyoutId | null {
  if (zone === 'trigger' || zone === 'panel') return id;
  return closeHeaderFlyoutIfCurrent(current, id);
}

export function shouldEnlargeAvatar(open: HeaderFlyoutId | null, reducedMotion: boolean): boolean {
  return open === 'account' && !reducedMotion;
}
