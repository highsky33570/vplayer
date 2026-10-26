import type { CategoryOwnership } from './categoryOwnership';

/** Display labels only. Stored values and API fields stay unchanged. */

export function activeLabel(active: boolean) {
  return active ? '启用' : '停用';
}

export function ownershipLabel(ownership: CategoryOwnership) {
  return ownership === 'Synced' ? '已同步' : '本地';
}

export function roleLabel(isAdmin: boolean) {
  return isAdmin ? '管理员' : '普通用户';
}

export function yesNo(value: boolean) {
  return value ? '是' : '否';
}

export function apiStatusLabel(status: string) {
  return status === 'ok' ? '正常' : status || '—';
}

export function jobLabel(jobType: string) {
  switch (jobType) {
    case 'incremental_sync':
      return '增量同步';
    case 'enrichment':
      return '待补全';
    case 'media_migration':
      return '媒体迁移';
    default:
      return jobType || '—';
  }
}

export function triggerLabel(triggerType: string) {
  switch (triggerType) {
    case 'cli':
      return 'CLI';
    case 'timer':
      return '定时';
    case 'incremental':
      return '增量';
    case 'manual':
      return '手动';
    case 'schedule':
      return '定时计划';
    default:
      return triggerType || '—';
  }
}

export function runStatusLabel(status: string) {
  switch (status) {
    case 'running':
      return '运行中';
    case 'succeeded':
      return '成功';
    case 'failed':
      return '失败';
    case 'rejected':
      return '已拒绝';
    default:
      return status || '—';
  }
}

export function catalogSyncResult(run: { has_error: boolean; failed_count: number } | null | undefined) {
  if (!run) return '—';
  if (run.has_error) return '记录有错误';
  if (run.failed_count > 0) return '部分失败';
  return '记录正常';
}

export function migrationStatusLabel(status?: string) {
  switch ((status ?? '').trim()) {
    case '':
    case 'pending':
      return '待处理';
    case 'done':
      return '已迁移';
    case 'failed':
      return '失败';
    default:
      return status || '待处理';
  }
}

export function playbackStatusLabel(status?: string) {
  switch ((status ?? '').trim()) {
    case '':
      return '—';
    case 'ok':
    case 'available':
      return '可播放';
    case 'unavailable':
      return '不可用';
    case 'pending_enrichment':
      return '待补全';
    case 'source_restricted':
      return '来源受限';
    default:
      return status || '—';
  }
}

export function videoStatusLabel(status?: string) {
  switch ((status ?? '').trim()) {
    case '':
      return '—';
    case 'ready':
      return '就绪';
    case 'draft':
      return '草稿';
    case 'disabled':
      return '已停用';
    case 'demo':
      return '演示';
    default:
      return status || '—';
  }
}

export function syncStatusLabel(status?: string) {
  switch ((status ?? '').trim()) {
    case '':
      return '—';
    case 'ok':
      return '正常';
    case 'error':
      return '错误';
    default:
      return status || '—';
  }
}

export function formatNullableCount(value?: number | null) {
  if (value === null || value === undefined) return '—';
  return String(value);
}

export function formatDuration(seconds?: number | null) {
  if (seconds === null || seconds === undefined) return '—';
  if (seconds < 60) return `${seconds} 秒`;
  const minutes = Math.floor(seconds / 60);
  const rest = seconds % 60;
  return rest ? `${minutes} 分 ${rest} 秒` : `${minutes} 分`;
}
