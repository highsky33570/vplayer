import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import {
  HOVER_PREVIEW_DELAY_MS,
  HOVER_PREVIEW_LOOP_SEC,
  HOVER_PREVIEW_MEDIA_QUERY,
  HOVER_PREVIEW_START_SEC,
  canResolveHoverPreview,
  isHlsPlayback,
  previewLoopEndSec,
  previewStartSec,
  supportsHoverPreview,
} from './hoverPreviewLogic.ts';

describe('hoverPreviewLogic', () => {
  it('uses a 500–700ms hover delay', () => {
    assert.ok(HOVER_PREVIEW_DELAY_MS >= 500 && HOVER_PREVIEW_DELAY_MS <= 700);
  });

  it('starts near 10s for long videos', () => {
    assert.equal(HOVER_PREVIEW_START_SEC, 10);
    assert.equal(previewStartSec(3600), 10);
    assert.ok(previewStartSec(200) <= 10);
  });

  it('avoids always starting at 0 for short clips', () => {
    const start = previewStartSec(8);
    assert.ok(start >= 0);
    assert.ok(start < 8);
  });

  it('loops roughly 8–12 seconds', () => {
    assert.ok(HOVER_PREVIEW_LOOP_SEC >= 8 && HOVER_PREVIEW_LOOP_SEC <= 12);
    assert.equal(previewLoopEndSec(10, 600), 20);
    assert.equal(previewLoopEndSec(10, 15), 15);
  });

  it('detects HLS URLs', () => {
    assert.equal(isHlsPlayback('https://cdn/a.m3u8', 'url'), true);
    assert.equal(isHlsPlayback('https://cdn/a.mp4', 'hls'), true);
    assert.equal(isHlsPlayback('https://cdn/a.mp4', 'url'), false);
  });

  it('requires auth token to resolve preview media', () => {
    assert.equal(canResolveHoverPreview(null), false);
    assert.equal(canResolveHoverPreview(''), false);
    assert.equal(canResolveHoverPreview('   '), false);
    assert.equal(canResolveHoverPreview('tok'), true);
  });

  it('gates on hover:hover + fine pointer', () => {
    assert.match(HOVER_PREVIEW_MEDIA_QUERY, /hover:\s*hover/);
    assert.match(HOVER_PREVIEW_MEDIA_QUERY, /pointer:\s*fine/);
    assert.equal(
      supportsHoverPreview(() => ({ matches: true })),
      true,
    );
    assert.equal(
      supportsHoverPreview(() => ({ matches: false })),
      false,
    );
  });
});
