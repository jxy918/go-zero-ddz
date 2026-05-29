// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package logic

import (
	"context"
	"database/sql"
	"errors"

	"go-zero-ddz/app/user/internal/svc"
	"go-zero-ddz/app/user/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UserInfoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取用户信息
func NewUserInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UserInfoLogic {
	return &UserInfoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UserInfoLogic) UserInfo() (resp *types.UserInfoResp, err error) {
	// 从 JWT context 中提取 UID
	uid, err := l.extractUID()
	if err != nil {
		return nil, errors.New("unauthorized")
	}

	// 查询用户信息
	user, err := l.svcCtx.GetUserByUID(uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("user not found")
		}
		l.Errorf("query user failed: %v", err)
		return nil, errors.New("query failed")
	}

	return &types.UserInfoResp{
		Uid:      user.UID,
		Nickname: user.Nickname,
		AvatarId: user.AvatarID,
		Elo:      user.ELO,
		Tier:     user.Tier,
		Gold:     user.Gold,
		Wins:     user.Wins,
		Losses:   user.Losses,
	}, nil
}

func (l *UserInfoLogic) extractUID() (string, error) {
	// go-zero JWT middleware 将 claims 直接添加到 context
	uid, ok := l.ctx.Value("uid").(string)
	if !ok || uid == "" {
		return "", errors.New("missing UID in context")
	}
	return uid, nil
}
