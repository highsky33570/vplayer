import { useEffect, useState } from 'react';
import { useLocation } from 'react-router-dom';

const SHOW_AFTER_PX = 420;

/** Desktop/window: bottom-right back-to-top, visible after scrolling down. */
export function ScrollToTop() {
  const { pathname } = useLocation();
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    if (pathname.startsWith('/watch') || pathname.startsWith('/admin')) {
      setVisible(false);
      return;
    }

    const onScroll = () => {
      setVisible(window.scrollY > SHOW_AFTER_PX);
    };
    onScroll();
    window.addEventListener('scroll', onScroll, { passive: true });
    return () => window.removeEventListener('scroll', onScroll);
  }, [pathname]);

  if (pathname.startsWith('/watch') || pathname.startsWith('/admin') || !visible) return null;

  return (
    <button
      type="button"
      className="scroll-to-top"
      aria-label="回到顶部"
      onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}
    >
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden>
        <path
          d="M6 14.5 12 8.5l6 6"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
    </button>
  );
}
