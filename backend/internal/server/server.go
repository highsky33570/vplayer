package server

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/catalogsync"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/httpapi"
	"github.com/tycdn/vplayer/internal/mail"
	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/play"
	"github.com/tycdn/vplayer/internal/scheduler"
	"github.com/tycdn/vplayer/internal/store"
)

type Server struct {
	engine *gin.Engine
	runner *scheduler.OleHdTvRunner
}

func New(cfg config.Config) (*Server, error) {
	if err := cfg.CheckMediaSigning(); err != nil {
		return nil, err
	}
	if cfg.AppEnv == "local" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	var mysql *store.MySQL
	db, err := store.OpenMySQL(cfg.MySQLDSN)
	if err != nil {
		log.Printf("mysql unavailable, using demo data: %v", err)
	} else {
		mysql = db
		if dir := store.FindMigrationsDir(); dir != "" {
			if err := mysql.ApplyMigrationsDir(dir); err != nil {
				_ = mysql.DB.Close()
				return nil, fmt.Errorf("migrate: %w", err)
			}
		}
	}

	localStore, err := media.NewLocalStore(cfg.OleHdTvLocalObjectsDir, cfg.OleHdTvMediaPublicBase)
	if err != nil {
		log.Printf("local object store: %v", err)
	}
	objStore := media.ObjectStore(localStore)
	if cfg.R2Endpoint != "" && cfg.R2AccessKey != "" {
		objStore = media.NewR2StoreRegion(cfg.R2Endpoint, cfg.R2Bucket, cfg.R2AccessKey, cfg.R2SecretKey, cfg.R2Region, cfg.R2PublicBase, localStore)
	}
	posters := media.NewPosterSyncer(objStore, cfg.OleHdTvFetchPosters)

	var runner *scheduler.OleHdTvRunner
	if mysql != nil {
		adapter, err := olehdtv.NewAdapter(cfg.OleHdTvSourceMode, cfg.OleHdTvMacCMSDSN, cfg.OleHdTvExportPath, cfg.OleHdTvFixturePath, cfg.OleHdTvHTMLDumpRoot, cfg.OleHdTvPublicBaseURL)
		if err != nil {
			log.Printf("olehdtv adapter: %v (sync disabled until fixed)", err)
		} else {
			svc := catalogsync.NewOleHdTvSyncService(mysql, adapter, posters)
			runner = scheduler.NewOleHdTvRunner(svc, cfg.OleHdTvSyncIntervalMin)
			if cfg.OleHdTvSyncEnabled {
				runner.Start()
				slog.Info("olehdtv scheduled sync enabled", "interval_min", cfg.OleHdTvSyncIntervalMin, "mode", cfg.OleHdTvSourceMode)
			}
		}
	}

	var users store.UserRepo
	if mysql != nil {
		users = mysql
	} else {
		users = store.NewMemoryUsers()
	}

	api := &httpapi.API{
		Cfg:        cfg,
		Store:      mysql,
		Users:      users,
		Resolver:   &play.Resolver{Cfg: cfg, Store: mysql},
		SyncRunner: runner,
		MediaBase:  cfg.OleHdTvMediaPublicBase,
	}
	api.StartMembershipReconcile()
	// Mail is optional at process start so login/admin keep working if SMTP is unset.
	// register/start and register/resend return a controlled 503 until configured.
	if cfg.RegistrationMailConfigured() {
		api.Mailer = &mail.SMTPMailer{Cfg: mail.SMTPConfig{
			Host:                  cfg.SMTPHost,
			Port:                  cfg.SMTPPort,
			Username:              cfg.SMTPUsername,
			Password:              cfg.SMTPPassword,
			FromEmail:             cfg.SMTPFromEmail,
			FromName:              cfg.SMTPFromName,
			TLSServerName:         cfg.SMTPTLSServerName,
			TLSInsecureSkipVerify: cfg.SMTPTLSInsecureSkipVerify,
		}}
	} else if cfg.AppEnv != "local" {
		slog.Warn("registration email verification mailer not configured; register/start will return unavailable until SMTP_* and EMAIL_CODE_PEPPER are set")
	}

	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger(), cors(cfg.CORSOrigins))

	if localStore != nil {
		r.StaticFS("/media", http.Dir(localStore.Root))
	}

	r.GET("/health", api.Health)
	v1 := r.Group("/api/v1")
	{
		v1.GET("/categories", api.ListCategories)
		v1.GET("/videos", api.ListVideos)
		v1.GET("/videos/:id", api.GetVideo)
		v1.GET("/videos/:id/play", api.RequireUserWhenPlaybackAuthRequired, api.ResolvePlay)
		v1.POST("/videos/:id/play", api.RequireUserWhenPlaybackAuthRequired, api.CreatePlaySession)
		v1.GET("/media/*path", api.ServeSignedMedia)
		v1.GET("/videos/:id/related", api.ListRelated)
		v1.GET("/videos/:id/episodes", api.ListEpisodes)
		v1.GET("/videos/:id/comments", api.ListComments)
		v1.POST("/videos/:id/comments", api.RequireUser, api.CreateComment)
		v1.DELETE("/comments/:id", api.RequireUser, api.DeleteComment)
		v1.GET("/auth/registration", api.RegistrationOptions)
		v1.POST("/auth/register", api.Register)
		v1.POST("/auth/register/start", api.RegisterStart)
		v1.POST("/auth/register/verify", api.RegisterVerify)
		v1.POST("/auth/register/resend", api.RegisterResend)
		v1.POST("/auth/password/forgot", api.PasswordForgot)
		v1.POST("/auth/password/reset", api.PasswordReset)
		v1.POST("/auth/login", api.Login)
		v1.GET("/auth/me", api.Me)
		v1.PATCH("/auth/me", api.RequireUser, api.UpdateMe)
		v1.GET("/membership/plans", api.ListMembershipPlans)
		v1.POST("/membership/orders", api.RequireUser, api.CreateMembershipOrder)
		v1.GET("/membership/orders/:orderNo", api.RequireUser, api.GetMembershipOrder)
		v1.POST("/payments/epusdt/notify", api.EpusdtNotify)
		v1.GET("/me/summary", api.RequireUser, api.MeSummary)
		v1.GET("/me/favorites", api.RequireUser, api.ListFavorites)
		v1.GET("/me/favorite-folders", api.RequireUser, api.ListFavoriteFolders)
		v1.GET("/me/favorites/:videoId", api.RequireUser, api.GetFavoriteStatus)
		v1.POST("/me/favorites/:videoId", api.RequireUser, api.AddFavorite)
		v1.DELETE("/me/favorites/:videoId", api.RequireUser, api.RemoveFavorite)
		v1.GET("/me/history", api.RequireUser, api.ListHistory)
		v1.POST("/me/history/:videoId", api.RequireUser, api.UpsertHistory)
		v1.DELETE("/me/history/:videoId", api.RequireUser, api.DeleteHistoryItem)
		v1.DELETE("/me/history", api.RequireUser, api.ClearHistory)
		v1.GET("/me/notifications", api.RequireUser, api.ListNotifications)
		v1.GET("/me/notifications/unread-count", api.RequireUser, api.UnreadNotificationCount)
		v1.POST("/me/notifications/read", api.RequireUser, api.MarkNotificationsRead)
		v1.GET("/admin/me", api.RequireUser, api.RequireAdmin, api.AdminMe)
		v1.GET("/admin/dashboard", api.RequireUser, api.RequireAdmin, api.AdminDashboard)
		v1.GET("/admin/videos", api.RequireUser, api.RequireAdmin, api.AdminListVideos)
		v1.POST("/admin/videos", api.RequireUser, api.RequireAdmin, api.AdminCreateVideo)
		// Static path before :id so "hot" is not captured as an id.
		v1.PATCH("/admin/videos/hot", api.RequireUser, api.RequireAdmin, api.AdminBatchSetVideoHot)
		v1.GET("/admin/videos/:id", api.RequireUser, api.RequireAdmin, api.AdminGetVideo)
		v1.PATCH("/admin/videos/:id", api.RequireUser, api.RequireAdmin, api.AdminUpdateVideo)
		v1.PATCH("/admin/videos/:id/membership", api.RequireUser, api.RequireAdmin, api.AdminSetVideoMembership)
		v1.GET("/admin/categories", api.RequireUser, api.RequireAdmin, api.AdminListCategories)
		v1.POST("/admin/categories/cleanup-duplicates", api.RequireUser, api.RequireAdmin, api.AdminCleanupDuplicateCategories)
		v1.GET("/admin/categories/:id", api.RequireUser, api.RequireAdmin, api.AdminGetCategory)
		v1.PATCH("/admin/categories/:id", api.RequireUser, api.RequireAdmin, api.AdminUpdateCategory)
		v1.GET("/admin/users", api.RequireUser, api.RequireAdmin, api.AdminListUsers)
		v1.GET("/admin/users/:id", api.RequireUser, api.RequireAdmin, api.AdminGetUser)
		v1.PATCH("/admin/users/:id", api.RequireUser, api.RequireAdmin, api.AdminUpdateUser)
		v1.DELETE("/admin/users/:id", api.RequireUser, api.RequireAdmin, api.AdminDeleteUser)
		v1.PATCH("/admin/users/:id/membership", api.RequireUser, api.RequireAdmin, api.AdminSetUserMembership)
		v1.GET("/admin/membership/plans", api.RequireUser, api.RequireAdmin, api.AdminListMembershipPlans)
		v1.POST("/admin/membership/plans", api.RequireUser, api.RequireAdmin, api.AdminCreateMembershipPlan)
		v1.PATCH("/admin/membership/plans/:id", api.RequireUser, api.RequireAdmin, api.AdminUpdateMembershipPlan)
		v1.GET("/admin/sync", api.RequireUser, api.RequireAdmin, api.AdminSyncOverview)
		v1.GET("/admin/sync/runs", api.RequireUser, api.RequireAdmin, api.AdminListSyncRuns)
		v1.GET("/admin/sync/runs/:id", api.RequireUser, api.RequireAdmin, api.AdminGetSyncRun)
		v1.GET("/admin/sync/operations", api.RequireUser, api.RequireAdmin, api.AdminListOperationalRuns)
		v1.GET("/admin/sync/operations/:id", api.RequireUser, api.RequireAdmin, api.AdminGetOperationalRun)
		v1.POST("/admin/sync/actions/incremental", api.RequireUser, api.RequireAdmin, api.AdminRunIncremental)
		v1.POST("/admin/sync/actions/enrichment", api.RequireUser, api.RequireAdmin, api.AdminRunEnrichment)
		v1.GET("/admin/media", api.RequireUser, api.RequireAdmin, api.AdminMediaOverview)
		v1.GET("/admin/media/episodes", api.RequireUser, api.RequireAdmin, api.AdminListMediaEpisodes)
		v1.GET("/admin/media/episodes/:id", api.RequireUser, api.RequireAdmin, api.AdminGetMediaEpisode)
		// Legacy ops token auth — unchanged; migrate later to RequireAdmin.
		v1.POST("/admin/integrations/olehdtv/sync", api.AdminOleHdTvSync)
	}

	return &Server{engine: r, runner: runner}, nil
}

func (s *Server) Run(addr string) error {
	return s.engine.Run(addr)
}

func (s *Server) Stop() {
	if s.runner != nil {
		s.runner.Stop()
	}
}

func cors(origins []string) gin.HandlerFunc {
	allow := map[string]bool{}
	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o != "" {
			allow[o] = true
		}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if allow[origin] || allow["*"] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, Range, X-Admin-Token")
			c.Header("Access-Control-Expose-Headers", "Accept-Ranges, Content-Length, Content-Range, Content-Type")
			c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
