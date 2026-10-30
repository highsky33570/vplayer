import { useEffect, useState } from 'react';
import {
  ApiError,
  deleteComment,
  listComments,
  postComment,
  type SourceComment,
  type VideoComment,
} from '../api';
import { useAuth } from '../auth/AuthContext';
import {
  canDeleteComment,
  commentValidationMessage,
  formatCommentTime,
  sourceAuthorName,
  sourceCommentTime,
  sourceLetter,
  sourceSectionState,
} from '../pages/watchPageLogic';

function SourceThreads({ items }: { items: SourceComment[] }) {
  return (
    <ul className="watch-comment-list">
      {items.map((comment, index) => (
        <li key={`${index}-${comment.source_created_raw}-${comment.body}`} className="watch-comment">
          <span className="banner-avatar" aria-hidden>
            {sourceLetter(comment.author.display_name)}
          </span>
          <div>
            <p className="watch-comment-head">
              <strong>{sourceAuthorName(comment.author.display_name)}</strong>
              <span className="watch-source-badge">原站</span>
            </p>
            <p className="watch-comment-body">{comment.body}</p>
            <p className="watch-comment-meta">
              {sourceCommentTime(comment.source_created_raw)}
              <span>
                {' '}
                👍 {comment.digg_up} 👎 {comment.digg_down}
              </span>
            </p>
            {comment.replies?.map((reply, replyIndex) => (
              <div key={`${replyIndex}-${reply.source_created_raw}-${reply.body}`} className="watch-comment watch-comment-reply">
                <span className="banner-avatar" aria-hidden>
                  {sourceLetter(reply.author.display_name)}
                </span>
                <div>
                  <p className="watch-comment-head">
                    <span aria-hidden>↳</span>
                    <strong>{sourceAuthorName(reply.author.display_name)}</strong>
                    <span className="watch-source-badge">原站</span>
                  </p>
                  <p className="watch-comment-body">{reply.body}</p>
                  <p className="watch-comment-meta">
                    {sourceCommentTime(reply.source_created_raw)}
                    <span>
                      {' '}
                      👍 {reply.digg_up} 👎 {reply.digg_down}
                    </span>
                  </p>
                </div>
              </div>
            ))}
          </div>
        </li>
      ))}
    </ul>
  );
}

function NativeComments({
  items,
  onDelete,
}: {
  items: VideoComment[];
  onDelete: (comment: VideoComment) => void;
}) {
  return (
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
              {canDeleteComment('vplayer', Boolean(comment.mine)) && (
                <button type="button" className="watch-comment-delete" onClick={() => onDelete(comment)}>
                  删除
                </button>
              )}
            </p>
          </div>
        </li>
      ))}
    </ul>
  );
}

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
  const { authRequired } = useAuth();
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
  const [sourceItems, setSourceItems] = useState<SourceComment[]>([]);
  const [sourceTotal, setSourceTotal] = useState(0);
  const [sourceHasMore, setSourceHasMore] = useState(false);
  const [sourceLoading, setSourceLoading] = useState(true);
  const [sourceError, setSourceError] = useState('');
  const [sourceReloadKey, setSourceReloadKey] = useState(0);
  const [sourceLoadingMore, setSourceLoadingMore] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setSourceLoading(true);
    setError('');
    setSourceError('');
    listComments(videoId, 0, token, 0)
      .then((page) => {
        if (cancelled) return;
        setItems(page.data);
        setTotal(page.total);
        setHasMore(page.has_more);
        setSourceItems(page.source.data);
        setSourceTotal(page.source.total);
        setSourceHasMore(page.source.has_more);
        setSourceError(page.source.error);
      })
      .catch(() => {
        if (!cancelled) {
          setError('评论加载失败');
          setSourceError('原站评论加载失败');
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
          setSourceLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [videoId, token, reloadKey, sourceReloadKey]);

  async function reloadFirstPage() {
    const page = await listComments(videoId, 0, token, 0);
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
      const page = await listComments(videoId, items.length, token, 0);
      setItems((current) => [...current, ...page.data]);
      setTotal(page.total);
      setHasMore(page.has_more);
    } catch (e) {
      setFormError(e instanceof ApiError ? e.message : '评论加载失败');
    } finally {
      setLoadingMore(false);
    }
  }

  async function loadMoreSource() {
    if (sourceLoadingMore || !sourceHasMore) return;
    setSourceLoadingMore(true);
    try {
      const page = await listComments(videoId, 0, token, sourceItems.length);
      if (page.source.error) {
        setSourceError(page.source.error);
        return;
      }
      setSourceItems((current) => [...current, ...page.source.data]);
      setSourceTotal(page.source.total);
      setSourceHasMore(page.source.has_more);
      setSourceError('');
    } catch {
      setSourceError('原站评论加载失败');
    } finally {
      setSourceLoadingMore(false);
    }
  }

  async function remove(comment: VideoComment) {
    if (!token || !canDeleteComment('vplayer', Boolean(comment.mine))) return;
    if (!window.confirm('确认删除这条评论？')) return;
    try {
      await deleteComment(comment.id, token);
      setItems((current) => current.filter((item) => item.id !== comment.id));
      setTotal((current) => Math.max(0, current - 1));
    } catch (e) {
      setFormError(e instanceof ApiError ? e.message : '删除失败');
    }
  }

  const letter = (nickname.trim() || '用').slice(0, 1).toUpperCase();
  const sourceState = sourceSectionState({ loading: sourceLoading, error: sourceError, count: sourceItems.length });

  return (
    <section className="watch-comments" aria-label="评论">
      <h3>评论</h3>
      <section className="watch-source-comments" aria-label="原站评论">
        <h4>原站评论 {sourceTotal}</h4>
        {sourceState === 'loading' && <p className="watch-comments-empty">原站评论加载中…</p>}
        {sourceState === 'error' && (
          <p className="watch-comments-error">
            原站评论加载失败
            <button type="button" className="watch-comments-retry" onClick={() => setSourceReloadKey((n) => n + 1)}>
              重试
            </button>
          </p>
        )}
        {sourceState === 'empty' && <p className="watch-comments-empty">暂无原站评论</p>}
        {sourceState === 'ready' && <SourceThreads items={sourceItems} />}
        {sourceHasMore && (
          <button
            type="button"
            className="watch-load-more"
            aria-label="加载更多原站评论"
            onClick={() => void loadMoreSource()}
            disabled={sourceLoadingMore}
          >
            {sourceLoadingMore ? '加载中…' : '加载更多'}
          </button>
        )}
      </section>
      <section className="watch-vplayer-comments" aria-label="天涯影视 评论">
        <h4>天涯影视 评论 {total}</h4>
        {authRequired && token ? (
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
        ) : authRequired ? (
          <button type="button" className="watch-comment-login" onClick={onLogin}>
            登录后发表评论
          </button>
        ) : null}
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
        <NativeComments items={items} onDelete={(comment) => void remove(comment)} />
        {hasMore && (
          <button type="button" className="watch-load-more" onClick={() => void loadMore()} disabled={loadingMore}>
            {loadingMore ? '加载中…' : '加载更多'}
          </button>
        )}
      </section>
    </section>
  );
}
