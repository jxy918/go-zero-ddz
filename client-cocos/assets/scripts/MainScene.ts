import { _decorator, Component, Node, Scene, Button, EditBox } from 'cc';
const { ccclass, property } = _decorator;
import { WebSocketManager } from './WebSocketManager';
import { MSG_ID } from './GameConstants';
import { GameState } from './GameState';
import { LoginPanel } from './LoginPanel';
import { GamePanel } from './GamePanel';

@ccclass('MainScene')
export class MainScene extends Component {
    @property(Node)
    loginPanel: Node = null!;
    
    @property(Node)
    lobbyPanel: Node = null!;
    
    @property(Node)
    gamePanel: Node = null!;
    
    @property(LoginPanel)
    loginComponent: LoginPanel = null!;
    
    @property(GamePanel)
    gameComponent: GamePanel = null!;
    
    private ws: WebSocketManager = WebSocketManager.instance;
    private gameState: GameState = GameState.instance;
    
    onLoad() {
        this.loginComponent.onLoginCallback = this.handleLogin.bind(this);
        
        this.setupLobbyButtons();
        
        this.showPanel('login');
    }
    
    handleLogin(data: any) {
        console.log('Login success:', data);
        console.log('Login data.uid:', data.uid, 'data.token:', data.token ? 'present' : 'missing');
        
        this.gameState.init(data.uid, data.nickname);
        console.log('GameState myUid after init:', this.gameState.myUid);
        
        this.connectWebSocket(data.token).then(() => {
            this.ws.send(MSG_ID.LOGIN_REQ, { token: data.token });
            this.showPanel('lobby');
        }).catch((error) => {
            console.error('WebSocket connection failed:', error);
        });
    }
    
    async connectWebSocket(token: string): Promise<void> {
        const wsUrl = `ws://localhost:8080/ws?token=${token}`;
        await this.ws.connect(wsUrl);
    }
    
    setupLobbyButtons() {
        const createRoomBtn = this.lobbyPanel.getChildByName('CreateRoomBtn')?.getComponent(Button);
        const joinRoomBtn = this.lobbyPanel.getChildByName('JoinRoomBtn')?.getComponent(Button);
        const roomIdInput = this.lobbyPanel.getChildByName('RoomIdInput')?.getComponent(EditBox);
        
        if (createRoomBtn) {
            createRoomBtn.node.on('click', () => {
                this.createRoom();
            });
        }
        
        if (joinRoomBtn && roomIdInput) {
            joinRoomBtn.node.on('click', () => {
                const roomId = roomIdInput.string;
                if (roomId) {
                    this.joinRoom(roomId);
                }
            });
        }
    }
    
    createRoom() {
        console.log('Creating room...');
        
        const payload = JSON.stringify({});
        const payloadBytes = new TextEncoder().encode(payload);
        const frame = new ArrayBuffer(4 + 2 + payloadBytes.length);
        const view = new DataView(frame);
        view.setUint32(0, 2 + payloadBytes.length, false);
        view.setUint16(4, MSG_ID.CREATE_ROOM_REQ, false);
        new Uint8Array(frame, 6).set(payloadBytes);
        
        if (this.ws.connected) {
            const wsInstance = (this.ws as any).ws;
            if (wsInstance) {
                wsInstance.send(frame);
                console.log('Create room message sent');
            }
        }
    }
    
    joinRoom(roomId: string) {
        this.ws.send(MSG_ID.JOIN_ROOM_REQ, { room_id: roomId });
    }
    
    showPanel(panelName: string) {
        this.loginPanel.active = panelName === 'login';
        this.lobbyPanel.active = panelName === 'lobby';
        this.gamePanel.active = panelName === 'game';
        
        if (panelName === 'game') {
            this.gameState.reset();
        }
    }
    
    start() {
        this.ws.on(MSG_ID.CREATE_ROOM_RESP, (data) => {
            if (data.success) {
                this.gameState.roomId = data.room_id;
                this.showPanel('game');
            }
        });
        
        this.ws.on(MSG_ID.JOIN_ROOM_RESP, (data) => {
            if (data.success) {
                this.showPanel('game');
            }
        });
        
        this.ws.on(MSG_ID.DEAL_CARDS_NOTIFY, () => {
            this.showPanel('game');
        });
        
        this.ws.on(MSG_ID.ERROR_RESP, (data) => {
            console.error('Server error:', data.message);
        });
    }
    
    onDestroy() {
        this.ws.disconnect();
    }
}