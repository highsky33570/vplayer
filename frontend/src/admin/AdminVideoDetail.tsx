import { useEffect, useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ApiError,
  fetchAdminCategories,
  fetchAdminVideo,
  updateAdminVideo,
  type AdminCategory,
  type AdminVideoDetail as AdminVideoDetailData,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import { loginWithNext } from '../auth/paths';
import { syncStatusLabel } from './adminLabels';

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
  const [categories, setCategories] = useState<AdminCategory[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [okMsg, setOkMsg] = useState<string | null>(null);
  const [forbidden, setForbidden] = useState(false);

  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [coverURL, setCoverURL] = useState('');
  const [year, setYear] = useState('');
  const [director, setDirector] = useState('');
  const [actors, setActors] = useState('');
  const [categoryId, setCategoryId] = useState(0);
  const [isActive, setIsActive] = useState(true);
  const [status, setStatus] = useState('ready');
  const [membershipRequired, setMembershipRequired] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void fetchAdminCategories(token)
      .then((cats) => {
        if (!cancelled) setCategories(cats);
      })
      .catch(() => {
        if (!cancelled) setCategories([]);
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

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
        if (cancelled) return;
        setData(next);
        setTitle(next.title ?? '');
        setDescription(next.description ?? '');
        setCoverURL(next.cover_url ?? '');
        setYear(next.year ?? '');
        setDirector(next.director ?? '');
        setActors(next.actors ?? '');
        setCategoryId(next.category_id ?? 0);
        setIsActive(Boolean(next.is_active));
        setStatus(next.status || 'ready');
        setMembershipRequired(Boolean(next.membership_required));
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

  async function onSave(e: FormEvent) {
    e.preventDefault();
    if (!data) return;
    setSaving(true);
    setError(null);
    setOkMsg(null);
    try {
      const updated = await updateAdminVideo(token, data.id, {
        title: title.trim(),
        description,
        year,
        director,
        actors,
        category_id: categoryId > 0 ? categoryId : undefined,
        is_active: isActive,
        status,
        cover_url: coverURL.trim(),
        membership_required: membershipRequired,
      });
      setData(updated);
      setCoverURL(updated.cover_url ?? coverURL);
      setOkMsg('已保存');
      window.setTimeout(() => setOkMsg((cur) => (cur === '已保存' ? null : cur)), 1600);
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 401) {
          logout();
          navigate(loginWithNext(`/admin/videos/${id}`), { replace: true });
          return;
        }
        setError(err.message);
      } else {
        setError(err instanceof Error ? err.message : '保存失败');
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="admin-page">
      <div className="admin-page-head">
        <p className="admin-breadcrumb">
          <Link to="/admin/videos">视频管理</Link> / 详情
        </p>
        <h1>{data?.title ?? '视频详情'}</h1>
        <p className="admin-muted">可编辑标题、分类、状态、封面与会员设置 · 播放密钥只读</p>
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
      {okMsg ? (
        <div className="admin-alert is-ok" role="status">
          {okMsg}
        </div>
      ) : null}

      {!loading && !forbidden && data ? (
        <form className="admin-detail-grid" onSubmit={onSave}>
          <div className="admin-detail-cover">
            {coverURL ? (
              <img src={coverURL} alt="" />
            ) : (
              <div className="admin-thumb is-empty is-large" />
            )}
          </div>

          <div>
            <div className="admin-form-grid">
              <label className="admin-filter-field">
                <span>ID</span>
                <input className="admin-input" value={data.id} disabled />
              </label>
              <label className="admin-filter-field">
                <span>标题</span>
                <input
                  className="admin-input"
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  required
                />
              </label>
              <label className="admin-filter-field">
                <span>分类</span>
                <select
                  className="admin-input"
                  value={categoryId || ''}
                  onChange={(e) => setCategoryId(e.target.value ? Number(e.target.value) : 0)}
                >
                  <option value="">未选择</option>
                  {categories.map((c) => (
                    <option key={c.id} value={c.id}>
                      #{c.id} {c.name}
                      {!c.is_active ? '（停用）' : ''}
                    </option>
                  ))}
                </select>
              </label>
              <label className="admin-filter-field">
                <span>发布状态</span>
                <select
                  className="admin-input"
                  value={status}
                  onChange={(e) => setStatus(e.target.value)}
                >
                  <option value="ready">就绪</option>
                  <option value="draft">草稿</option>
                  <option value="disabled">禁用</option>
                  <option value="demo">演示</option>
                </select>
              </label>
              <label className="admin-filter-field">
                <span>启用</span>
                <select
                  className="admin-input"
                  value={isActive ? '1' : '0'}
                  onChange={(e) => setIsActive(e.target.value === '1')}
                >
                  <option value="1">启用</option>
                  <option value="0">停用</option>
                </select>
              </label>
              <label className="admin-filter-field">
                <span>会员专属</span>
                <select
                  className="admin-input"
                  value={membershipRequired ? '1' : '0'}
                  onChange={(e) => setMembershipRequired(e.target.value === '1')}
                >
                  <option value="0">否</option>
                  <option value="1">是</option>
                </select>
              </label>
              <label className="admin-filter-field">
                <span>年份</span>
                <input className="admin-input" value={year} onChange={(e) => setYear(e.target.value)} />
              </label>
              <label className="admin-filter-field">
                <span>导演</span>
                <input
                  className="admin-input"
                  value={director}
                  onChange={(e) => setDirector(e.target.value)}
                />
              </label>
              <label className="admin-filter-field admin-detail-wide">
                <span>演员</span>
                <input className="admin-input" value={actors} onChange={(e) => setActors(e.target.value)} />
              </label>
              <label className="admin-filter-field admin-detail-wide">
                <span>封面链接</span>
                <input
                  className="admin-input"
                  value={coverURL}
                  onChange={(e) => setCoverURL(e.target.value)}
                  placeholder="https://…"
                />
              </label>
              <label className="admin-filter-field admin-detail-wide">
                <span>简介</span>
                <textarea
                  className="admin-textarea"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
              </label>
            </div>

            <dl className="admin-detail-fields" style={{ marginTop: 18 }}>
              <div>
                <dt>来源</dt>
                <dd>
                  {data.source_system || '—'}
                  {data.source_id ? ` / ${data.source_id}` : ''}
                </dd>
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
            </dl>

            <div className="admin-form-actions">
              <button type="submit" className="admin-btn-primary" disabled={saving}>
                {saving ? '保存中…' : '保存更改'}
              </button>
              <Link className="admin-btn-ghost" to="/admin/videos" style={{ textDecoration: 'none' }}>
                返回列表
              </Link>
            </div>
          </div>
        </form>
      ) : null}
    </section>
  );
}
