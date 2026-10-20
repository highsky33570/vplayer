import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import type { FavoriteVideo, HistoryVideo, Video } from '../api.ts';
import {
  ACCOUNT_FAVORITES_PREVIEW_LIMIT,
  ACCOUNT_HISTORY_PREVIEW_LIMIT,
  ACCOUNT_RECOMMEND_LIMIT,
  ACCOUNT_USER_UPLOADS_SUPPORTED,
  accountFavoritesPreview,
  accountHistoryPreview,
  accountOwnedUploadVideos,
  accountOwnedUploadsEmptyCopy,
  accountRecommendMeta,
  accountRecommendVideos,
} from './accountPageLogic.ts';

const here = dirname(fileURLToPath(import.meta.url));
const accountPage = readFileSync(join(here, 'AccountPage.tsx'), 'utf8');
const watch = readFileSync(join(here, 'Watch.tsx'), 'utf8');
const css = readFileSync(join(here, '../styles.css'), 'utf8');

function video(id: number, title = `v${id}`): Video {
  return {
    id,
    category_id: 1,
    title,
    description: '',
    cover_url: `/c/${id}.jpg`,
    duration_sec: 120,
    view_count: id * 10,
    status: 'published',
    year: '2024',
    area: '华语',
  };
}

function history(id: number): HistoryVideo {
  return { ...video(id), last_watched_at: '2026-10-20T12:00:00Z', progress_sec: 30 };
}

function favorite(id: number): FavoriteVideo {
  return { ...video(id), favorited_at: '2026-10-19T12:00:00Z', folder_id: 1 };
}

describe('account page content strategy', () => {
  it('keeps recommendation limit at 10–15 and personal previews bounded', () => {
    assert.ok(ACCOUNT_RECOMMEND_LIMIT >= 10 && ACCOUNT_RECOMMEND_LIMIT <= 15);
    assert.equal(ACCOUNT_HISTORY_PREVIEW_LIMIT, 8);
    assert.equal(ACCOUNT_FAVORITES_PREVIEW_LIMIT, 6);
  });

  it('new accounts with empty personal data still get catalog recommendations', () => {
    const catalog = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16].map(video);
    assert.deepEqual(accountHistoryPreview([]), []);
    assert.deepEqual(accountFavoritesPreview([]), []);
    assert.equal(accountOwnedUploadVideos(1).length, 0);
    const recs = accountRecommendVideos(catalog);
    assert.equal(recs.length, ACCOUNT_RECOMMEND_LIMIT);
    assert.equal(recs[0]?.id, 1);
    assert.equal(recs[recs.length - 1]?.id, ACCOUNT_RECOMMEND_LIMIT);
  });

  it('history and favorites previews use personal rows only', () => {
    const hist = [history(9), history(8), history(7)];
    const favs = [favorite(3), favorite(2)];
    assert.deepEqual(
      accountHistoryPreview(hist).map((v) => v.id),
      [9, 8, 7],
    );
    assert.deepEqual(
      accountFavoritesPreview(favs).map((v) => v.id),
      [3, 2],
    );
    // Catalog must not leak into owned uploads.
    assert.equal(ACCOUNT_USER_UPLOADS_SUPPORTED, false);
    assert.deepEqual(accountOwnedUploadVideos(42), []);
    assert.match(accountOwnedUploadsEmptyCopy().title, /暂无投稿/);
  });

  it('我的视频 never consumes catalog recommendations in AccountPage', () => {
    assert.match(accountPage, /accountOwnedUploadVideos/);
    assert.match(accountPage, /listVideosPage/);
    assert.match(accountPage, /excludeCategorySlug:\s*'midnight'/);
    assert.match(accountPage, /猜你喜欢/);
    assert.match(accountPage, /data-testid="account-recommend"/);
    assert.match(accountPage, /data-testid="account-uploads-empty"/);
    // Owned section uses accountOwnedUploadVideos, not recommended state.
    const uploadsBlock = accountPage.slice(
      accountPage.indexOf('aria-label="我的视频"'),
      accountPage.indexOf('aria-label="收藏夹"'),
    );
    assert.match(uploadsBlock, /ownedUploads/);
    assert.equal(uploadsBlock.includes('recommended'), false);
    assert.equal(uploadsBlock.includes('listVideosPage'), false);
  });

  it('recommendation cards are labeled separately and link to /watch/:id', () => {
    assert.match(accountPage, /站内片库推荐/);
    assert.match(accountPage, /to=\{`\/watch\/\$\{video\.id\}`\}/);
    assert.match(accountPage, /data-testid="account-recommend-grid"/);
    assert.equal(accountRecommendMeta(video(1)), '2024 · 华语 · 10 播放');
  });

  it('uses compact empty states instead of giant empty panels for personal sections', () => {
    assert.match(accountPage, /space-empty--compact/);
    assert.match(css, /\.space-empty--compact/);
    const compact = css.slice(css.indexOf('.space-empty--compact'), css.indexOf('.space-empty--panel'));
    assert.match(compact, /min-height:\s*0/);
    assert.equal(/min-height:\s*140px/.test(compact), false);
  });
});

describe('watch → account personal integration wiring', () => {
  it('records history and favorites from Watch via separate APIs (not playback core)', () => {
    assert.match(watch, /void upsertHistory\(token, videoId\)/);
    assert.match(watch, /getFavoriteStatus/);
    assert.match(watch, /addFavorite/);
    assert.match(watch, /removeFavorite/);
    assert.match(accountPage, /listHistoryPage/);
    assert.match(accountPage, /listFavoritesPage/);
    assert.equal(watch.includes('watchPlaybackLogic'), true);
    // History effect stays outside the playback session block.
    const historyEffect = watch.indexOf('// Record watch history');
    const playbackEffect = watch.indexOf('const session = new WatchPlaybackSession()');
    assert.ok(historyEffect > 0 && playbackEffect > 0 && historyEffect !== playbackEffect);
  });
});
