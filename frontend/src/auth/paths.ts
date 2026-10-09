/** Build /login?next=... for a watch (or other internal) path. */
export function loginWithNext(nextPath: string): string {
  const next = nextPath.startsWith('/') ? nextPath : `/${nextPath}`;
  return `/login?next=${encodeURIComponent(next)}`;
}

/** Href for opening a video: login redirect for guests, watch for authenticated. */
export function videoEntryHref(
  ready: boolean,
  authenticated: boolean,
  videoId: number,
): string | null {
  if (!Number.isFinite(videoId) || videoId <= 0) return null;
  if (!ready) return null;
  const watch = `/watch/${videoId}`;
  if (!authenticated) return loginWithNext(watch);
  return watch;
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
