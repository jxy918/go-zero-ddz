package svc

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/redis/go-redis/v9"

	_ "github.com/go-sql-driver/mysql"
	"go-zero-ddz/app/game/internal/ai"
	"go-zero-ddz/app/game/internal/cluster"
	"go-zero-ddz/app/game/internal/config"
	"go-zero-ddz/app/game/internal/handler"
	"go-zero-ddz/app/game/internal/match"
	"go-zero-ddz/app/game/internal/room"
	"go-zero-ddz/app/game/internal/websocket"
)

// initLogging 初始化日志系统，同时输出到控制台和文件
func initLogging() *os.File {
	// 创建 logs 目录
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("Warning: failed to create log directory: %v", err)
		return nil
	}

	// 生成日志文件名（按日期）
	logFileName := filepath.Join(logDir, fmt.Sprintf("game-%s.log", time.Now().Format("2006-01-02")))
	
	// 以追加模式打开日志文件，如果不存在则创建
	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Warning: failed to open log file: %v", err)
		return nil
	}

	// 设置日志输出同时到控制台和文件
	log.SetOutput(os.Stdout)
	
	// 创建一个多写者，可以同时写入多个目的地
	multiWriter := &multiLogger{writers: []io.Writer{os.Stdout, logFile}}
	log.SetOutput(multiWriter)
	
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	
	log.Printf("Log file: %s", logFileName)
	return logFile
}

// multiLogger 多写者日志器
type multiLogger struct {
	writers []io.Writer
}

func (m *multiLogger) Write(p []byte) (n int, err error) {
	for _, w := range m.writers {
		w.Write(p)
	}
	return len(p), nil
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

	// 日志文件
	logFile *os.File

	// 服务状态
	ctx    context.Context
	cancel context.CancelFunc
}

// NewServiceContext 创建服务上下文
func NewServiceContext(cfg *config.Config) (*ServiceContext, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// 初始化日志系统（同时输出到控制台和文件）
	logFile := initLogging()

	// 初始化 Redis
	rdb, err := initRedis(cfg.Redis)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("init redis: %w", err)
	}

	// 初始化 MySQL
	db, err := initMySQL(cfg.MySQL)
	if err != nil {
		log.Printf("MySQL init failed: %v (running without database)", err)
		db = nil
	}

	svcCtx := &ServiceContext{
		Config:   cfg,
		Redis:     rdb,
		DB:        db,
		logFile:   logFile,
		ctx:       ctx,
		cancel:    cancel,
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
		log.Printf("Instance registered: %s", s.Registry.GetInstanceID())
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
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("metrics endpoint"))
		})
	}

	// 静态文件服务（提供前端界面）
	staticDir := s.Config.WebStaticDir
	if staticDir == "" {
		staticDir = "../../client-web" // 相对于 app/game/cmd/server 的默认路径
	}
	log.Printf("Serving static files from: %s", staticDir)
	fs := http.FileServer(http.Dir(staticDir))
	mux.Handle("/", fs)

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	log.Printf("Starting WebSocket server on %s", addr)
	log.Printf("WebSocket endpoint: ws://%s/ws", addr)
	log.Printf("Web frontend: http://%s/", addr)
	log.Printf("Health check: http://%s/health", addr)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	log.Println("Service started successfully")
	return nil
}

// Stop 停止服务
func (s *ServiceContext) Stop() {
	// 关闭日志文件
	if s.logFile != nil {
		s.logFile.Close()
		s.logFile = nil
	}

	// 关闭 HTTP 服务器
	if s.httpServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		s.httpServer.Shutdown(shutdownCtx)
	}

	// 注销实例（集群模式）
	if s.Config.Cluster.Enabled && s.Registry != nil {
		if err := s.Registry.Unregister(); err != nil {
			log.Printf("Failed to unregister instance: %v", err)
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

	log.Printf("Redis connected: %s (type: %s)", cfg.Host, cfg.Type)
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

	log.Printf("MySQL connected: %s", cfg.DSN)
	return db, nil
}

// generateInstanceID 生成实例 ID
func generateInstanceID() string {
	hostname, _ := os.Hostname()
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}