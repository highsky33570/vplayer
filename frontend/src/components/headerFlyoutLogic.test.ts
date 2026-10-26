import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import {
  DESTINATION_PATHS,
  FLYOUT_CLOSE_DELAY_MS,
  FLYOUT_OPEN_DELAY_MS,
  HEADER_FLYOUTS,
  OMITTED_HEADER_PREVIEWS,
  VIP_PANEL,
  accountActions,
  accountDisplayName,
  accountStatusLabel,
  canUseFlyout,
  closeHeaderFlyoutIfCurrent,
  flyoutAfterPointer,
  flyoutPanelClass,
  HEADER_NEW_TAB_REL,
  headerDestinationActive,
  isDestinationPath,
  openHeaderFlyout,
  shouldEnlargeAvatar,
  usesCinematicBanner,
} from './headerFlyoutLogic.ts';
import {
  accountStats,
  favoritePreviewItems,
  formatHistoryTime,
  formatProgress,
  historyPreviewItems,
  normalizeFolders,
  parseMessageFilter,
  previewState,
  unreadBadgeLabel,
} from './headerFlyoutData.ts';

const here = dirname(fileURLToPath(import.meta.url));
const header = readFileSync(join(here, 'Header.tsx'), 'utf8');
const flyout = readFileSync(join(here, 'HeaderFlyout.tsx'), 'utf8');
const panels = readFileSync(join(here, 'HeaderFlyoutPanels.tsx'), 'utf8');
const css = readFileSync(join(here, '../styles.css'), 'utf8');

describe('header flyout state', () => {
  it('opens the account panel and enlarges the avatar only then', () => {
    assert.equal(openHeaderFlyout(null, 'account'), 'account');
    assert.equal(shouldEnlargeAvatar('account', false), true);
    assert.equal(shouldEnlargeAvatar('account', true), false);
    assert.equal(shouldEnlargeAvatar('vip', false), false);
    assert.equal(shouldEnlargeAvatar(null, false), false);
  });

  it('keeps the panel open while the pointer moves from trigger into the panel', () => {
    assert.equal(flyoutAfterPointer('account', 'account', 'trigger'), 'account');
    assert.equal(flyoutAfterPointer('account', 'account', 'panel'), 'account');
    assert.equal(flyoutAfterPointer('account', 'account', 'outside'), null);
  });

  it('lets only one flyout stay open when moving between header actions', () => {
    assert.equal(openHeaderFlyout('account', 'vip'), 'vip');
    assert.equal(openHeaderFlyout('vip', 'favorites'), 'favorites');
    assert.equal(closeHeaderFlyoutIfCurrent('favorites', 'history'), 'favorites');
    assert.equal(closeHeaderFlyoutIfCurrent('favorites', 'favorites'), null);
  });

  it('closes on Escape and outside pointer by clearing the current id', () => {
    assert.equal(closeHeaderFlyoutIfCurrent('account', 'account'), null);
    assert.equal(closeHeaderFlyoutIfCurrent('messages', 'messages'), null);
  });

  it('uses a short open delay and a longer close delay', () => {
    assert.ok(FLYOUT_OPEN_DELAY_MS > 0 && FLYOUT_OPEN_DELAY_MS <= 120);
    assert.ok(FLYOUT_CLOSE_DELAY_MS > FLYOUT_OPEN_DELAY_MS);
  });
});

describe('who can open a flyout', () => {
  it('requires a logged-in desktop user for all five flyouts', () => {
    for (const id of HEADER_FLYOUTS) {
      assert.equal(canUseFlyout(true, true, id), true);
      assert.equal(canUseFlyout(false, true, id), false);
      assert.equal(canUseFlyout(true, false, id), false);
    }
    assert.equal(canUseFlyout(true, true, 'feed'), false);
    assert.equal(canUseFlyout(true, true, 'creator'), false);
  });

  it('builds a truthful account card with real destinations and no invented stats', () => {
    assert.equal(accountDisplayName('  Ada  '), 'Ada');
    assert.equal(accountDisplayName(' '), '已登录');
    assert.equal(accountStatusLabel(true), '管理员');
    assert.equal(accountStatusLabel(false), '普通用户');
    assert.deepEqual(
      accountActions(true, 'dark').map((action) => action.id),
      ['profile', 'favorites', 'history', 'messages', 'admin', 'theme', 'logout'],
    );
    assert.equal(accountActions(false, 'light').some((action) => action.id === 'admin'), false);
    assert.equal(accountActions(false, 'light').find((action) => action.id === 'theme')?.label, '切换到深色');
  });
});

describe('destination shell helpers', () => {
  it('marks destination routes; cinematic banner everywhere except /messages and /history', () => {
    assert.equal(headerDestinationActive('/favorites'), 'favorites');
    assert.equal(isDestinationPath('/messages'), true);
    assert.equal(isDestinationPath('/'), false);
    assert.equal(usesCinematicBanner('/'), true);
    assert.equal(usesCinematicBanner('/account'), true);
    assert.equal(usesCinematicBanner('/messages'), false);
    assert.equal(usesCinematicBanner('/history'), false);
    assert.equal(HEADER_NEW_TAB_REL, 'noopener noreferrer');
    assert.deepEqual([...DESTINATION_PATHS], ['/account', '/vip', '/messages', '/favorites', '/history']);
  });
});

describe('wired flyouts', () => {
  it('exposes five real flyouts and omits creator/upload; 动态 is gone', () => {
    assert.deepEqual(HEADER_FLYOUTS, ['account', 'vip', 'messages', 'favorites', 'history']);
    const omitted = new Set(OMITTED_HEADER_PREVIEWS.map((item) => item.id));
    assert.equal(omitted.has('feed'), false);
    assert.equal(omitted.has('creator'), true);
    assert.equal(omitted.has('upload'), true);
    assert.match(VIP_PANEL.body, /没有付费会员/);
    assert.equal(VIP_PANEL.href, '/vip');
    assert.equal(header.includes('动态'), false);
    assert.equal(header.includes('FeedIcon'), false);
    for (const phrase of ['粉丝', '关注', '硬币', '经验', '免广告', '4K', '专属装扮', 'B币']) {
      assert.equal(header.includes(phrase), false);
      assert.equal(VIP_PANEL.body.includes(phrase), false);
      assert.equal(panels.includes(phrase), false);
    }
  });

  it('wires hover flyouts and new-tab destination anchors', () => {
    for (const id of HEADER_FLYOUTS) {
      assert.match(header, new RegExp(`id="${id}"`));
      assert.equal(flyoutPanelClass(id), `header-flyout header-flyout--${id}`);
    }
    assert.match(header, /HeaderAccountAvatar/);
    const avatarSrc = readFileSync(join(here, 'HeaderAccountAvatar.tsx'), 'utf8');
    assert.match(avatarSrc, /href="\/account"/);
    assert.match(avatarSrc, /target="_blank"/);
    assert.match(avatarSrc, /rel="noopener noreferrer"/);
    assert.match(header, /function DestNavLink/);
    assert.match(header, /href=\{VIP_PANEL\.href\}/);
    assert.match(header, /href="\/messages"/);
    assert.match(header, /href="\/favorites"/);
    assert.match(header, /href="\/history"/);
    assert.match(header, /target="_blank"/);
    assert.match(header, /rel=\{HEADER_NEW_TAB_REL\}/);
    assert.match(header, /label="大会员"/);
    assert.match(header, /label="消息"/);
    assert.match(header, /label="收藏"/);
    assert.match(header, /label="历史"/);
    assert.equal(header.includes('创作中心'), false);
    assert.match(header, /投稿/);
    assert.match(flyout, /Escape/);
    assert.match(flyout, /pointerdown/);
    assert.match(flyout, /onMouseEnter/);
    assert.match(css, /\.is-enlarged \.banner-avatar/);
    assert.match(css, /\.header-flyout--favorites/);
    assert.match(css, /\.vip-dest/);
    assert.match(css, /\.banner-shell--app/);
    assert.match(css, /\.msg-center-grid/);
    assert.match(css, /\.hist-grid/);
    assert.equal(header.includes('BannerCarousel'), false);
    assert.match(header, /banner-shell banner-shell--app/);
    assert.equal(header.includes('isHomeShell'), false);
    const phone = css.slice(css.indexOf('/* bp:phone'), css.indexOf('/* bp:phone-narrow'));
    assert.equal(/header-flyout-anchor/.test(phone), false);
  });
});

describe('preview and badge helpers', () => {
  it('formats unread badges, history times, and empty/populated previews', () => {
    assert.equal(unreadBadgeLabel(0), '');
    assert.equal(unreadBadgeLabel(3), '3');
    assert.equal(unreadBadgeLabel(100), '99+');

    const now = new Date('2026-10-20T12:00:00Z');
    assert.equal(formatHistoryTime('2026-10-20T11:59:30Z', now), '刚刚');
    assert.equal(formatHistoryTime('2026-10-20T11:30:00Z', now), '30 分钟前');
    assert.equal(formatProgress(0), '');
    assert.equal(formatProgress(90), '看到 01:30');

    assert.deepEqual(normalizeFolders([]), [{ id: 0, name: '默认收藏', count: 0, isDefault: true }]);
    assert.deepEqual(accountStats({ favorites: 2, history: 5, unread_notifications: 1 }), [
      { label: '收藏', value: '2' },
      { label: '历史', value: '5' },
    ]);
    assert.equal(previewState(true, false, 0), 'loading');
    assert.equal(previewState(false, false, 0), 'empty');
    assert.equal(previewState(false, false, 3), 'ready');
    assert.equal(parseMessageFilter('comment_reply'), 'comment_reply');
    assert.equal(parseMessageFilter('nope'), 'all');

    const fav = favoritePreviewItems([
      {
        id: 9,
        title: 'Film',
        cover_url: '/c.jpg',
        favorited_at: '2026-10-20T11:00:00Z',
      } as never,
    ], now);
    assert.equal(fav[0]?.href, '/watch/9');
    assert.equal(fav.length, 1);

    const hist = historyPreviewItems([
      {
        id: 3,
        title: 'Show',
        cover_url: '/h.jpg',
        last_watched_at: '2026-10-20T11:00:00Z',
        progress_sec: 65,
      } as never,
    ], now);
    assert.match(hist[0]?.sub ?? '', /看到/);
  });
});

describe('desktop homepage gutter', () => {
  it('uses 24px / 32px / 40px portal gutters instead of the cinematic inset', () => {
    const phase = css.slice(css.indexOf('/* ——— Phase 1'));
    assert.match(phase, /width:\s*min\(1840px,\s*calc\(100% - 48px\)\)/);
    assert.match(phase, /@media \(min-width: 1440px\)/);
    assert.match(phase, /width:\s*min\(1840px,\s*calc\(100% - 64px\)\)/);
    assert.match(phase, /@media \(min-width: 1920px\)/);
    assert.match(phase, /width:\s*min\(1840px,\s*calc\(100% - 80px\)\)/);
    assert.match(phase, /\.page:not\(\.watch\):not\(\.auth-page\)/);
    assert.match(phase, /\.category-rail/);
  });
});
