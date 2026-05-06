-- +goose Up
CREATE TABLE IF NOT EXISTS active_grabs (
  id          TEXT PRIMARY KEY,
  client_id   TEXT NOT NULL,
  download_id TEXT NOT NULL,
  title       TEXT NOT NULL DEFAULT '',
  grabbed_at  TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE(client_id, download_id)
);

CREATE TABLE IF NOT EXISTS active_grab_episodes (
  grab_id    TEXT NOT NULL REFERENCES active_grabs(id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL,
  PRIMARY KEY (grab_id, episode_id)
);
CREATE INDEX idx_active_grab_episodes_episode ON active_grab_episodes(episode_id);

CREATE TABLE IF NOT EXISTS active_grab_movies (
  grab_id  TEXT NOT NULL REFERENCES active_grabs(id) ON DELETE CASCADE,
  movie_id TEXT NOT NULL,
  PRIMARY KEY (grab_id, movie_id)
);
CREATE INDEX idx_active_grab_movies_movie ON active_grab_movies(movie_id);

-- +goose Down
DROP TABLE IF EXISTS active_grab_movies;
DROP TABLE IF EXISTS active_grab_episodes;
DROP TABLE IF EXISTS active_grabs;
