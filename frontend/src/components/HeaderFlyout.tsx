import { useEffect, useRef, type ReactNode } from 'react';
import {
  FLYOUT_CLOSE_DELAY_MS,
  FLYOUT_OPEN_DELAY_MS,
  flyoutPanelClass,
  type HeaderFlyoutId,
} from './headerFlyoutLogic';

type Props = {
  id: HeaderFlyoutId;
  open: boolean;
  enabled: boolean;
  label: string;
  enlarged?: boolean;
  onOpen: (id: HeaderFlyoutId) => void;
  onClose: (id: HeaderFlyoutId) => void;
  trigger: ReactNode;
  children: ReactNode;
};

export function HeaderFlyout({
  id,
  open,
  enabled,
  label,
  enlarged = false,
  onOpen,
  onClose,
  trigger,
  children,
}: Props) {
  const rootRef = useRef<HTMLDivElement>(null);
  const openTimer = useRef<number | null>(null);
  const closeTimer = useRef<number | null>(null);

  function clearTimers() {
    if (openTimer.current != null) window.clearTimeout(openTimer.current);
    if (closeTimer.current != null) window.clearTimeout(closeTimer.current);
    openTimer.current = null;
    closeTimer.current = null;
  }

  useEffect(() => () => clearTimers(), []);

  useEffect(() => {
    if (!open) return;
    function onKey(event: KeyboardEvent) {
      if (event.key === 'Escape') onClose(id);
    }
    function onPointerDown(event: PointerEvent) {
      const root = rootRef.current;
      if (!root || !root.contains(event.target as Node)) onClose(id);
    }
    document.addEventListener('keydown', onKey);
    document.addEventListener('pointerdown', onPointerDown);
    return () => {
      document.removeEventListener('keydown', onKey);
      document.removeEventListener('pointerdown', onPointerDown);
    };
  }, [id, onClose, open]);

  if (!enabled) return trigger;

  return (
    <div
      ref={rootRef}
      className={`header-flyout-anchor${open ? ' is-open' : ''}${enlarged ? ' is-enlarged' : ''}`}
      onMouseEnter={() => {
        clearTimers();
        openTimer.current = window.setTimeout(() => onOpen(id), FLYOUT_OPEN_DELAY_MS);
      }}
      onMouseLeave={() => {
        clearTimers();
        closeTimer.current = window.setTimeout(() => onClose(id), FLYOUT_CLOSE_DELAY_MS);
      }}
      onFocus={() => {
        clearTimers();
        onOpen(id);
      }}
      onBlur={(event) => {
        const next = event.relatedTarget;
        if (next instanceof Node && event.currentTarget.contains(next)) return;
        clearTimers();
        closeTimer.current = window.setTimeout(() => onClose(id), FLYOUT_CLOSE_DELAY_MS);
      }}
    >
      {trigger}
      {open ? (
        <div className={flyoutPanelClass(id)} role="dialog" aria-label={label}>
          {children}
        </div>
      ) : null}
    </div>
  );
}
