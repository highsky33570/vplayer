import {
  useEffect,
  useId,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from 'react';
import { useAuth } from '../auth/AuthContext';
import { attachHoverPreview, type AttachedHoverPreview } from '../media/attachHoverPreview';
import { coverImgSrc, onCoverImgError } from '../media/coverFallback';
import {
  claimHoverPreview,
  stopActivePreview,
} from '../media/hoverPreviewCoordinator';
import {
  HOVER_PREVIEW_DELAY_MS,
  canResolveHoverPreview,
  supportsHoverPreview,
} from '../media/hoverPreviewLogic';
import { resolveHoverPreviewMedia } from '../media/hoverPreviewResolve';

type Props = {
  videoId: number;
  coverUrl: string;
  durationSec?: number;
  className?: string;
  imgClassName?: string;
  /** Extra overlays inside the thumb (duration badge, progress bar, etc.). */
  children?: ReactNode;
  style?: CSSProperties;
  'data-testid'?: string;
};

type Phase = 'idle' | 'pending' | 'loading' | 'playing' | 'failed';

/**
 * Poster thumb that optionally swaps in a muted in-card hover preview on desktop.
 * Fail-closed: guests, restricted media, and errors keep the poster.
 */
export function HoverPreviewThumb({
  videoId,
  coverUrl,
  durationSec = 0,
  className,
  imgClassName,
  children,
  style,
  'data-testid': testId,
}: Props) {
  const ownerId = useId();
  const { token, authRequired } = useAuth();
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const attachedRef = useRef<AttachedHoverPreview | null>(null);
  const releaseClaimRef = useRef<(() => void) | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const genRef = useRef(0);
  const failedRef = useRef(false);
  const phaseRef = useRef<Phase>('idle');
  const [phase, setPhase] = useState<Phase>('idle');

  function setPhaseBoth(next: Phase) {
    phaseRef.current = next;
    setPhase(next);
  }

  function clearTimer() {
    if (timerRef.current != null) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }

  function destroyAttached() {
    attachedRef.current?.destroy();
    attachedRef.current = null;
    const el = videoRef.current;
    if (el) {
      try {
        el.pause();
        el.removeAttribute('src');
        el.load();
      } catch {
        /* ignore */
      }
    }
  }

  function teardownMedia() {
    genRef.current += 1;
    clearTimer();
    releaseClaimRef.current?.();
    releaseClaimRef.current = null;
    destroyAttached();
    if (!failedRef.current) setPhaseBoth('idle');
  }

  useEffect(() => {
    return () => {
      genRef.current += 1;
      clearTimer();
      releaseClaimRef.current?.();
      releaseClaimRef.current = null;
      destroyAttached();
    };
  }, []);

  async function activatePreview() {
    if (failedRef.current) return;
    if (!supportsHoverPreview()) return;
    if (!canResolveHoverPreview(token, authRequired)) return;

    const myGen = ++genRef.current;
    setPhaseBoth('loading');

    releaseClaimRef.current?.();
    releaseClaimRef.current = claimHoverPreview(ownerId, () => {
      genRef.current += 1;
      clearTimer();
      destroyAttached();
      releaseClaimRef.current = null;
      if (!failedRef.current) setPhaseBoth('idle');
    });

    const media = await resolveHoverPreviewMedia(videoId, token, authRequired);
    if (myGen !== genRef.current) return;
    if (!media) {
      failedRef.current = true;
      setPhaseBoth('failed');
      releaseClaimRef.current?.();
      releaseClaimRef.current = null;
      return;
    }

    const video = videoRef.current;
    if (!video) {
      failedRef.current = true;
      setPhaseBoth('failed');
      releaseClaimRef.current?.();
      releaseClaimRef.current = null;
      return;
    }

    destroyAttached();
    attachedRef.current = attachHoverPreview(video, {
      url: media.url,
      type: media.type,
      durationSec,
      onReady: () => {
        if (myGen !== genRef.current) return;
        setPhaseBoth('playing');
      },
      onFailure: () => {
        if (myGen !== genRef.current) return;
        failedRef.current = true;
        destroyAttached();
        releaseClaimRef.current?.();
        releaseClaimRef.current = null;
        setPhaseBoth('failed');
      },
    });
  }

  function onMouseEnter() {
    // Touch/coarse pointers never match the hover media query.
    if (!supportsHoverPreview()) return;
    if (failedRef.current) return;
    if (!canResolveHoverPreview(token, authRequired)) return;
    const cur = phaseRef.current;
    if (cur === 'playing' || cur === 'loading' || cur === 'pending') return;

    setPhaseBoth('pending');
    clearTimer();
    timerRef.current = setTimeout(() => {
      timerRef.current = null;
      void activatePreview();
    }, HOVER_PREVIEW_DELAY_MS);
  }

  function onMouseLeave() {
    clearTimer();
    if (phaseRef.current === 'pending') {
      setPhaseBoth(failedRef.current ? 'failed' : 'idle');
      return;
    }
    teardownMedia();
  }

  const showVideo = phase === 'loading' || phase === 'playing';
  const posterFaded = phase === 'playing';

  return (
    <span
      className={`hover-preview-thumb${className ? ` ${className}` : ''}${
        showVideo ? ' is-previewing' : ''
      }`}
      style={style}
      data-testid={testId}
      data-preview-phase={phase}
      onMouseEnter={onMouseEnter}
      onMouseLeave={onMouseLeave}
    >
      <img
        className={`hover-preview-poster${imgClassName ? ` ${imgClassName}` : ''}${
          posterFaded ? ' is-faded' : ''
        }`}
        src={coverImgSrc(coverUrl)}
        alt=""
        loading="lazy"
        onError={onCoverImgError}
        draggable={false}
      />
      <video
        ref={videoRef}
        className={`hover-preview-video${posterFaded ? ' is-visible' : ''}`}
        muted
        playsInline
        preload="none"
        controls={false}
        tabIndex={-1}
        aria-hidden
        style={{ pointerEvents: 'none' }}
      />
      {children}
    </span>
  );
}

/** Stop any in-flight page preview (e.g. on route change). */
export function stopAllHoverPreviews() {
  stopActivePreview();
}
