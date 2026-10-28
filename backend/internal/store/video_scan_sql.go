package store

// videoScanSelect is the column list scanVideo expects (unaliased videos table).
const videoScanSelect = `
		id, COALESCE(source_system,''), COALESCE(source_id,''), category_id, COALESCE(source_category_id,''),
		COALESCE(leaf_source_id,''),
		title, COALESCE(description,''), COALESCE(year,''), COALESCE(area,''), COALESCE(director,''),
		COALESCE(actors,''), COALESCE(rating,0), cover_key, COALESCE(cover_r2_key,''), COALESCE(poster_source_url,''),
		duration_sec, view_count, hls_master_key, COALESCE(playback_source,''), COALESCE(play_sid,1), COALESCE(play_nid,1),
		COALESCE(playback_url,''), source_updated_at, last_synced_at, COALESCE(sync_status,'ok'),
		COALESCE(is_active,1), status, COALESCE(membership_required,0), COALESCE(is_hot,0), created_at`
