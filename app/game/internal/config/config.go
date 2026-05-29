package config

import (
	"github.com/zeromicro/go-zero/core/conf"
)

type Config struct {
	Name        string          `json:",default=game-service"`
	Host        string          `json:",default=0.0.0.0"`
	Port        int             `json:",default=8080"`
	InstanceId  string          `json:",optional"`
	WebStaticDir string         `json:",default=./client-web,optional"`
	Cluster     ClusterConfig   `json:",optional"`
	Redis       RedisConfig     `json:",optional"`
	MySQL       MySQLConfig     `json:",optional"`
	WebSocket   WebSocketConfig `json:",optional"`
	Room        RoomConfig      `json:",optional"`
	Match       MatchConfig     `json:",optional"`
	AI          AIConfig        `json:",optional"`
	Log         LogConfig       `json:",optional"`
	Metrics     MetricsConfig   `json:",optional"`
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
	MaxRooms         int `json:",default=1000"`
	MaxPlayersPerRoom int `json:",default=3"`
	ReadyTimeout     int `json:",default=60"`
	PlayTimeout      int `json:",default=15"`
	ReconnectTimeout int `json:",default=300"`
	SnapshotInterval int `json:",default=30"`
	BotJoinTimeout   int `json:",default=60"`
}

type MatchConfig struct {
	Enabled       bool `json:",default=true"`
	ScanInterval  int  `json:",default=2"`
	RandomTimeout int  `json:",default=15"`
	RankedTimeout int  `json:",default=30"`
	EloRange      int  `json:",default=100"`
	BotFillTimeout int `json:",default=30"`
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

// LoadConfig 从 YAML 文件加载配置
func LoadConfig(path string) (*Config, error) {
	var c Config
	err := conf.Load(path, &c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}
