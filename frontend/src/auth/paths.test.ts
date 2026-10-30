import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import {
  adminLoginWithNext,
  isAdminAppPath,
  loginWithNext,
  resolveAdminPostLoginPath,
  resolvePostLoginPath,
  safeInternalPath,
  videoEntryHref,
} from './paths.ts';

describe('videoEntryHref', () => {
  it('returns null while auth is unresolved', () => {
    assert.equal(videoEntryHref(false, false, 12), null);
    assert.equal(videoEntryHref(false, true, 12), null);
  });

  it('sends guests and members to the public watch page', () => {
    assert.equal(videoEntryHref(true, false, 42), '/watch/42');
    assert.equal(videoEntryHref(true, true, 42), '/watch/42');
  });
});

describe('loginWithNext', () => {
  it('encodes the next path', () => {
    assert.equal(loginWithNext('/watch/7'), '/login?next=%2Fwatch%2F7');
  });

  it('sends admin destinations to /admin/login', () => {
    assert.equal(loginWithNext('/admin'), '/admin/login?next=%2Fadmin');
    assert.equal(
      loginWithNext('/admin/videos'),
      '/admin/login?next=%2Fadmin%2Fvideos',
    );
  });
});

describe('adminLoginWithNext / resolveAdminPostLoginPath', () => {
  it('builds admin login URLs', () => {
    assert.equal(adminLoginWithNext(), '/admin/login?next=%2Fadmin');
    assert.equal(adminLoginWithNext('/admin/payments'), '/admin/login?next=%2Fadmin%2Fpayments');
    assert.equal(isAdminAppPath('/admin'), true);
    assert.equal(isAdminAppPath('/admin/login'), false);
  });

  it('keeps post-login inside admin', () => {
    assert.equal(resolveAdminPostLoginPath('/admin/videos'), '/admin/videos');
    assert.equal(resolveAdminPostLoginPath('/watch/1'), '/admin');
    assert.equal(resolveAdminPostLoginPath('/admin/login'), '/admin');
    assert.equal(resolveAdminPostLoginPath(null), '/admin');
  });
});

describe('safeInternalPath / resolvePostLoginPath', () => {
  it('accepts internal watch paths', () => {
    assert.equal(safeInternalPath('/watch/9'), '/watch/9');
    assert.equal(resolvePostLoginPath('/watch/9'), '/watch/9');
    assert.equal(safeInternalPath('%2Fwatch%2F9'), '/watch/9');
  });

  it('accepts /admin as post-login next', () => {
    assert.equal(safeInternalPath('/admin'), '/admin');
    assert.equal(resolvePostLoginPath('/admin'), '/admin');
  });

  it('accepts /admin/videos paths', () => {
    assert.equal(safeInternalPath('/admin/videos'), '/admin/videos');
    assert.equal(resolvePostLoginPath('/admin/videos/12'), '/admin/videos/12');
  });

  it('accepts /admin/categories paths', () => {
    assert.equal(safeInternalPath('/admin/categories'), '/admin/categories');
    assert.equal(resolvePostLoginPath('/admin/categories/3'), '/admin/categories/3');
  });

  it('accepts /admin/users paths', () => {
    assert.equal(safeInternalPath('/admin/users'), '/admin/users');
    assert.equal(resolvePostLoginPath('/admin/users/5'), '/admin/users/5');
  });

  it('accepts /admin/sync paths', () => {
    assert.equal(safeInternalPath('/admin/sync'), '/admin/sync');
    assert.equal(resolvePostLoginPath('/admin/sync/runs/9'), '/admin/sync/runs/9');
    assert.equal(
      resolvePostLoginPath('/admin/sync/operations/3'),
      '/admin/sync/operations/3',
    );
  });

  it('accepts /admin/media paths', () => {
    assert.equal(safeInternalPath('/admin/media'), '/admin/media');
    assert.equal(resolvePostLoginPath('/admin/media/episodes/4'), '/admin/media/episodes/4');
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
