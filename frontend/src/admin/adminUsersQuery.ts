/** Build admin users list query for URL search params / API. */
export type AdminUsersRoleFilter = 'all' | '1' | '0';

export function parseAdminUsersSearch(params: URLSearchParams): {
  q: string;
  isAdmin: AdminUsersRoleFilter;
  page: number;
  pageSize: number;
} {
  const pageSize = 24;
  const q = (params.get('q') ?? '').trim();
  const adminRaw = params.get('is_admin');
  let isAdmin: AdminUsersRoleFilter = 'all';
  if (adminRaw === '1' || adminRaw === '0') isAdmin = adminRaw;
  const pageRaw = Number(params.get('page') ?? '1');
  const page = Number.isFinite(pageRaw) && pageRaw > 0 ? Math.floor(pageRaw) : 1;
  return { q, isAdmin, page, pageSize };
}

export function buildAdminUsersSearch(input: {
  q: string;
  isAdmin: AdminUsersRoleFilter;
  page: number;
}): string {
  const params = new URLSearchParams();
  if (input.q.trim()) params.set('q', input.q.trim());
  if (input.isAdmin === '1' || input.isAdmin === '0') params.set('is_admin', input.isAdmin);
  if (input.page > 1) params.set('page', String(input.page));
  const s = params.toString();
  return s ? `?${s}` : '';
}
