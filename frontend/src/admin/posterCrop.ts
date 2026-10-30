/** Default movie-poster aspect when the current cover has unknown dimensions. */
export const DEFAULT_POSTER_ASPECT = { w: 2, h: 3 };

/**
 * Center-crop an image file to aspect w:h and return a JPEG blob.
 * Matches server-side PutAdminPoster crop behavior.
 */
export async function cropImageFileToAspect(
  file: File | Blob,
  aspectW: number,
  aspectH: number,
  quality = 0.9,
): Promise<Blob> {
  const w = aspectW > 0 ? aspectW : DEFAULT_POSTER_ASPECT.w;
  const h = aspectH > 0 ? aspectH : DEFAULT_POSTER_ASPECT.h;
  const target = w / h;

  const bitmap = await createImageBitmap(file);
  try {
    const sw = bitmap.width;
    const sh = bitmap.height;
    const cur = sw / sh;
    let cw: number;
    let ch: number;
    if (cur > target) {
      ch = sh;
      cw = Math.max(1, Math.round(sh * target));
    } else {
      cw = sw;
      ch = Math.max(1, Math.round(sw / target));
    }
    const sx = Math.max(0, Math.floor((sw - cw) / 2));
    const sy = Math.max(0, Math.floor((sh - ch) / 2));
    const canvas = document.createElement('canvas');
    canvas.width = cw;
    canvas.height = ch;
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new Error('canvas unavailable');
    ctx.drawImage(bitmap, sx, sy, cw, ch, 0, 0, cw, ch);
    const blob = await new Promise<Blob>((resolve, reject) => {
      canvas.toBlob(
        (b) => (b ? resolve(b) : reject(new Error('encode failed'))),
        'image/jpeg',
        quality,
      );
    });
    return blob;
  } finally {
    bitmap.close();
  }
}

/** Read natural size of an image URL (current poster). */
export function loadImageNaturalSize(src: string): Promise<{ w: number; h: number }> {
  return new Promise((resolve) => {
    if (!src) {
      resolve(DEFAULT_POSTER_ASPECT);
      return;
    }
    const img = new Image();
    img.onload = () => {
      if (img.naturalWidth > 0 && img.naturalHeight > 0) {
        resolve({ w: img.naturalWidth, h: img.naturalHeight });
      } else {
        resolve(DEFAULT_POSTER_ASPECT);
      }
    };
    img.onerror = () => resolve(DEFAULT_POSTER_ASPECT);
    img.src = src;
  });
}
