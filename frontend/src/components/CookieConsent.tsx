import { useCallback, useEffect, useId, useState } from 'react';
import { acceptCookieConsent, clearCookieConsent, hasCookieConsent, verifyCookieConsent } from '../auth/cookieConsent';

const ACCEPT_LOADING_MS = 1600;

type Props = {
  onAccepted?: () => void;
};

export function CookieConsent({ onAccepted }: Props) {
  const titleId = useId();
  const descId = useId();
  const [visible, setVisible] = useState(() => !hasCookieConsent());
  const [accepting, setAccepting] = useState(false);
  const [declineHint, setDeclineHint] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!visible) return;
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    document.documentElement.classList.add('cookie-gate-open');
    return () => {
      document.body.style.overflow = prevOverflow;
      document.documentElement.classList.remove('cookie-gate-open');
    };
  }, [visible]);

  const onDecline = useCallback(() => {
    if (accepting) return;
    clearCookieConsent();
    setError('');
    setDeclineHint(true);
  }, [accepting]);

  const onAccept = useCallback(async () => {
    if (accepting) return;
    setAccepting(true);
    setError('');
    setDeclineHint(false);

    await new Promise((r) => setTimeout(r, ACCEPT_LOADING_MS));

    const wrote = acceptCookieConsent();
    const ok = wrote && verifyCookieConsent();
    if (!ok) {
      clearCookieConsent();
      setAccepting(false);
      setError('无法写入 Cookie，请检查浏览器隐私设置后重试');
      return;
    }

    onAccepted?.();
    setVisible(false);
    setAccepting(false);
  }, [accepting, onAccepted]);

  if (!visible) return null;

  return (
    <div
      className="cookie-gate"
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      aria-describedby={descId}
    >
      <div className="cookie-gate-backdrop" aria-hidden="true" />
      <div className="cookie-gate-card">
        <div className="cookie-gate-mark" aria-hidden="true">
          <svg viewBox="0 0 48 48" width="40" height="40" fill="none">
            <circle cx="24" cy="24" r="22" stroke="currentColor" strokeWidth="2" opacity="0.35" />
            <circle cx="24" cy="24" r="14" stroke="currentColor" strokeWidth="2" />
            <circle cx="18" cy="20" r="2.2" fill="currentColor" />
            <circle cx="27" cy="17" r="1.8" fill="currentColor" />
            <circle cx="30" cy="26" r="2.4" fill="currentColor" />
            <circle cx="20" cy="29" r="1.6" fill="currentColor" />
          </svg>
        </div>
        <h2 id={titleId} className="cookie-gate-title">
          Cookie 与隐私
        </h2>
        <p id={descId} className="cookie-gate-desc">
          为保持登录状态、记住偏好并保障账户安全，本站需要使用 Cookie
          与本地存储。请先选择是否接受；接受后即可正常使用网站。
        </p>

        {declineHint ? (
          <p className="cookie-gate-hint" role="status">
            需接受 Cookie 后才能继续使用本站
          </p>
        ) : null}
        {error ? (
          <p className="cookie-gate-error" role="alert">
            {error}
          </p>
        ) : null}

        <div className="cookie-gate-actions">
          <button
            type="button"
            className="cookie-gate-decline"
            disabled={accepting}
            onClick={onDecline}
          >
            拒绝
          </button>
          <button
            type="button"
            className="cookie-gate-accept"
            disabled={accepting}
            onClick={() => void onAccept()}
          >
            {accepting ? (
              <>
                <span className="cookie-gate-spinner" aria-hidden="true" />
                正在启用…
              </>
            ) : (
              '接受'
            )}
          </button>
        </div>
      </div>
    </div>
  );
}
