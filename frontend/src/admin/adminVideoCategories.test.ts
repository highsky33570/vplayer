import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import type { AdminCategory } from '../api';
import {
  adminMainCategories,
  adminSubcategories,
  resolveAdminCategorySelection,
} from './adminVideoCategories.ts';

function cat(
  partial: Partial<AdminCategory> & Pick<AdminCategory, 'id' | 'name' | 'slug'>,
): AdminCategory {
  return {
    sort: 0,
    is_active: true,
    source_system: 'olehdtv',
    source_id: String(partial.id),
    sync_status: 'ok',
    has_error: false,
    created_at: '2026-01-01T00:00:00Z',
    video_count: 0,
    parent_id: null,
    ...partial,
  };
}

const sample: AdminCategory[] = [
  cat({ id: 2, name: '电影', slug: 'movie', source_id: '1' }),
  cat({ id: 3, name: '连续剧', slug: 'tv', source_id: '2' }),
  cat({ id: 13, name: '综艺', slug: 'variety', source_id: '3' }),
  cat({ id: 4, name: '动漫', slug: 'anime', source_id: '4' }),
  cat({ id: 16, name: '午夜影院', slug: 'midnight', source_id: '5' }),
  cat({ id: 17, name: 'VIP蓝光影院', slug: 'vip-bluray', source_id: '6' }),
  cat({ id: 100, name: '动作片', slug: 'type-101', parent_id: 2, source_id: '101' }),
  cat({ id: 101, name: '喜剧片', slug: 'type-102', parent_id: 2, source_id: '102' }),
  cat({ id: 999, name: '杂项', slug: 'misc', source_id: '99' }),
];

describe('adminVideoCategories', () => {
  it('lists only the six main channels (首页 is empty selection)', () => {
    const mains = adminMainCategories(sample);
    assert.deepEqual(
      mains.map((c) => c.name),
      ['电影', '连续剧', '综艺', '动漫', '午夜影院', 'VIP蓝光影院'],
    );
  });

  it('lists subcategories under the selected main', () => {
    const subs = adminSubcategories(sample, 2);
    assert.deepEqual(
      subs.map((c) => c.name),
      ['动作片', '喜剧片'],
    );
    assert.equal(adminSubcategories(sample, 0).length, 0);
  });

  it('resolves main vs subcategory selection from category_id', () => {
    assert.deepEqual(resolveAdminCategorySelection(sample, 0), { mainId: 0, subId: 0 });
    assert.deepEqual(resolveAdminCategorySelection(sample, 2), { mainId: 2, subId: 0 });
    assert.deepEqual(resolveAdminCategorySelection(sample, 100), { mainId: 2, subId: 100 });
  });
});
