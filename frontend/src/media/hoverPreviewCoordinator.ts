/**
 * Ensures at most one hover preview plays at a time and stops on tab hide.
 */

export type HoverPreviewStopper = () => void;

type ActiveSlot = {
  ownerId: string;
  stop: HoverPreviewStopper;
};

let active: ActiveSlot | null = null;
let visibilityBound = false;

function onVisibilityChange() {
  if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
    stopActivePreview();
  }
}

function ensureVisibilityListener() {
  if (visibilityBound || typeof document === 'undefined') return;
  document.addEventListener('visibilitychange', onVisibilityChange);
  visibilityBound = true;
}

/** Test helper: tear down global listener + active slot. */
export function resetHoverPreviewCoordinator() {
  stopActivePreview();
  if (visibilityBound && typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', onVisibilityChange);
  }
  visibilityBound = false;
}

export function getActiveHoverPreviewOwnerId(): string | null {
  return active?.ownerId ?? null;
}

export function stopActivePreview() {
  const slot = active;
  active = null;
  if (slot) {
    try {
      slot.stop();
    } catch {
      /* ignore cleanup errors */
    }
  }
}

/**
 * Claim exclusive preview ownership. Previous owner is stopped immediately.
 * Returns a release function that only clears the slot if still this owner.
 */
export function claimHoverPreview(ownerId: string, stop: HoverPreviewStopper): () => void {
  ensureVisibilityListener();
  if (active && active.ownerId !== ownerId) {
    const prev = active;
    active = null;
    try {
      prev.stop();
    } catch {
      /* ignore */
    }
  }
  active = { ownerId, stop };
  return () => {
    if (active?.ownerId === ownerId) {
      active = null;
    }
  };
}
