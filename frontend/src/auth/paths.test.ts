import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import {
  loginWithNext,
  resolvePostLoginPath,
  safeInternalPath,
  videoEntryHref,
} from './paths.ts';

describe('videoEntryHref', () => {
  it('returns null while auth is unresolved', () => {
    assert.equal(videoEntryHref(false, false, 12), null);
    assert.equal(videoEntryHref(false, true, 12), null);
  });

  it('sends guests to /login?next=/watch/:id', () => {
    assert.equal(videoEntryHref(true, false, 42), '/login?next=%2Fwatch%2F42');
  });

  it('sends authenticated users to /watch/:id', () => {
    assert.equal(videoEntryHref(true, true, 42), '/watch/42');
  });
});

describe('loginWithNext', () => {
  it('encodes the next path', () => {
    assert.equal(loginWithNext('/watch/7'), '/login?next=%2Fwatch%2F7');
  });
});

describe('safeInternalPath / resolvePostLoginPath', () => {
  it('accepts internal watch paths', () => {
    assert.equal(safeInternalPath('/watch/9'), '/watch/9');
    assert.equal(resolvePostLoginPath('/watch/9'), '/watch/9');
    assert.equal(safeInternalPath('%2Fwatch%2F9'), '/watch/9');
  });

  it('rejects external and protocol-relative redirects', () => {
    assert.equal(safeInternalPath('//example.com'), null);
    assert.equal(safeInternalPath('http://evil.test/x'), null);
    assert.equal(safeInternalPath('https://evil.test/x'), null);
    assert.equal(safeInternalPath('/\\evil'), null);
    assert.equal(safeInternalPath('watch/1'), null);
    assert.equal(resolvePostLoginPath('https://evil.test'), '/');
    assert.equal(resolvePostLoginPath(null), '/');
    assert.equal(resolvePostLoginPath(''), '/');
  });
});
