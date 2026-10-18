import { useEffect, useState } from 'react';
import {
  ApiError,
  deleteComment,
  listComments,
  postComment,
  type VideoComment,
} from '../api';
import { commentValidationMessage, formatCommentTime } from '../pages/watchPageLogic';

export function CommentSection({
  videoId,
  token,
  nickname,
  onLogin,
}: {
  videoId: number;
  token: string | null;
  nickname: string;
  onLogin: () => void;
}) {
  const [items, setItems] = useState<VideoComment[]>([]);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [draft, setDraft] = useState('');
  const [posting, setPosting] = useState(false);
  const [formError, setFormError] = useState('');
  const [loadingMore, setLoadingMore] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError('');
    listComments(videoId, 0, token)
      .then((page) => {
        if (cancelled) return;
        setItems(page.data);
        setTotal(page.total);
        setHasMore(page.has_more);
      })
      .catch(() => {
        if (!cancelled) setError('评论加载失败');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [videoId, token, reloadKey]);

  async function reloadFirstPage() {
    const page = await listComments(videoId, 0, token);
    setItems(page.data);
    setTotal(page.total);
    setHasMore(page.has_more);
  }

  async function submit() {
    if (posting || !token) return;
    const message = commentValidationMessage(draft);
    if (message) {
      setFormError(message);
      return;
    }
    setPosting(true);
    setFormError('');
    try {
      await postComment(videoId, draft.trim(), token);
      setDraft('');
      await reloadFirstPage();
    } catch (e) {
      setFormError(e instanceof ApiError ? e.message : '评论发布失败');
    } finally {
      setPosting(false);
    }
  }

  async function loadMore() {
    if (loadingMore || !hasMore) return;
    setLoadingMore(true);
    try {
      const page = await listComments(videoId, items.length, token);
      setItems((current) => [...current, ...page.data]);
      setTotal(page.total);
      setHasMore(page.has_more);
    } catch (e) {
      setFormError(e instanceof ApiError ? e.message : '评论加载失败');
    } finally {
      setLoadingMore(false);
    }
  }

  async function remove(comment: VideoComment) {
    if (!token || !window.confirm('确认删除这条评论？')) return;
    try {
      await deleteComment(comment.id, token);
      setItems((current) => current.filter((item) => item.id !== comment.id));
      setTotal((current) => Math.max(0, current - 1));
    } catch (e) {
      setFormError(e instanceof ApiError ? e.message : '删除失败');
    }
  }

  const letter = (nickname.trim() || '用').slice(0, 1).toUpperCase();

  return (
    <section className="watch-comments" aria-label="评论">
      <h3>评论 {total}</h3>
      {token ? (
        <form
          className="watch-composer"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <span className="banner-avatar" aria-hidden>
            {letter}
          </span>
          <textarea
            value={draft}
            maxLength={1000}
            placeholder="发一条评论"
            aria-label="评论内容"
            onChange={(event) => setDraft(event.target.value)}
          />
          <button type="submit" disabled={posting}>
            {posting ? '发布中…' : '发布'}
          </button>
        </form>
      ) : (
        <button type="button" className="watch-comment-login" onClick={onLogin}>
          登录后发表评论
        </button>
      )}
      {formError && <p className="watch-comments-error">{formError}</p>}
      {loading && <p className="watch-comments-empty">加载中…</p>}
      {!loading && error && (
        <p className="watch-comments-error">
          评论加载失败
          <button type="button" className="watch-comments-retry" onClick={() => setReloadKey((n) => n + 1)}>
            重试
          </button>
        </p>
      )}
      {!loading && !error && items.length === 0 && <p className="watch-comments-empty">暂无评论，来说两句吧</p>}
      <ul className="watch-comment-list">
        {items.map((comment) => (
          <li key={comment.id} className="watch-comment">
            <span className="banner-avatar" aria-hidden>
              {(comment.author.display_name || '用').slice(0, 1).toUpperCase()}
            </span>
            <div>
              <strong>{comment.author.display_name}</strong>
              <p className="watch-comment-body">{comment.body}</p>
              <p className="watch-comment-meta">
                {formatCommentTime(comment.created_at)}
                {comment.mine && (
                  <button type="button" className="watch-comment-delete" onClick={() => void remove(comment)}>
                    删除
                  </button>
                )}
              </p>
            </div>
          </li>
        ))}
      </ul>
      {hasMore && (
        <button type="button" className="watch-load-more" onClick={() => void loadMore()} disabled={loadingMore}>
          {loadingMore ? '加载中…' : '加载更多'}
        </button>
      )}
    </section>
  );
}
