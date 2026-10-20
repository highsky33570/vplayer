import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import type { Category, FavoriteVideo } from '../api.ts';
import { usesCinematicBanner } from '../components/headerFlyoutLogic.ts';
import {
  FAVORITES_PERSONAL_NAV,
  applyFavoritesView,
  favoritesCategoryOptions,
  favoritesMainSlugByCategoryId,
  filterFavoritesByQuery,
  folderPreviewCover,
  initialFavoriteFolderId,
  sortFavorites,
} from './favoritesPageLogic.ts';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..');
const header = readFileSync(join(root, 'components/Header.tsx'), 'utf8');
const discovery = readFileSync(join(root, 'components/discoveryNavLogic.ts'), 'utf8');
const favorites = readFileSync(join(root, 'pages/FavoritesPage.tsx'), 'utf8');
const favLogic = readFileSync(join(root, 'pages/favoritesPageLogic.ts'), 'utf8');
const messages = readFileSync(join(root, 'pages/MessagesPage.tsx'), 'utf8');
const history = readFileSync(join(root, 'pages/HistoryPage.tsx'), 'utf8');
const css = readFileSync(join(root, 'styles.css'), 'utf8');
const watch = readFileSync(join(root, 'pages/Watch.tsx'), 'utf8');

function fav(
  id: number,
  opts?: Partial<FavoriteVideo>,
): FavoriteVideo {
  return {
    id,
    category_id: opts?.category_id ?? 2,
    title: opts?.title ?? `F${id}`,
    description: '',
    cover_url: opts?.cover_url ?? `/c${id}.jpg`,
    duration_sec: opts?.duration_sec ?? 120,
    view_count: 0,
    status: 'ready',
    favorited_at: opts?.favorited_at ?? '2026-10-20T12:00:00Z',
    folder_id: opts?.folder_id,
  };
}

describe('创作中心 removed from global header', () => {
  it('1/2/3: desktop and mobile omit 创作中心; 投稿 remains', () => {
    assert.equal(header.includes('创作中心'), false);
    assert.equal(header.includes('CreateIcon'), false);
    assert.equal(header.includes('创作中心即将推出'), false);
    assert.match(header, /投稿/);
    assert.match(header, /btn-upload/);
    assert.equal(discovery.includes('创作中心'), false);
    assert.match(discovery, /投稿/);
  });
});

describe('favorites shell and workspace', () => {
  it('4/5/6: /favorites keeps cinematic banner; messages/history stay compact', () => {
    assert.equal(usesCinematicBanner('/favorites'), true);
    assert.equal(usesCinematicBanner('/messages'), false);
    assert.equal(usesCinematicBanner('/history'), false);
    assert.equal(favorites.includes('BannerCarousel'), false); // banner lives in Header
    assert.match(messages, /msg-dest/);
    assert.match(history, /hist-dest/);
  });

  it('7: centered favorites workspace ~1280px', () => {
    assert.match(favorites, /data-testid="favorites-workspace"/);
    assert.match(css, /min\(1280px/);
    assert.match(favorites, /fav-dest/);
  });
});

describe('favorites library structure', () => {
  it('8-12: personal nav, sidebar, folder preview, no fake create folder', () => {
    assert.deepEqual(
      FAVORITES_PERSONAL_NAV.map((n) => n.label),
      ['主页', '我的收藏', '观看历史', '消息'],
    );
    assert.match(favorites, /fav-personal-nav/);
    assert.match(favorites, /fav-sidebar/);
    assert.match(favorites, /我创建的收藏夹/);
    assert.match(favorites, /全部收藏/);
    assert.match(favorites, /fav-folder-preview/);
    assert.equal(favorites.includes('新建收藏夹'), false);
    assert.equal(favorites.includes('动态'), false);
    assert.equal(favorites.includes('合集和系列'), false);
    assert.equal(favorites.includes('追番追剧'), false);
    assert.equal(favorites.includes('公开'), false);
    assert.equal(favorites.includes('最多播放'), false);
    assert.equal(favorites.includes('最近投稿'), false);
    assert.equal(favorites.includes('播放全部'), false);
    assert.match(favorites, /播放第一个/);
  });

  it('folder preview and initial selection helpers', () => {
    assert.equal(
      folderPreviewCover([fav(1, { cover_url: '' }), fav(2, { cover_url: '/x.jpg' })]),
      '/x.jpg',
    );
    assert.equal(
      initialFavoriteFolderId([
        { id: 9, name: '默认收藏', count: 2, isDefault: true },
        { id: 3, name: '其他', count: 1, isDefault: false },
      ]),
      9,
    );
  });
});

describe('favorites filters grid actions', () => {
  it('13-15: search, sort, category options', () => {
    const rows = [
      fav(1, { title: '星际穿越', favorited_at: '2026-10-20T12:00:00Z', category_id: 2 }),
      fav(2, { title: '流浪地球', favorited_at: '2026-10-18T12:00:00Z', category_id: 3 }),
    ];
    assert.equal(filterFavoritesByQuery(rows, '流浪').length, 1);
    assert.equal(sortFavorites(rows, 'oldest')[0]?.id, 2);
    assert.equal(sortFavorites(rows, 'recent')[0]?.id, 1);
    const cats: Category[] = [
      { id: 2, name: '电影', slug: 'movie', sort: 1, parent_id: null },
      { id: 3, name: '连续剧', slug: 'tv', sort: 2, parent_id: null },
    ];
    const opts = favoritesCategoryOptions(rows, cats);
    assert.ok(opts.some((o) => o.id === 'movie'));
    const map = favoritesMainSlugByCategoryId(cats);
    assert.equal(
      applyFavoritesView(rows, { query: '', categorySlug: 'tv', sort: 'recent' }, map).length,
      1,
    );
    assert.match(favorites, /favorites-search/);
    assert.match(favorites, /FAVORITES_SORT_OPTIONS/);
    assert.match(favLogic, /最近收藏/);
    assert.match(favLogic, /最早收藏/);
  });

  it('16-24: remove, batch, empty, load more, watch href', () => {
    assert.match(favorites, /removeFavorite/);
    assert.match(favorites, /favorites-batch/);
    assert.match(favorites, /favorites-batch-bar/);
    assert.match(favorites, /全选/);
    assert.match(favorites, /移出收藏/);
    assert.match(favorites, /取消管理/);
    assert.match(favorites, /confirm\(`确认将选中的/);
    assert.match(favorites, /favorites-empty/);
    assert.match(favorites, /favorites-load-more/);
    assert.match(favorites, /to=\{`\/watch\/\$\{video\.id\}`\}/);
    assert.match(favorites, /fav-grid/);
    assert.match(css, /\.fav-grid[\s\S]*repeat\(4,/);
  });

  it('25/26: header 收藏 new-tab + Watch favorite action exists', () => {
    assert.match(header, /href="\/favorites"/);
    assert.match(header, /target="_blank"/);
    assert.match(header, /rel=\{HEADER_NEW_TAB_REL\}/);
    assert.match(header, /FavoritesPanel/);
    assert.match(watch, /addFavorite/);
    assert.match(watch, /removeFavorite/);
  });
});
