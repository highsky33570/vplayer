import { Link } from 'react-router-dom';
import { formatViews, type RelatedVideo } from '../api';
import { coverImgSrc, onCoverImgError } from '../media/coverFallback';
import { recommendationMeta } from '../pages/watchPageLogic';

export function RecommendationList({
  items,
  categoryName,
}: {
  items: RelatedVideo[];
  categoryName: (categoryId: number) => string;
}) {
  const list = Array.isArray(items) ? items : [];
  if (list.length === 0) return null;
  return (
    <aside className="watch-recs" aria-label="推荐视频">
      <h3>推荐视频</h3>
      <ul className="watch-rec-list">
        {list.map((item) => (
          <li key={item.id}>
            <Link className="watch-rec" to={`/watch/${item.id}`}>
              <img
                className="watch-rec-thumb"
                src={coverImgSrc(item.cover_url)}
                alt=""
                onError={onCoverImgError}
              />
              <span>
                <span className="watch-rec-title">{item.title}</span>
                <span className="watch-rec-meta">
                  {recommendationMeta([
                    `${formatViews(item.view_count)} 播放`,
                    item.year,
                    categoryName(item.category_id),
                  ])}
                </span>
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </aside>
  );
}
