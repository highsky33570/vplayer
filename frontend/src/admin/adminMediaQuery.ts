/** Build admin media episode list query for URL search params / API. */
export type AdminMediaMigrationFilter = 'all' | 'done' | 'failed' | 'pending';
export type AdminMediaProviderFilter = 'all' | 'r2';

export function parseAdminMediaSearch(params: URLSearchParams): {
  q: string;
  migrationStatus: AdminMediaMigrationFilter;
  storageProvider: AdminMediaProviderFilter;
  page: number;
  pageSize: number;
} {
  const pageSize = 24;
  const q = (params.get('q') ?? '').trim();
  const migRaw = params.get('migration_status');
  let migrationStatus: AdminMediaMigrationFilter = 'all';
  if (migRaw === 'done' || migRaw === 'failed' || migRaw === 'pending') {
    migrationStatus = migRaw;
  }
  const provRaw = params.get('storage_provider');
  let storageProvider: AdminMediaProviderFilter = 'all';
  if (provRaw === 'r2') storageProvider = 'r2';
  const pageRaw = Number(params.get('page') ?? '1');
  const page = Number.isFinite(pageRaw) && pageRaw > 0 ? Math.floor(pageRaw) : 1;
  return { q, migrationStatus, storageProvider, page, pageSize };
}

export function buildAdminMediaSearch(input: {
  q: string;
  migrationStatus: AdminMediaMigrationFilter;
  storageProvider: AdminMediaProviderFilter;
  page: number;
}): string {
  const params = new URLSearchParams();
  if (input.q.trim()) params.set('q', input.q.trim());
  if (input.migrationStatus !== 'all') params.set('migration_status', input.migrationStatus);
  if (input.storageProvider !== 'all') params.set('storage_provider', input.storageProvider);
  if (input.page > 1) params.set('page', String(input.page));
  const s = params.toString();
  return s ? `?${s}` : '';
}
