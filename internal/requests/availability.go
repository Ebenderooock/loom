package requests

// completionPredicate preserves the scope of fulfillment: currently due,
// monitored regular episodes and selected music releases, not specials or future releases.
const completionPredicate = `(
			(media_type='movie' AND EXISTS (
				SELECT 1 FROM movie_files f JOIN movies m ON m.id=f.movie_id
				WHERE f.movie_id=media_requests.media_id
					AND f.deleted_at IS NULL AND m.deleted_at IS NULL
			))
			OR (media_type='series'
				AND EXISTS (
					SELECT 1 FROM episode_files f
					JOIN episodes e ON e.id=f.episode_id
					JOIN seasons sn ON sn.id=e.season_id
					WHERE f.series_id=media_requests.media_id AND sn.season_number<>0
				)
				AND NOT EXISTS (
					SELECT 1 FROM episodes e JOIN seasons sn ON sn.id=e.season_id
					WHERE e.series_id=media_requests.media_id
						AND e.monitored=true AND sn.monitored=true
						AND sn.season_number<>0
						AND (
							(e.air_date_utc<>'' AND e.air_date_utc<=?)
							OR (e.air_date_utc='' AND (e.air_date IS NULL OR e.air_date='' OR e.air_date<=?))
						)
						AND NOT EXISTS (
							SELECT 1 FROM episode_files f WHERE f.episode_id=e.id
						)
				)
			)
			OR (media_type='artist'
				AND EXISTS (
					SELECT 1 FROM artists a WHERE a.id=media_requests.media_id AND a.deleted_at IS NULL
				)
				AND EXISTS (
					SELECT 1 FROM tracks t
					JOIN albums al ON al.id=t.album_id
					JOIN track_files f ON f.track_id=t.id AND f.deleted_at IS NULL
					WHERE al.artist_id=media_requests.media_id AND al.deleted_at IS NULL
						AND al.monitored=true AND t.monitored=true
						AND t.release_id=al.selected_release_id
				)
				AND NOT EXISTS (
					SELECT 1 FROM albums al
					WHERE al.artist_id=media_requests.media_id AND al.deleted_at IS NULL
						AND al.monitored=true AND (al.release_date='' OR al.release_date<=?)
						AND (
							NOT EXISTS (
								SELECT 1 FROM tracks t
								WHERE t.album_id=al.id AND t.release_id=al.selected_release_id AND t.monitored=true
							)
							OR EXISTS (
								SELECT 1 FROM tracks t
								WHERE t.album_id=al.id AND t.release_id=al.selected_release_id
									AND t.monitored=true AND NOT EXISTS (
										SELECT 1 FROM track_files f WHERE f.track_id=t.id AND f.deleted_at IS NULL
									)
							)
						)
				)
			)
		)`
