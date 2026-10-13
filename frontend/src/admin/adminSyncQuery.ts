/** Build admin sync run history query for URL search params / API. */
export function parseAdminSyncSearch(params: URLSearchParams): {
  triggerType: string;
  page: number;
  pageSize: number;
} {
  const pageSize = 24;
  const triggerType = (params.get('trigger_type') ?? '').trim();
  const pageRaw = Number(params.get('page') ?? '1');
  const page = Number.isFinite(pageRaw) && pageRaw > 0 ? Math.floor(pageRaw) : 1;
  return { triggerType, page, pageSize };
}

export function buildAdminSyncSearch(input: {
  triggerType: string;
  page: number;
}): string {
  const params = new URLSearchParams();
  if (input.triggerType.trim()) params.set('trigger_type', input.triggerType.trim());
  if (input.page > 1) params.set('page', String(input.page));
  const s = params.toString();
  return s ? `?${s}` : '';
}
