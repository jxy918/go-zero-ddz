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
	"github.com/zeromicro/go-zero/core/logx"
)

type LoginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 用户登录
func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LoginLogic) Login(req *types.LoginReq) (resp *types.LoginResp, err error) {
	// 查询用户
	user, err := l.svcCtx.GetUserByUsername(req.Username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("user not found")
		}
		l.Errorf("query user failed: %v", err)
		return nil, errors.New("login failed")
	}

	// 验证密码
	expectedHash := svc.HashPassword(req.Password)
	if user.Password != expectedHash {
		return nil, errors.New("wrong password")
	}

	// 生成 JWT token
	token, err := l.generateToken(user.UID, user.Username)
	if err != nil {
		l.Errorf("generate token failed: %v", err)
		return nil, errors.New("login failed")
	}

	return &types.LoginResp{
		Uid:      user.UID,
		Token:    token,
		Nickname: user.Nickname,
		AvatarId: user.AvatarID,
		Elo:      user.ELO,
		Tier:     user.Tier,
		Gold:     user.Gold,
	}, nil
}

func (l *LoginLogic) generateToken(uid, username string) (string, error) {
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
