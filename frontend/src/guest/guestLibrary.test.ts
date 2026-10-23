import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import {
  GUEST_FAVORITES_KEY,
  GUEST_HISTORY_KEY,
  addGuestFavorite,
  clearGuestHistory,
  isGuestFavorite,
  listGuestFavorites,
  listGuestHistory,
  removeGuestFavorite,
  removeGuestHistory,
  upsertGuestHistory,
} from './guestLibrary.ts';

function memory(): Storage {
  const map = new Map<string, string>();
  return {
    get length() {
      return map.size;
    },
    clear: () => map.clear(),
    getItem: (key) => map.get(key) ?? null,
    key: (index) => [...map.keys()][index] ?? null,
    removeItem: (key) => {
      map.delete(key);
    },
    setItem: (key, value) => {
      map.set(key, value);
    },
  };
}

const sample = {
  id: 7,
  title: '登山',
  cover_url: '/c.jpg',
  category_id: 2,
  duration_sec: 120,
  description: 'd',
  year: '2020',
  area: '日本',
};

describe('guest library', () => {
  it('stores favorites only in the namespaced key and can remove them', () => {
    const store = memory();
    assert.equal(isGuestFavorite(7, store), false);
    assert.equal(addGuestFavorite(sample, store), true);
    assert.equal(isGuestFavorite(7, store), true);
    assert.equal(store.getItem(GUEST_FAVORITES_KEY)?.includes('登山'), true);
    assert.equal(store.getItem('vplayer_auth_token'), null);
    const rows = listGuestFavorites(store);
    assert.equal(rows.length, 1);
    assert.equal(rows[0].title, '登山');
    assert.equal(rows[0].cover_url, '/c.jpg');
    assert.equal(removeGuestFavorite(7, store), false);
    assert.equal(listGuestFavorites(store).length, 0);
  });

  it('records history with progress, remove, and clear without shrinking progress', () => {
    const store = memory();
    upsertGuestHistory(sample, 30, store);
    upsertGuestHistory(sample, 0, store);
    const rows = listGuestHistory(store);
    assert.equal(rows.length, 1);
    assert.equal(rows[0].progress_sec, 30);
    assert.equal(rows[0].title, '登山');
    assert.equal(store.getItem(GUEST_HISTORY_KEY)?.includes('"progress_sec":30'), true);
    upsertGuestHistory({ ...sample, id: 8, title: '另一部' }, 1, store);
    assert.equal(listGuestHistory(store)[0].id, 8);
    removeGuestHistory(8, store);
    assert.equal(listGuestHistory(store).map((row) => row.id).join(','), '7');
    clearGuestHistory(store);
    assert.equal(listGuestHistory(store).length, 0);
  });
});
