// HTTP API 调用模块
const API = {
    baseURL: 'http://localhost:8888',
    token: null,
    user: null,

    // 用户注册
    async register(username, password, nickname) {
        const response = await fetch(`${this.baseURL}/user/register`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username, password, nickname: nickname || '' })
        });
        
        const data = await response.json();
        
        if (!response.ok) {
            throw new Error(data.message || '注册失败');
        }
        
        this.token = data.token;
        this.user = { uid: data.uid };
        localStorage.setItem('ddz_token', data.token);
        localStorage.setItem('ddz_uid', data.uid);
        
        return data;
    },

    // 用户登录
    async login(username, password) {
        const response = await fetch(`${this.baseURL}/user/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username, password })
        });
        
        const data = await response.json();
        
        if (!response.ok) {
            throw new Error(data.message || '登录失败');
        }
        
        this.token = data.token;
        this.user = {
            uid: data.uid,
            nickname: data.nickname,
            avatarId: data.avatar_id,
            elo: data.elo,
            tier: data.tier,
            gold: data.gold
        };
        localStorage.setItem('ddz_token', data.token);
        localStorage.setItem('ddz_uid', data.uid);
        
        return data;
    },

    // 获取用户信息
    async getUserInfo() {
        if (!this.token) {
            throw new Error('未登录');
        }
        
        const response = await fetch(`${this.baseURL}/user/info`, {
            method: 'GET',
            headers: {
                'Authorization': `Bearer ${this.token}`
            }
        });
        
        const data = await response.json();
        
        if (!response.ok) {
            throw new Error(data.message || '获取信息失败');
        }
        
        this.user = data;
        return data;
    },

    // 登出
    logout() {
        this.token = null;
        this.user = null;
        localStorage.removeItem('ddz_token');
        localStorage.removeItem('ddz_uid');
    },

    // 检查是否已登录
    isLoggedIn() {
        return !!this.token;
    },

    // 从本地存储恢复登录状态
    restoreSession() {
        const token = localStorage.getItem('ddz_token');
        const uid = localStorage.getItem('ddz_uid');
        if (token && uid) {
            this.token = token;
            this.user = { uid };
            return true;
        }
        return false;
    }
};
