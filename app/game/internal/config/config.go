package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

type Config struct {
	Name         string          `json:",default=game-service"`
	Host         string          `json:",default=0.0.0.0"`
	Port         int             `json:",default=8080"`
	InstanceId   string          `json:",optional"`
	WebStaticDir string          `json:",default=./client-web,optional"`
	Cluster      ClusterConfig   `json:",optional"`
	Redis        RedisConfig     `json:",optional"`
	MySQL        MySQLConfig     `json:",optional"`
	WebSocket    WebSocketConfig `json:",optional"`
	Room         RoomConfig      `json:",optional"`
	Match        MatchConfig     `json:",optional"`
	AI           AIConfig        `json:",optional"`
	Log          LogConfig       `json:",optional"`
	Metrics      MetricsConfig   `json:",optional"`
}

type MySQLConfig struct {
	DSN string `json:",optional"`
}

type ClusterConfig struct {
	Enabled    bool   `json:",default=false"`
	InstanceId string `json:",optional"`
	Host       string `json:",default=127.0.0.1"`
	Port       int    `json:",default=8080"`
}

type RedisConfig struct {
	Host           string   `json:",default=localhost:6379"`
	Type           string   `json:",default=node"`
	Pass           string   `json:",optional"`
	Tls            bool     `json:",default=false"`
	ClusterNodes   []string `json:",optional"`
	SentinelMaster string   `json:",optional"`
	SentinelNodes  []string `json:",optional"`
}

type WebSocketConfig struct {
	ReadBufferSize   int `json:",default=4096"`
	WriteBufferSize  int `json:",default=4096"`
	HandshakeTimeout int `json:",default=10"`
	PingPeriod       int `json:",default=30"`
	PongWait         int `json:",default=60"`
	MaxMessageSize   int `json:",default=65536"`
	WriteWait        int `json:",default=10"`
}

type RoomConfig struct {
	MaxRooms          int `json:",default=1000"`
	MaxPlayersPerRoom int `json:",default=3"`
	ReadyTimeout      int `json:",default=60"`
	PlayTimeout       int `json:",default=15"`
	ReconnectTimeout  int `json:",default=300"`
	SnapshotInterval  int `json:",default=30"`
	BotJoinTimeout    int `json:",default=60"`
}

type MatchConfig struct {
	Enabled        bool `json:",default=true"`
	ScanInterval   int  `json:",default=2"`
	RandomTimeout  int  `json:",default=15"`
	RankedTimeout  int  `json:",default=30"`
	EloRange       int  `json:",default=100"`
	BotFillTimeout int  `json:",default=30"`
}

type AIConfig struct {
	Enabled           bool   `json:",default=true"`
	DefaultDifficulty string `json:",default=normal"`
	AutoEnableTimeout int    `json:",default=30"`
	PlayDelayMin      int    `json:",default=500"`
	PlayDelayMax      int    `json:",default=2000"`
}

type LogConfig struct {
	Mode     string `json:",default=console"`
	Path     string `json:",optional"`
	Level    string `json:",default=info"`
	Compress bool   `json:",default=false"`
	KeepDays int    `json:",default=7"`
}

type MetricsConfig struct {
	Enabled bool   `json:",default=false"`
	Path    string `json:",default=/metrics"`
}

// LoadConfig 从 YAML 文件加载配置，支持环境变量覆盖
func LoadConfig(path string) (*Config, error) {
	var c Config

	err := conf.Load(path, &c)
	if err != nil {
		return nil, fmt.Errorf("load config file: %w", err)
	}

	// 应用环境变量覆盖
	c.applyEnvOverrides()

	// 验证配置
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	logx.Info("Config loaded and validated successfully")
	return &c, nil
}

// applyEnvOverrides 应用环境变量覆盖配置
func (c *Config) applyEnvOverrides() {
	// 环境变量格式: GAME_<SECTION>_<KEY>，全部大写，下划线分隔
	// 示例: GAME_REDIS_HOST, GAME_ROOM_MAXROOMS

	// Redis
	if val := getEnv("GAME_REDIS_HOST"); val != "" {
		c.Redis.Host = val
	}
	if val := getEnv("GAME_REDIS_PASS"); val != "" {
		c.Redis.Pass = val
	}

	// MySQL
	if val := getEnv("GAME_MYSQL_DSN"); val != "" {
		c.MySQL.DSN = val
	}

	// Server
	if val := getEnv("GAME_HOST"); val != "" {
		c.Host = val
	}
	if val := getEnv("GAME_PORT"); val != "" {
		fmt.Sscanf(val, "%d", &c.Port)
	}

	// Room
	if val := getEnv("GAME_ROOM_MAXROOMS"); val != "" {
		fmt.Sscanf(val, "%d", &c.Room.MaxRooms)
	}

	// Log
	if val := getEnv("GAME_LOG_LEVEL"); val != "" {
		c.Log.Level = val
	}
	if val := getEnv("GAME_LOG_MODE"); val != "" {
		c.Log.Mode = val
	}
	if val := getEnv("GAME_LOG_PATH"); val != "" {
		c.Log.Path = val
	}
}

// getEnv 获取环境变量
func getEnv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

// Validate 验证配置
func (c *Config) Validate() error {
	// 验证端口范围
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}

	// 验证房间配置
	if c.Room.MaxRooms <= 0 {
		return fmt.Errorf("invalid MaxRooms: %d", c.Room.MaxRooms)
	}
	if c.Room.MaxPlayersPerRoom <= 0 {
		return fmt.Errorf("invalid MaxPlayersPerRoom: %d", c.Room.MaxPlayersPerRoom)
	}
	if c.Room.ReadyTimeout <= 0 {
		return fmt.Errorf("invalid ReadyTimeout: %d", c.Room.ReadyTimeout)
	}
	if c.Room.PlayTimeout <= 0 {
		return fmt.Errorf("invalid PlayTimeout: %d", c.Room.PlayTimeout)
	}

	// 验证日志级别
	validLogLevels := []string{"debug", "info", "warn", "error", "panic", "fatal"}
	isValidLevel := false
	for _, level := range validLogLevels {
		if strings.EqualFold(c.Log.Level, level) {
			isValidLevel = true
			break
		}
	}
	if !isValidLevel {
		return fmt.Errorf("invalid log level: %s", c.Log.Level)
	}

	// 验证日志模式
	if c.Log.Mode != "console" && c.Log.Mode != "file" {
		return fmt.Errorf("invalid log mode: %s (must be 'console' or 'file')", c.Log.Mode)
	}

	// 如果是文件模式，检查路径
	if c.Log.Mode == "file" && c.Log.Path == "" {
		return fmt.Errorf("log path is required when mode is 'file'")
	}

	// 验证 AI 难度
	validDifficulties := []string{"easy", "normal", "hard"}
	isValidDifficulty := false
	for _, diff := range validDifficulties {
		if strings.EqualFold(c.AI.DefaultDifficulty, diff) {
			isValidDifficulty = true
			break
		}
	}
	if !isValidDifficulty {
		return fmt.Errorf("invalid AI difficulty: %s (must be 'easy', 'normal', or 'hard')", c.AI.DefaultDifficulty)
	}

	// 验证匹配配置
	if c.Match.ScanInterval <= 0 {
		return fmt.Errorf("invalid Match.ScanInterval: %d", c.Match.ScanInterval)
	}

	return nil
}

// GetAddr 获取服务地址
func (c *Config) GetAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// IsClusterEnabled 检查是否启用集群模式
func (c *Config) IsClusterEnabled() bool {
	return c.Cluster.Enabled
}

// IsRedisEnabled 检查是否配置了 Redis
func (c *Config) IsRedisEnabled() bool {
	return c.Redis.Host != ""
}

// IsMySQLEnabled 检查是否配置了 MySQL
func (c *Config) IsMySQLEnabled() bool {
	return c.MySQL.DSN != ""
}
