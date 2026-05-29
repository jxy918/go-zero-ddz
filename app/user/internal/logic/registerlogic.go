// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package logic

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go-zero-ddz/app/user/internal/svc"
	"go-zero-ddz/app/user/internal/types"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
)

type RegisterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 用户注册
func NewRegisterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterLogic {
	return &RegisterLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RegisterLogic) Register(req *types.RegisterReq) (resp *types.RegisterResp, err error) {
	// 检查用户名是否已存在
	existing, err := l.svcCtx.GetUserByUsername(req.Username)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("database error")
	}
	if existing != nil {
		return nil, errors.New("username already exists")
	}

	// 创建新用户
	uid := uuid.New().String()
	now := time.Now()
	nickname := req.Nickname
	if nickname == "" {
		nickname = "Player_" + uid[:4]
	}

	user := &svc.User{
		UID:       uid,
		Username:  req.Username,
		Password:  svc.HashPassword(req.Password),
		Nickname:  nickname,
		AvatarID:  1,
		ELO:       1000,
		Tier:      "Bronze I",
		Gold:      5000,
		Wins:      0,
		Losses:    0,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := l.svcCtx.CreateUser(user); err != nil {
		l.Errorf("create user failed: %v", err)
		return nil, errors.New("registration failed")
	}

	// 生成 JWT token
	token, err := l.generateToken(user.UID, user.Username)
	if err != nil {
		l.Errorf("generate token failed: %v", err)
		return nil, errors.New("registration failed")
	}

	return &types.RegisterResp{
		Uid:   user.UID,
		Token: token,
	}, nil
}

func (l *RegisterLogic) generateToken(uid, username string) (string, error) {
	now := time.Now().Unix()
	claims := jwt.MapClaims{
		"uid":      uid,
		"username": username,
		"iat":      now,
		"exp":      now + l.svcCtx.Config.Auth.AccessExpire,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(l.svcCtx.Config.Auth.AccessSecret))
}
