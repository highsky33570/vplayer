package server

import (
	"log"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tycdn/vplayer/internal/catalogsync"
	"github.com/tycdn/vplayer/internal/config"
	"github.com/tycdn/vplayer/internal/httpapi"
	"github.com/tycdn/vplayer/internal/media"
	"github.com/tycdn/vplayer/internal/olehdtv"
	"github.com/tycdn/vplayer/internal/play"
	"github.com/tycdn/vplayer/internal/scheduler"
	"github.com/tycdn/vplayer/internal/store"
)

type Server struct {
	engine  *gin.Engine
	runner  *scheduler.OleHdTvRunner
}

func New(cfg config.Config) (*Server, error) {
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
				log.Printf("migrate warning: %v", err)
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
		v1.GET("/videos/:id/play", api.ResolvePlay)
		v1.POST("/videos/:id/play", api.CreatePlaySession)
		v1.POST("/auth/register", api.Register)
		v1.POST("/auth/login", api.Login)
		v1.GET("/auth/me", api.Me)
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
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Admin-Token")
			c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
