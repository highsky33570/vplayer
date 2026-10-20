/** Tiny window-level channel so the Messages page can refresh the header unread badge. */
const EVENT_NAME = 'vplayer:unread-count';

export function publishUnreadCount(count: number): void {
  if (typeof window === 'undefined') return;
  window.dispatchEvent(new CustomEvent<number>(EVENT_NAME, { detail: Math.max(0, count) }));
}

export function subscribeUnreadCount(listener: (count: number) => void): () => void {
  const handler = (event: Event) => {
    const detail = (event as CustomEvent<number>).detail;
    if (typeof detail === 'number') listener(detail);
  };
  window.addEventListener(EVENT_NAME, handler);
  return () => window.removeEventListener(EVENT_NAME, handler);
}
