/** Build admin sync + operational runs query for URL search params / API. */

export type AdminOpsJobFilter = '' | 'incremental_sync' | 'enrichment';
export type AdminOpsTriggerFilter = '' | 'cli' | 'timer';
export type AdminOpsStatusFilter = '' | 'running' | 'succeeded' | 'failed' | 'rejected';

export function parseAdminSyncSearch(params: URLSearchParams): {
  triggerType: string;
  page: number;
  pageSize: number;
  opJob: AdminOpsJobFilter;
  opTrigger: AdminOpsTriggerFilter;
  opStatus: AdminOpsStatusFilter;
  opPage: number;
  opPageSize: number;
} {
  const pageSize = 24;
  const opPageSize = 24;
  const triggerType = (params.get('trigger_type') ?? '').trim();
  const pageRaw = Number(params.get('page') ?? '1');
  const page = Number.isFinite(pageRaw) && pageRaw > 0 ? Math.floor(pageRaw) : 1;

  const opJobRaw = (params.get('op_job') ?? '').trim();
  const opJob: AdminOpsJobFilter =
    opJobRaw === 'incremental_sync' || opJobRaw === 'enrichment' ? opJobRaw : '';

  const opTriggerRaw = (params.get('op_trigger') ?? '').trim();
  const opTrigger: AdminOpsTriggerFilter =
    opTriggerRaw === 'cli' || opTriggerRaw === 'timer' ? opTriggerRaw : '';

  const opStatusRaw = (params.get('op_status') ?? '').trim();
  const opStatus: AdminOpsStatusFilter =
    opStatusRaw === 'running' ||
    opStatusRaw === 'succeeded' ||
    opStatusRaw === 'failed' ||
    opStatusRaw === 'rejected'
      ? opStatusRaw
      : '';

  const opPageRaw = Number(params.get('op_page') ?? '1');
  const opPage = Number.isFinite(opPageRaw) && opPageRaw > 0 ? Math.floor(opPageRaw) : 1;

  return { triggerType, page, pageSize, opJob, opTrigger, opStatus, opPage, opPageSize };
}

export function buildAdminSyncSearch(input: {
  triggerType: string;
  page: number;
  opJob?: AdminOpsJobFilter;
  opTrigger?: AdminOpsTriggerFilter;
  opStatus?: AdminOpsStatusFilter;
  opPage?: number;
}): string {
  const params = new URLSearchParams();
  if (input.triggerType.trim()) params.set('trigger_type', input.triggerType.trim());
  if (input.page > 1) params.set('page', String(input.page));

  const opJob = input.opJob ?? '';
  if (opJob) params.set('op_job', opJob);
  const opTrigger = input.opTrigger ?? '';
  if (opTrigger) params.set('op_trigger', opTrigger);
  const opStatus = input.opStatus ?? '';
  if (opStatus) params.set('op_status', opStatus);
  const opPage = input.opPage ?? 1;
  if (opPage > 1) params.set('op_page', String(opPage));

  const s = params.toString();
  return s ? `?${s}` : '';
}
