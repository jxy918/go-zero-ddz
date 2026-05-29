// WebSocket 连接管理模块
const WS = {
    ws: null,
    url: 'ws://localhost:8080/ws',
    connected: false,
    uid: null,
    roomId: null,
    handlers: {},
    reconnectAttempts: 0,
    maxReconnectAttempts: 5,
    heartbeatTimer: null,
    token: null,
    onReconnectSuccess: null,

    // 消息 ID 常量
    MSG: {
        HEARTBEAT_REQ: 0x0001,
        HEARTBEAT_RESP: 0x0002,
        ERROR: 0x0003,
        LOGIN_REQ: 0x0101,
        LOGIN_RESP: 0x0102,
        CREATE_ROOM_REQ: 0x0201,
        CREATE_ROOM_RESP: 0x0202,
        JOIN_ROOM_REQ: 0x0203,
        JOIN_ROOM_RESP: 0x0204,
        ROOM_STATE_NOTIFY: 0x0206,
        PLAYER_READY_REQ: 0x0207,
        MATCH_START_REQ: 0x0301,
        MATCH_SUCCESS_NOTIFY: 0x0303,
        DEAL_CARDS_NOTIFY: 0x0401,
        CALL_LANDLORD_REQ: 0x0402,
        CALL_LANDLORD_NOTIFY: 0x0403,
        PLAY_CARDS_REQ: 0x0404,
        PLAY_CARDS_NOTIFY: 0x0405,
        PASS_NOTIFY: 0x0406,
        GAME_END_NOTIFY: 0x0407,
        TIMER_NOTIFY: 0x0408,
        CANCEL_AI_CONTROL_REQ: 0x0409,
        RECONNECT_REQ: 0x0501,
        RECONNECT_RESP: 0x0502
    },

    // 连接 WebSocket
    connect(token) {
        this.token = token; // 保存token用于重连
        return new Promise((resolve, reject) => {
            const url = `${this.url}?token=${encodeURIComponent(token)}`;
            this.ws = new WebSocket(url);
            this.ws.binaryType = 'arraybuffer';

            this.ws.onopen = () => {
                console.log('[WS] Connected');
                this.connected = true;
                this.reconnectAttempts = 0;
                this.startHeartbeat();
                
                // 如果是重连，触发重连成功回调
                if (this.onReconnectSuccess) {
                    this.onReconnectSuccess();
                }
                
                resolve();
            };

            this.ws.onmessage = (event) => {
                this.handleMessage(event.data);
            };

            this.ws.onclose = (event) => {
                console.log(`[WS] Closed: ${event.code} ${event.reason}`);
                this.connected = false;
                this.stopHeartbeat();
                this.onDisconnect?.(event.code, event.reason);
                
                if (this.reconnectAttempts < this.maxReconnectAttempts) {
                    this.scheduleReconnect();
                }
            };

            this.ws.onerror = (error) => {
                console.error('[WS] Error:', error);
                reject(error);
            };
        });
    },

    // 发送消息
    send(msgId, data) {
        if (!this.connected || !this.ws) {
            console.warn('[WS] Not connected');
            return;
        }

        const payload = typeof data === 'string' ? data : JSON.stringify(data);
        const payloadBytes = new TextEncoder().encode(payload);
        
        // 构建消息帧: [Length 4B][MsgID 2B][Payload]
        const frame = new ArrayBuffer(4 + 2 + payloadBytes.length);
        const view = new DataView(frame);
        
        // Length (big-endian)
        view.setUint32(0, 2 + payloadBytes.length, false);
        // MsgID (big-endian)
        view.setUint16(4, msgId, false);
        // Payload
        new Uint8Array(frame, 6).set(payloadBytes);
        
        console.log(`[WS] Sending msgId=0x${msgId.toString(16).padStart(4, '0')}, payload=${payload}`);
        this.ws.send(frame);
        console.log('[WS] Message sent successfully');
    },

    // 处理接收消息
    handleMessage(data) {
        try {
            const buffer = new Uint8Array(data);
            let offset = 0;
            
            while (offset < buffer.length) {
                // 解析消息长度 (4 bytes)
                if (offset + 4 > buffer.length) break;
                const view = new DataView(buffer.buffer, buffer.byteOffset + offset);
                const msgLength = view.getUint32(0, false);
                
                // 解析消息 ID (2 bytes, at offset+4)
                if (offset + 6 > buffer.length) break;
                const msgId = view.getUint16(4, false);
                
                // 解析 payload (msgLength - 2 bytes, starting at offset+6)
                const payloadBytes = buffer.slice(offset + 6, offset + 4 + msgLength);
                const payloadStr = new TextDecoder().decode(payloadBytes);
                const payload = JSON.parse(payloadStr);
                
                console.log(`[WS] Received msgId=0x${msgId.toString(16).padStart(4, '0')}`, payload);
                
                // 触发对应处理器
                if (this.handlers[msgId]) {
                    this.handlers[msgId](payload);
                }
                
                // 触发全局处理器
                this.onMessage?.(msgId, payload);
                
                // 移动偏移量: 4 (Length) + msgLength (MsgID + Payload)
                offset += 4 + msgLength;
            }
        } catch (e) {
            console.error('[WS] Failed to decode message:', e);
        }
    },

    // 注册消息处理器
    on(msgId, handler) {
        this.handlers[msgId] = handler;
    },

    // 取消消息处理器
    off(msgId) {
        delete this.handlers[msgId];
    },

    // 开始心跳
    startHeartbeat() {
        this.heartbeatTimer = setInterval(() => {
            this.send(this.MSG.HEARTBEAT_REQ, {
                client_timestamp: Date.now()
            });
        }, 30000);
    },

    // 停止心跳
    stopHeartbeat() {
        if (this.heartbeatTimer) {
            clearInterval(this.heartbeatTimer);
            this.heartbeatTimer = null;
        }
    },

    // 调度重连
    scheduleReconnect() {
        this.reconnectAttempts++;
        const delay = 1000 * Math.pow(2, this.reconnectAttempts - 1);
        console.log(`[WS] Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`);
        
        setTimeout(() => {
            this.connect(this.token).catch(() => {});
        }, delay);
    },

    // 断开连接
    disconnect() {
        this.stopHeartbeat();
        this.reconnectAttempts = this.maxReconnectAttempts;
        if (this.ws) {
            this.ws.close(1000, 'Client disconnect');
        }
        this.connected = false;
    },

    // 回调函数
    onMessage: null,
    onDisconnect: null
};
