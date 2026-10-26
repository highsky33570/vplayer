import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ApiError,
  fetchAdminVideo,
  listCategories,
  type AdminVideoDetail as AdminVideoDetailData,
  type Category,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import { activeLabel, syncStatusLabel, videoStatusLabel } from './adminLabels';

function formatTime(value?: string) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

export function AdminVideoDetail() {
  const { id: idParam } = useParams();
  const id = Number(idParam);
  const { token, logout } = useAuth();
  const navigate = useNavigate();
  const [data, setData] = useState<AdminVideoDetailData | null>(null);
  const [categories, setCategories] = useState<Category[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void listCategories()
      .then((cats) => {
        if (!cancelled) setCategories(cats);
      })
      .catch(() => {
        if (!cancelled) setCategories([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      if (!Number.isFinite(id) || id <= 0) {
        setError('无效的视频 ID');
        setLoading(false);
        return;
      }
      setLoading(true);
      setError(null);
      setForbidden(false);
      try {
        const next = await fetchAdminVideo(id, token);
        if (!cancelled) setData(next);
      } catch (e) {
        if (cancelled) return;
        if (e instanceof ApiError) {
          if (e.status === 401) {
            logout();
            navigate(loginWithNext(`/admin/videos/${id}`), { replace: true });
            return;
          }
          if (e.status === 403) {
            setForbidden(true);
            setError(e.message || '需要管理员权限');
            return;
          }
          setError(e.message);
          return;
        }
        setError(e instanceof Error ? e.message : '加载失败');
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [id, token, logout, navigate]);

  const categoryName =
    categories.find((c) => c.id === data?.category_id)?.name ??
    (data ? String(data.category_id) : '—');

  return (
    <section className="admin-page">
      <div className="admin-page-head">
        <p className="admin-breadcrumb">
          <Link to="/admin/videos">视频管理</Link> / 详情
        </p>
        <h1>{data?.title ?? '视频详情'}</h1>
        <p className="admin-muted">只读详情 · 不含播放地址与密钥</p>
      </div>

      {loading ? <p className="admin-muted">加载中…</p> : null}
      {forbidden ? (
        <div className="admin-alert is-danger" role="alert">
          无权访问（403）。
        </div>
      ) : null}
      {!loading && !forbidden && error ? (
        <div className="admin-alert is-danger" role="alert">
          {error}
        </div>
      ) : null}

      {!loading && !forbidden && !error && data ? (
        <div className="admin-detail-grid">
          <div className="admin-detail-cover">
            {data.cover_url ? (
              <img src={data.cover_url} alt="" />
            ) : (
              <div className="admin-thumb is-empty is-large" />
            )}
          </div>
          <dl className="admin-detail-fields">
            <div>
              <dt>ID</dt>
              <dd>{data.id}</dd>
            </div>
            <div>
              <dt>标题</dt>
              <dd>{data.title}</dd>
            </div>
            <div>
              <dt>分类</dt>
              <dd>{categoryName}</dd>
            </div>
            <div>
              <dt>状态</dt>
              <dd>{videoStatusLabel(data.status)}</dd>
            </div>
            <div>
              <dt>启用状态</dt>
              <dd>{activeLabel(data.is_active)}</dd>
            </div>
            <div>
              <dt>来源</dt>
              <dd>
                {data.source_system || '—'}
                {data.source_id ? ` / ${data.source_id}` : ''}
              </dd>
            </div>
            <div>
              <dt>来源分类</dt>
              <dd>{data.source_category_id || '—'}</dd>
            </div>
            <div>
              <dt>年份</dt>
              <dd>{data.year || '—'}</dd>
            </div>
            <div>
              <dt>导演</dt>
              <dd>{data.director || '—'}</dd>
            </div>
            <div>
              <dt>演员</dt>
              <dd>{data.actors || '—'}</dd>
            </div>
            <div>
              <dt>评分</dt>
              <dd>{data.rating || '—'}</dd>
            </div>
            <div>
              <dt>同步</dt>
              <dd>{syncStatusLabel(data.sync_status)}</dd>
            </div>
            <div>
              <dt>播放来源</dt>
              <dd>
                {data.playback_source || '—'}（SID {data.play_sid}，NID {data.play_nid}）
              </dd>
            </div>
            <div>
              <dt>播放量 / 时长</dt>
              <dd>
                {data.view_count} / {data.duration_sec} 秒
              </dd>
            </div>
            <div>
              <dt>创建时间</dt>
              <dd>{formatTime(data.created_at)}</dd>
            </div>
            <div>
              <dt>最近同步</dt>
              <dd>{formatTime(data.last_synced_at)}</dd>
            </div>
            <div>
              <dt>来源更新时间</dt>
              <dd>{formatTime(data.source_updated_at)}</dd>
            </div>
            <div className="admin-detail-wide">
              <dt>简介</dt>
              <dd>{data.description || '—'}</dd>
            </div>
          </dl>
        </div>
      ) : null}
    </section>
  );
}
