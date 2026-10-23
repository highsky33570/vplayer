import type { FavoriteVideo, HistoryVideo, Video } from '../api';

/** Browser-local guest data. Never a server user id. */
export const GUEST_FAVORITES_KEY = 'vplayer_guest_favorites';
export const GUEST_HISTORY_KEY = 'vplayer_guest_history';

const MAX_ITEMS = 200;

export type GuestVideoInput = Pick<Video, 'id' | 'title'> &
  Partial<Pick<Video, 'cover_url' | 'category_id' | 'duration_sec' | 'description' | 'year' | 'area' | 'view_count'>>;

type StoredFavorite = GuestVideoInput & { favorited_at: string };
type StoredHistory = GuestVideoInput & { last_watched_at: string; progress_sec: number };

function readJSON<T>(storage: Storage, key: string): T[] {
  try {
    const raw = storage.getItem(key);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as unknown;
    return Array.isArray(parsed) ? (parsed as T[]) : [];
  } catch {
    return [];
  }
}

function writeJSON(storage: Storage, key: string, rows: unknown[]) {
  storage.setItem(key, JSON.stringify(rows.slice(0, MAX_ITEMS)));
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new Event('vplayer-guest-library'));
  }
}

function asFavorite(row: StoredFavorite): FavoriteVideo {
  return {
    id: row.id,
    title: row.title || '未命名',
    description: row.description ?? '',
    cover_url: row.cover_url ?? '',
    category_id: row.category_id ?? 0,
    duration_sec: row.duration_sec ?? 0,
    view_count: row.view_count ?? 0,
    status: 'ready',
    year: row.year,
    area: row.area,
    favorited_at: row.favorited_at,
  };
}

function asHistory(row: StoredHistory): HistoryVideo {
  return {
    id: row.id,
    title: row.title || '未命名',
    description: row.description ?? '',
    cover_url: row.cover_url ?? '',
    category_id: row.category_id ?? 0,
    duration_sec: row.duration_sec ?? 0,
    view_count: row.view_count ?? 0,
    status: 'ready',
    year: row.year,
    area: row.area,
    last_watched_at: row.last_watched_at,
    progress_sec: row.progress_sec ?? 0,
  };
}

function snapshot(video: GuestVideoInput): GuestVideoInput {
  return {
    id: video.id,
    title: video.title,
    cover_url: video.cover_url ?? '',
    category_id: video.category_id ?? 0,
    duration_sec: video.duration_sec ?? 0,
    description: video.description ?? '',
    year: video.year,
    area: video.area,
    view_count: video.view_count ?? 0,
  };
}

export function listGuestFavorites(storage: Storage = localStorage): FavoriteVideo[] {
  return readJSON<StoredFavorite>(storage, GUEST_FAVORITES_KEY)
    .filter((row) => row && row.id > 0)
    .map(asFavorite);
}

export function isGuestFavorite(videoId: number, storage: Storage = localStorage): boolean {
  return listGuestFavorites(storage).some((row) => row.id === videoId);
}

export function addGuestFavorite(video: GuestVideoInput, storage: Storage = localStorage): boolean {
  if (!video.id) return false;
  const rows = readJSON<StoredFavorite>(storage, GUEST_FAVORITES_KEY).filter((row) => row.id !== video.id);
  rows.unshift({ ...snapshot(video), favorited_at: new Date().toISOString() });
  writeJSON(storage, GUEST_FAVORITES_KEY, rows);
  return true;
}

export function removeGuestFavorite(videoId: number, storage: Storage = localStorage): boolean {
  const rows = readJSON<StoredFavorite>(storage, GUEST_FAVORITES_KEY).filter((row) => row.id !== videoId);
  writeJSON(storage, GUEST_FAVORITES_KEY, rows);
  return false;
}

export function listGuestHistory(storage: Storage = localStorage): HistoryVideo[] {
  return readJSON<StoredHistory>(storage, GUEST_HISTORY_KEY)
    .filter((row) => row && row.id > 0)
    .map(asHistory);
}

/** Records a visit. Progress never decreases, so a later 0 does not wipe a real position. */
export function upsertGuestHistory(
  video: GuestVideoInput,
  progressSec = 0,
  storage: Storage = localStorage,
): void {
  if (!video.id) return;
  const prev = readJSON<StoredHistory>(storage, GUEST_HISTORY_KEY).find((row) => row.id === video.id);
  const nextProgress = Math.max(0, Math.floor(progressSec) || 0, prev?.progress_sec ?? 0);
  const rows = readJSON<StoredHistory>(storage, GUEST_HISTORY_KEY).filter((row) => row.id !== video.id);
  rows.unshift({
    ...snapshot(video),
    last_watched_at: new Date().toISOString(),
    progress_sec: nextProgress,
  });
  writeJSON(storage, GUEST_HISTORY_KEY, rows);
}

export function removeGuestHistory(videoId: number, storage: Storage = localStorage): void {
  const rows = readJSON<StoredHistory>(storage, GUEST_HISTORY_KEY).filter((row) => row.id !== videoId);
  writeJSON(storage, GUEST_HISTORY_KEY, rows);
}

export function clearGuestHistory(storage: Storage = localStorage): void {
  writeJSON(storage, GUEST_HISTORY_KEY, []);
}
