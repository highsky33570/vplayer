import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import type { Category, Video } from '../api.ts';
import {
  VIP_FEATURE_CARDS,
  pickVipHero,
  resolveVipCategory,
  vipAccessStatus,
  vipCatalogGrid,
  vipFeaturedStrip,
} from './vipPageLogic.ts';

const here = dirname(fileURLToPath(import.meta.url));

function video(id: number, title: string): Video {
  return {
    id,
    category_id: 17,
    title,
    description: '',
    cover_url: `/c${id}.jpg`,
    duration_sec: 3600,
    view_count: 1,
    status: 'ready',
  };
}

describe('vip page logic', () => {
  it('resolves VIP蓝光影院 from the category list', () => {
    const cats: Category[] = [
      { id: 2, name: '电影', slug: 'movie', sort: 2 },
      { id: 17, name: 'VIP蓝光影院', slug: 'vip-bluray', sort: 7 },
    ];
    assert.equal(resolveVipCategory(cats)?.slug, 'vip-bluray');
    assert.equal(resolveVipCategory([]), undefined);
  });

  it('builds hero/featured/catalog slices without inventing membership', () => {
    const rows = [video(1, 'A'), video(2, 'B'), video(3, 'C')];
    assert.equal(pickVipHero(rows)?.id, 1);
    assert.equal(pickVipHero([]), null);
    assert.deepEqual(
      vipFeaturedStrip(rows, 2).map((v) => v.id),
      [1, 2],
    );
    assert.deepEqual(
      vipCatalogGrid(rows, 1, 10).map((v) => v.id),
      [2, 3],
    );
    assert.equal(vipAccessStatus(true).label.includes('付费'), false);
    assert.match(vipAccessStatus(false).detail, /没有付费订阅/);
    assert.equal(VIP_FEATURE_CARDS.length, 4);
    for (const phrase of ['4K', '免广告', '到期', '续费', '优惠券']) {
      assert.equal(VIP_FEATURE_CARDS.some((c) => `${c.title}${c.body}`.includes(phrase)), false);
    }
  });

  it('wires /vip route and header click destination', () => {
    const app = readFileSync(join(here, '../App.tsx'), 'utf8');
    const header = readFileSync(join(here, '../components/Header.tsx'), 'utf8');
    const logic = readFileSync(join(here, '../components/headerFlyoutLogic.ts'), 'utf8');
    assert.match(app, /path="\/vip"/);
    assert.match(app, /VipPage/);
    assert.match(logic, /href:\s*'\/vip'/);
    assert.match(header, /navigate\('\/account'\)/);
    assert.match(header, /public:\s*true/);
    assert.equal(header.includes('动态'), false);
    assert.equal(header.includes('动态即将推出'), false);
  });
});
