import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import type { AdminCategory } from '../api';
import { buildAdminCategoryTree, orphanAdminCategories } from './adminCategoryTree.ts';

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
    video_count: 1,
    parent_id: null,
    ...partial,
  };
}

describe('adminCategoryTree', () => {
  it('nests subcategories under canonical mains', () => {
    const rows = [
      cat({ id: 2, name: '电影', slug: 'movie', source_id: '1' }),
      cat({ id: 100, name: '动作片', slug: 'type-101', parent_id: 2, source_id: '101' }),
      cat({ id: 999, name: '杂项', slug: 'misc', source_id: '99' }),
    ];
    const tree = buildAdminCategoryTree(rows);
    const movie = tree.find((n) => n.main.slug === 'movie');
    assert.ok(movie);
    assert.equal(movie!.children.length, 1);
    assert.equal(movie!.children[0].name, '动作片');
    assert.deepEqual(
      orphanAdminCategories(rows).map((c) => c.id),
      [999],
    );
  });
});
