import type { AuthUser } from '../api';
import { avatarHue, avatarInitial, avatarStyle } from './avatar';

type Props = {
  user: Pick<AuthUser, 'id' | 'nickname' | 'avatar_hue'>;
  /** Overrides the hue (e.g. live preview while picking). */
  hue?: number;
  className?: string;
};

/** Round gradient avatar with the nickname initial. */
export function UserAvatar({ user, hue, className = '' }: Props) {
  const h = hue != null && hue >= 0 ? hue : avatarHue(user);
  return (
    <span
      className={`banner-avatar ${className}`.trim()}
      style={{ background: avatarStyle(h) }}
      aria-hidden
    >
      {avatarInitial(user.nickname)}
    </span>
  );
}
