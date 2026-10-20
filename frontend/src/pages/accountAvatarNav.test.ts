/**
 * Integration: click the real HeaderAccountAvatar (same trigger Header mounts)
 * and assert window.location.pathname becomes /account.
 */
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { after, before, describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';
import { JSDOM } from 'jsdom';
import React, { createElement, type ReactNode } from 'react';
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { BrowserRouter, MemoryRouter, Outlet, Route, Routes } from 'react-router-dom';
import { AuthTestProvider } from '../auth/AuthContext.tsx';
import { HeaderAccountAvatar } from '../components/HeaderAccountAvatar.tsx';
import { HeaderFlyout } from '../components/HeaderFlyout.tsx';
import { ThemeProvider } from '../theme/ThemeContext.tsx';
import { AccountPage } from './AccountPage.tsx';

// Classic JSX emit in some deps expects a global React; act() needs the flag.
(globalThis as { React?: typeof React }).React = React;
(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const here = dirname(fileURLToPath(import.meta.url));
const rootDir = join(here, '..');

const TEST_USER = {
  id: 42,
  email: 'space@vplayer.test',
  nickname: '空间测试',
  is_admin: false,
  avatar_hue: 210,
};

function installDom(url = 'http://localhost/') {
  const dom = new JSDOM('<!DOCTYPE html><html><body><div id="root"></div></body></html>', {
    url,
    pretendToBeVisual: true,
  });
  const win = dom.window;
  Object.defineProperty(globalThis, 'window', { value: win, configurable: true });
  Object.defineProperty(globalThis, 'document', { value: win.document, configurable: true });
  Object.defineProperty(globalThis, 'navigator', { value: win.navigator, configurable: true });
  Object.defineProperty(globalThis, 'HTMLElement', { value: win.HTMLElement, configurable: true });
  Object.defineProperty(globalThis, 'Node', { value: win.Node, configurable: true });
  Object.defineProperty(globalThis, 'MutationObserver', {
    value: win.MutationObserver,
    configurable: true,
  });
  Object.defineProperty(globalThis, 'getComputedStyle', {
    value: win.getComputedStyle.bind(win),
    configurable: true,
  });
  Object.defineProperty(globalThis, 'localStorage', {
    value: win.localStorage,
    configurable: true,
  });
  win.matchMedia = ((query: string) => ({
    matches: String(query).includes('min-width: 1024px'),
    media: query,
    onchange: null,
    addListener() {},
    removeListener() {},
    addEventListener() {},
    removeEventListener() {},
    dispatchEvent() {
      return false;
    },
  })) as typeof win.matchMedia;
  win.scrollTo = () => undefined;
  win.HTMLElement.prototype.scrollIntoView = () => undefined;
  return dom;
}

function AvatarHeaderChrome({ onClose }: { onClose: () => void }) {
  return createElement(
    'div',
    { className: 'banner-actions', 'data-testid': 'header-chrome' },
    createElement(
      HeaderFlyout,
      {
        id: 'account',
        open: false,
        enabled: true,
        label: '账号',
        onOpen: () => undefined,
        onClose: () => undefined,
        trigger: createElement(HeaderAccountAvatar, {
          user: TEST_USER,
          active: false,
          flyoutOpen: false,
          onNavigate: onClose,
        }),
      },
      createElement('div', null, 'preview'),
    ),
  );
}

function shellTree(child: ReactNode) {
  return createElement(
    ThemeProvider,
    null,
    createElement(
      AuthTestProvider,
      { user: TEST_USER },
      createElement(
        'div',
        { className: 'shell' },
        createElement(AvatarHeaderChrome, { onClose: () => undefined }),
        child,
      ),
    ),
  );
}

function stubFetch() {
  const original = globalThis.fetch;
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.includes('/api/v1/me/summary')) {
      return new Response(
        JSON.stringify({ ok: true, data: { favorites: 2, history: 3, unread_notifications: 0 } }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    }
    if (
      url.includes('/api/v1/me/favorite-folders') ||
      url.includes('/api/v1/me/favorites') ||
      url.includes('/api/v1/me/history')
    ) {
      return new Response(
        JSON.stringify({ ok: true, data: [], total: 0, limit: 24, offset: 0, has_more: false }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      );
    }
    if (typeof original === 'function') return original(input);
    return new Response(JSON.stringify({ ok: false }), { status: 404 });
  }) as typeof fetch;
  return () => {
    globalThis.fetch = original;
  };
}

describe('avatar click navigates to /account', () => {
  let dom: JSDOM;
  let reactRoot: Root;
  let restoreFetch: () => void;

  before(() => {
    dom = installDom('http://localhost/');
    restoreFetch = stubFetch();
    reactRoot = createRoot(document.getElementById('root')!);
  });

  after(() => {
    reactRoot.unmount();
    restoreFetch();
    dom.window.close();
  });

  it('Header mounts HeaderAccountAvatar Link to /account', () => {
    const headerSrc = readFileSync(join(rootDir, 'components/Header.tsx'), 'utf8');
    const avatarSrc = readFileSync(join(rootDir, 'components/HeaderAccountAvatar.tsx'), 'utf8');
    assert.match(headerSrc, /HeaderAccountAvatar/);
    assert.match(avatarSrc, /to="\/account"/);
    assert.match(avatarSrc, /data-testid="header-account-avatar"/);
    assert.equal(avatarSrc.includes("navigate('/account')"), false);
  });

  it('clicking the avatar sets window.location.pathname to /account and renders AccountPage', async () => {
    assert.equal(window.location.pathname, '/');

    await act(async () => {
      reactRoot.render(
        createElement(
          BrowserRouter,
          null,
          createElement(
            Routes,
            null,
            createElement(
              Route,
              {
                path: '/',
                element: shellTree(createElement(Outlet, null)),
              },
              createElement(Route, {
                index: true,
                element: createElement('div', { 'data-testid': 'home-marker' }, 'Home'),
              }),
              createElement(Route, {
                path: 'account',
                element: createElement(AccountPage, null),
              }),
            ),
          ),
        ),
      );
    });

    const avatar = document.querySelector(
      '[data-testid="header-account-avatar"]',
    ) as HTMLAnchorElement | null;
    assert.ok(avatar, 'avatar trigger missing');
    assert.equal(avatar.getAttribute('href'), '/account');
    assert.ok(document.querySelector('[data-testid="header-chrome"]'));

    await act(async () => {
      avatar.dispatchEvent(
        new window.MouseEvent('click', { bubbles: true, cancelable: true, view: window }),
      );
    });

    assert.equal(
      window.location.pathname,
      '/account',
      `expected /account after avatar click, got ${window.location.pathname}`,
    );
    assert.ok(document.querySelector('[data-testid="account-page"]'), 'AccountPage missing');
    assert.equal(document.querySelector('[data-testid="home-marker"]'), null);
    assert.equal(document.querySelector('.discovery'), null);
    assert.equal(document.querySelector('.banner-carousel'), null);
  });

  it('direct /account entry renders AccountPage without homepage DiscoveryNav/BannerCarousel', async () => {
    const mount = document.createElement('div');
    document.body.appendChild(mount);
    const localRoot = createRoot(mount);

    await act(async () => {
      localRoot.render(
        createElement(
          MemoryRouter,
          { initialEntries: ['/account'] },
          createElement(
            Routes,
            null,
            createElement(
              Route,
              {
                path: '/',
                element: shellTree(createElement(Outlet, null)),
              },
              createElement(Route, {
                index: true,
                element: createElement('div', { 'data-testid': 'home-marker' }, 'Home'),
              }),
              createElement(Route, {
                path: 'account',
                element: createElement(AccountPage, null),
              }),
            ),
          ),
        ),
      );
    });

    assert.ok(mount.querySelector('[data-testid="account-page"]'));
    assert.equal(mount.querySelector('[data-testid="home-marker"]'), null);
    assert.equal(mount.querySelector('.discovery'), null);
    assert.equal(mount.querySelector('.banner-carousel'), null);

    localRoot.unmount();
    mount.remove();
  });
});
