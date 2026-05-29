package types

// MessageID 消息ID常量定义
const (
	// 系统消息 (0x00xx)
	MsgHeartbeatReq  uint16 = 0x0001
	MsgHeartbeatResp uint16 = 0x0002
	MsgErrorResponse uint16 = 0x0003

	// 认证消息 (0x01xx)
	MsgLoginReq  uint16 = 0x0101
	MsgLoginResp uint16 = 0x0102

	// 房间消息 (0x02xx)
	MsgCreateRoomReq   uint16 = 0x0201
	MsgCreateRoomResp  uint16 = 0x0202
	MsgJoinRoomReq     uint16 = 0x0203
	MsgJoinRoomResp    uint16 = 0x0204
	MsgRoomStateNotify uint16 = 0x0206
	MsgPlayerReadyReq  uint16 = 0x0207

	// 匹配消息 (0x03xx)
	MsgMatchStartReq      uint16 = 0x0301
	MsgMatchCancelReq     uint16 = 0x0302
	MsgMatchSuccessNotify uint16 = 0x0303

	// 游戏消息 (0x04xx)
	MsgDealCardsNotify    uint16 = 0x0401
	MsgCallLandlordReq    uint16 = 0x0402
	MsgCallLandlordNotify uint16 = 0x0403
	MsgPlayCardsReq       uint16 = 0x0404
	MsgPlayCardsNotify    uint16 = 0x0405
	MsgPassNotify         uint16 = 0x0406
	MsgGameEndNotify      uint16 = 0x0407
	MsgTimerNotify        uint16 = 0x0408
	MsgCancelAIControlReq uint16 = 0x0409

	// 重连消息 (0x05xx)
	MsgReconnectReq  uint16 = 0x0501
	MsgReconnectResp uint16 = 0x0502
)
