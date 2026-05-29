import { WebSocketManager } from './WebSocketManager';
import { MSG_ID, CARD_VALUE, CARD_SUIT, GAME_STATE } from './GameConstants';

export interface Card {
    value: number;
    suit: number;
}

export interface Player {
    uid: string;
    nickname: string;
    isBot: boolean;
    isLandlord: boolean;
    cards: Card[];
}

export class GameState {
    private static _instance: GameState | null = null;
    
    public myUid: string = '';
    public myNickname: string = '';
    public roomId: string = '';
    public players: Map<string, Player> = new Map();
    public myCards: Card[] = [];
    public selectedCards: Card[] = [];
    public landlordUid: string = '';
    public currentTurnUid: string = '';
    public gameState: string = GAME_STATE.DEALING;
    public lastPlayedCards: Card[] = [];
    public lastPlayedUid: string = '';
    public remainingTime: number = 15;
    
    public static get instance(): GameState {
        if (!this._instance) {
            this._instance = new GameState();
        }
        return this._instance;
    }

    init(uid: string, nickname: string) {
        this.myUid = uid;
        this.myNickname = nickname;
        this.setupEventHandlers();
    }

    private setupEventHandlers() {
        const ws = WebSocketManager.instance;
        
        ws.on(MSG_ID.DEAL_CARDS_NOTIFY, (data) => {
            this.handleDealCards(data);
        });
        
        ws.on(MSG_ID.CALL_LANDLORD_NOTIFY, (data) => {
            this.handleCallLandlord(data);
        });
        
        ws.on(MSG_ID.PLAY_CARDS_NOTIFY, (data) => {
            this.handlePlayCards(data);
        });
        
        ws.on(MSG_ID.PASS_NOTIFY, (data) => {
            this.handlePass(data);
        });
        
        ws.on(MSG_ID.TIMER_NOTIFY, (data) => {
            this.handleTimer(data);
        });
        
        ws.on(MSG_ID.GAME_END_NOTIFY, (data) => {
            this.handleGameEnd(data);
        });
        
        ws.on(MSG_ID.CREATE_ROOM_RESP, (data) => {
            this.handleCreateRoom(data);
        });
        
        ws.on(MSG_ID.JOIN_ROOM_RESP, (data) => {
            this.handleJoinRoom(data);
        });
        
        ws.on(MSG_ID.ROOM_STATE_NOTIFY, (data) => {
            this.handleRoomState(data);
        });
    }

    private handleDealCards(data: any) {
        this.myCards = data.my_cards || [];
        this.selectedCards = [];
        this.gameState = GAME_STATE.CALLING;
        
        if (data.bottom_cards && data.bottom_cards.length > 0) {
            console.log('Bottom cards:', data.bottom_cards);
        }
        
        if (this.onStateChange) {
            this.onStateChange('dealCards', { cards: this.myCards });
        }
    }

    private handleCallLandlord(data: any) {
        if (data.landlord_uid) {
            this.landlordUid = data.landlord_uid;
            this.gameState = GAME_STATE.PLAYING;
            
            if (data.players) {
                data.players.forEach((uid: string) => {
                    if (!this.players.has(uid)) {
                        this.players.set(uid, {
                            uid,
                            nickname: uid === this.myUid ? this.myNickname : `AI_${uid.slice(-4)}`,
                            isBot: uid !== this.myUid,
                            isLandlord: uid === data.landlord_uid,
                            cards: []
                        });
                    } else {
                        const player = this.players.get(uid)!;
                        player.isLandlord = uid === data.landlord_uid;
                    }
                });
            }
            
            if (this.onStateChange) {
                this.onStateChange('landlordConfirmed', { 
                    landlordUid: this.landlordUid,
                    isMyTurn: this.currentTurnUid === this.myUid
                });
            }
        } else if (data.action === 0 && data.uid === this.myUid) {
            if (this.onStateChange) {
                this.onStateChange('callLandlord', { canCall: true });
            }
        }
    }

    private handlePlayCards(data: any) {
        this.lastPlayedUid = data.uid;
        this.lastPlayedCards = data.cards || [];
        
        if (data.uid === this.myUid) {
            this.myCards = this.myCards.filter(card => 
                !this.lastPlayedCards.some(played => 
                    played.value === card.value && played.suit === card.suit
                )
            );
            this.selectedCards = [];
        }
        
        if (this.onStateChange) {
            this.onStateChange('playCards', {
                uid: data.uid,
                cards: this.lastPlayedCards,
                isMyTurn: this.currentTurnUid === this.myUid,
                cardCount: this.myCards.length
            });
        }
    }

    private handlePass(data: any) {
        this.lastPlayedUid = data.uid;
        this.lastPlayedCards = [];
        
        if (this.onStateChange) {
            this.onStateChange('pass', { uid: data.uid });
        }
    }

    private handleTimer(data: any) {
        const prevTurn = this.currentTurnUid;
        this.currentTurnUid = data.current_turn_uid;
        this.remainingTime = data.remaining_seconds;
        
        const isMyTurn = this.currentTurnUid === this.myUid;
        console.log(`[GameState] handleTimer: prevTurn=${prevTurn}, currentTurn=${this.currentTurnUid}, myUid=${this.myUid}, isMyTurn=${isMyTurn}`);
        
        if (this.onStateChange) {
            this.onStateChange('timer', {
                remainingTime: this.remainingTime,
                isMyTurn: isMyTurn
            });
        }
    }

    private handleGameEnd(data: any) {
        this.gameState = GAME_STATE.SETTLEMENT;
        
        if (this.onStateChange) {
            this.onStateChange('gameEnd', {
                winnerUid: data.winner_uid,
                isWin: data.winner_uid === this.myUid
            });
        }
    }

    private handleCreateRoom(data: any) {
        if (data.success) {
            this.roomId = data.room_id;
            if (this.onStateChange) {
                this.onStateChange('roomCreated', { roomId: this.roomId });
            }
        }
    }

    private handleJoinRoom(data: any) {
        if (data.success) {
            if (this.onStateChange) {
                this.onStateChange('roomJoined', {});
            }
        }
    }

    private handleRoomState(data: any) {
        if (this.onStateChange) {
            this.onStateChange('roomState', data);
        }
    }

    toggleCard(card: Card) {
        const index = this.selectedCards.findIndex(c => 
            c.value === card.value && c.suit === card.suit
        );
        
        if (index >= 0) {
            this.selectedCards.splice(index, 1);
        } else {
            this.selectedCards.push(card);
        }
        
        if (this.onStateChange) {
            this.onStateChange('cardSelected', { 
                selectedCards: this.selectedCards,
                myCards: this.myCards
            });
        }
    }

    clearSelection() {
        this.selectedCards = [];
        if (this.onStateChange) {
            this.onStateChange('cardSelected', { 
                selectedCards: this.selectedCards,
                myCards: this.myCards
            });
        }
    }

    formatCard(card: Card): { value: string; suit: string; isRed: boolean } {
        const value = CARD_VALUE[card.value] || '?';
        const suit = CARD_SUIT[card.suit] || '';
        const isRed = card.suit === 2 || card.suit === 4;
        
        return { value, suit, isRed };
    }

    isMyTurn(): boolean {
        return this.currentTurnUid === this.myUid;
    }

    isLandlord(): boolean {
        return this.landlordUid === this.myUid;
    }

    reset() {
        this.roomId = '';
        this.players.clear();
        this.myCards = [];
        this.selectedCards = [];
        this.landlordUid = '';
        this.currentTurnUid = '';
        this.gameState = GAME_STATE.DEALING;
        this.lastPlayedCards = [];
        this.lastPlayedUid = '';
        this.remainingTime = 15;
    }

    onStateChange?: (event: string, data: any) => void;
}