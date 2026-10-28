/** Pure helpers for the account, favorites, history and messages pages. */

export const NICKNAME_MAX_RUNES = 64;

/** Returns an error message, or null when the nickname can be saved. */
export function nicknameValidationMessage(raw: string): string | null {
  const name = raw.trim();
  if (!name) return '昵称不能为空';
  if (Array.from(name).length > NICKNAME_MAX_RUNES) return `昵称不能超过 ${NICKNAME_MAX_RUNES} 个字`;
  return null;
}

export function roleLabel(isAdmin: boolean | undefined): string {
  return isAdmin ? '管理员' : '普通用户';
}

/** Account membership badge / expiry copy. */
export function membershipBadgeLabel(user: {
  is_admin?: boolean;
  is_member?: boolean;
  membership_days_remaining?: number;
  membership_expires_at?: string;
} | null | undefined): { badge: string; detail: string } | null {
  if (!user) return null;
  const days = user.membership_days_remaining ?? 0;
  // Paid / time-bounded membership wins over bare admin lifetime copy.
  if (user.membership_expires_at && (user.is_member || user.is_admin)) {
    const until = new Date(user.membership_expires_at).toLocaleDateString();
    return {
      badge: '大会员',
      detail: days > 0 ? `剩余 ${days} 天 · 有效至 ${until}` : `有效至 ${until}`,
    };
  }
  if (user.is_admin) {
    return { badge: '大会员', detail: '管理员权限 · 永久有效' };
  }
  if (!user.is_member) return null;
  return { badge: '大会员', detail: '已开通' };
}

/** True when paid membership still has remaining days — purchase UI should lock. */
export function membershipPurchaseLocked(user: {
  membership_days_remaining?: number;
  membership_expires_at?: string;
} | null | undefined): boolean {
  if (!user) return false;
  const days = user.membership_days_remaining ?? 0;
  if (days > 0) return true;
  if (!user.membership_expires_at) return false;
  const until = Date.parse(user.membership_expires_at);
  return Number.isFinite(until) && until > Date.now();
}

/** Status line inside the VIP purchase modal for an already-active member. */
export function membershipModalActiveCopy(user: {
  is_admin?: boolean;
  membership_days_remaining?: number;
  membership_expires_at?: string;
} | null | undefined): string | null {
  if (!user) return null;
  const days = user.membership_days_remaining ?? 0;
  if (membershipPurchaseLocked(user)) {
    return days > 0
      ? `你已是大会员，剩余 ${days} 天。有效期内无需重复购买。`
      : '你已是大会员，有效期内无需重复购买。';
  }
  if (user.is_admin) {
    return '当前为管理员，已具备大会员权限。';
  }
  return null;
}

/** Append a page of rows, skipping ids that are already present. */
export function appendUnique<T extends { id: number }>(existing: T[], next: T[]): T[] {
  const seen = new Set(existing.map((row) => row.id));
  const out = existing.slice();
  for (const row of next) {
    if (!seen.has(row.id)) {
      seen.add(row.id);
      out.push(row);
    }
  }
  return out;
}

export function removeById<T extends { id: number }>(rows: T[], id: number): T[] {
  return rows.filter((row) => row.id !== id);
}

/** Page loader offset: how many rows are already loaded. */
export function nextOffset(loaded: number): number {
  return Math.max(0, loaded);
}

/** Mark the given notification ids read; returns a new array. */
export function markRead<T extends { id: number; is_read: boolean }>(rows: T[], ids: number[] | 'all'): T[] {
  const set = ids === 'all' ? null : new Set(ids);
  return rows.map((row) => (set === null || set.has(row.id) ? { ...row, is_read: true } : row));
}

export function countUnread(rows: { is_read: boolean }[]): number {
  return rows.reduce((n, row) => n + (row.is_read ? 0 : 1), 0);
}
