import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import {
  DESTINATION_PATHS,
  HEADER_NEW_TAB_REL,
  headerDestinationActive,
  isDestinationPath,
} from '../components/headerFlyoutLogic.ts';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..');
const app = readFileSync(join(root, 'App.tsx'), 'utf8');
const header = readFileSync(join(root, 'components/Header.tsx'), 'utf8');
const avatar = readFileSync(join(root, 'components/HeaderAccountAvatar.tsx'), 'utf8');
const home = readFileSync(join(root, 'pages/Home.tsx'), 'utf8');
const vip = readFileSync(join(root, 'pages/VipPage.tsx'), 'utf8');
const messages = readFileSync(join(root, 'pages/MessagesPage.tsx'), 'utf8');
const favorites = readFileSync(join(root, 'pages/FavoritesPage.tsx'), 'utf8');
const history = readFileSync(join(root, 'pages/HistoryPage.tsx'), 'utf8');
const account = readFileSync(join(root, 'pages/AccountPage.tsx'), 'utf8');
const css = readFileSync(join(root, 'styles.css'), 'utf8');

const NEW_TAB_LINKS = [
  { label: 'avatar', href: '/account', source: avatar },
  { label: '大会员', href: '/vip', source: header },
  { label: '消息', href: '/messages', source: header },
  { label: '收藏', href: '/favorites', source: header },
  { label: '历史', href: '/history', source: header },
] as const;

describe('shared homepage banner on destinations', () => {
  it('uses the shared banner shell, compact on messages and history', () => {
    assert.match(header, /usesCinematicBanner\(pathname\)/);
    assert.match(header, /<BannerCarousel /);
    assert.match(header, /banner-shell\$\{cinematic \? '' : ' banner-shell--app'\}/);
    assert.match(header, /banner-shell--app/);
    assert.match(css, /\.banner-shell--app/);
    assert.equal(header.includes('headerShellMode'), false);
    assert.equal(header.includes('isHomeShell'), false);
    assert.equal(vip.includes('BannerCarousel'), false);
    assert.equal(messages.includes('BannerCarousel'), false);
    assert.equal(favorites.includes('BannerCarousel'), false);
    assert.equal(history.includes('BannerCarousel'), false);
    assert.equal(account.includes('BannerCarousel'), false);
  });

  it('maps each destination pathname to its header active id', () => {
    assert.equal(headerDestinationActive('/account'), 'account');
    assert.equal(headerDestinationActive('/vip'), 'vip');
    assert.equal(headerDestinationActive('/messages'), 'messages');
    assert.equal(headerDestinationActive('/favorites'), 'favorites');
    assert.equal(headerDestinationActive('/history'), 'history');
    assert.equal(headerDestinationActive('/'), null);
    assert.equal(headerDestinationActive('/watch/9'), null);
  });

  it('flags destination paths without changing the shared top shell', () => {
    for (const path of DESTINATION_PATHS) {
      assert.equal(isDestinationPath(path), true);
    }
    assert.equal(isDestinationPath('/'), false);
    assert.equal(isDestinationPath('/watch/1'), false);
  });
});

describe('router registers real destination routes', () => {
  it('declares dedicated Route elements for each destination', () => {
    assert.match(app, /path="\/account"/);
    assert.match(app, /path="\/vip"/);
    assert.match(app, /path="\/messages"/);
    assert.match(app, /path="\/favorites"/);
    assert.match(app, /path="\/history"/);
    assert.match(app, /<Route path="\/" element=\{<Home \/>\}/);
  });
});

describe('desktop top-right destinations open in a new browser tab', () => {
  it('renders real anchors with href, target=_blank, and noopener noreferrer', () => {
    assert.equal(HEADER_NEW_TAB_REL, 'noopener noreferrer');
    assert.match(avatar, /href="\/account"/);
    assert.match(avatar, /target="_blank"/);
    assert.match(avatar, /rel="noopener noreferrer"/);
    assert.match(avatar, /data-testid="header-account-avatar"/);
    assert.match(avatar, /matchMedia\('\(max-width: 767px\)'\)/);
    assert.match(avatar, /navigate\('\/account'\)/);
    assert.equal(avatar.includes('to="/account"'), false);

    assert.match(header, /function DestNavLink/);
    assert.match(header, /href=\{VIP_PANEL\.href\}/);
    assert.match(header, /href="\/messages"/);
    assert.match(header, /href="\/favorites"/);
    assert.match(header, /href="\/history"/);
    assert.match(header, /target="_blank"/);
    assert.match(header, /rel=\{HEADER_NEW_TAB_REL\}/);
    // Desktop top-right uses DestNavLink (new tab); mobile drawer may still goOrLogin same-tab.
    assert.match(header, /<DestNavLink[\s\S]*href=\{VIP_PANEL\.href\}/);
    assert.match(header, /<DestNavLink[\s\S]*href="\/messages"/);
    assert.match(header, /<DestNavLink[\s\S]*href="\/favorites"/);
    assert.match(header, /<DestNavLink[\s\S]*href="\/history"/);
  });

  it('documents the expected new-tab href mapping', () => {
    assert.match(avatar, /href="\/account"/);
    assert.match(header, /VIP_PANEL\.href/);
    assert.match(header, /href="\/messages"/);
    assert.match(header, /href="\/favorites"/);
    assert.match(header, /href="\/history"/);
    for (const item of NEW_TAB_LINKS) {
      assert.ok(item.href.startsWith('/'));
      assert.ok(headerDestinationActive(item.href));
    }
  });

  it('closes the open flyout on avatar click while the anchor owns new-tab navigation', () => {
    assert.match(header, /setOpenFlyout\(null\)/);
    assert.match(header, /hideFlyout\('account'\)/);
    assert.match(avatar, /href="\/account"/);
    assert.match(css, /\.header-flyout--account[\s\S]*pointer-events:\s*none/);
  });
});

describe('homepage DiscoveryNav stays on Home only', () => {
  it('keeps the channel bar in the global header, not inside destination pages', () => {
    assert.match(header, /<DiscoveryNav/);
    assert.equal(/<DiscoveryNav/.test(home), false);
    assert.equal(vip.includes('DiscoveryNav'), false);
    assert.equal(messages.includes('DiscoveryNav'), false);
    assert.equal(favorites.includes('DiscoveryNav'), false);
    assert.equal(history.includes('DiscoveryNav'), false);
    assert.equal(account.includes('DiscoveryNav'), false);
  });

  it('does not embed destination bodies inside Home', () => {
    assert.equal(home.includes('VipPage'), false);
    assert.equal(home.includes('MessagesPage'), false);
    assert.equal(home.includes('FavoritesPage'), false);
    assert.equal(home.includes('HistoryPage'), false);
    assert.equal(home.includes('AccountPage'), false);
  });
});

describe('destination main content reuses homepage gutters', () => {
  it('shares .page width tokens with destination mains (messages uses its own centered workspace)', () => {
    const desk = css.slice(css.indexOf('/* Desktop subcategory rail'), css.indexOf('/* bp:compact'));
    assert.match(desk, /width:\s*min\(2200px,\s*calc\(100% - 64px\)\)/);
    assert.match(desk, /@media \(min-width: 1280px\)/);
    assert.match(desk, /width:\s*min\(2200px,\s*calc\(100% - 112px\)\)/);
    assert.match(desk, /\.page:not\(\.watch\):not\(\.auth-page\)/);
    assert.match(desk, /\.vip-dest-body/);
    assert.match(account, /className="page space-page"/);
    assert.match(messages, /msg-dest/);
    assert.match(css, /min\(1160px/);
    assert.match(css, /min\(1240px/);
    assert.match(favorites, /fav-dest/);
    assert.match(css, /min\(1280px/);
    assert.match(history, /hist-dest/);
    assert.equal(account.includes('space-cover'), false);
    assert.match(account, /space-identity/);
  });
});

describe('动态 is removed', () => {
  it('has no 动态 label or feed destination in header and destinations', () => {
    assert.equal(header.includes('动态'), false);
    assert.equal(app.includes('动态'), false);
    assert.equal(app.includes('/feed'), false);
    assert.equal(messages.includes('动态'), false);
    assert.equal(favorites.includes('动态'), false);
  });
});

describe('destination page composition markers', () => {
  it('vip uses a full-bleed hero stage under the shared banner', () => {
    assert.match(vip, /vip-dest/);
    assert.match(vip, /vip-hero-stage/);
    assert.match(vip, /vip-hero-body/);
    assert.match(vip, /vip-status-card/);
    assert.match(vip, /vip-featured-overlap/);
  });

  it('messages uses a three-column workspace', () => {
    const tabs = readFileSync(join(root, 'components/headerFlyoutData.ts'), 'utf8');
    assert.match(messages, /msg-dest/);
    assert.match(messages, /msg-sidebar/);
    assert.match(messages, /msg-list-pane/);
    assert.match(messages, /msg-detail-pane/);
    assert.match(messages, /MESSAGE_TABS/);
    assert.match(messages, /系统通知/);
    assert.match(tabs, /label: '我的消息'/);
    assert.match(tabs, /label: '回复我的'/);
    assert.match(tabs, /label: '系统通知'/);
  });

  it('favorites uses centered library sidebar + folder main under cinematic shell', () => {
    assert.match(favorites, /fav-dest/);
    assert.match(favorites, /fav-sidebar/);
    assert.match(favorites, /全部收藏/);
    assert.match(favorites, /fav-grid/);
  });

  it('history uses centered timeline workspace destination', () => {
    assert.match(history, /hist-dest/);
    assert.match(history, /hist-title/);
    assert.match(history, /hist-grid/);
    assert.match(history, /清空历史/);
    assert.match(history, /批量管理/);
  });
});
