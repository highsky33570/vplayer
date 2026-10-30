/** True for admin-panel paths (excludes the dedicated admin login page itself). */
export function isAdminAppPath(path: string): boolean {
  const p = path.trim();
  if (p === '/admin' || p.startsWith('/admin/')) {
    if (p === '/admin/login' || p.startsWith('/admin/login?')) return false;
    return true;
  }
  return false;
}

/** Build login URL. Admin destinations use /admin/login; everything else uses /login. */
export function loginWithNext(nextPath: string): string {
  const next = nextPath.startsWith('/') ? nextPath : `/${nextPath}`;
  if (isAdminAppPath(next)) {
    return `/admin/login?next=${encodeURIComponent(next)}`;
  }
  return `/login?next=${encodeURIComponent(next)}`;
}

/** Dedicated admin login URL (defaults next to /admin). */
export function adminLoginWithNext(nextPath?: string): string {
  const next = nextPath && nextPath.startsWith('/') ? nextPath : '/admin';
  const safe = isAdminAppPath(next) ? next : '/admin';
  return `/admin/login?next=${encodeURIComponent(safe)}`;
}

/** Href for opening a video. The watch page is public; playback stays authenticated. */
export function videoEntryHref(
  ready: boolean,
  _authenticated: boolean,
  videoId: number,
): string | null {
  if (!Number.isFinite(videoId) || videoId <= 0) return null;
  if (!ready) return null;
  return `/watch/${videoId}`;
}

/**
 * Validate `next` as an internal app path only.
 * Rejects protocol-relative, absolute URLs, and other open-redirect shapes.
 */
export function safeInternalPath(next: string | null | undefined): string | null {
  if (next == null) return null;
  let value = next.trim();
  if (!value) return null;
  try {
    value = decodeURIComponent(value);
  } catch {
    return null;
  }
  value = value.trim();
  if (!value.startsWith('/')) return null;
  // Reject "//evil.com", "/\\evil", and scheme-like paths.
  if (value.startsWith('//') || value.startsWith('/\\')) return null;
  if (/^\/[a-zA-Z][a-zA-Z0-9+.-]*:/.test(value)) return null;
  if (/[\r\n\0]/.test(value)) return null;
  return value;
}

/** Post-login navigation target from the `next` query param. */
export function resolvePostLoginPath(next: string | null | undefined): string {
  return safeInternalPath(next) ?? '/';
}

/** Post-login target for the admin login page — always stays in /admin*. */
export function resolveAdminPostLoginPath(next: string | null | undefined): string {
  const p = safeInternalPath(next);
  if (p && isAdminAppPath(p)) return p;
  return '/admin';
}
