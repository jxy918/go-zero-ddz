// 主入口文件
document.addEventListener('DOMContentLoaded', () => {
    initTabs();
    initLoginForm();
    initRegisterForm();
    initLobby();
    initRoom();
    initGame();
    checkAutoLogin();
});

// 检查自动登录
function checkAutoLogin() {
    if (API.restoreSession()) {
        // 重置游戏状态
        Game.roomId = null;
        Game.myCards = [];
        Game.selectedCards = [];
        UI.showScreen('lobby');
        connectWebSocket();
    }
}

// 初始化标签页切换
function initTabs() {
    document.querySelectorAll('.tab').forEach(tab => {
        tab.addEventListener('click', () => {
            document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.form').forEach(f => f.classList.remove('active'));
            
            tab.classList.add('active');
            const formId = `${tab.dataset.tab}-form`;
            document.getElementById(formId).classList.add('active');
        });
    });
}

// 初始化登录表单
function initLoginForm() {
    document.getElementById('login-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        
        const username = document.getElementById('login-username').value;
        const password = document.getElementById('login-password').value;
        
        try {
            const data = await API.login(username, password);
            UI.showMessage('login', '登录成功', 'success');
            
            // 更新大厅信息
            document.getElementById('player-nickname').textContent = data.nickname || username;
            document.getElementById('player-elo').textContent = data.elo || 1000;
            
            // 重置游戏状态
            Game.roomId = null;
            Game.myCards = [];
            Game.selectedCards = [];
            Game.landlordUid = null;
            Game.currentTurnUid = null;
            Game.state = 'waiting';
            
            UI.showScreen('lobby');
            connectWebSocket();
        } catch (error) {
            UI.showMessage('login', error.message, 'error');
        }
    });
}

// 初始化注册表单
function initRegisterForm() {
    document.getElementById('register-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        
        const username = document.getElementById('register-username').value;
        const password = document.getElementById('register-password').value;
        const nickname = document.getElementById('register-nickname').value;
        
        try {
            const data = await API.register(username, password, nickname);
            UI.showMessage('register', '注册成功', 'success');
            
            // 自动登录
            document.getElementById('player-nickname').textContent = nickname || username;
            document.getElementById('player-elo').textContent = 1000;
            
            UI.showScreen('lobby');
            connectWebSocket();
        } catch (error) {
            UI.showMessage('register', error.message, 'error');
        }
    });
}

// 初始化大厅
function initLobby() {
    // 重置房间状态
    Game.roomId = null;
    
    // 创建房间
    document.getElementById('create-room-btn').addEventListener('click', () => {
        console.log('[Main] Create room button clicked');
        console.log('[Main] Game.roomId:', Game.roomId);
        console.log('[Main] WS.connected:', WS.connected);
        
        if (Game.roomId) {
            UI.showMessage('lobby', '你已经在房间中了', 'error');
            return;
        }
        Game.createRoom();
    });

    // 加入房间
    document.getElementById('join-room-btn').addEventListener('click', () => {
        const roomId = document.getElementById('room-id-input').value.trim();
        if (!roomId) {
            UI.showMessage('lobby', '请输入房间号', 'error');
            return;
        }
        Game.joinRoom(roomId);
    });

    // 退出登录
    document.getElementById('logout-btn').addEventListener('click', () => {
        API.logout();
        WS.disconnect();
        UI.showScreen('login');
    });
}

// 初始化房间
function initRoom() {
    // 准备
    document.getElementById('ready-btn').addEventListener('click', () => {
        Game.ready();
        document.getElementById('ready-btn').disabled = true;
        document.getElementById('ready-btn').textContent = '已准备';
    });

    // 离开房间
    document.getElementById('leave-room-btn').addEventListener('click', () => {
        Game.roomId = null;
        Game.myCards = [];
        Game.selectedCards = [];
        Game.landlordUid = null;
        Game.currentTurnUid = null;
        Game.players = {};
        Game.state = 'waiting';
        UI.showScreen('lobby');
        // 重置准备按钮状态
        document.getElementById('ready-btn').disabled = false;
        document.getElementById('ready-btn').textContent = '准备';
    });
}

// 初始化游戏
function initGame() {
    Game.init(API.user?.uid || 'unknown');

    // 出牌按钮
    document.getElementById('play-btn').addEventListener('click', () => {
        Game.playCards();
    });

    // 不出按钮
    document.getElementById('pass-btn').addEventListener('click', () => {
        Game.passPlay();
    });

    // 提示按钮
    document.getElementById('hint-btn').addEventListener('click', () => {
        // 简单提示：选中第一张牌
        if (Game.myCards.length > 0) {
            Game.selectedCards = [Game.myCards[0]];
            UI.renderMyCards(Game.myCards, Game.selectedCards);
        }
    });

    // 取消托管按钮
    document.getElementById('cancel-ai-btn').addEventListener('click', () => {
        console.log('[Main] Cancel AI control clicked');
        WS.send(WS.MSG.CANCEL_AI_CONTROL_REQ, {});
        Game.isAIControlled = false;
        UI.hideCancelAIControlBtn();
        UI.showMessage('game', '已取消托管', 'info');
    });

    // 叫地主按钮
    document.querySelectorAll('.call-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const score = parseInt(btn.dataset.score);
            Game.callLandlord(score);
        });
    });

    document.getElementById('pass-call-btn').addEventListener('click', () => {
        Game.passCall();
    });

    // 历史记录收起/展开
    document.getElementById('history-header').addEventListener('click', () => {
        const panel = document.querySelector('.history-panel');
        panel.classList.toggle('collapsed');
    });

    // 返回大厅按钮（结算界面）
    document.getElementById('back-to-lobby-btn').addEventListener('click', () => {
        // 重置游戏状态
        Game.roomId = null;
        Game.myCards = [];
        Game.selectedCards = [];
        Game.landlordUid = null;
        Game.currentTurnUid = null;
        Game.state = 'waiting';
        
        // 清空游戏数据
        UI.clearPlayHistory();
        UI.clearLastPlay();
        
        // 返回大厅
        UI.showScreen('lobby');
    });
}

// 调试创建房间函数
function debugCreateRoom() {
    console.log('=== Debug Create Room ===');
    console.log('Button clicked at:', new Date().toLocaleTimeString());
    
    // 检查 Game 对象
    console.log('Game object exists:', typeof Game !== 'undefined');
    console.log('Game:', Game);
    
    // 检查 Game.createRoom 方法
    console.log('Game.createRoom exists:', typeof Game.createRoom === 'function');
    
    // 检查 WS 对象
    console.log('WS object exists:', typeof WS !== 'undefined');
    console.log('WS.connected:', WS ? WS.connected : 'WS is undefined');
    console.log('WS.ws:', WS ? WS.ws : 'WS is undefined');
    
    // 检查 API.token
    console.log('API.token:', API ? API.token : 'API is undefined');
    
    // 如果一切正常，尝试创建房间
    if (typeof Game !== 'undefined' && typeof Game.createRoom === 'function') {
        console.log('Calling Game.createRoom()...');
        try {
            Game.createRoom();
        } catch (e) {
            console.error('Error calling Game.createRoom():', e);
        }
    } else {
        console.error('Game.createRoom is not available!');
    }
}

// 连接 WebSocket
async function connectWebSocket() {
    try {
        await WS.connect(API.token);
        console.log('[Main] WebSocket connected');
        
        // 发送登录消息到游戏服务
        WS.send(WS.MSG.LOGIN_REQ, { token: API.token });
        
        // 设置全局消息处理
        WS.onMessage = (msgId, data) => {
            // 可以在这里添加全局消息处理逻辑
        };
        
        // 设置断开连接回调
        WS.onDisconnect = (code, reason) => {
            UI.showMessage('lobby', '连接已断开', 'error');
        };
        
        // 设置重连成功回调
        WS.onReconnectSuccess = () => {
            console.log('[Main] Reconnected, re-sending login');
            // 重连成功后重新发送登录
            WS.send(WS.MSG.LOGIN_REQ, { token: API.token });
            
            // 如果在游戏中，发送重连请求
            if (Game.roomId) {
                console.log('[Main] Reconnecting to game room:', Game.roomId);
                setTimeout(() => {
                    WS.send(WS.MSG.RECONNECT_REQ, { 
                        session_key: '', 
                        room_id: Game.roomId 
                    });
                }, 500);
            }
        };
        
    } catch (error) {
        console.error('[Main] WebSocket connection failed:', error);
        UI.showMessage('lobby', 'WebSocket 连接失败', 'error');
    }
}
