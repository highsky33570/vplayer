const API_BASE = import.meta.env?.VITE_API_BASE ?? '';

export type AuthUser = {
  id: number;
  email: string;
  nickname: string;
  is_admin?: boolean;
  /** Site membership: can view membership_required catalog titles. */
  is_member?: boolean;
  /** ISO expiry for paid membership; omitted for admin lifetime grants. */
  membership_expires_at?: string;
  /** Whole days remaining for paid membership. */
  membership_days_remaining?: number;
  /** 0..359 custom hue; -1 (or missing) means derive from user id. */
  avatar_hue?: number;
};

export type MembershipPlan = {
  slug: string;
  name: string;
  price_usd: number;
  duration_days: number;
  sort: number;
};

export type MembershipOrder = {
  order_no: string;
  plan_slug: string;
  plan_name: string;
  amount_usd: number;
  duration_days: number;
  status: string;
  payment_url?: string;
  receive_address?: string;
  actual_amount?: number;
  token?: string;
  network?: string;
  expire_at?: string;
  paid_at?: string;
  membership_granted_until?: string;
  gateway_provider?: string;
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
  membership_required: boolean;
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
  play_sid?: number;
  play_nid?: number;
  /** When true, playback requires site membership (is_member / admin). */
  membership_required?: boolean;
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

/** Catalog list endpoints may return JSON null for empty data; treat as []. */
async function getJSONArray<T>(path: string, token?: string | null): Promise<T[]> {
  const data = await getJSON<T[] | null | undefined>(path, token);
  return Array.isArray(data) ? data : [];
}

export function listCategories() {
  return getJSONArray<Category>('/api/v1/categories');
}

export async function listVideosPage(opts?: {
  categoryId?: number;
  q?: string;
  limit?: number;
  offset?: number;
  /** Optional; homepage Featured/最新上架 only. Never set for category/search/browse. */
  excludeCategorySlug?: string;
  /** "hot" lists most-played titles. Omitted keeps newest-added order. */
  sort?: 'hot';
  /** When set, members receive membership_required titles. */
  token?: string | null;
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
  if (opts?.sort === 'hot') params.set('sort', 'hot');
  const headers: Record<string, string> = {};
  if (opts?.token) headers.Authorization = `Bearer ${opts.token}`;
  const res = await fetch(`${API_BASE}/api/v1/videos?${params}`, { headers });
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

export function getVideo(id: number, token?: string | null) {
  return getJSON<Video>(`/api/v1/videos/${id}`, token);
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

export type PublicEpisode = {
  sid: number;
  nid: number;
  title: string;
};

export type RelatedVideo = {
  id: number;
  title: string;
  cover_url: string;
  view_count: number;
  year?: string;
  category_id: number;
  duration_sec: number;
};

export type VideoComment = {
  id: number;
  video_id: number;
  body: string;
  created_at: string;
  author: { display_name: string };
  mine?: boolean;
  origin?: 'vplayer';
};

export type SourceComment = {
  origin: 'source';
  mine: false;
  body: string;
  source_created_raw: string;
  digg_up: number;
  digg_down: number;
  author: { display_name: string };
  replies?: SourceComment[];
};

export type SourceCommentPage = {
  data: SourceComment[];
  total: number;
  limit: number;
  offset: number;
  has_more: boolean;
  error: string;
};

export type CommentPage = {
  data: VideoComment[];
  total: number;
  limit: number;
  offset: number;
  has_more: boolean;
  source: SourceCommentPage;
};

export function listRelated(id: number) {
  return getJSONArray<RelatedVideo>(`/api/v1/videos/${id}/related?limit=10`);
}

export function listEpisodes(id: number) {
  return getJSONArray<PublicEpisode>(`/api/v1/videos/${id}/episodes`);
}

export async function listComments(
  id: number,
  offset = 0,
  token?: string | null,
  sourceOffset = 0,
): Promise<CommentPage> {
  const headers: Record<string, string> = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(
    `${API_BASE}/api/v1/videos/${id}/comments?limit=20&offset=${offset}&source_limit=20&source_offset=${sourceOffset}`,
    { headers },
  );
  const json = (await res.json().catch(() => ({}))) as {
    ok?: boolean;
    message?: string;
    data?: VideoComment[];
    total?: number;
    limit?: number;
    offset?: number;
    has_more?: boolean;
    source?: Partial<SourceCommentPage>;
  };
  if (!res.ok || json.ok === false) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
  }
  return {
    data: json.data ?? [],
    total: Number(json.total ?? 0),
    limit: Number(json.limit ?? 20),
    offset: Number(json.offset ?? offset),
    has_more: Boolean(json.has_more),
    source: {
      data: json.source?.data ?? [],
      total: Number(json.source?.total ?? 0),
      limit: Number(json.source?.limit ?? 20),
      offset: Number(json.source?.offset ?? sourceOffset),
      has_more: Boolean(json.source?.has_more),
      error: json.source?.error ?? '',
    },
  };
}

export async function postComment(
  id: number,
  body: string,
  token: string,
  parentId?: number,
): Promise<VideoComment> {
  const res = await fetch(`${API_BASE}/api/v1/videos/${id}/comments`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(parentId != null && parentId > 0 ? { body, parent_id: parentId } : { body }),
  });
  const json = await parseJSONResponse(res);
  if (!res.ok || json.ok === false) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
  }
  return json.data as VideoComment;
}

export async function deleteComment(id: number, token: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/v1/comments/${id}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  });
  const json = await parseJSONResponse(res);
  if (!res.ok || json.ok === false) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
  }
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
  /** Absent means auth stays required (fail closed). */
  auth_required?: boolean;
  /** Absent means playback stays login-gated (fail closed). Independent of auth_required. */
  playback_auth_required?: boolean;
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

/** PATCH /api/v1/auth/me — update nickname and/or avatar hue. */
export async function updateMe(
  token: string | null | undefined,
  patch: { nickname?: string; avatar_hue?: number },
): Promise<AuthUser> {
  return sendJSON<AuthUser>('/api/v1/auth/me', 'PATCH', token, patch);
}

// ——— Personal library: favorites, history, notifications ———

export type MeSummary = {
  favorites: number;
  history: number;
  unread_notifications: number;
};

export type FavoriteFolder = {
  id: number;
  name: string;
  is_default: boolean;
  count: number;
  created_at: string;
};

export type FavoriteVideo = Video & {
  favorited_at: string;
  folder_id?: number;
};

export type HistoryVideo = Video & {
  last_watched_at: string;
  progress_sec: number;
};

export type NotificationKind = 'comment_reply' | 'system';

export type UserNotification = {
  id: number;
  kind: NotificationKind | string;
  title: string;
  body: string;
  video_id?: number;
  comment_id?: number;
  actor_name?: string;
  is_read: boolean;
  created_at: string;
};

export type Page<T> = {
  data: T[];
  total: number;
  limit: number;
  offset: number;
  has_more: boolean;
};

/** JSON request with optional bearer token; returns the `data` field. */
async function sendJSON<T>(
  path: string,
  method: 'GET' | 'POST' | 'PATCH' | 'DELETE',
  token: string | null | undefined,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = { ...bearerAuthHeader(token) };
  const init: RequestInit = { method, headers };
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(body);
  }
  const res = await fetch(`${API_BASE}${path}`, init);
  const json = await parseJSONResponse(res);
  if (!res.ok || json.ok === false) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status, json.error);
  }
  return json.data as T;
}

/** Authenticated paginated GET: parses the full envelope like listVideosPage. */
async function getPage<T>(
  path: string,
  token: string | null | undefined,
  params: URLSearchParams,
  limit: number,
  offset: number,
): Promise<Page<T>> {
  const res = await fetch(`${API_BASE}${path}?${params}`, { headers: bearerAuthHeader(token) });
  const json = (await res.json().catch(() => ({}))) as {
    ok?: boolean;
    message?: string;
    error?: string;
    data?: T[];
    total?: number;
    limit?: number;
    offset?: number;
    has_more?: boolean;
  };
  if (!res.ok || json.ok === false) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status, json.error);
  }
  return {
    data: json.data ?? [],
    total: Number(json.total ?? 0),
    limit: Number(json.limit ?? limit),
    offset: Number(json.offset ?? offset),
    has_more: Boolean(json.has_more),
  };
}

export function fetchMeSummary(token: string | null | undefined) {
  return sendJSON<MeSummary>('/api/v1/me/summary', 'GET', token);
}

export function listFavoriteFolders(token: string | null | undefined) {
  return sendJSON<FavoriteFolder[]>('/api/v1/me/favorite-folders', 'GET', token);
}

export function listFavoritesPage(
  token: string | null | undefined,
  opts?: { folderId?: number; limit?: number; offset?: number },
): Promise<Page<FavoriteVideo>> {
  const limit = opts?.limit ?? 24;
  const offset = opts?.offset ?? 0;
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  if (opts?.folderId && opts.folderId > 0) params.set('folder_id', String(opts.folderId));
  return getPage<FavoriteVideo>('/api/v1/me/favorites', token, params, limit, offset);
}

export async function getFavoriteStatus(
  token: string | null | undefined,
  videoId: number,
): Promise<boolean> {
  const data = await sendJSON<{ favorited: boolean }>(`/api/v1/me/favorites/${videoId}`, 'GET', token);
  return Boolean(data?.favorited);
}

export async function addFavorite(token: string | null | undefined, videoId: number): Promise<boolean> {
  const data = await sendJSON<{ favorited: boolean }>(`/api/v1/me/favorites/${videoId}`, 'POST', token);
  return Boolean(data?.favorited);
}

export async function removeFavorite(token: string | null | undefined, videoId: number): Promise<boolean> {
  const data = await sendJSON<{ favorited: boolean }>(`/api/v1/me/favorites/${videoId}`, 'DELETE', token);
  return Boolean(data?.favorited);
}

export function listHistoryPage(
  token: string | null | undefined,
  opts?: { limit?: number; offset?: number },
): Promise<Page<HistoryVideo>> {
  const limit = opts?.limit ?? 24;
  const offset = opts?.offset ?? 0;
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  return getPage<HistoryVideo>('/api/v1/me/history', token, params, limit, offset);
}

export async function upsertHistory(
  token: string | null | undefined,
  videoId: number,
  progressSec?: number,
): Promise<void> {
  const body = progressSec != null && progressSec > 0 ? { progress_sec: Math.floor(progressSec) } : undefined;
  await sendJSON<unknown>(`/api/v1/me/history/${videoId}`, 'POST', token, body);
}

export async function deleteHistoryItem(token: string | null | undefined, videoId: number): Promise<void> {
  await sendJSON<unknown>(`/api/v1/me/history/${videoId}`, 'DELETE', token);
}

export async function clearHistory(token: string | null | undefined): Promise<void> {
  await sendJSON<unknown>('/api/v1/me/history', 'DELETE', token);
}

export function listNotificationsPage(
  token: string | null | undefined,
  opts?: { kind?: NotificationKind; limit?: number; offset?: number },
): Promise<Page<UserNotification>> {
  const limit = opts?.limit ?? 20;
  const offset = opts?.offset ?? 0;
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  if (opts?.kind) params.set('kind', opts.kind);
  return getPage<UserNotification>('/api/v1/me/notifications', token, params, limit, offset);
}

export async function fetchUnreadCount(token: string | null | undefined): Promise<number> {
  const data = await sendJSON<{ unread_count: number }>('/api/v1/me/notifications/unread-count', 'GET', token);
  return Number(data?.unread_count ?? 0);
}

export async function markNotificationsRead(
  token: string | null | undefined,
  req: { ids?: number[]; all?: boolean },
): Promise<number> {
  const body = req.all ? { all: true } : { ids: req.ids ?? [] };
  const data = await sendJSON<{ unread_count: number }>('/api/v1/me/notifications/read', 'POST', token, body);
  return Number(data?.unread_count ?? 0);
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
  is_member: boolean;
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

export type AdminCreateVideoInput = {
  title: string;
  playback_url: string;
  cover_url?: string;
  description?: string;
  as_vip?: boolean;
  category_id?: number;
};

export async function createAdminVideo(
  token: string | null | undefined,
  input: AdminCreateVideoInput,
): Promise<AdminVideoDetail> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(`${API_BASE}/api/v1/admin/videos`, {
    method: 'POST',
    headers,
    body: JSON.stringify(input),
  });
  const json = (await res.json().catch(() => ({}))) as {
    ok?: boolean;
    message?: string;
    data?: AdminVideoDetail;
  };
  if (!res.ok || json.ok === false || !json.data) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
  }
  return json.data;
}

export async function setAdminVideoMembership(
  token: string | null | undefined,
  id: number,
  membershipRequired: boolean,
): Promise<AdminVideoDetail> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(`${API_BASE}/api/v1/admin/videos/${id}/membership`, {
    method: 'PATCH',
    headers,
    body: JSON.stringify({ membership_required: membershipRequired }),
  });
  const json = (await res.json().catch(() => ({}))) as {
    ok?: boolean;
    message?: string;
    data?: AdminVideoDetail;
  };
  if (!res.ok || json.ok === false || !json.data) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
  }
  return json.data;
}

export async function setAdminUserMembership(
  token: string | null | undefined,
  id: number,
  isMember: boolean,
): Promise<AdminUser> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(`${API_BASE}/api/v1/admin/users/${id}/membership`, {
    method: 'PATCH',
    headers,
    body: JSON.stringify({ is_member: isMember }),
  });
  const json = (await res.json().catch(() => ({}))) as {
    ok?: boolean;
    message?: string;
    data?: AdminUser;
  };
  if (!res.ok || json.ok === false || !json.data) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
  }
  return json.data;
}

export async function listMembershipPlans(): Promise<MembershipPlan[]> {
  const res = await fetch(`${API_BASE}/api/v1/membership/plans`);
  const json = (await res.json().catch(() => ({}))) as {
    ok?: boolean;
    message?: string;
    data?: MembershipPlan[];
  };
  if (!res.ok || json.ok === false) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
  }
  return Array.isArray(json.data) ? json.data : [];
}

export async function createMembershipOrder(token: string, planSlug: string): Promise<MembershipOrder> {
  const res = await fetch(`${API_BASE}/api/v1/membership/orders`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...bearerAuthHeader(token) },
    body: JSON.stringify({ plan_slug: planSlug }),
  });
  const json = (await res.json().catch(() => ({}))) as {
    ok?: boolean;
    message?: string;
    data?: MembershipOrder;
    error?: string;
  };
  if (!res.ok || json.ok === false || !json.data) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status, json.error);
  }
  return json.data;
}

export async function getMembershipOrder(token: string, orderNo: string): Promise<MembershipOrder> {
  const res = await fetch(`${API_BASE}/api/v1/membership/orders/${encodeURIComponent(orderNo)}`, {
    headers: { ...bearerAuthHeader(token) },
  });
  const json = (await res.json().catch(() => ({}))) as {
    ok?: boolean;
    message?: string;
    data?: MembershipOrder;
  };
  if (!res.ok || json.ok === false || !json.data) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
  }
  return json.data;
}
