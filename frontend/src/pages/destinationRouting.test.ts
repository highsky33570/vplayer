import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import {
  DESTINATION_PATHS,
  headerDestinationActive,
  headerShellMode,
  isDestinationPath,
} from '../components/headerFlyoutLogic.ts';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..');
const app = readFileSync(join(root, 'App.tsx'), 'utf8');
const header = readFileSync(join(root, 'components/Header.tsx'), 'utf8');
const home = readFileSync(join(root, 'pages/Home.tsx'), 'utf8');
const vip = readFileSync(join(root, 'pages/VipPage.tsx'), 'utf8');
const messages = readFileSync(join(root, 'pages/MessagesPage.tsx'), 'utf8');
const favorites = readFileSync(join(root, 'pages/FavoritesPage.tsx'), 'utf8');
const history = readFileSync(join(root, 'pages/HistoryPage.tsx'), 'utf8');
const account = readFileSync(join(root, 'pages/AccountPage.tsx'), 'utf8');

const CLICK_NAV = [
  { label: 'account', path: '/account' },
  { label: '大会员', path: '/vip' },
  { label: '消息', path: '/messages' },
  { label: '收藏', path: '/favorites' },
  { label: '历史', path: '/history' },
] as const;

describe('destination shell mode', () => {
  it('keeps cinematic banner shell only on homepage', () => {
    assert.equal(headerShellMode('/'), 'home');
    for (const path of DESTINATION_PATHS) {
      assert.equal(headerShellMode(path), 'app');
    }
    assert.equal(headerShellMode('/watch/1'), 'app');
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

  it('flags destination paths for homepage chrome exclusion', () => {
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

describe('header click navigates to destination pathnames', () => {
  it('wires avatar and right-header actions to navigate(pathname)', () => {
    assert.match(header, /navigate\('\/account'\)/);
    assert.match(header, /goOrLogin\(VIP_PANEL\.href/);
    assert.match(header, /goOrLogin\('\/messages'\)/);
    assert.match(header, /goOrLogin\('\/favorites'\)/);
    assert.match(header, /goOrLogin\('\/history'\)/);
    assert.equal(
      readFileSync(join(root, 'components/headerFlyoutLogic.ts'), 'utf8').includes("href: '/vip'"),
      true,
    );
  });

  it('documents the expected click → pathname mapping', () => {
    for (const item of CLICK_NAV) {
      assert.ok(item.path.startsWith('/'));
      assert.equal(headerShellMode(item.path), 'app');
      assert.ok(headerDestinationActive(item.path));
    }
  });

  it('closes the open flyout before navigating so click is not swallowed', () => {
    assert.match(header, /setOpenFlyout\(null\)/);
    assert.match(header, /hideFlyout\('account'\)/);
    assert.match(header, /onClick=\{\(\) => \{\s*hideFlyout\('account'\);\s*navigate\('\/account'\);/);
  });
});

describe('homepage-only chrome stays off destination pages', () => {
  it('renders BannerCarousel only in home shell mode', () => {
    assert.match(header, /isHomeShell \? <BannerCarousel \/> : null/);
    assert.match(header, /headerShellMode\(pathname\)/);
    assert.equal(vip.includes('BannerCarousel'), false);
    assert.equal(messages.includes('BannerCarousel'), false);
    assert.equal(favorites.includes('BannerCarousel'), false);
    assert.equal(history.includes('BannerCarousel'), false);
    assert.equal(account.includes('BannerCarousel'), false);
  });

  it('keeps DiscoveryNav on Home only', () => {
    assert.match(home, /<DiscoveryNav/);
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
  it('vip uses a full-bleed hero stage', () => {
    assert.match(vip, /vip-dest/);
    assert.match(vip, /vip-hero-stage/);
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

  it('favorites uses library sidebar + folder main', () => {
    assert.match(favorites, /lib-dest/);
    assert.match(favorites, /lib-banner/);
    assert.match(favorites, /全部收藏/);
    assert.match(favorites, /me-folders/);
  });

  it('history uses dense grouped grid destination', () => {
    assert.match(history, /history-dest/);
    assert.match(history, /history-title/);
    assert.match(history, /history-grid/);
    assert.match(history, /清空历史/);
  });
});
