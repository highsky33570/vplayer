import assert from 'node:assert/strict';
import { afterEach, describe, it } from 'node:test';
import { JSDOM } from 'jsdom';

const dom = new JSDOM('<!doctype html><html><body></body></html>', {
  url: 'http://localhost/',
});
const win = dom.window;
Object.defineProperty(globalThis, 'window', { value: win, configurable: true });
Object.defineProperty(globalThis, 'document', { value: win.document, configurable: true });
Object.defineProperty(globalThis, 'localStorage', { value: win.localStorage, configurable: true });
Object.defineProperty(globalThis, 'location', { value: win.location, configurable: true });

const {
  acceptCookieConsent,
  clearCookieConsent,
  hasCookieConsent,
  verifyCookieConsent,
} = await import('./cookieConsent.ts');

describe('cookieConsent', () => {
  afterEach(() => {
    clearCookieConsent();
  });

  it('starts without consent', () => {
    clearCookieConsent();
    assert.equal(hasCookieConsent(), false);
  });

  it('accept writes durable consent and verify passes', () => {
    clearCookieConsent();
    const ok = acceptCookieConsent();
    assert.equal(ok, true);
    assert.equal(verifyCookieConsent(), true);
    assert.equal(hasCookieConsent(), true);
    assert.equal(localStorage.getItem('vplayer_cookie_consent'), '1');
    assert.match(document.cookie, /(?:^|;\s*)vplayer_consent=1(?:;|$)/);
  });

  it('clear removes consent', () => {
    assert.equal(acceptCookieConsent(), true);
    clearCookieConsent();
    assert.equal(hasCookieConsent(), false);
  });
});
