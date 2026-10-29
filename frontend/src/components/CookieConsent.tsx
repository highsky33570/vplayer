import { useCallback, useState } from 'react';
import { acceptCookieConsent, hasCookieConsent } from '../auth/cookieConsent';

type Props = {
  onAccepted?: () => void;
};

export function CookieConsent({ onAccepted }: Props) {
  const [visible, setVisible] = useState(() => !hasCookieConsent());

  const accept = useCallback(() => {
    acceptCookieConsent();
    setVisible(false);
    onAccepted?.();
  }, [onAccepted]);

  if (!visible) return null;

  return (
    <div className="cookie-consent" role="dialog" aria-labelledby="cookie-consent-title" aria-describedby="cookie-consent-desc">
      <div className="cookie-consent-inner">
        <div className="cookie-consent-copy">
          <p id="cookie-consent-title" className="cookie-consent-title">
            Cookie 使用提示
          </p>
          <p id="cookie-consent-desc" className="cookie-consent-desc">
            我们使用 Cookie 与本地存储以保持登录状态、记住偏好并改善浏览体验。继续使用前请点击接受。
          </p>
        </div>
        <button type="button" className="cookie-consent-accept" onClick={accept}>
          接受
        </button>
      </div>
    </div>
  );
}
