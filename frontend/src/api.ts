const API_BASE = import.meta.env?.VITE_API_BASE ?? '';

export type AuthUser = {
  id: number;
  email: string;
  nickname: string;
};

export type AuthResult = {
  token: string;
  user: AuthUser;
};

export type Category = {
  id: number;
  name: string;
  slug: string;
  sort: number;
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

  constructor(message: string, status: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function parseJSONResponse(res: Response): Promise<{ ok?: boolean; message?: string; data?: unknown }> {
  return (await res.json().catch(() => ({}))) as {
    ok?: boolean;
    message?: string;
    data?: unknown;
  };
}

async function getJSON<T>(path: string, token?: string | null): Promise<T> {
  const headers: Record<string, string> = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(`${API_BASE}${path}`, { headers });
  const json = await parseJSONResponse(res);
  if (!res.ok || json.ok === false) {
    throw new ApiError(json.message ?? `请求失败（${res.status}）`, res.status);
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

export function register(email: string, password: string, nickname?: string) {
  return postJSON<AuthResult>('/api/v1/auth/register', {
    email,
    password,
    nickname: nickname ?? '',
  });
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
