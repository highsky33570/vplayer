import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import { usesCinematicBanner } from '../components/headerFlyoutLogic.ts';
import {
  emptyDetailCopy,
  emptyListCopy,
  messageListPreview,
  messageListPrimary,
  messageWatchHref,
  messagesPageHasComposer,
} from './messagesPageLogic.ts';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..');
const header = readFileSync(join(root, 'components/Header.tsx'), 'utf8');
const messages = readFileSync(join(root, 'pages/MessagesPage.tsx'), 'utf8');
const logic = readFileSync(join(root, 'pages/messagesPageLogic.ts'), 'utf8');
const css = readFileSync(join(root, 'styles.css'), 'utf8');
const flyoutData = readFileSync(join(root, 'components/headerFlyoutData.ts'), 'utf8');

describe('messages shell — no cinematic banner', () => {
  it('A/B: /messages uses compact shell; BannerCarousel gated off', () => {
    assert.equal(usesCinematicBanner('/messages'), false);
    assert.equal(usesCinematicBanner('/messages/'), false);
    assert.equal(usesCinematicBanner('/'), true);
    assert.equal(usesCinematicBanner('/account'), false);
    assert.equal(usesCinematicBanner('/vip'), false);
    assert.equal(usesCinematicBanner('/favorites'), false);
    assert.equal(usesCinematicBanner('/history'), false);
    assert.match(header, /usesCinematicBanner\(pathname\)/);
    assert.match(header, /<BannerCarousel /);
    assert.match(header, /banner-shell\$\{cinematic \? '' : ' banner-shell--app'\}/);
    assert.match(header, /banner-shell--app/);
    assert.equal(messages.includes('BannerCarousel'), false);
    assert.equal(messages.includes('DiscoveryNav'), false);
  });

  it('C: centered workspace container (~1100–1200px), not homepage 2200px', () => {
    assert.match(messages, /data-testid="messages-workspace"/);
    assert.match(messages, /className=\{`msg-dest/);
    assert.match(css, /width:\s*min\(1160px/);
    assert.equal(messages.includes('className="page msg-dest"'), false);
  });
});

describe('messages categories and list/detail', () => {
  it('D/E/F: tabs — 我的消息 / 查找用户 / 系统通知', () => {
    assert.match(flyoutData, /label: '我的消息'/);
    assert.match(flyoutData, /label: '查找用户'/);
    assert.match(flyoutData, /label: '系统通知'/);
    assert.match(flyoutData, /id: 'search'/);
    assert.match(flyoutData, /kind: 'system'/);
    assert.equal(flyoutData.includes("label: '回复我的'"), false);
    assert.equal(flyoutData.includes('@我的'), false);
    assert.equal(flyoutData.includes('收到的赞'), false);
    assert.match(messages, /MESSAGE_TABS/);
    assert.match(messages, /listNotificationsPage\(token, \{ kind/);
  });

  it('G/H/I/J: unread styling, select marks read, publishes unread count', () => {
    assert.match(messages, /is-unread/);
    assert.match(messages, /markNotificationsRead\(token, \{ ids: \[item\.id\] \}\)/);
    assert.match(messages, /publishUnreadCount/);
    assert.match(messages, /fetchUnreadCount/);
    assert.match(messages, /setSelectedId\(item\.id\)/);
    assert.equal(messages.includes('setSelectedId(page.data[0]'), false);
  });

  it('K: comment_reply related video → /watch/:id', () => {
    assert.equal(
      messageWatchHref({
        id: 1,
        kind: 'comment_reply',
        title: 't',
        body: 'b',
        video_id: 42,
        is_read: false,
        created_at: '2026-01-01T00:00:00Z',
      }),
      '/watch/42',
    );
    assert.match(messages, /查看视频/);
    assert.match(messages, /data-testid="message-watch-link"/);
  });

  it('L/M: empty list and empty detail copy', () => {
    assert.equal(emptyListCopy('all'), '暂无消息');
    assert.equal(emptyListCopy('search'), '输入用户 ID、邮箱或昵称查找并发送私信');
    assert.match(messages, /searchPublicUsers/);
    assert.equal(emptyListCopy('system'), '暂无系统通知');
    assert.equal(emptyDetailCopy(), '选择一条消息查看详情');
    assert.equal(emptyDetailCopy('search'), '查找用户后，在此编辑并发送私信');
    assert.match(messages, /messages-list-empty/);
    assert.match(messages, /messages-detail-empty/);
  });

  it('list primary/preview stay truthful', () => {
    assert.equal(
      messageListPrimary({
        id: 1,
        kind: 'comment_reply',
        title: '回复了你',
        body: '好看',
        actor_name: 'Ada',
        is_read: true,
        created_at: '2026-01-01T00:00:00Z',
      }),
      'Ada',
    );
    assert.equal(
      messageListPreview({
        id: 1,
        kind: 'system',
        title: '维护通知',
        body: '今晚升级',
        is_read: true,
        created_at: '2026-01-01T00:00:00Z',
      }),
      '今晚升级',
    );
    assert.equal(
      messageListPrimary({
        id: 2,
        kind: 'direct_message',
        title: '私信',
        body: '你好',
        actor_name: 'Bob',
        is_read: false,
        created_at: '2026-01-01T00:00:00Z',
      }),
      'Bob',
    );
    assert.equal(
      messageListPrimary({
        id: 3,
        kind: 'direct_message_sent',
        title: '私信',
        body: '你好',
        actor_name: 'Bob',
        is_read: true,
        created_at: '2026-01-01T00:00:00Z',
      }),
      '发给 Bob',
    );
  });
});

describe('messages safety and header link', () => {
  it('N: desktop 消息 DestNavLink is new-tab /messages with unread red dot', () => {
    assert.match(header, /href="\/messages"/);
    assert.match(header, /target="_blank"/);
    assert.match(header, /rel=\{HEADER_NEW_TAB_REL\}/);
    assert.match(header, /banner-unread-dot/);
    assert.match(header, /has-unread/);
  });

  it('O: search/DM composer is present for 查找用户', () => {
    assert.equal(messagesPageHasComposer(messages), true);
    assert.match(messages, /msg-composer/);
    assert.match(messages, /msg-search-pane/);
    assert.match(messages, /sendDirectMessage/);
    assert.match(messages, /searchPublicUsers/);
    assert.match(logic, /msg-composer/);
  });

  it('column proportions and independent scroll', () => {
    assert.match(css, /grid-template-columns:\s*168px\s+300px\s+minmax\(0,\s*1fr\)/);
    assert.match(css, /\.msg-list--center[\s\S]*overflow:\s*auto/);
    assert.match(css, /\.msg-detail-pane[\s\S]*overflow:\s*auto/);
  });
});
