-- Remove OLEHDTV VIP蓝光影院 titles (parent 6 / leaves 601–613).
-- Public VIP channel is reserved for admin-created local videos only.
-- Sync already skips ingesting these. This cleans historical rows.

DELETE FROM video_episodes
 WHERE video_id IN (
   SELECT id FROM videos
    WHERE COALESCE(source_system,'') = 'olehdtv'
      AND COALESCE(source_category_id,'') IN (
        '6','601','602','603','604','605','606','607',
        '608','609','610','611','612','613'
      )
 );

DELETE FROM video_comments
 WHERE video_id IN (
   SELECT id FROM videos
    WHERE COALESCE(source_system,'') = 'olehdtv'
      AND COALESCE(source_category_id,'') IN (
        '6','601','602','603','604','605','606','607',
        '608','609','610','611','612','613'
      )
 );

DELETE FROM video_source_comments
 WHERE video_id IN (
   SELECT id FROM videos
    WHERE COALESCE(source_system,'') = 'olehdtv'
      AND COALESCE(source_category_id,'') IN (
        '6','601','602','603','604','605','606','607',
        '608','609','610','611','612','613'
      )
 );

DELETE FROM user_favorites
 WHERE video_id IN (
   SELECT id FROM videos
    WHERE COALESCE(source_system,'') = 'olehdtv'
      AND COALESCE(source_category_id,'') IN (
        '6','601','602','603','604','605','606','607',
        '608','609','610','611','612','613'
      )
 );

DELETE FROM user_watch_history
 WHERE video_id IN (
   SELECT id FROM videos
    WHERE COALESCE(source_system,'') = 'olehdtv'
      AND COALESCE(source_category_id,'') IN (
        '6','601','602','603','604','605','606','607',
        '608','609','610','611','612','613'
      )
 );

DELETE FROM user_notifications
 WHERE video_id IN (
   SELECT id FROM videos
    WHERE COALESCE(source_system,'') = 'olehdtv'
      AND COALESCE(source_category_id,'') IN (
        '6','601','602','603','604','605','606','607',
        '608','609','610','611','612','613'
      )
 );

DELETE FROM videos
 WHERE COALESCE(source_system,'') = 'olehdtv'
   AND COALESCE(source_category_id,'') IN (
     '6','601','602','603','604','605','606','607',
     '608','609','610','611','612','613'
   );
