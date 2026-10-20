import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import type { HistoryVideo } from '../api.ts';
import {
  filterHistoryByQuery,
  groupHistoryItems,
  historyGroupId,
  historyProgressRatio,
} from './historyPageLogic.ts';

function row(id: number, iso: string, title = `V${id}`): HistoryVideo {
  return {
    id,
    category_id: 1,
    title,
    description: '',
    cover_url: '',
    duration_sec: 100,
    view_count: 0,
    status: 'ready',
    last_watched_at: iso,
    progress_sec: 40,
  };
}

describe('history page logic', () => {
  it('groups by today / yesterday / week / earlier', () => {
    const now = new Date('2026-10-20T15:00:00');
    assert.equal(historyGroupId('2026-10-20T10:00:00', now), 'today');
    assert.equal(historyGroupId('2026-10-19T10:00:00', now), 'yesterday');
    assert.equal(historyGroupId('2026-10-17T10:00:00', now), 'week');
    assert.equal(historyGroupId('2026-09-01T10:00:00', now), 'earlier');

    const groups = groupHistoryItems(
      [
        row(1, '2026-10-20T12:00:00'),
        row(2, '2026-10-19T12:00:00'),
        row(3, '2026-09-01T12:00:00'),
      ],
      now,
    );
    assert.deepEqual(
      groups.map((g) => g.id),
      ['today', 'yesterday', 'earlier'],
    );
  });

  it('filters by title and computes progress ratio', () => {
    const rows = [row(1, '2026-10-20T12:00:00', '星际穿越'), row(2, '2026-10-20T11:00:00', '流浪地球')];
    assert.equal(filterHistoryByQuery(rows, '流浪').length, 1);
    assert.equal(filterHistoryByQuery(rows, '').length, 2);
    assert.equal(historyProgressRatio(25, 100), 0.25);
    assert.equal(historyProgressRatio(0, 100), 0);
    assert.equal(historyProgressRatio(200, 100), 1);
  });
});
