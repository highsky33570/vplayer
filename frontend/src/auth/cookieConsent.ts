const CONSENT_STORAGE_KEY = 'vplayer_cookie_consent';
const CONSENT_COOKIE = 'vplayer_consent';
const CONSENT_MAX_AGE_SEC = 60 * 60 * 24 * 365; // 1 year

export function hasCookieConsent(): boolean {
  try {
    if (localStorage.getItem(CONSENT_STORAGE_KEY) === '1') return true;
  } catch {
    /* ignore */
  }
  if (typeof document === 'undefined') return false;
  return document.cookie.split(';').some((part) => part.trim().startsWith(`${CONSENT_COOKIE}=1`));
}

export function acceptCookieConsent(): void {
  try {
    localStorage.setItem(CONSENT_STORAGE_KEY, '1');
  } catch {
    /* ignore */
  }
  if (typeof document === 'undefined') return;
  const secure = typeof location !== 'undefined' && location.protocol === 'https:' ? '; Secure' : '';
  document.cookie = `${CONSENT_COOKIE}=1; Path=/; Max-Age=${CONSENT_MAX_AGE_SEC}; SameSite=Lax${secure}`;
}
