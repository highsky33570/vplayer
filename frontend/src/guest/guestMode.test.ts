import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const src = join(here, '..');

function read(path: string) {
  return readFileSync(join(src, path), 'utf8');
}

describe('guest mode keeps auth and hides it when AUTH_REQUIRED is false', () => {
  it('loads authRequired from the public registration response', () => {
    const api = read('api.ts');
    const auth = read('auth/AuthContext.tsx');
    assert.match(api, /auth_required\?: boolean/);
    assert.match(auth, /setAuthRequired\(opts\.auth_required !== false\)/);
    assert.match(api, /playback_auth_required\?: boolean/);
    assert.match(auth, /setPlaybackAuthRequired\(opts\.playback_auth_required !== false\)/);
    assert.match(read('pages/Watch.tsx'), /playbackNeedsLogin = authRequired && playbackAuthRequired/);
    assert.match(auth, /authRequired/);
    assert.doesNotMatch(auth, /VITE_/);
  });

  it('keeps login UI for auth-required mode and redirects /login in guest mode', () => {
    const login = read('pages/Login.tsx');
    const app = read('App.tsx');
    const gate = read('auth/AuthGate.tsx');
    const admin = read('admin/AdminGuard.tsx');
    assert.match(login, /AuthForm/);
    assert.match(login, /ready && !authRequired/);
    assert.match(login, /Navigate to="\/"/);
    assert.match(app, /authRequired \? <LoginModal \/> : null/);
    assert.match(app, /authRequired \? <LoginReminder \/> : null/);
    assert.match(gate, /authRequired && !user/);
    assert.match(admin, /loginWithNext\('\/admin'\)/);
    assert.doesNotMatch(admin, /authRequired/);
  });

  it('does not call server library APIs from guest pages', () => {
    const favorites = read('guest/GuestFavoritesPage.tsx');
    const history = read('guest/GuestHistoryPage.tsx');
    const messages = read('guest/GuestMessagesPage.tsx');
    const account = read('guest/GuestAccountPage.tsx');
    for (const source of [favorites, history, messages]) {
      assert.doesNotMatch(source, /\/me\/favorites|\/me\/history|\/me\/notifications|listFavoritesPage|listHistoryPage|listNotificationsPage/);
    }
    assert.match(favorites, /removeGuestFavorite/);
    assert.match(history, /clearGuestHistory/);
    assert.match(messages, /暂无消息|emptyListCopy/);
    assert.doesNotMatch(account, /user\.email|logout|updateProfile/);
    assert.match(account, /listVideosPage/);
  });

  it('keeps authenticated implementations beside the guest branch', () => {
    assert.match(read('pages/FavoritesPage.tsx'), /AuthGate/);
    assert.match(read('pages/FavoritesPage.tsx'), /listFavoritesPage/);
    assert.match(read('pages/HistoryPage.tsx'), /clearHistory\(token\)/);
    assert.match(read('pages/MessagesPage.tsx'), /fetchUnreadCount/);
    assert.match(read('pages/AccountPage.tsx'), /user\.email/);
    assert.match(read('pages/Watch.tsx'), /upsertHistory\(token, videoId\)/);
    assert.match(read('pages/Watch.tsx'), /addGuestFavorite/);
  });
});
