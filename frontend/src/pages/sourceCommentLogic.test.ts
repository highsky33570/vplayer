import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import {
  canDeleteComment,
  sourceAuthorName,
  sourceCommentTime,
  sourceSectionState,
} from './watchPageLogic.ts';

const here = dirname(fileURLToPath(import.meta.url));
const logic = readFileSync(join(here, 'watchPageLogic.ts'), 'utf8');
const timeFn = logic.slice(logic.indexOf('export function sourceCommentTime'), logic.indexOf('export function sourceAuthorName'));

describe('source comment presentation', () => {
  it('shows the source timestamp as provided, without a timezone conversion', () => {
    assert.equal(sourceCommentTime('2026-10-19 01:56:45'), '2026-10-19 01:56:45');
    assert.equal(sourceCommentTime('  10:28  '), '10:28');
    assert.equal(sourceCommentTime(''), '');
    assert.equal(sourceCommentTime(null), '');
    assert.equal(timeFn.includes('Date'), false);
    assert.equal(timeFn.includes('UTC'), false);
  });

  it('keeps source comments undeletable and native own comments deletable', () => {
    assert.equal(canDeleteComment('source', true), false);
    assert.equal(canDeleteComment('source', false), false);
    assert.equal(canDeleteComment('vplayer', true), true);
    assert.equal(canDeleteComment('vplayer', false), false);
  });

  it('falls back to 游客 and distinguishes loading, empty, and error', () => {
    assert.equal(sourceAuthorName(''), '游客');
    assert.equal(sourceAuthorName('  '), '游客');
    assert.equal(sourceAuthorName(' AKA Sunk '), 'AKA Sunk');
    assert.equal(sourceSectionState({ loading: true, error: '', count: 0 }), 'loading');
    assert.equal(sourceSectionState({ loading: false, error: '原站评论加载失败', count: 0 }), 'error');
    assert.equal(sourceSectionState({ loading: false, error: '', count: 0 }), 'empty');
    assert.equal(sourceSectionState({ loading: false, error: '', count: 2 }), 'ready');
  });
});
