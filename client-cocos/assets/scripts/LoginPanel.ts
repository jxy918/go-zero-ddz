import { _decorator, Component, Node, Label, EditBox, Button, Color } from 'cc';
const { ccclass, property } = _decorator;

@ccclass('LoginPanel')
export class LoginPanel extends Component {
    @property(EditBox)
    usernameEdit: EditBox = null!;
    
    @property(EditBox)
    passwordEdit: EditBox = null!;
    
    @property(Label)
    messageLabel: Label = null!;
    
    @property(Button)
    loginBtn: Button = null!;
    
    @property(Button)
    registerBtn: Button = null!;
    
    onLoad() {
        this.loginBtn.node.on('click', this.onLogin, this);
        this.registerBtn.node.on('click', this.onRegister, this);
        
        this.usernameEdit.string = 'test';
        this.passwordEdit.string = '123';
    }
    
    onLogin() {
        const username = this.usernameEdit.string;
        const password = this.passwordEdit.string;
        
        if (!username || !password) {
            this.showMessage('请输入用户名和密码', true);
            return;
        }
        
        this.loginBtn.interactable = false;
        
        this.login(username, password).then((data) => {
            this.showMessage('登录成功', false);
            this.onLoginSuccess(data);
        }).catch((error) => {
            this.showMessage(error.message, true);
            this.loginBtn.interactable = true;
        });
    }
    
    async login(username: string, password: string): Promise<any> {
        const response = await fetch('http://localhost:8888/user/login', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ username, password })
        });
        
        const data = await response.json();
        if (!data.success) {
            throw new Error(data.message || '登录失败');
        }
        return data;
    }
    
    async onRegister() {
        const username = this.usernameEdit.string;
        const password = this.passwordEdit.string;
        
        if (!username || !password) {
            this.showMessage('请输入用户名和密码', true);
            return;
        }
        
        try {
            const response = await fetch('http://localhost:8888/user/register', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ username, password, nickname: username })
            });
            
            const data = await response.json();
            if (data.success) {
                this.showMessage('注册成功', false);
            } else {
                this.showMessage(data.message || '注册失败', true);
            }
        } catch (error) {
            this.showMessage('注册失败', true);
        }
    }
    
    showMessage(msg: string, isError: boolean) {
        this.messageLabel.string = msg;
        this.messageLabel.color = isError ? new Color(255, 100, 100) : new Color(100, 255, 100);
        this.messageLabel.node.active = true;
        
        setTimeout(() => {
            this.messageLabel.node.active = false;
        }, 3000);
    }
    
    onLoginSuccess(data: any) {
        if (this.onLoginCallback) {
            this.onLoginCallback(data);
        }
    }
    
    onLoginCallback?: (data: any) => void;
}