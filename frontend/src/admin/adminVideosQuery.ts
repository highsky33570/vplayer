/** Build admin videos list query for URL search params / API. */
export type AdminVideosActiveFilter = 'all' | '1' | '0';
export type AdminVideosHotFilter = 'all' | '1' | '0';

export function parseAdminVideosSearch(params: URLSearchParams): {
  q: string;
  categoryId: number;
  isActive: AdminVideosActiveFilter;
  isHot: AdminVideosHotFilter;
  page: number;
  pageSize: number;
} {
  const pageSize = 24;
  const q = (params.get('q') ?? '').trim();
  const categoryRaw = params.get('category_id') ?? '';
  let categoryId = 0;
  if (categoryRaw) {
    const n = Number(categoryRaw);
    if (Number.isFinite(n) && n > 0) categoryId = Math.floor(n);
  }
  const activeRaw = params.get('is_active');
  let isActive: AdminVideosActiveFilter = 'all';
  if (activeRaw === '1' || activeRaw === '0') isActive = activeRaw;
  const hotRaw = params.get('is_hot');
  let isHot: AdminVideosHotFilter = 'all';
  if (hotRaw === '1' || hotRaw === '0') isHot = hotRaw;
  const pageRaw = Number(params.get('page') ?? '1');
  const page = Number.isFinite(pageRaw) && pageRaw > 0 ? Math.floor(pageRaw) : 1;
  return { q, categoryId, isActive, isHot, page, pageSize };
}

export function buildAdminVideosSearch(input: {
  q: string;
  categoryId: number;
  isActive: AdminVideosActiveFilter;
  isHot: AdminVideosHotFilter;
  page: number;
}): string {
  const params = new URLSearchParams();
  if (input.q.trim()) params.set('q', input.q.trim());
  if (input.categoryId > 0) params.set('category_id', String(input.categoryId));
  if (input.isActive === '1' || input.isActive === '0') params.set('is_active', input.isActive);
  if (input.isHot === '1' || input.isHot === '0') params.set('is_hot', input.isHot);
  if (input.page > 1) params.set('page', String(input.page));
  const s = params.toString();
  return s ? `?${s}` : '';
}
