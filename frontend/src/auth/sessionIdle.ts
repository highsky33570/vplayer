/** Logged-in session expires after this much idle time. */
export const SESSION_IDLE_MS = 60 * 60 * 1000; // 1 hour

export const LAST_ACTIVITY_KEY = 'vplayer_last_activity';

export function readLastActivity(): number | null {
  try {
    const raw = localStorage.getItem(LAST_ACTIVITY_KEY);
    if (!raw) return null;
    const n = Number(raw);
    return Number.isFinite(n) && n > 0 ? n : null;
  } catch {
    return null;
  }
}

export function touchLastActivity(at = Date.now()): void {
  try {
    localStorage.setItem(LAST_ACTIVITY_KEY, String(at));
  } catch {
    /* ignore */
  }
}

export function clearLastActivity(): void {
  try {
    localStorage.removeItem(LAST_ACTIVITY_KEY);
  } catch {
    /* ignore */
  }
}

export function isSessionIdle(now = Date.now()): boolean {
  const last = readLastActivity();
  if (last == null) return false;
  return now - last >= SESSION_IDLE_MS;
}

/** Bind user/visibility activity → touchLastActivity. Returns cleanup. */
export function bindSessionActivity(onActivity: () => void): () => void {
  let lastFire = 0;
  const throttleMs = 1000;
  const fire = () => {
    const now = Date.now();
    if (now - lastFire < throttleMs) return;
    lastFire = now;
    onActivity();
  };

  const opts: AddEventListenerOptions = { capture: true, passive: true };
  const events = ['pointerdown', 'keydown', 'touchstart', 'scroll', 'mousemove'] as const;
  for (const ev of events) {
    window.addEventListener(ev, fire, opts);
  }

  const onVisibility = () => {
    if (document.visibilityState === 'visible') fire();
  };
  document.addEventListener('visibilitychange', onVisibility);

  // Watching a long video without clicks still counts as using the site.
  const heartbeat = window.setInterval(() => {
    if (document.visibilityState === 'visible') fire();
  }, 5 * 60 * 1000);

  return () => {
    for (const ev of events) {
      window.removeEventListener(ev, fire, opts);
    }
    document.removeEventListener('visibilitychange', onVisibility);
    window.clearInterval(heartbeat);
  };
}
