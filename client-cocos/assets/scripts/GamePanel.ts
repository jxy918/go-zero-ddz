import { _decorator, Component, Node, Label, Button, Sprite, Color, instantiate } from 'cc';
const { ccclass, property } = _decorator;
import { GameState, Card } from './GameState';
import { WebSocketManager } from './WebSocketManager';
import { MSG_ID } from './GameConstants';

@ccclass('GamePanel')
export class GamePanel extends Component {
    @property(Node)
    cardPrefab: Node = null!;
    
    @property(Node)
    myCardsContainer: Node = null!;
    
    @property(Node)
    opponentLeftCards: Node = null!;
    
    @property(Node)
    opponentRightCards: Node = null!;
    
    @property(Node)
    lastPlayArea: Node = null!;
    
    @property(Label)
    timerLabel: Label = null!;
    
    @property(Label)
    currentPlayerLabel: Label = null!;
    
    @property(Label)
    myRoleLabel: Label = null!;
    
    @property(Button)
    playBtn: Button = null!;
    
    @property(Button)
    passBtn: Button = null!;
    
    @property(Button)
    callBtn: Button = null!;
    
    @property(Button)
    passCallBtn: Button = null!;
    
    @property(Node)
    callPanel: Node = null!;
    
    @property(Node)
    leftPlayerInfo: Node = null!;
    
    @property(Node)
    rightPlayerInfo: Node = null!;
    
    @property(Sprite)
    timerSprite: Sprite = null!;
    
    private gameState: GameState = GameState.instance;
    private ws: WebSocketManager = WebSocketManager.instance;
    
    onLoad() {
        this.gameState.onStateChange = this.handleStateChange.bind(this);
        
        this.playBtn.node.on('click', this.onPlayCards, this);
        this.passBtn.node.on('click', this.onPass, this);
        this.callBtn.node.on('click', this.onCallLandlord, this);
        this.passCallBtn.node.on('click', this.onPassCall, this);
        
        this.hideCallPanel();
    }
    
    handleStateChange(event: string, data: any) {
        switch (event) {
            case 'dealCards':
                this.renderMyCards(data.cards);
                break;
            case 'callLandlord':
                if (data.canCall) {
                    this.showCallPanel();
                }
                break;
            case 'landlordConfirmed':
                this.hideCallPanel();
                this.updateRole();
                break;
            case 'playCards':
                this.renderMyCards(this.gameState.myCards);
                this.showLastPlay(data.uid, data.cards);
                this.setActionButtons(data.isMyTurn);
                break;
            case 'pass':
                this.showLastPlay(data.uid, []);
                break;
            case 'timer':
                this.updateTimer(data.remainingTime);
                this.setActionButtons(data.isMyTurn);
                this.updateCurrentPlayer(data.isMyTurn);
                break;
            case 'gameEnd':
                this.handleGameEnd(data);
                break;
        }
    }
    
    renderMyCards(cards: Card[]) {
        this.myCardsContainer.removeAllChildren();
        
        cards.sort((a, b) => {
            if (a.value !== b.value) {
                return a.value - b.value;
            }
            return a.suit - b.suit;
        });
        
        const cardWidth = 60;
        const cardHeight = 90;
        const spacing = -15;
        const totalWidth = Math.max(cards.length * (cardWidth + spacing), this.myCardsContainer.width);
        const startX = -totalWidth / 2 + cardWidth / 2;
        
        cards.forEach((card, index) => {
            const cardNode = cc.instantiate(this.cardPrefab);
            cardNode.parent = this.myCardsContainer;
            
            const x = startX + index * (cardWidth + spacing);
            cardNode.setPosition(x, 0);
            
            const formatted = this.gameState.formatCard(card);
            this.setupCard(cardNode, formatted.value, formatted.suit, formatted.isRed);
            
            const isSelected = this.gameState.selectedCards.some(
                s => s.value === card.value && s.suit === card.suit
            );
            this.setCardSelected(cardNode, isSelected);
            
            cardNode.on('click', () => {
                if (this.gameState.isMyTurn()) {
                    this.gameState.toggleCard(card);
                }
            });
        });
    }
    
    setupCard(node: Node, value: string, suit: string, isRed: boolean) {
        // 延迟设置，确保节点完全准备好
        this.scheduleOnce(() => {
            const valueLabel = node.getChildByName('Value')?.getComponent(Label);
            const suitLabel = node.getChildByName('Suit')?.getComponent(Label);
            const centerSuit = node.getChildByName('CenterSuit')?.getComponent(Label);
            
            if (valueLabel) valueLabel.string = value;
            if (suitLabel) suitLabel.string = suit;
            if (centerSuit) centerSuit.string = suit;
            
            const color = isRed ? new Color(220, 38, 38) : new Color(31, 41, 55);
            if (valueLabel) valueLabel.color = color;
            if (suitLabel) suitLabel.color = color;
            if (centerSuit) centerSuit.color = color;
            
            // 设置卡牌背景颜色
            const sprite = node.getComponent(Sprite);
            if (sprite) {
                sprite.color = new Color(255, 255, 255);
                sprite.type = Sprite.Type.SLICE;
            }
            
            // 添加卡牌背景（如果没有spriteFrame）
            this.ensureCardBackground(node, isRed);
        }, 0);
    }
    
    ensureCardBackground(node: Node, isRed: boolean) {
        // 检查是否已有背景
        let bg = node.getChildByName('CardBG');
        if (!bg) {
            bg = new Node('CardBG');
            bg.parent = node;
            bg.setPosition(0, 0);
            const bgSprite = bg.addComponent(Sprite);
            bgSprite.type = Sprite.Type.SLICE;
            // 使用纯色背景
            bgSprite.color = new Color(245, 245, 220); // 米白色背景
            const size = node.getComponent(Sprite)?.node.getContentSize();
            if (size) {
                bg.setContentSize(size);
            } else {
                bg.setContentSize(60, 90);
            }
            // 确保背景在最下层
            bg.setSiblingIndex(0);
        }
    }
    
    setCardSelected(node: Node, selected: boolean) {
        const position = node.position;
        node.setPosition(position.x, selected ? -20 : 0);
        
        const sprite = node.getComponent(Sprite);
        if (sprite) {
            sprite.color = selected ? new Color(74, 222, 128) : new Color(255, 255, 255);
        }
    }
    
    showLastPlay(uid: string, cards: Card[]) {
        this.lastPlayArea.removeAllChildren();
        
        const isMe = uid === this.gameState.myUid;
        const playerName = isMe ? '我' : this.getOpponentName(uid);
        
        const nameLabel = new Label();
        nameLabel.string = playerName;
        nameLabel.color = new Color(255, 255, 255);
        this.lastPlayArea.addChild(nameLabel);
        
        if (cards.length === 0) {
            const passLabel = new Label();
            passLabel.string = '不出';
            passLabel.color = new Color(156, 163, 175);
            passLabel.fontSize = 24;
            this.lastPlayArea.addChild(passLabel);
            passLabel.setPosition(0, -30);
        } else {
            cards.forEach((card, index) => {
                const cardNode = instantiate(this.cardPrefab);
                cardNode.parent = this.lastPlayArea;
                
                const scale = 0.7;
                cardNode.setScale(scale);
                cardNode.setPosition(index * 40 - (cards.length - 1) * 20, -30);
                
                const formatted = this.gameState.formatCard(card);
                this.setupCard(cardNode, formatted.value, formatted.suit, formatted.isRed);
            });
        }
    }
    
    getOpponentName(uid: string): string {
        const player = this.gameState.players.get(uid);
        return player?.nickname || uid.slice(0, 4);
    }
    
    updateTimer(time: number) {
        this.timerLabel.string = time.toString();
        
        if (time <= 5) {
            this.timerSprite.color = new Color(251, 191, 36);
            this.timerSprite.node.scale = 1 + Math.sin(Date.now() / 200) * 0.1;
        } else {
            this.timerSprite.color = new Color(239, 68, 68);
            this.timerSprite.node.scale = 1;
        }
    }
    
    updateCurrentPlayer(isMyTurn: boolean) {
        this.currentPlayerLabel.string = isMyTurn ? '轮到你出牌' : '对手出牌中...';
        this.currentPlayerLabel.color = isMyTurn ? new Color(74, 222, 128) : new Color(255, 255, 255);
    }
    
    setActionButtons(enabled: boolean) {
        this.playBtn.interactable = enabled;
        this.passBtn.interactable = enabled;
    }
    
    updateRole() {
        const isLandlord = this.gameState.isLandlord();
        this.myRoleLabel.string = isLandlord ? '地主 👑' : '农民';
        this.myRoleLabel.color = isLandlord ? new Color(251, 191, 36) : new Color(255, 255, 255);
    }
    
    showCallPanel() {
        this.callPanel.active = true;
    }
    
    hideCallPanel() {
        this.callPanel.active = false;
    }
    
    onPlayCards() {
        if (this.gameState.selectedCards.length === 0) {
            return;
        }
        
        this.ws.send(MSG_ID.PLAY_CARDS_REQ, { cards: this.gameState.selectedCards });
    }
    
    onPass() {
        this.ws.send(MSG_ID.PLAY_CARDS_REQ, { cards: [] });
    }
    
    onCallLandlord() {
        this.ws.send(MSG_ID.CALL_LANDLORD_REQ, { action: 1, score: 1 });
        this.hideCallPanel();
    }
    
    onPassCall() {
        this.ws.send(MSG_ID.CALL_LANDLORD_REQ, { action: 0, score: 0 });
        this.hideCallPanel();
    }
    
    handleGameEnd(data: any) {
        const result = data.isWin ? '胜利！' : '失败！';
        this.currentPlayerLabel.string = `游戏结束 - ${result}`;
        this.currentPlayerLabel.color = data.isWin ? new Color(74, 222, 128) : new Color(239, 68, 68);
        
        this.setActionButtons(false);
    }
    
    updatePlayerInfo(uid: string, info: any) {
        if (uid === this.gameState.myUid) return;
        
        const isLeft = this.isLeftPlayer(uid);
        const playerNode = isLeft ? this.leftPlayerInfo : this.rightPlayerInfo;
        const nameLabel = playerNode.getChildByName('Name')?.getComponent(Label);
        const roleLabel = playerNode.getChildByName('Role')?.getComponent(Label);
        
        if (nameLabel) nameLabel.string = info.nickname || uid.slice(-4);
        if (roleLabel && info.isLandlord) {
            roleLabel.string = '👑';
        }
        
        // 为机器人添加视觉区分的背景指示
        this.updatePlayerIndicator(playerNode, uid, info);
    }
    
    updatePlayerIndicator(playerNode: Node, uid: string, info: any) {
        // 检查是否已有指示器
        let indicator = playerNode.getChildByName('BotIndicator');
        
        // 只有机器人才需要指示器
        const isBot = uid.startsWith('bot_') || (info.nickname && info.nickname.startsWith('AI_'));
        
        if (isBot) {
            if (!indicator) {
                indicator = new Node('BotIndicator');
                indicator.parent = playerNode;
                // 放在最底层
                indicator.setSiblingIndex(0);
                const sprite = indicator.addComponent(Sprite);
                sprite.type = Sprite.Type.SOLID;
                sprite.color = new Color(100, 100, 100, 100); // 半透明灰色
                indicator.setContentSize(120, 40);
                indicator.setPosition(0, 0);
            }
            
            // 根据机器人索引设置不同颜色
            // 从昵称中提取机器人编号
            const nickname = info.nickname || '';
            if (nickname.includes('_1') || nickname.includes('01')) {
                // 第一个机器人 - 蓝色
                indicator.getComponent(Sprite).color = new Color(59, 130, 246, 150); // 蓝色半透明
            } else if (nickname.includes('_2') || nickname.includes('02')) {
                // 第二个机器人 - 紫色
                indicator.getComponent(Sprite).color = new Color(147, 51, 234, 150); // 紫色半透明
            } else {
                // 默认 - 绿色
                indicator.getComponent(Sprite).color = new Color(34, 197, 94, 150); // 绿色半透明
            }
        } else if (indicator) {
            // 人类玩家移除指示器
            indicator.destroy();
        }
    }
    
    isLeftPlayer(uid: string): boolean {
        const uids = Array.from(this.gameState.players.keys());
        const myIndex = uids.indexOf(this.gameState.myUid);
        const targetIndex = uids.indexOf(uid);
        
        if (myIndex === -1) return true;
        
        const leftIndex = (myIndex + 2) % 3;
        return targetIndex === leftIndex;
    }
}