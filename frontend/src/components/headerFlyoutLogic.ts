/** Desktop header flyouts backed by real VPlayer data or pages. */
export const HEADER_FLYOUTS = ['account', 'vip', 'messages', 'favorites', 'history'] as const;

export type HeaderFlyoutId = (typeof HEADER_FLYOUTS)[number];

/**
 * Consumer destinations. Most share the homepage cinematic banner;
 * /messages and /history use compact app chrome (see usesCinematicBanner).
 */
export const DESTINATION_PATHS = ['/account', '/vip', '/messages', '/favorites', '/history'] as const;

export function isDestinationPath(pathname: string): boolean {
  return DESTINATION_PATHS.some((p) => pathname === p || pathname.startsWith(`${p}/`));
}

const COMPACT_SHELL_PATHS = ['/messages', '/history'] as const;

/**
 * Homepage + account/vip/favorites keep BannerCarousel.
 * /messages and /history are workspace pages: compact global top bar only.
 */
export function usesCinematicBanner(pathname: string): boolean {
  return !COMPACT_SHELL_PATHS.some((p) => pathname === p || pathname.startsWith(`${p}/`));
}

/** Which header action matches the current destination route (for active styling). */
export function headerDestinationActive(pathname: string): HeaderFlyoutId | null {
  if (pathname === '/account' || pathname.startsWith('/account/')) return 'account';
  if (pathname === '/vip' || pathname.startsWith('/vip/')) return 'vip';
  if (pathname === '/messages' || pathname.startsWith('/messages/')) return 'messages';
  if (pathname === '/favorites' || pathname.startsWith('/favorites/')) return 'favorites';
  if (pathname === '/history' || pathname.startsWith('/history/')) return 'history';
  return null;
}

/** Desktop header destinations open in a new browser tab. */
export const HEADER_NEW_TAB_REL = 'noopener noreferrer';

export const FLYOUT_OPEN_DELAY_MS = 80;
export const FLYOUT_CLOSE_DELAY_MS = 180;

/** CSS modifier class for a flyout panel size variant. */
export function flyoutPanelClass(id: HeaderFlyoutId): string {
  return `header-flyout header-flyout--${id}`;
}

/**
 * Header previews with no VPlayer page, API, or stored data.
 * These stay simple direct actions and do not grow a flyout.
 * 动态 was removed entirely — do not restore a feed preview.
 */
export const OMITTED_HEADER_PREVIEWS = [
  { id: 'creator', reason: 'no creator dashboard' },
  { id: 'upload', reason: 'no member upload manager' },
] as const;

/** 大会员 / VIP蓝光影院: truthful copy, no invented paid benefits. Click opens /vip. */
export const VIP_PANEL = {
  title: '大会员',
  subtitle: 'VIP蓝光影院',
  body: 'VPlayer 没有付费会员或会员等级。VIP蓝光影院是站内的高质量片库频道，登录后即可浏览观看。',
  bullets: [
    '站内精选的 VIP蓝光 片库频道',
    '按分类集中浏览，随时进入观看',
    '无需开通或付费，登录即可使用',
  ],
  href: '/vip',
  channelHref: '/?ch=vip-bluray',
  cta: '进入大会员页',
} as const;

export const ACCOUNT_UPLOAD_NOTE = '投稿管理 · 即将推出';

export type AccountAction =
  | { id: 'profile'; kind: 'link'; label: '个人中心'; href: '/account' }
  | { id: 'favorites'; kind: 'link'; label: '我的收藏'; href: '/favorites' }
  | { id: 'history'; kind: 'link'; label: '观看历史'; href: '/history' }
  | { id: 'messages'; kind: 'link'; label: '我的消息'; href: '/messages' }
  | { id: 'admin'; kind: 'link'; label: '管理后台'; href: '/admin' }
  | { id: 'theme'; kind: 'theme'; label: string }
  | { id: 'logout'; kind: 'logout'; label: '退出登录' };

export function accountDisplayName(nickname: string | null | undefined): string {
  const name = (nickname ?? '').trim();
  return name || '已登录';
}

export function accountStatusLabel(isAdmin: boolean | undefined): string {
  return isAdmin ? '管理员' : '普通用户';
}

export function accountActions(isAdmin: boolean | undefined, theme: 'light' | 'dark'): AccountAction[] {
  const actions: AccountAction[] = [
    { id: 'profile', kind: 'link', label: '个人中心', href: '/account' },
    { id: 'favorites', kind: 'link', label: '我的收藏', href: '/favorites' },
    { id: 'history', kind: 'link', label: '观看历史', href: '/history' },
    { id: 'messages', kind: 'link', label: '我的消息', href: '/messages' },
  ];
  if (isAdmin) actions.push({ id: 'admin', kind: 'link', label: '管理后台', href: '/admin' });
  actions.push({ id: 'theme', kind: 'theme', label: theme === 'dark' ? '切换到浅色' : '切换到深色' });
  actions.push({ id: 'logout', kind: 'logout', label: '退出登录' });
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

/** Flyouts that need data fetched when they open. */
export function flyoutNeedsData(id: HeaderFlyoutId): boolean {
  return id === 'account' || id === 'messages' || id === 'favorites' || id === 'history';
}
