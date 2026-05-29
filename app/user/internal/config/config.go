package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf
	Auth  AuthConfig
	Redis RedisConfig
	MySQL MySQLConfig
}

type AuthConfig struct {
	AccessSecret string `json:",default=ddz-secret-key-2025"`
	AccessExpire int64  `json:",default=86400"`
}

type RedisConfig struct {
	Host string `json:",default=localhost:6379"`
	Type string `json:",default=node"`
	Pass string `json:",optional"`
}

type MySQLConfig struct {
	DSN string `json:",optional"`
}
