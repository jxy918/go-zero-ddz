package errors

import (
	"fmt"
)

// ErrorCode 错误码类型
type ErrorCode int

// 错误码定义
const (
	// 系统错误 0x00xx
	ErrCodeSystemError   ErrorCode = 0x0001
	ErrCodeRedisError    ErrorCode = 0x0002
	ErrCodeDatabaseError ErrorCode = 0x0003
	ErrCodeConfigError   ErrorCode = 0x0004

	// 认证错误 0x01xx
	ErrCodeAuthFailed      ErrorCode = 0x0101
	ErrCodeTokenInvalid    ErrorCode = 0x0102
	ErrCodeTokenExpired    ErrorCode = 0x0103
	ErrCodeUserNotFound    ErrorCode = 0x0104
	ErrCodeInvalidPassword ErrorCode = 0x0105

	// 房间错误 0x02xx
	ErrCodeRoomNotFound     ErrorCode = 0x0201
	ErrCodeRoomFull         ErrorCode = 0x0202
	ErrCodeRoomExists       ErrorCode = 0x0203
	ErrCodeRoomNotReady     ErrorCode = 0x0204
	ErrCodeRoomAlreadyStart ErrorCode = 0x0205

	// 玩家错误 0x021x
	ErrCodePlayerNotFound  ErrorCode = 0x0211
	ErrCodePlayerExists    ErrorCode = 0x0212
	ErrCodePlayerNotInRoom ErrorCode = 0x0213
	ErrCodePlayerNotReady  ErrorCode = 0x0214
	ErrCodePlayerOffline   ErrorCode = 0x0215

	// 匹配错误 0x03xx
	ErrCodeMatchDisabled ErrorCode = 0x0301
	ErrCodeMatchTimeout  ErrorCode = 0x0302
	ErrCodeMatchFailed   ErrorCode = 0x0303
	ErrCodePlayerInRoom  ErrorCode = 0x0304

	// 游戏错误 0x04xx
	ErrCodeGameNotStarted   ErrorCode = 0x0401
	ErrCodeInvalidState     ErrorCode = 0x0402
	ErrCodeNotYourTurn      ErrorCode = 0x0403
	ErrCodeInvalidCards     ErrorCode = 0x0404
	ErrCodeCannotPass       ErrorCode = 0x0405
	ErrCodeCannotBeat       ErrorCode = 0x0406
	ErrCodeLandlordNotFound ErrorCode = 0x0407
	ErrCodeNotEnoughPlayers ErrorCode = 0x0408

	// 重连错误 0x05xx
	ErrCodeReconnectTimeout ErrorCode = 0x0501
	ErrCodeSessionExpired   ErrorCode = 0x0502
	ErrCodeRoomDestroyed    ErrorCode = 0x0503
)

// 错误消息映射
var errorMessages = map[ErrorCode]string{
	// 系统错误
	ErrCodeSystemError:   "系统错误",
	ErrCodeRedisError:    "Redis 操作失败",
	ErrCodeDatabaseError: "数据库操作失败",
	ErrCodeConfigError:   "配置错误",

	// 认证错误
	ErrCodeAuthFailed:      "认证失败",
	ErrCodeTokenInvalid:    "Token 无效",
	ErrCodeTokenExpired:    "Token 已过期",
	ErrCodeUserNotFound:    "用户不存在",
	ErrCodeInvalidPassword: "密码错误",

	// 房间错误
	ErrCodeRoomNotFound:     "房间不存在",
	ErrCodeRoomFull:         "房间已满",
	ErrCodeRoomExists:       "房间已存在",
	ErrCodeRoomNotReady:     "房间未准备就绪",
	ErrCodeRoomAlreadyStart: "房间已开始游戏",

	// 玩家错误
	ErrCodePlayerNotFound:  "玩家不存在",
	ErrCodePlayerExists:    "玩家已在房间中",
	ErrCodePlayerNotInRoom: "玩家不在房间中",
	ErrCodePlayerNotReady:  "玩家未准备",
	ErrCodePlayerOffline:   "玩家离线",

	// 匹配错误
	ErrCodeMatchDisabled: "匹配功能未启用",
	ErrCodeMatchTimeout:  "匹配超时",
	ErrCodeMatchFailed:   "匹配失败",
	ErrCodePlayerInRoom:  "玩家已在房间中",

	// 游戏错误
	ErrCodeGameNotStarted:   "游戏未开始",
	ErrCodeInvalidState:     "无效的游戏状态",
	ErrCodeNotYourTurn:      "不是你的回合",
	ErrCodeInvalidCards:     "无效的牌型",
	ErrCodeCannotPass:       "不能 PASS",
	ErrCodeCannotBeat:       "无法管上",
	ErrCodeLandlordNotFound: "地主不存在",
	ErrCodeNotEnoughPlayers: "玩家数量不足",

	// 重连错误
	ErrCodeReconnectTimeout: "重连超时",
	ErrCodeSessionExpired:   "会话已过期",
	ErrCodeRoomDestroyed:    "房间已销毁",
}

// GameError 游戏错误
type GameError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Detail  string    `json:"detail,omitempty"`
}

// New 创建错误
func New(code ErrorCode) *GameError {
	return &GameError{
		Code:    code,
		Message: errorMessages[code],
	}
}

// NewWithDetail 创建带详情的错误
func NewWithDetail(code ErrorCode, detail string) *GameError {
	return &GameError{
		Code:    code,
		Message: errorMessages[code],
		Detail:  detail,
	}
}

// Error 实现 error 接口
func (e *GameError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("[%04X] %s: %s", e.Code, e.Message, e.Detail)
	}
	return fmt.Sprintf("[%04X] %s", e.Code, e.Message)
}

// Wrap 包装错误
func Wrap(err error, code ErrorCode) *GameError {
	return &GameError{
		Code:    code,
		Message: errorMessages[code],
		Detail:  err.Error(),
	}
}

// Wrapf 包装错误（带格式化）
func Wrapf(err error, code ErrorCode, format string, args ...interface{}) *GameError {
	return &GameError{
		Code:    code,
		Message: errorMessages[code],
		Detail:  fmt.Sprintf(format, args...) + ": " + err.Error(),
	}
}

// IsGameError 判断是否为 GameError
func IsGameError(err error) bool {
	_, ok := err.(*GameError)
	return ok
}

// GetGameError 获取 GameError
func GetGameError(err error) (*GameError, bool) {
	e, ok := err.(*GameError)
	return e, ok
}

// GetCode 获取错误码
func GetCode(err error) ErrorCode {
	if e, ok := err.(*GameError); ok {
		return e.Code
	}
	return ErrCodeSystemError
}

// Predefined errors for easy use
var (
	ErrSystemError   = New(ErrCodeSystemError)
	ErrRedisError    = New(ErrCodeRedisError)
	ErrDatabaseError = New(ErrCodeDatabaseError)
	ErrConfigError   = New(ErrCodeConfigError)

	ErrAuthFailed      = New(ErrCodeAuthFailed)
	ErrTokenInvalid    = New(ErrCodeTokenInvalid)
	ErrTokenExpired    = New(ErrCodeTokenExpired)
	ErrUserNotFound    = New(ErrCodeUserNotFound)
	ErrInvalidPassword = New(ErrCodeInvalidPassword)

	ErrRoomNotFound     = New(ErrCodeRoomNotFound)
	ErrRoomFull         = New(ErrCodeRoomFull)
	ErrRoomExists       = New(ErrCodeRoomExists)
	ErrRoomNotReady     = New(ErrCodeRoomNotReady)
	ErrRoomAlreadyStart = New(ErrCodeRoomAlreadyStart)

	ErrPlayerNotFound  = New(ErrCodePlayerNotFound)
	ErrPlayerExists    = New(ErrCodePlayerExists)
	ErrPlayerNotInRoom = New(ErrCodePlayerNotInRoom)
	ErrPlayerNotReady  = New(ErrCodePlayerNotReady)
	ErrPlayerOffline   = New(ErrCodePlayerOffline)

	ErrMatchDisabled = New(ErrCodeMatchDisabled)
	ErrMatchTimeout  = New(ErrCodeMatchTimeout)
	ErrMatchFailed   = New(ErrCodeMatchFailed)
	ErrPlayerInRoom  = New(ErrCodePlayerInRoom)

	ErrGameNotStarted   = New(ErrCodeGameNotStarted)
	ErrInvalidState     = New(ErrCodeInvalidState)
	ErrNotYourTurn      = New(ErrCodeNotYourTurn)
	ErrInvalidCards     = New(ErrCodeInvalidCards)
	ErrCannotPass       = New(ErrCodeCannotPass)
	ErrCannotBeat       = New(ErrCodeCannotBeat)
	ErrLandlordNotFound = New(ErrCodeLandlordNotFound)
	ErrNotEnoughPlayers = New(ErrCodeNotEnoughPlayers)

	ErrReconnectTimeout = New(ErrCodeReconnectTimeout)
	ErrSessionExpired   = New(ErrCodeSessionExpired)
	ErrRoomDestroyed    = New(ErrCodeRoomDestroyed)
)
