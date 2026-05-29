import { _decorator, Component, Node } from 'cc';
const { ccclass, property } = _decorator;

export interface Message {
    msgId: number;
    data: any;
}

@ccclass('WebSocketManager')
export class WebSocketManager extends Component {
    private static _instance: WebSocketManager | null = null;
    private ws: WebSocket | null = null;
    private callbacks: Map<number, ((data: any) => void)[]> = new Map();
    private reconnectAttempts = 0;
    private maxReconnectAttempts = 5;
    private reconnectDelay = 3000;

    public static get instance(): WebSocketManager {
        if (!this._instance) {
            this._instance = new WebSocketManager();
        }
        return this._instance;
    }

    private constructor() { }

    connect(url: string): Promise<void> {
        return new Promise((resolve, reject) => {
            this.ws = new WebSocket(url);
            this.ws.binaryType = 'arraybuffer';

            this.ws.onopen = () => {
                console.log('[WS] Connected');
                this.reconnectAttempts = 0;
                resolve();
            };

            this.ws.onclose = (event) => {
                console.log('[WS] Disconnected:', event.code, event.reason);
                this.attemptReconnect(url);
            };

            this.ws.onerror = (error) => {
                console.error('[WS] Error:', error);
                reject(error);
            };

            this.ws.onmessage = (event) => {
                this.handleMessage(event);
            };
        });
    }

    private attemptReconnect(url: string) {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.error('[WS] Max reconnect attempts reached');
            return;
        }

        this.reconnectAttempts++;
        console.log(`[WS] Reconnecting attempt ${this.reconnectAttempts}...`);
        
        setTimeout(() => {
            this.connect(url).catch(() => {});
        }, this.reconnectDelay * this.reconnectAttempts);
    }

    private handleMessage(event: MessageEvent) {
        try {
            const buffer = event.data as ArrayBuffer;
            if (buffer.byteLength < 6) {
                console.error('[WS] Message too short');
                return;
            }

            const view = new DataView(buffer);
            const length = view.getUint32(0, false);
            const msgId = view.getUint16(4, false);
            const payload = buffer.slice(6, 6 + length);

            const decoder = new TextDecoder('utf-8');
            const data = JSON.parse(decoder.decode(payload));

            console.log(`[WS] Received msgId: 0x${msgId.toString(16)}, data:`, data);

            const handlers = this.callbacks.get(msgId);
            if (handlers) {
                handlers.forEach(handler => handler(data));
            }
        } catch (error) {
            console.error('[WS] Failed to decode message:', error);
        }
    }

    send(msgId: number, data: any): void {
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
            console.error('[WS] Not connected');
            return;
        }

        try {
            const payload = JSON.stringify(data);
            const payloadBytes = new TextEncoder().encode(payload);
            const frame = new ArrayBuffer(4 + 2 + payloadBytes.length);
            const view = new DataView(frame);
            
            view.setUint32(0, 2 + payloadBytes.length, false);
            view.setUint16(4, msgId, false);
            new Uint8Array(frame, 6).set(payloadBytes);

            this.ws.send(frame);
            console.log(`[WS] Sent msgId: 0x${msgId.toString(16)}, data:`, data);
        } catch (error) {
            console.error('[WS] Failed to send message:', error);
        }
    }

    on(msgId: number, callback: (data: any) => void): void {
        const handlers = this.callbacks.get(msgId) || [];
        handlers.push(callback);
        this.callbacks.set(msgId, handlers);
    }

    off(msgId: number, callback?: (data: any) => void): void {
        const handlers = this.callbacks.get(msgId);
        if (!handlers) return;

        if (callback) {
            const index = handlers.indexOf(callback);
            if (index !== -1) {
                handlers.splice(index, 1);
            }
        } else {
            this.callbacks.delete(msgId);
        }
    }

    disconnect(): void {
        if (this.ws) {
            this.ws.close();
            this.ws = null;
        }
    }

    get connected(): boolean {
        return this.ws !== null && this.ws.readyState === WebSocket.OPEN;
    }
}