import type { PublicEpisode } from '../api';
import { showEpisodeSelector } from '../pages/watchPageLogic';

export function EpisodeStrip({
  episodes,
  activeSid,
  activeNid,
  onSelect,
}: {
  episodes: PublicEpisode[];
  activeSid: number;
  activeNid: number;
  onSelect: (episode: PublicEpisode) => void;
}) {
  const list = Array.isArray(episodes) ? episodes : [];
  if (!showEpisodeSelector(list.length)) return null;
  return (
    <section className="watch-episodes" aria-label="选集">
      <h3>选集</h3>
      <div className="watch-episode-grid">
        {list.map((episode) => {
          const selected = episode.sid === activeSid && episode.nid === activeNid;
          const label = (episode.title ?? '').trim() || `${episode.sid}-${episode.nid}`;
          return (
            <button
              key={`${episode.sid}-${episode.nid}`}
              type="button"
              className={selected ? 'is-selected' : undefined}
              aria-pressed={selected}
              onClick={() => onSelect(episode)}
            >
              {label}
            </button>
          );
        })}
      </div>
    </section>
  );
}
