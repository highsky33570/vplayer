import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import type { Category, HistoryVideo } from '../api.ts';
import { usesCinematicBanner } from '../components/headerFlyoutLogic.ts';
import {
  HISTORY_COMPLETE_RATIO,
  applyHistoryFilters,
  filterHistoryByQuery,
  filterHistoryByStatus,
  filterHistoryByTime,
  formatHistoryCardTime,
  formatHistoryProgressClock,
  groupHistoryItems,
  historyFiltersActive,
  historyGroupId,
  historyMainSlugByCategoryId,
  historyPrimaryTabs,
  historyProgressRatio,
  isHistoryCompleted,
} from './historyPageLogic.ts';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..');
const header = readFileSync(join(root, 'components/Header.tsx'), 'utf8');
const history = readFileSync(join(root, 'pages/HistoryPage.tsx'), 'utf8');
const messages = readFileSync(join(root, 'pages/MessagesPage.tsx'), 'utf8');
const account = readFileSync(join(root, 'pages/AccountPage.tsx'), 'utf8');
const css = readFileSync(join(root, 'styles.css'), 'utf8');
const flyoutLogic = readFileSync(join(root, 'components/headerFlyoutLogic.ts'), 'utf8');

function row(
  id: number,
  iso: string,
  opts?: Partial<HistoryVideo>,
): HistoryVideo {
  return {
    id,
    category_id: opts?.category_id ?? 2,
    title: opts?.title ?? `V${id}`,
    description: '',
    cover_url: '',
    duration_sec: opts?.duration_sec ?? 100,
    view_count: 0,
    status: 'ready',
    last_watched_at: iso,
    progress_sec: opts?.progress_sec ?? 40,
  };
}

const cats: Category[] = [
  { id: 2, name: '电影', slug: 'movie', sort: 1, parent_id: null },
  { id: 3, name: '连续剧', slug: 'tv', sort: 2, parent_id: null },
  { id: 20, name: '动作', slug: 'action', sort: 1, parent_id: 2 },
];

describe('history shell — compact like /messages', () => {
  it('1/2/3: /history has no BannerCarousel; compact shell; centered workspace', () => {
    assert.equal(usesCinematicBanner('/history'), false);
    assert.equal(usesCinematicBanner('/messages'), false);
    assert.equal(usesCinematicBanner('/'), true);
    assert.equal(usesCinematicBanner('/account'), false);
    assert.equal(usesCinematicBanner('/vip'), false);
    assert.equal(usesCinematicBanner('/favorites'), false);
    assert.match(flyoutLogic, /COMPACT_SHELL_PATHS|\/history/);
    assert.match(header, /usesCinematicBanner\(pathname\)/);
    assert.match(header, /<BannerCarousel /);
    assert.match(header, /banner-shell\$\{cinematic \? '' : ' banner-shell--app'\}/);
    assert.equal(history.includes('BannerCarousel'), false);
    assert.equal(history.includes('DiscoveryNav'), false);
    assert.match(history, /data-testid="history-workspace"/);
    assert.match(css, /min\(1240px/);
  });

  it('25/26: /messages and /account use the compact shell', () => {
    assert.equal(usesCinematicBanner('/messages'), false);
    assert.equal(usesCinematicBanner('/account'), false);
    assert.match(messages, /msg-dest/);
    assert.match(account, /space-page/);
  });

  it('24: header 历史 is new-tab link', () => {
    assert.match(header, /href="\/history"/);
    assert.match(header, /target="_blank"/);
    assert.match(header, /rel=\{HEADER_NEW_TAB_REL\}/);
  });
});

describe('history grouping filters search progress', () => {
  it('6/7: timeline groups omit empty buckets', () => {
    const now = new Date('2026-10-20T15:00:00');
    assert.equal(historyGroupId('2026-10-20T10:00:00', now), 'today');
    assert.equal(historyGroupId('2026-10-19T10:00:00', now), 'yesterday');
    assert.equal(historyGroupId('2026-10-17T10:00:00', now), 'week');
    assert.equal(historyGroupId('2026-09-01T10:00:00', now), 'earlier');
    const groups = groupHistoryItems(
      [row(1, '2026-10-20T12:00:00'), row(3, '2026-09-01T12:00:00')],
      now,
    );
    assert.deepEqual(
      groups.map((g) => g.id),
      ['today', 'earlier'],
    );
  });

  it('8/9: search and time/status filters', () => {
    const now = new Date('2026-10-20T15:00:00');
    const rows = [
      row(1, '2026-10-20T12:00:00', { title: '星际穿越', progress_sec: 95, duration_sec: 100 }),
      row(2, '2026-10-19T12:00:00', { title: '流浪地球', progress_sec: 10, duration_sec: 100 }),
      row(3, '2026-09-01T12:00:00', { title: '盗梦空间', progress_sec: 20, duration_sec: 100 }),
    ];
    assert.equal(filterHistoryByQuery(rows, '流浪').length, 1);
    assert.equal(filterHistoryByTime(rows, 'yesterday', now).length, 1);
    assert.equal(filterHistoryByStatus(rows, 'completed').length, 1);
    assert.equal(filterHistoryByStatus(rows, 'unfinished').length, 2);
  });

  it('10/11/12: more-filter wiring + progress + completion only when valid', () => {
    assert.match(history, /history-more-filters/);
    assert.match(history, /HISTORY_TIME_FILTER_OPTIONS/);
    assert.match(history, /HISTORY_STATUS_FILTER_OPTIONS/);
    assert.equal(historyProgressRatio(25, 100), 0.25);
    assert.equal(isHistoryCompleted(89, 100), false);
    assert.equal(isHistoryCompleted(90, 100), true);
    assert.equal(HISTORY_COMPLETE_RATIO, 0.9);
    assert.equal(isHistoryCompleted(100, 0), false);
    assert.equal(formatHistoryProgressClock(65, 3600), '01:05 / 1:00:00');
    assert.equal(formatHistoryProgressClock(0, 100), '');
  });

  it('category primary tabs only for present mains', () => {
    const items = [
      row(1, '2026-10-20T12:00:00', { category_id: 20 }),
      row(2, '2026-10-20T11:00:00', { category_id: 3 }),
    ];
    const tabs = historyPrimaryTabs(items, cats);
    assert.deepEqual(
      tabs.map((t) => t.id),
      ['all', 'movie', 'tv'],
    );
    const map = historyMainSlugByCategoryId(cats);
    assert.equal(map.get(20), 'movie');
    const filtered = applyHistoryFilters(
      items,
      { query: '', mainSlug: 'movie', time: 'all', status: 'all' },
      map,
    );
    assert.equal(filtered.length, 1);
    assert.equal(historyFiltersActive({ query: '', mainSlug: 'all', time: 'all', status: 'all' }), false);
  });

  it('19: card timestamp format', () => {
    const now = new Date('2026-10-20T15:00:00');
    assert.match(formatHistoryCardTime('2026-10-20T14:32:00', now), /^今天 14:32$/);
    assert.match(formatHistoryCardTime('2026-10-19T21:05:00', now), /^昨天 21:05$/);
  });
});

describe('history page actions wired', () => {
  it('4/5/13-23: title, cards, delete, batch, clear, empty, load more, watch href', () => {
    assert.match(history, /历史记录/);
    assert.match(history, /hist-title/);
    assert.match(history, /hist-card/);
    assert.match(history, /deleteHistoryItem/);
    assert.match(history, /history-batch/);
    assert.match(history, /history-batch-bar/);
    assert.match(history, /全选/);
    assert.match(history, /删除所选/);
    assert.match(history, /取消管理/);
    assert.match(history, /confirm\('确认清空全部观看历史/);
    assert.match(history, /clearHistory\(token\)/);
    assert.match(history, /history-empty/);
    assert.match(history, /history-load-more/);
    assert.match(history, /to=\{`\/watch\/\$\{video\.id\}`\}/);
    assert.match(history, /已看完/);
    assert.match(history, /hist-progress/);
    assert.equal(history.includes('记录播放历史'), false);
    assert.equal(history.includes('直播'), false);
    assert.equal(history.includes('专栏'), false);
  });

  it('grid is dense 4-col on desktop workspace', () => {
    assert.match(css, /\.hist-grid[\s\S]*repeat\(4,/);
    assert.match(css, /\.hist-group::before/);
    assert.match(history, /history-timeline/);
  });
});
