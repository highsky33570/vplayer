import assert from 'node:assert/strict';
import { afterEach, describe, it } from 'node:test';
import {
  claimHoverPreview,
  getActiveHoverPreviewOwnerId,
  resetHoverPreviewCoordinator,
  stopActivePreview,
} from './hoverPreviewCoordinator.ts';

describe('hoverPreviewCoordinator', () => {
  afterEach(() => {
    resetHoverPreviewCoordinator();
  });

  it('keeps only one active preview owner', () => {
    const stopped: string[] = [];
    claimHoverPreview('a', () => stopped.push('a'));
    assert.equal(getActiveHoverPreviewOwnerId(), 'a');
    claimHoverPreview('b', () => stopped.push('b'));
    assert.deepEqual(stopped, ['a']);
    assert.equal(getActiveHoverPreviewOwnerId(), 'b');
  });

  it('stopActivePreview clears the slot', () => {
    let n = 0;
    claimHoverPreview('x', () => {
      n += 1;
    });
    stopActivePreview();
    assert.equal(n, 1);
    assert.equal(getActiveHoverPreviewOwnerId(), null);
  });

  it('release only clears matching owner', () => {
    const releaseA = claimHoverPreview('a', () => undefined);
    claimHoverPreview('b', () => undefined);
    releaseA();
    assert.equal(getActiveHoverPreviewOwnerId(), 'b');
  });
});
