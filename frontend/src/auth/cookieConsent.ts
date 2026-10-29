const CONSENT_STORAGE_KEY = 'vplayer_cookie_consent';
const CONSENT_COOKIE = 'vplayer_consent';
const CONSENT_MAX_AGE_SEC = 60 * 60 * 24 * 365; // 1 year

function readConsentCookie(): boolean {
  if (typeof document === 'undefined') return false;
  return document.cookie.split(';').some((part) => {
    const [name, ...rest] = part.trim().split('=');
    return name === CONSENT_COOKIE && rest.join('=') === '1';
  });
}

function readConsentStorage(): boolean {
  try {
    return localStorage.getItem(CONSENT_STORAGE_KEY) === '1';
  } catch {
    return false;
  }
}

/** True when the user has accepted cookies (storage and/or cookie). */
export function hasCookieConsent(): boolean {
  return readConsentStorage() || readConsentCookie();
}

/**
 * Persist consent to localStorage + document.cookie, then verify both sides.
 * Returns true only when at least one durable write is readable back.
 */
export function acceptCookieConsent(): boolean {
  let storageOk = false;
  try {
    localStorage.setItem(CONSENT_STORAGE_KEY, '1');
    storageOk = localStorage.getItem(CONSENT_STORAGE_KEY) === '1';
  } catch {
    storageOk = false;
  }

  let cookieOk = false;
  if (typeof document !== 'undefined') {
    const secure = typeof location !== 'undefined' && location.protocol === 'https:' ? '; Secure' : '';
    document.cookie = `${CONSENT_COOKIE}=1; Path=/; Max-Age=${CONSENT_MAX_AGE_SEC}; SameSite=Lax${secure}`;
    cookieOk = readConsentCookie();
  }

  return storageOk || cookieOk;
}

/** Clear consent (used if decline / reset). Does not unlock the site. */
export function clearCookieConsent(): void {
  try {
    localStorage.removeItem(CONSENT_STORAGE_KEY);
  } catch {
    /* ignore */
  }
  if (typeof document === 'undefined') return;
  const secure = typeof location !== 'undefined' && location.protocol === 'https:' ? '; Secure' : '';
  document.cookie = `${CONSENT_COOKIE}=; Path=/; Max-Age=0; SameSite=Lax${secure}`;
}

/** Re-check that consent is actually stored after accept. */
export function verifyCookieConsent(): boolean {
  return hasCookieConsent();
}
