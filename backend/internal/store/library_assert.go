package store

// Compile-time checks that both backends satisfy the library repos.
var (
	_ FavoriteRepo     = (*MySQL)(nil)
	_ FavoriteRepo     = (*MemoryFavorites)(nil)
	_ HistoryRepo      = (*MySQL)(nil)
	_ HistoryRepo      = (*MemoryHistory)(nil)
	_ NotificationRepo = (*MySQL)(nil)
	_ NotificationRepo = (*MemoryNotifications)(nil)
	_ CommentRepo      = (*MySQL)(nil)
	_ CommentRepo      = (*MemoryComments)(nil)
	_ UserRepo         = (*MySQL)(nil)
	_ UserRepo         = (*MemoryUsers)(nil)
)
