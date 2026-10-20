import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import {
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
  openHeaderFlyout,
  shouldEnlargeAvatar,
} from './headerFlyoutLogic.ts';

const here = dirname(fileURLToPath(import.meta.url));
const header = readFileSync(join(here, 'Header.tsx'), 'utf8');
const flyout = readFileSync(join(here, 'HeaderFlyout.tsx'), 'utf8');
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
    assert.equal(closeHeaderFlyoutIfCurrent('vip', 'account'), 'vip');
    assert.equal(closeHeaderFlyoutIfCurrent('vip', 'vip'), null);
  });

  it('closes on Escape and outside pointer by clearing the current id', () => {
    assert.equal(closeHeaderFlyoutIfCurrent('account', 'account'), null);
    assert.equal(closeHeaderFlyoutIfCurrent('vip', 'vip'), null);
  });

  it('uses a short open delay and a longer close delay', () => {
    assert.ok(FLYOUT_OPEN_DELAY_MS > 0 && FLYOUT_OPEN_DELAY_MS <= 120);
    assert.ok(FLYOUT_CLOSE_DELAY_MS > FLYOUT_OPEN_DELAY_MS);
  });
});

describe('who can open a flyout', () => {
  it('requires a logged-in desktop user', () => {
    assert.equal(canUseFlyout(true, true, 'account'), true);
    assert.equal(canUseFlyout(true, true, 'vip'), true);
    assert.equal(canUseFlyout(false, true, 'account'), false);
    assert.equal(canUseFlyout(true, false, 'vip'), false);
    assert.equal(canUseFlyout(true, true, 'messages'), false);
    assert.equal(canUseFlyout(true, true, 'favorites'), false);
    assert.equal(canUseFlyout(true, true, 'history'), false);
  });

  it('builds a truthful account card without invented stats', () => {
    assert.equal(accountDisplayName('  Ada  '), 'Ada');
    assert.equal(accountDisplayName(' '), '已登录');
    assert.equal(accountStatusLabel(true), '管理员');
    assert.equal(accountStatusLabel(false), '普通用户');
    assert.deepEqual(
      accountActions(true, 'dark').map((action) => action.id),
      ['admin', 'theme', 'logout'],
    );
    assert.equal(accountActions(false, 'light').some((action) => action.id === 'admin'), false);
    assert.equal(accountActions(false, 'light').find((action) => action.id === 'theme')?.label, '切换到深色');
  });
});

describe('omitted previews', () => {
  it('does not invent messages, favorites, history, or creator panels', () => {
    const omitted = new Set(OMITTED_HEADER_PREVIEWS.map((item) => item.id));
    assert.equal(omitted.has('messages'), true);
    assert.equal(omitted.has('favorites'), true);
    assert.equal(omitted.has('history'), true);
    assert.deepEqual(HEADER_FLYOUTS, ['account', 'vip']);
    assert.match(VIP_PANEL.body, /没有会员等级/);
    assert.equal(VIP_PANEL.href, '/?ch=vip-bluray');
    for (const phrase of ['粉丝', '关注', '硬币', '经验', '免广告', '4K', '专属装扮']) {
      assert.equal(header.includes(phrase), false);
      assert.equal(VIP_PANEL.body.includes(phrase), false);
    }
  });

  it('wires account and VIP flyouts and leaves the other header actions direct', () => {
    assert.match(header, /id="account"/);
    assert.match(header, /id="vip"/);
    assert.match(header, /HeaderFlyout/);
    assert.match(flyout, /Escape/);
    assert.match(flyout, /pointerdown/);
    assert.match(flyout, /onMouseEnter/);
    assert.match(flyout, /onMouseLeave/);
    assert.match(header, /aria-haspopup="dialog"/);
    assert.match(header, /aria-expanded=/);
    assert.match(header, /onClick=\{logout\}/);
    assert.match(header, /toggleTheme/);
    assert.equal(header.includes('header-flyout--messages'), false);
    assert.equal(header.includes('header-flyout--favorites'), false);
    assert.equal(header.includes('header-flyout--history'), false);
    assert.match(header, /label="消息"/);
    assert.match(header, /label="收藏"/);
    assert.match(header, /label="历史"/);
    assert.match(css, /prefers-reduced-motion: reduce/);
    assert.match(css, /\.is-enlarged \.banner-avatar/);
    assert.match(css, /transform-origin:\s*center bottom/);
    assert.match(css, /@media \(min-width: 1024px\)/);
    const phone = css.slice(css.indexOf('/* bp:phone'), css.indexOf('/* bp:phone-narrow'));
    assert.equal(/header-flyout-anchor/.test(phone), false);
  });
});

describe('desktop homepage gutter', () => {
  it('keeps mid-desktop breathing room and widens large-desktop gutters to ~56px/side', () => {
    const desk = css.slice(css.indexOf('/* Desktop subcategory rail'), css.indexOf('/* bp:compact'));
    assert.match(desk, /width:\s*min\(2200px,\s*calc\(100% - 64px\)\)/);
    assert.match(desk, /@media \(min-width: 1280px\)/);
    assert.match(desk, /width:\s*min\(2200px,\s*calc\(100% - 112px\)\)/);
    assert.match(desk, /\.page:not\(\.watch\):not\(\.auth-page\)/);
    assert.match(desk, /\.desktop-rail/);
    assert.match(desk, /\.category-rail/);
  });
});
