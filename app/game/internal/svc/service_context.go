package svc

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zeromicro/go-zero/core/logx"

	"go-zero-ddz/app/game/internal/ai"
	"go-zero-ddz/app/game/internal/cluster"
	"go-zero-ddz/app/game/internal/config"
	"go-zero-ddz/app/game/internal/handler"
	"go-zero-ddz/app/game/internal/match"
	"go-zero-ddz/app/game/internal/room"
	"go-zero-ddz/app/game/internal/websocket"

	_ "github.com/go-sql-driver/mysql"
)

// initLogging 初始化日志系统
func initLogging(cfg *config.LogConfig) {
	logx.Info("Logging initialized")
}

// ServiceContext 服务上下文
type ServiceContext struct {
	Config *config.Config

	// Redis 客户端
	Redis redis.UniversalClient

	// MySQL 数据库
	DB *sql.DB

	// WebSocket Hub
	Hub *websocket.Hub

	// 房间管理器
	RoomManager *room.Manager

	// 消息处理器
	HandlerManager *handler.HandlerManager

	// 匹配协调器
	MatchCoordinator *match.Coordinator

	// 集群组件
	Registry   *cluster.Registry
	Router     *cluster.Router
	MessageBus *cluster.MessageBus

	// HTTP 服务器
	httpServer *http.Server

	// 服务状态
	ctx    context.Context
	cancel context.CancelFunc
}

// NewServiceContext 创建服务上下文
func NewServiceContext(cfg *config.Config) (*ServiceContext, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// 初始化日志系统
	initLogging(&cfg.Log)

	// 初始化 Redis
	rdb, err := initRedis(cfg.Redis)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("init redis: %w", err)
	}

	// 初始化 MySQL
	db, err := initMySQL(cfg.MySQL)
	if err != nil {
		logx.Errorf("MySQL init failed: %v (running without database)", err)
		db = nil
	}

	svcCtx := &ServiceContext{
		Config: cfg,
		Redis:  rdb,
		DB:     db,
		ctx:    ctx,
		cancel: cancel,
	}

	// 创建 WebSocket Hub
	svcCtx.Hub = websocket.NewHub(&cfg.WebSocket)

	// 创建房间管理器
	aiEngine := ai.NewAIEngine(&ai.AIConfig{
		RememberCards: true,
		UseBomb:       true,
		Strategy:      "minimum",
		ResponseRate:  0.9,
		DelayMsMin:    300,
		DelayMsMax:    800,
		CardInference: false,
	})

	svcCtx.RoomManager = room.NewManager(rdb, &room.RoomConfig{
		MaxRooms:          cfg.Room.MaxRooms,
		MaxPlayersPerRoom: cfg.Room.MaxPlayersPerRoom,
		ReadyTimeout:      cfg.Room.ReadyTimeout,
		PlayTimeout:       cfg.Room.PlayTimeout,
		ReconnectTimeout:  cfg.Room.ReconnectTimeout,
		SnapshotInterval:  cfg.Room.SnapshotInterval,
		BotJoinTimeout:    cfg.Room.BotJoinTimeout,
	}, aiEngine, svcCtx.Hub)

	// 创建消息处理器
	svcCtx.MatchCoordinator = match.NewCoordinator(match.Config{
		Enabled:        true,
		ScanInterval:   5,
		RandomTimeout:  30,
		RankedTimeout:  30,
		EloRange:       100,
		BotFillTimeout: 30,
	}, rdb, svcCtx.RoomManager, svcCtx.Hub)

	svcCtx.HandlerManager = handler.NewHandlerManager(svcCtx.Hub, svcCtx.RoomManager, svcCtx.MatchCoordinator, svcCtx.DB)
	svcCtx.HandlerManager.RegisterAll()

	// 如果启用集群模式，初始化集群组件
	if cfg.Cluster.Enabled {
		instanceID := cfg.Cluster.InstanceId
		if instanceID == "" {
			instanceID = generateInstanceID()
		}

		svcCtx.Registry = cluster.NewRegistry(rdb, cfg.Cluster.Host, cfg.Cluster.Port)
		svcCtx.Router = cluster.NewRouter(rdb, instanceID)
		svcCtx.MessageBus = cluster.NewMessageBus(rdb, instanceID)
	}

	return svcCtx, nil
}

// Start 启动服务
func (s *ServiceContext) Start() error {
	// 注册实例到 Redis（集群模式）
	if s.Config.Cluster.Enabled && s.Registry != nil {
		if err := s.Registry.Register(); err != nil {
			return fmt.Errorf("register instance: %w", err)
		}
		logx.Infof("Instance registered: %s", s.Registry.GetInstanceID())
	}

	// 启动消息总线（集群模式）
	if s.Config.Cluster.Enabled && s.MessageBus != nil {
		s.MessageBus.Start(s.ctx)
	}

	// 启动 WebSocket Hub
	go s.Hub.Run()

	// 启动匹配系统
	if s.MatchCoordinator != nil {
		s.MatchCoordinator.Start()
	}

	// 启动 HTTP 服务器（WebSocket 端点 + 健康检查）
	addr := fmt.Sprintf("%s:%d", s.Config.Host, s.Config.Port)
	mux := http.NewServeMux()

	// WebSocket 端点
	mux.HandleFunc("/ws", s.Hub.ServeHTTP)

	// 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// 就绪检查
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("READY"))
	})

	// 指标端点
	if s.Config.Metrics.Enabled {
		mux.HandleFunc(s.Config.Metrics.Path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(s.collectMetrics()))
		})
	}

	// 静态文件服务（提供前端界面）
	staticDir := s.Config.WebStaticDir
	if staticDir == "" {
		staticDir = "../../client-web" // 相对于 app/game/cmd/server 的默认路径
	}
	logx.Infof("Serving static files from: %s", staticDir)
	fs := http.FileServer(http.Dir(staticDir))
	mux.Handle("/", fs)

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	logx.Infof("Starting WebSocket server on %s", addr)
	logx.Infof("WebSocket endpoint: ws://%s/ws", addr)
	logx.Infof("Web frontend: http://%s/", addr)
	logx.Infof("Health check: http://%s/health", addr)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logx.Errorf("HTTP server error: %v", err)
		}
	}()

	logx.Info("Service started successfully")
	return nil
}

// Stop 停止服务
func (s *ServiceContext) Stop() {
	// 关闭 HTTP 服务器
	if s.httpServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		s.httpServer.Shutdown(shutdownCtx)
	}

	// 注销实例（集群模式）
	if s.Config.Cluster.Enabled && s.Registry != nil {
		if err := s.Registry.Unregister(); err != nil {
			logx.Errorf("Failed to unregister instance: %v", err)
		}
	}

	// 关闭消息总线
	if s.MessageBus != nil {
		s.MessageBus.Close()
	}

	// 关闭房间管理器
	if s.RoomManager != nil {
		s.RoomManager.Stop()
	}

	// 停止匹配系统
	if s.MatchCoordinator != nil {
		s.MatchCoordinator.Stop()
	}

	// 关闭 WebSocket Hub
	if s.Hub != nil {
		s.Hub.Stop()
	}

	// 关闭 Redis 连接
	if s.Redis != nil {
		s.Redis.Close()
	}

	// 取消上下文
	s.cancel()
}

// collectMetrics 收集监控指标
func (s *ServiceContext) collectMetrics() string {
	var metrics strings.Builder

	// WebSocket 连接统计
	if s.Hub != nil {
		stats := s.Hub.GetStats()
		metrics.WriteString(fmt.Sprintf("# HELP ddz_websocket_connections_total Total WebSocket connections\n"))
		metrics.WriteString(fmt.Sprintf("# TYPE ddz_websocket_connections_total counter\n"))
		metrics.WriteString(fmt.Sprintf("ddz_websocket_connections_total %d\n", stats.TotalConnections))

		metrics.WriteString(fmt.Sprintf("# HELP ddz_websocket_connections_current Current WebSocket connections\n"))
		metrics.WriteString(fmt.Sprintf("# TYPE ddz_websocket_connections_current gauge\n"))
		metrics.WriteString(fmt.Sprintf("ddz_websocket_connections_current %d\n", stats.CurrentConnections))

		metrics.WriteString(fmt.Sprintf("# HELP ddz_websocket_connections_max Max WebSocket connections\n"))
		metrics.WriteString(fmt.Sprintf("# TYPE ddz_websocket_connections_max gauge\n"))
		metrics.WriteString(fmt.Sprintf("ddz_websocket_connections_max %d\n", stats.MaxConnections))

		metrics.WriteString(fmt.Sprintf("# HELP ddz_websocket_messages_total Total WebSocket messages\n"))
		metrics.WriteString(fmt.Sprintf("# TYPE ddz_websocket_messages_total counter\n"))
		metrics.WriteString(fmt.Sprintf("ddz_websocket_messages_total %d\n", stats.MessageCount))

		metrics.WriteString(fmt.Sprintf("# HELP ddz_websocket_errors_total Total WebSocket errors\n"))
		metrics.WriteString(fmt.Sprintf("# TYPE ddz_websocket_errors_total counter\n"))
		metrics.WriteString(fmt.Sprintf("ddz_websocket_errors_total %d\n", stats.ErrorCount))
	}

	// 房间统计
	if s.RoomManager != nil {
		roomCount := s.RoomManager.GetRoomCount()
		metrics.WriteString(fmt.Sprintf("# HELP ddz_rooms_total Total rooms\n"))
		metrics.WriteString(fmt.Sprintf("# TYPE ddz_rooms_total gauge\n"))
		metrics.WriteString(fmt.Sprintf("ddz_rooms_total %d\n", roomCount))
	}

	// 匹配队列统计（需要在 Coordinator 中添加方法）
	if s.MatchCoordinator != nil {
		queueStats := s.MatchCoordinator.GetQueueStats()
		metrics.WriteString(fmt.Sprintf("# HELP ddz_match_queue_random Waiting players in random queue\n"))
		metrics.WriteString(fmt.Sprintf("# TYPE ddz_match_queue_random gauge\n"))
		metrics.WriteString(fmt.Sprintf("ddz_match_queue_random %d\n", queueStats.RandomCount))

		metrics.WriteString(fmt.Sprintf("# HELP ddz_match_queue_ranked Waiting players in ranked queue\n"))
		metrics.WriteString(fmt.Sprintf("# TYPE ddz_match_queue_ranked gauge\n"))
		metrics.WriteString(fmt.Sprintf("ddz_match_queue_ranked %d\n", queueStats.RankedCount))

		metrics.WriteString(fmt.Sprintf("# HELP ddz_match_matched_total Total matched players\n"))
		metrics.WriteString(fmt.Sprintf("# TYPE ddz_match_matched_total counter\n"))
		metrics.WriteString(fmt.Sprintf("ddz_match_matched_total %d\n", queueStats.MatchedCount))

		metrics.WriteString(fmt.Sprintf("# HELP ddz_match_timeout_total Total match timeouts\n"))
		metrics.WriteString(fmt.Sprintf("# TYPE ddz_match_timeout_total counter\n"))
		metrics.WriteString(fmt.Sprintf("ddz_match_timeout_total %d\n", queueStats.TimeoutCount))
	}

	// 系统信息
	metrics.WriteString(fmt.Sprintf("# HELP ddz_start_time_seconds Service start time\n"))
	metrics.WriteString(fmt.Sprintf("# TYPE ddz_start_time_seconds gauge\n"))
	metrics.WriteString(fmt.Sprintf("ddz_start_time_seconds %d\n", time.Now().Unix()))

	return metrics.String()
}

// initRedis 初始化 Redis 客户端
func initRedis(cfg config.RedisConfig) (redis.UniversalClient, error) {
	var rdb redis.UniversalClient

	switch cfg.Type {
	case "cluster":
		rdb = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    cfg.ClusterNodes,
			Password: cfg.Pass,
		})
	case "sentinel":
		rdb = redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:    cfg.SentinelMaster,
			SentinelAddrs: cfg.SentinelNodes,
			Password:      cfg.Pass,
		})
	default: // node
		rdb = redis.NewClient(&redis.Options{
			Addr:     cfg.Host,
			Password: cfg.Pass,
		})
	}

	// 测试连接
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	logx.Infof("Redis connected: %s (type: %s)", cfg.Host, cfg.Type)
	return rdb, nil
}

// initMySQL 初始化 MySQL 数据库
func initMySQL(cfg config.MySQLConfig) (*sql.DB, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("MySQL DSN not configured")
	}

	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// 配置连接池
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 测试连接
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("mysql ping failed: %w", err)
	}

	logx.Infof("MySQL connected: %s", cfg.DSN)
	return db, nil
}

// generateInstanceID 生成实例 ID
func generateInstanceID() string {
	hostname, _ := os.Hostname()
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}
