package svc

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"go-zero-ddz/app/user/internal/config"

	_ "github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/logx"
)

// User 用户模型
type User struct {
	UID        string    `db:"uid"`
	Username   string    `db:"username"`
	Password   string    `db:"password"`
	Nickname   string    `db:"nickname"`
	AvatarID   uint32    `db:"avatar_id"`
	ELO        int32     `db:"elo"`
	Tier       string    `db:"tier"`
	Gold       int32     `db:"gold"`
	Wins       int32     `db:"wins"`
	Losses     int32     `db:"losses"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

type ServiceContext struct {
	Config config.Config
	DB     *sql.DB
}

func NewServiceContext(c config.Config) *ServiceContext {
	var db *sql.DB

	if c.MySQL.DSN != "" {
		var err error
		db, err = sql.Open("mysql", c.MySQL.DSN)
		if err != nil {
			logx.Errorf("open database: %v", err)
			db = nil
		} else {
			// 配置连接池
			db.SetMaxOpenConns(25)
			db.SetMaxIdleConns(10)
			db.SetConnMaxLifetime(5 * time.Minute)

			if err := db.Ping(); err != nil {
				logx.Errorf("MySQL ping failed: %v (running in memory mode)", err)
				db = nil
			}
		}
	} else {
		logx.Info("MySQL DSN not configured, running in memory mode")
	}

	return &ServiceContext{
		Config: c,
		DB:     db,
	}
}

// GetUserByUsername 根据用户名查询用户
func (s *ServiceContext) GetUserByUsername(username string) (*User, error) {
	if s.DB == nil {
		// 内存模式：返回模拟用户（仅用于开发测试）
		if username == "test" {
			return &User{
				UID:      "test-uid-001",
				Username: "test",
				Password: "a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3", // "123" SHA256
				Nickname: "TestPlayer",
				AvatarID: 1,
				ELO:      1000,
				Tier:     "Bronze I",
				Gold:     5000,
				Wins:     0,
				Losses:   0,
			}, nil
		}
		return nil, sql.ErrNoRows
	}

	var user User
	query := `SELECT uid, username, password, nickname, avatar_id, elo, tier, gold, wins, losses, created_at, updated_at 
			  FROM users WHERE username = ? LIMIT 1`
	err := s.DB.QueryRowContext(context.Background(), query, username).Scan(
		&user.UID, &user.Username, &user.Password, &user.Nickname,
		&user.AvatarID, &user.ELO, &user.Tier, &user.Gold,
		&user.Wins, &user.Losses, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByUID 根据 UID 查询用户
func (s *ServiceContext) GetUserByUID(uid string) (*User, error) {
	if s.DB == nil {
		// 内存模式
		if uid == "test-uid-001" {
			return &User{
				UID:      "test-uid-001",
				Username: "test",
				Password: "",
				Nickname: "TestPlayer",
				AvatarID: 1,
				ELO:      1000,
				Tier:     "Bronze I",
				Gold:     5000,
				Wins:     0,
				Losses:   0,
			}, nil
		}
		return nil, sql.ErrNoRows
	}

	var user User
	query := `SELECT uid, username, password, nickname, avatar_id, elo, tier, gold, wins, losses, created_at, updated_at 
			  FROM users WHERE uid = ? LIMIT 1`
	err := s.DB.QueryRowContext(context.Background(), query, uid).Scan(
		&user.UID, &user.Username, &user.Password, &user.Nickname,
		&user.AvatarID, &user.ELO, &user.Tier, &user.Gold,
		&user.Wins, &user.Losses, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// CreateUser 创建用户
func (s *ServiceContext) CreateUser(user *User) error {
	if s.DB == nil {
	// 内存模式：不持久化，仅返回成功
	logx.Infof("[Memory Mode] User created: %s (%s)", user.UID, user.Username)
		return nil
	}

	query := `INSERT INTO users (uid, username, password, nickname, avatar_id, elo, tier, gold, wins, losses, created_at, updated_at)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.DB.ExecContext(context.Background(), query,
		user.UID, user.Username, user.Password, user.Nickname,
		user.AvatarID, user.ELO, user.Tier, user.Gold,
		user.Wins, user.Losses, user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("execute insert: %w", err)
	}
	return nil
}

// UpdateUserStats 更新用户统计（胜负、ELO）
func (s *ServiceContext) UpdateUserStats(uid string, elo int32, tier string, wins, losses int32) error {
	if s.DB == nil {
		logx.Infof("[Memory Mode] User stats updated: %s ELO=%d", uid, elo)
		return nil
	}

	query := `UPDATE users SET elo = ?, tier = ?, wins = ?, losses = ?, updated_at = ? WHERE uid = ?`
	_, err := s.DB.ExecContext(context.Background(), query,
		elo, tier, wins, losses, time.Now(), uid,
	)
	if err != nil {
		return fmt.Errorf("execute update: %w", err)
	}
	return nil
}

// HashPassword 使用 SHA256 哈希密码（生产环境应使用 bcrypt）
func HashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}
