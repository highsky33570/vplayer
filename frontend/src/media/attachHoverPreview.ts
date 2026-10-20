import { isHlsPlayback, previewLoopEndSec, previewStartSec } from './hoverPreviewLogic';

export type AttachHoverPreviewOptions = {
  url: string;
  type?: string;
  durationSec?: number | null;
  onReady?: () => void;
  onFailure?: () => void;
};

export type AttachedHoverPreview = {
  destroy: () => void;
};

type HlsLike = {
  load: (url?: string) => Promise<void>;
  destroy: () => Promise<void>;
};

type HlsCtor = {
  isSupported: (mediaType?: 'video' | 'audio' | null) => boolean;
  new (cfg: Record<string, unknown>): HlsLike;
};

function canNativeHls(video: HTMLVideoElement): boolean {
  return (
    video.canPlayType('application/vnd.apple.mpegurl') !== '' ||
    video.canPlayType('application/x-mpegURL') !== ''
  );
}

async function loadHlsCtor(): Promise<HlsCtor | null> {
  try {
    const mod = (await import('xgplayer-hls')) as {
      Hls?: HlsCtor;
      default?: { Hls?: HlsCtor } | HlsCtor;
    };
    if (mod.Hls) return mod.Hls;
    const d = mod.default;
    if (d && typeof d === 'object' && 'Hls' in d && d.Hls) return d.Hls;
    return null;
  } catch {
    return null;
  }
}

/**
 * Attach muted preview media to a video element.
 * Progressive URLs use native src; HLS uses xgplayer-hls when native HLS is unavailable.
 * xgplayer-hls is loaded lazily so catalog pages stay free of a hard dependency at import time.
 */
export function attachHoverPreview(
  video: HTMLVideoElement,
  opts: AttachHoverPreviewOptions,
): AttachedHoverPreview {
  const start = previewStartSec(opts.durationSec);
  const loopEnd = previewLoopEndSec(start, opts.durationSec);
  let destroyed = false;
  let hls: HlsLike | null = null;
  let failed = false;

  const fail = () => {
    if (destroyed || failed) return;
    failed = true;
    opts.onFailure?.();
  };

  const onTimeUpdate = () => {
    if (destroyed || failed) return;
    if (video.currentTime >= loopEnd - 0.05) {
      try {
        video.currentTime = start;
      } catch {
        /* ignore seek errors */
      }
    }
  };

  const onPlaying = () => {
    if (destroyed || failed) return;
    opts.onReady?.();
  };

  const onError = () => fail();

  video.muted = true;
  video.defaultMuted = true;
  video.playsInline = true;
  video.setAttribute('playsinline', '');
  video.setAttribute('webkit-playsinline', '');
  video.controls = false;
  video.preload = 'metadata';
  video.disablePictureInPicture = true;

  video.addEventListener('timeupdate', onTimeUpdate);
  video.addEventListener('playing', onPlaying);
  video.addEventListener('error', onError);

  const tryPlay = () => {
    if (destroyed || failed) return;
    const p = video.play();
    if (p && typeof p.then === 'function') {
      p.catch(() => fail());
    }
  };

  const applySeek = () => {
    if (destroyed || failed || start <= 0) return;
    try {
      if (Number.isFinite(video.duration) && video.duration > 0) {
        video.currentTime = Math.min(start, Math.max(0, video.duration - 0.5));
      } else {
        video.currentTime = start;
      }
    } catch {
      /* some sources reject early seek */
    }
  };

  const seekThenPlay = () => {
    if (destroyed || failed) return;
    const onMeta = () => {
      video.removeEventListener('loadedmetadata', onMeta);
      applySeek();
    };
    video.addEventListener('loadedmetadata', onMeta);
    if (video.readyState >= 1) applySeek();
    // Start playback immediately; seek applies when metadata is ready.
    tryPlay();
  };

  const url = opts.url.trim();
  const hlsMode = isHlsPlayback(url, opts.type);

  void (async () => {
    if (destroyed || failed) return;
    try {
      if (hlsMode && !canNativeHls(video)) {
        const Hls = await loadHlsCtor();
        if (destroyed || failed) return;
        if (!Hls || !Hls.isSupported()) {
          fail();
          return;
        }
        hls = new Hls({
          media: video,
          startTime: start,
          loadTimeout: 12_000,
          manifestLoadTimeout: 12_000,
          retryCount: 1,
        });
        await hls.load(url);
        if (destroyed || failed) return;
        tryPlay();
        return;
      }
      video.src = url;
      seekThenPlay();
    } catch {
      fail();
    }
  })();

  return {
    destroy() {
      if (destroyed) return;
      destroyed = true;
      video.removeEventListener('timeupdate', onTimeUpdate);
      video.removeEventListener('playing', onPlaying);
      video.removeEventListener('error', onError);
      try {
        video.pause();
      } catch {
        /* ignore */
      }
      if (hls) {
        const instance = hls;
        hls = null;
        void instance.destroy().catch(() => undefined);
      }
      try {
        video.removeAttribute('src');
        video.load();
      } catch {
        /* ignore */
      }
    },
  };
}
