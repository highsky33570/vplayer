const API_BASE = import.meta.env?.VITE_API_BASE ?? '';

export type AuthUser = {
  id: number;
  email: string;
  nickname: string;
  is_admin?: boolean;
};

export type AdminSyncRunSummary = {
  id: number;
  source_system: string;
  trigger_type: string;
  started_at: string;
  finished_at?: string;
  scanned: number;
  created_count: number;
  updated_count: number;
  unchanged_count: number;
  failed_count: number;
  deactivated_count: number;
  has_error: boolean;
};

export type AdminDashboardData = {
  status: string;
  users_total: number;
  videos_total: number;
  videos_active: number;
  categories_total: number;
  latest_sync: AdminSyncRunSummary | null;
};

export type AdminVideoListItem = {
  id: number;
  title: string;
  category_id: number;
  status: string;
  is_active: boolean;
  source_system: string;
  source_id: string;
  year: string;
  sync_status: string;
  created_at: string;
  last_synced_at?: string;
  view_count: number;
  duration_sec: number;
  cover_url: string;
};

export type AdminVideoDetail = AdminVideoListItem & {
  description: string;
  director: string;
  actors: string;
  rating: number;
  source_category_id: string;
  source_updated_at?: string;
  playback_source: string;
  play_sid: number;
  play_nid: number;
};

export type AdminVideoPage = {
  data: AdminVideoListItem[];
  total: number;
  limit: number;
  offset: number;
  has_more: boolean;
};

export type AuthResult = {
  token: string;
  user: AuthUser;
};

export type RegisterStartResult = {
  verification_required: boolean;
  expires_in_sec: number;
  resend_after_sec: number;
};

export type Category = {
  id: number;
  name: string;
  slug: string;
  sort: number;
  /** null/undefined => top-level; set for OLEHDTV child types */
  parent_id?: number | null;
  source_id?: string | number | null;
};

export type Video = {
  id: number;
  category_id: number;
  title: string;
  description: string;
  cover_url: string;
  duration_sec: number;
  view_count: number;
  status: string;
  year?: string;
  area?: string;
  director?: string;
  actors?: string;
  rating?: number;
};

export type PlaybackInfo = {
  type: string;
  url: string;
  source: string;
  sid: number;
  nid: number;
  ticket?: string;
  m3u8_url?: string;
  /** Optional gated source URL when primary is migrated object-key delivery. */
  fallback_url?: string;
};

export type VideoPage = {
  data: Video[];
  total: number;
  limit: number;
  offset: number;
  has_more: boolean;
};

export class ApiError extends Error {
  status: number;
  code?: string;

  constructor(message: string, status: number, code?: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }
}

async function parseJSONResponse(res: Response): Promise<{
  ok?: boolean;
  message?: string;
  error?: string;
  data?: unknown;
}> {
  return (await res.json().catch(() => ({}))) as {
    ok?: boolean;
    message?: string;
    error?: string;
    data?: unknown;
  };
}

async function getJSON<T>(path: string, token?: string | null): Promise<T> {
  const headers: Record<string, string> = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(`${API_BASE}${path}`, { headers });
  const json = await parseJSONResponse(res);
  if (!res.ok || json.ok === false) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status, json.error);
  }
  return json.data as T;
}

export function listCategories() {
  return getJSON<Category[]>('/api/v1/categories');
}

export async function listVideosPage(opts?: {
  categoryId?: number;
  q?: string;
  limit?: number;
  offset?: number;
  /** Optional; homepage Featured/最新上架 only. Never set for category/search/browse. */
  excludeCategorySlug?: string;
}): Promise<VideoPage> {
  const limit = opts?.limit ?? 48;
  const offset = opts?.offset ?? 0;
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset),
  });
  if (opts?.categoryId) params.set('category_id', String(opts.categoryId));
  const q = opts?.q?.trim();
  if (q) params.set('q', q);
  const excludeSlug = opts?.excludeCategorySlug?.trim();
  if (excludeSlug) params.set('exclude_category_slug', excludeSlug);
  const res = await fetch(`${API_BASE}/api/v1/videos?${params}`);
  const json = (await res.json().catch(() => ({}))) as {
    ok?: boolean;
    message?: string;
    data?: Video[];
    total?: number;
    limit?: number;
    offset?: number;
    has_more?: boolean;
  };
  if (!res.ok || json.ok === false) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
  }
  return {
    data: (json.data ?? []) as Video[],
    total: Number(json.total ?? 0),
    limit: Number(json.limit ?? limit),
    offset: Number(json.offset ?? offset),
    has_more: Boolean(json.has_more),
  };
}

export async function listVideos(categoryId?: number) {
  const page = await listVideosPage({ categoryId, limit: 48, offset: 0 });
  return page.data;
}

export function getVideo(id: number) {
  return getJSON<Video>(`/api/v1/videos/${id}`);
}

/** Build Authorization header value for authenticated API calls. */
export function bearerAuthHeader(token: string | null | undefined): Record<string, string> {
  if (!token) return {};
  return { Authorization: `Bearer ${token}` };
}

export async function getPlayback(
  id: number,
  sid = 0,
  nid = 0,
  token?: string | null,
): Promise<PlaybackInfo> {
  const qs = new URLSearchParams();
  if (sid > 0) qs.set('sid', String(sid));
  if (nid > 0) qs.set('nid', String(nid));
  const suffix = qs.toString() ? `?${qs}` : '';
  const data = await getJSON<PlaybackInfo>(`/api/v1/videos/${id}/play${suffix}`, token);
  if (!data.url && data.m3u8_url) {
    data.url = data.m3u8_url;
    data.type = data.type || 'hls';
  }
  return data;
}

export async function createPlaySession(id: number, token?: string | null) {
  const p = await getPlayback(id, 0, 0, token);
  return { ticket: p.ticket ?? '', m3u8_url: p.url, expires_in: 300 };
}

export function formatDuration(sec: number) {
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
}

export function formatViews(n: number) {
  if (n >= 10000) return `${(n / 10000).toFixed(1)}万`;
  return String(n);
}

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  const json = await parseJSONResponse(res);
  if (!res.ok || json.ok === false) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
  }
  return json.data as T;
}

export function login(email: string, password: string) {
  return postJSON<AuthResult>('/api/v1/auth/login', { email, password });
}

export type RegistrationOptions = {
  email_verification_enabled: boolean;
};

export function fetchRegistrationOptions() {
  return getJSON<RegistrationOptions>('/api/v1/auth/registration');
}

export function register(email: string, password: string, nickname?: string) {
  return postJSON<AuthResult>('/api/v1/auth/register', {
    email,
    password,
    nickname: nickname ?? '',
  });
}

export function registerStart(email: string, password: string, nickname?: string) {
  return postJSON<RegisterStartResult>('/api/v1/auth/register/start', {
    email,
    password,
    nickname: nickname ?? '',
  });
}

export function registerVerify(email: string, code: string) {
  return postJSON<AuthResult>('/api/v1/auth/register/verify', { email, code });
}

export function registerResend(email: string) {
  return postJSON<RegisterStartResult>('/api/v1/auth/register/resend', { email });
}

export async function fetchMe(token: string): Promise<AuthUser> {
  const res = await fetch(`${API_BASE}/api/v1/auth/me`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  const json = await parseJSONResponse(res);
  if (!res.ok || json.ok === false) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
  }
  return json.data as AuthUser;
}

export function fetchAdminDashboard(token: string | null | undefined) {
  return getJSON<AdminDashboardData>('/api/v1/admin/dashboard', token);
}

export type AdminSyncOverview = {
  latest_sync: AdminSyncRunSummary | null;
  latest_incremental: AdminSyncRunSummary | null;
  latest_full: AdminSyncRunSummary | null;
  pending_enrichment_count: number;
  videos_sync_error_count: number;
  categories_sync_error_count: number;
  videos_active: number;
  videos_inactive: number;
};

export type AdminSyncRunPage = {
  data: AdminSyncRunSummary[];
  total: number;
  limit: number;
  offset: number;
  has_more: boolean;
};

export function fetchAdminSyncOverview(token: string | null | undefined) {
  return getJSON<AdminSyncOverview>('/api/v1/admin/sync', token);
}

export async function fetchAdminSyncRuns(
  token: string | null | undefined,
  opts?: {
    limit?: number;
    offset?: number;
    triggerType?: string;
  },
): Promise<AdminSyncRunPage> {
  const limit = opts?.limit ?? 24;
  const offset = opts?.offset ?? 0;
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset),
  });
  const trigger = opts?.triggerType?.trim();
  if (trigger) params.set('trigger_type', trigger);
  const headers: Record<string, string> = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(`${API_BASE}/api/v1/admin/sync/runs?${params}`, { headers });
  const json = (await res.json().catch(() => ({}))) as {
    ok?: boolean;
    message?: string;
    data?: AdminSyncRunSummary[];
    total?: number;
    limit?: number;
    offset?: number;
    has_more?: boolean;
  };
  if (!res.ok || json.ok === false) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
  }
  return {
    data: json.data ?? [],
    total: Number(json.total ?? 0),
    limit: Number(json.limit ?? limit),
    offset: Number(json.offset ?? offset),
    has_more: Boolean(json.has_more),
  };
}

export function fetchAdminSyncRun(id: number, token: string | null | undefined) {
  return getJSON<AdminSyncRunSummary>(`/api/v1/admin/sync/runs/${id}`, token);
}

export type AdminOperationalRun = {
  id: number;
  job_type: string;
  trigger_type: string;
  status: string;
  requested_by_user_id?: number | null;
  requested_at: string;
  started_at?: string | null;
  finished_at?: string | null;
  duration_seconds?: number | null;
  stale: boolean;
  scanned?: number | null;
  created_count?: number | null;
  updated_count?: number | null;
  unchanged_count?: number | null;
  failed_count?: number | null;
  selected_count?: number | null;
  succeeded_count?: number | null;
  skipped_count?: number | null;
  unavailable_count?: number | null;
  has_error: boolean;
  error_code: string;
  sync_run_id?: number | null;
};

export type AdminOperationalRunPage = {
  data: AdminOperationalRun[];
  total: number;
  limit: number;
  offset: number;
  has_more: boolean;
};

export async function fetchAdminOperationalRuns(
  token: string | null | undefined,
  opts?: {
    limit?: number;
    offset?: number;
    jobType?: string;
    triggerType?: string;
    status?: string;
  },
): Promise<AdminOperationalRunPage> {
  const limit = opts?.limit ?? 24;
  const offset = opts?.offset ?? 0;
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset),
  });
  const job = opts?.jobType?.trim();
  if (job) params.set('job_type', job);
  const trigger = opts?.triggerType?.trim();
  if (trigger) params.set('trigger_type', trigger);
  const status = opts?.status?.trim();
  if (status) params.set('status', status);
  const headers: Record<string, string> = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(`${API_BASE}/api/v1/admin/sync/operations?${params}`, { headers });
  const json = (await res.json().catch(() => ({}))) as {
    ok?: boolean;
    message?: string;
    data?: AdminOperationalRun[];
    total?: number;
    limit?: number;
    offset?: number;
    has_more?: boolean;
  };
  if (!res.ok || json.ok === false) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
  }
  return {
    data: json.data ?? [],
    total: Number(json.total ?? 0),
    limit: Number(json.limit ?? limit),
    offset: Number(json.offset ?? offset),
    has_more: Boolean(json.has_more),
  };
}

export function fetchAdminOperationalRun(id: number, token: string | null | undefined) {
  return getJSON<AdminOperationalRun>(`/api/v1/admin/sync/operations/${id}`, token);
}

export type AdminSyncActionResult = {
  ok: boolean;
  message?: string;
  job_type?: string;
  error?: string;
};

async function postAdminSyncAction(
  path: string,
  token: string | null | undefined,
): Promise<{ status: number; body: AdminSyncActionResult }> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(`${API_BASE}${path}`, { method: 'POST', headers, body: '{}' });
  const json = (await res.json().catch(() => ({}))) as AdminSyncActionResult;
  return { status: res.status, body: json };
}

export function postAdminRunIncremental(token: string | null | undefined) {
  return postAdminSyncAction('/api/v1/admin/sync/actions/incremental', token);
}

export function postAdminRunEnrichment(token: string | null | undefined) {
  return postAdminSyncAction('/api/v1/admin/sync/actions/enrichment', token);
}

export type AdminMediaOverview = {
  episodes_total: number;
  migrated_count: number;
  failed_count: number;
  not_migrated_count: number;
  eligible_count: number;
  storage_r2_count: number;
  latest_migrated_at?: string | null;
};

export type AdminMediaEpisode = {
  episode_id: number;
  video_id: number;
  video_title: string;
  sid: number;
  nid: number;
  episode_title: string;
  storage_provider: string;
  migration_status: string;
  migrated_at?: string;
  has_error: boolean;
  playback_status: string;
  has_hls_object?: boolean;
};

export type AdminMediaEpisodePage = {
  data: AdminMediaEpisode[];
  total: number;
  limit: number;
  offset: number;
  has_more: boolean;
};

export function fetchAdminMediaOverview(token: string | null | undefined) {
  return getJSON<AdminMediaOverview>('/api/v1/admin/media', token);
}

export async function fetchAdminMediaEpisodes(
  token: string | null | undefined,
  opts?: {
    limit?: number;
    offset?: number;
    q?: string;
    migrationStatus?: 'all' | 'done' | 'failed' | 'pending';
    storageProvider?: 'all' | 'r2';
  },
): Promise<AdminMediaEpisodePage> {
  const limit = opts?.limit ?? 24;
  const offset = opts?.offset ?? 0;
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset),
  });
  const q = opts?.q?.trim();
  if (q) params.set('q', q);
  if (opts?.migrationStatus && opts.migrationStatus !== 'all') {
    params.set('migration_status', opts.migrationStatus);
  }
  if (opts?.storageProvider && opts.storageProvider !== 'all') {
    params.set('storage_provider', opts.storageProvider);
  }
  const headers: Record<string, string> = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(`${API_BASE}/api/v1/admin/media/episodes?${params}`, { headers });
  const json = (await res.json().catch(() => ({}))) as {
    ok?: boolean;
    message?: string;
    data?: AdminMediaEpisode[];
    total?: number;
    limit?: number;
    offset?: number;
    has_more?: boolean;
  };
  if (!res.ok || json.ok === false) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
  }
  return {
    data: json.data ?? [],
    total: Number(json.total ?? 0),
    limit: Number(json.limit ?? limit),
    offset: Number(json.offset ?? offset),
    has_more: Boolean(json.has_more),
  };
}

export function fetchAdminMediaEpisode(id: number, token: string | null | undefined) {
  return getJSON<AdminMediaEpisode>(`/api/v1/admin/media/episodes/${id}`, token);
}

export type AdminCategory = {
  id: number;
  name: string;
  slug: string;
  sort: number;
  is_active: boolean;
  source_system: string;
  source_id: string;
  last_synced_at?: string;
  sync_status: string;
  has_error: boolean;
  created_at: string;
  video_count: number;
};

export function fetchAdminCategories(token: string | null | undefined) {
  return getJSON<AdminCategory[]>('/api/v1/admin/categories', token);
}

export function fetchAdminCategory(id: number, token: string | null | undefined) {
  return getJSON<AdminCategory>(`/api/v1/admin/categories/${id}`, token);
}

export type AdminUser = {
  id: number;
  email: string;
  nickname: string;
  is_admin: boolean;
  created_at: string;
};

export type AdminUserPage = {
  data: AdminUser[];
  total: number;
  limit: number;
  offset: number;
  has_more: boolean;
};

export async function fetchAdminUsers(
  token: string | null | undefined,
  opts?: {
    limit?: number;
    offset?: number;
    q?: string;
    isAdmin?: 'all' | '1' | '0';
  },
): Promise<AdminUserPage> {
  const limit = opts?.limit ?? 24;
  const offset = opts?.offset ?? 0;
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset),
  });
  const q = opts?.q?.trim();
  if (q) params.set('q', q);
  if (opts?.isAdmin === '1' || opts?.isAdmin === '0') {
    params.set('is_admin', opts.isAdmin);
  }
  const headers: Record<string, string> = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(`${API_BASE}/api/v1/admin/users?${params}`, { headers });
  const json = (await res.json().catch(() => ({}))) as {
    ok?: boolean;
    message?: string;
    data?: AdminUser[];
    total?: number;
    limit?: number;
    offset?: number;
    has_more?: boolean;
  };
  if (!res.ok || json.ok === false) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
  }
  return {
    data: json.data ?? [],
    total: Number(json.total ?? 0),
    limit: Number(json.limit ?? limit),
    offset: Number(json.offset ?? offset),
    has_more: Boolean(json.has_more),
  };
}

export function fetchAdminUser(id: number, token: string | null | undefined) {
  return getJSON<AdminUser>(`/api/v1/admin/users/${id}`, token);
}

export async function fetchAdminVideos(
  token: string | null | undefined,
  opts?: {
    limit?: number;
    offset?: number;
    q?: string;
    categoryId?: number;
    isActive?: 'all' | '1' | '0';
  },
): Promise<AdminVideoPage> {
  const limit = opts?.limit ?? 24;
  const offset = opts?.offset ?? 0;
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset),
  });
  const q = opts?.q?.trim();
  if (q) params.set('q', q);
  if (opts?.categoryId && opts.categoryId > 0) {
    params.set('category_id', String(opts.categoryId));
  }
  if (opts?.isActive === '1' || opts?.isActive === '0') {
    params.set('is_active', opts.isActive);
  }
  const headers: Record<string, string> = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(`${API_BASE}/api/v1/admin/videos?${params}`, { headers });
  const json = (await res.json().catch(() => ({}))) as {
    ok?: boolean;
    message?: string;
    data?: AdminVideoListItem[];
    total?: number;
    limit?: number;
    offset?: number;
    has_more?: boolean;
  };
  if (!res.ok || json.ok === false) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
  }
  return {
    data: json.data ?? [],
    total: Number(json.total ?? 0),
    limit: Number(json.limit ?? limit),
    offset: Number(json.offset ?? offset),
    has_more: Boolean(json.has_more),
  };
}

export function fetchAdminVideo(id: number, token: string | null | undefined) {
  return getJSON<AdminVideoDetail>(`/api/v1/admin/videos/${id}`, token);
}
