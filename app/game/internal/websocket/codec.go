package websocket

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	// HeaderSize = 4 bytes (Length) + 2 bytes (MsgID)
	HeaderSize = 6
	// MaxMessageSize 最大消息大小 64KB
	MaxMessageSize = 64 * 1024
)

// Encode 编码消息帧
// 格式: [Length(4B)][MsgID(2B)][Payload(NB)]
// Length 不包含自身，仅包含 MsgID + Payload
func Encode(msgID uint16, payload []byte) ([]byte, error) {
	if len(payload) > MaxMessageSize {
		return nil, fmt.Errorf("payload too large: %d bytes", len(payload))
	}

	totalLen := 2 + len(payload) // MsgID + Payload
	buf := make([]byte, HeaderSize+len(payload))

	// Length (4 bytes, big-endian)
	binary.BigEndian.PutUint32(buf[0:4], uint32(totalLen))

	// MsgID (2 bytes, big-endian)
	binary.BigEndian.PutUint16(buf[4:6], msgID)

	// Payload
	copy(buf[6:], payload)

	return buf, nil
}

// Decode 从 io.Reader 解码消息帧（处理粘包/半包）
func Decode(reader io.Reader) (uint16, []byte, error) {
	// 读取 Length (4 bytes)
	var lengthBuf [4]byte
	if _, err := io.ReadFull(reader, lengthBuf[:]); err != nil {
		return 0, nil, fmt.Errorf("read length: %w", err)
	}

	length := binary.BigEndian.Uint32(lengthBuf[:])
	if length > MaxMessageSize {
		return 0, nil, fmt.Errorf("message too large: %d bytes", length)
	}

	// 读取剩余数据 (MsgID + Payload)
	frameData := make([]byte, length)
	if _, err := io.ReadFull(reader, frameData); err != nil {
		return 0, nil, fmt.Errorf("read frame: %w", err)
	}

	// 解析 MsgID
	msgID := binary.BigEndian.Uint16(frameData[0:2])

	// 提取 Payload
	payload := frameData[2:]

	return msgID, payload, nil
}

// DecodeFromBytes 从完整字节切片解码（用于 WebSocket 已接收完整消息）
func DecodeFromBytes(data []byte) (uint16, []byte, error) {
	if len(data) < 4 {
		return 0, nil, fmt.Errorf("data too short: %d bytes", len(data))
	}

	length := binary.BigEndian.Uint32(data[0:4])
	if int(length) != len(data)-4 {
		return 0, nil, fmt.Errorf("length mismatch: header says %d, got %d", length, len(data)-4)
	}

	if len(data) < HeaderSize {
		return 0, nil, fmt.Errorf("frame too short for header: %d bytes", len(data))
	}

	msgID := binary.BigEndian.Uint16(data[4:6])
	payload := data[6:]

	return msgID, payload, nil
}
