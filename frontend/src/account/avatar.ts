/** Avatar helpers: stable per-user hue, initial letter, and CSS gradient. */

export type AvatarUserLike = {
  id: number;
  avatar_hue?: number | null;
};

/** Hue swatches offered on the account page (plus "default" = -1). */
export const AVATAR_HUE_CHOICES = [0, 30, 60, 120, 180, 210, 270, 320] as const;

/** Hue used when the user has not picked one. Deterministic per user id. */
export function derivedHue(id: number): number {
  const n = Number.isFinite(id) ? Math.abs(Math.trunc(id)) : 0;
  // 137 is coprime with 360, so neighbouring ids spread around the wheel.
  return (n * 137 + 200) % 360;
}

/** Custom hue (0..359) when set, otherwise a stable hue derived from the user id. */
export function avatarHue(user: AvatarUserLike): number {
  const custom = user.avatar_hue;
  if (typeof custom === 'number' && Number.isFinite(custom) && custom >= 0) {
    return Math.floor(custom) % 360;
  }
  return derivedHue(user.id);
}

/** First visible character, upper-cased; falls back to 用 for blank names. */
export function avatarInitial(nickname: string | null | undefined): string {
  const name = (nickname ?? '').trim();
  if (!name) return '用';
  return Array.from(name)[0].toUpperCase();
}

/** CSS `background` value for an avatar circle. */
export function avatarStyle(hue: number): string {
  const h = ((Math.round(hue) % 360) + 360) % 360;
  const h2 = (h + 40) % 360;
  return `linear-gradient(145deg, hsl(${h} 78% 70%), hsl(${h2} 70% 46%))`;
}
