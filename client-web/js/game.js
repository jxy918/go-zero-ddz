// 游戏逻辑模块
const Game = {
    myCards: [],
    selectedCards: [],
    roomId: null,
    landlordUid: null,
    currentTurnUid: null,
    players: {},
    state: 'waiting', // waiting, playing, settlement
    isAIControlled: false, // 是否被AI托管
    
    // 历史出牌记录
    historyRecords: [],
    totalPlayedCount: 0,

    // 牌面映射
    CARD_VALUES: {
        3: '3', 4: '4', 5: '5', 6: '6', 7: '7', 8: '8', 9: '9',
        10: '10', 11: 'J', 12: 'Q', 13: 'K', 14: 'A', 15: '2',
        16: '小', 17: '大'
    },

    CARD_SUITS: {
        1: '♠', 2: '♥', 3: '♣', 4: '♦', 5: '🃏', 6: '🃏'
    },

    // 初始化游戏
    init(myUid) {
        this.myUid = myUid;
        this.setupEventHandlers();
    },

    // 设置 WebSocket 事件处理
    setupEventHandlers() {
        // 心跳响应
        WS.on(WS.MSG.HEARTBEAT_RESP, (data) => {
            console.log('[Game] Heartbeat received, ping:', data.ping);
        });

        // 登录响应
        WS.on(WS.MSG.LOGIN_RESP, (data) => {
            console.log('[Game] Login success:', data);
            this.myUid = data.uid;
            UI.showMessage('game', '登录成功', 'success');
        });
        
        // 断线处理
        WS.onDisconnect = (code, reason) => {
            console.log('[Game] Disconnected:', code, reason);
            UI.showMessage('game', '已断线，正在重连...', 'warning');
        };

        // 创建房间响应
        WS.on(WS.MSG.CREATE_ROOM_RESP, (data) => {
            console.log('[Game] Create room response received:', data);
            if (data.success) {
                this.roomId = data.room_id;
                console.log(`[Game] Room created successfully: roomId=${this.roomId}, myUid=${this.myUid}`);
                UI.showScreen('room');
                document.getElementById('current-room-id').textContent = this.roomId;
                UI.showMessage('room', '房间创建成功', 'success');
            }
        });

        // 加入房间响应
        WS.on(WS.MSG.JOIN_ROOM_RESP, (data) => {
            console.log('[Game] Join room response received:', data);
            if (data.success) {
                console.log(`[Game] Joined room successfully: myUid=${this.myUid}`);
                UI.showScreen('room');
                UI.showMessage('room', '加入房间成功', 'success');
            }
        });

        // 房间状态通知
        WS.on(WS.MSG.ROOM_STATE_NOTIFY, (data) => {
            console.log('[Game] Room state:', data);
            if (data.event === 'player_joined') {
                UI.updatePlayerCount(data.count);
                // 有新玩家加入，隐藏倒计时
                UI.hideBotCountdown();
            } else if (data.event === 'player_ready') {
                UI.markPlayerReady(data.uid);
            } else if (data.event === 'bot_join_countdown') {
                // 显示机器人加入倒计时
                UI.showBotCountdown(data.seconds);
            }
        });

        // 发牌通知
        WS.on(WS.MSG.DEAL_CARDS_NOTIFY, (data) => {
            console.log('[Game] ===== DEAL_CARDS_NOTIFY =====');
            console.log('[Game] Data:', JSON.stringify(data, null, 2));
            console.log('[Game] your_cards length:', data.your_cards ? data.your_cards.length : 'undefined');
            console.log('[Game] my_cards length:', data.my_cards ? data.my_cards.length : 'undefined');
            console.log('[Game] is_landlord:', data.is_landlord);
            console.log('[Game] bottom_cards length:', data.bottom_cards ? data.bottom_cards.length : 'undefined');
            console.log('[Game] landlord_uid:', data.landlord_uid);
            
            // 更新手牌（优先使用 your_cards，这是地主确认后的完整手牌）
            if (data.your_cards) {
                console.log('[Game] Updating cards from your_cards:', data.your_cards.length);
                this.myCards = data.your_cards.map(c => ({ value: c.value, suit: c.suit }));
            } else if (data.my_cards) {
                console.log('[Game] Updating cards from my_cards:', data.my_cards.length);
                this.myCards = data.my_cards.map(c => ({ value: c.value, suit: c.suit }));
            }
            
            this.selectedCards = [];
            UI.renderMyCards(this.myCards);
            UI.updateCardCount(this.myCards.length);
            console.log('[Game] My cards after update:', this.myCards.length);
            
            // 更新所有玩家手牌数量
            if (data.counts) {
                console.log('[Game] Updating card counts:', data.counts);
                for (const uid in data.counts) {
                    if (uid === this.myUid) {
                        UI.updateCardCount(data.counts[uid]);
                    } else {
                        UI.updateOpponentCardCount(uid, data.counts[uid]);
                    }
                }
            }
            
            // 显示底牌：发牌阶段显示盖着的，地主确认后显示翻开的
            if (data.bottom_cards && data.bottom_cards.length > 0) {
                const isRevealed = data.landlord_uid !== undefined && data.landlord_uid !== '';
                console.log('[Game] SHOWING BOTTOM CARDS, revealed:', isRevealed);
                UI.showBottomCards(data.bottom_cards, isRevealed);
                if (isRevealed) {
                    UI.showMessage('game', `底牌: ${this.cardsToString(data.bottom_cards)}`, 'info');
                }
            }
            
            // 设置玩家信息（包含地主标记）
            if (data.players && data.players.length > 0) {
                console.log('[Game] Setting players:', data.players, 'landlord:', data.landlord_uid);
                UI.setPlayers(data.players, data.landlord_uid || "", this.myUid);
            }
            
            // 首次发牌或地主确认后都切换到游戏界面
            UI.showScreen('game');
            if (data.your_cards === undefined && data.my_cards !== undefined) {
                UI.showMessage('game', '游戏开始！', 'success');
            } else if (data.is_landlord) {
                UI.showMessage('game', '你成为地主！', 'success');
            }
        });

        // 叫地主通知
        WS.on(WS.MSG.CALL_LANDLORD_NOTIFY, (data) => {
            console.log('[Game] Call landlord:', data);
            console.log('[Game] myUid:', this.myUid);
            console.log('[Game] players:', this.players);
            
            // 先处理地主确认（只有当所有玩家都叫完后才会有 landlord_uid）
            if (data.landlord_uid) {
                // 地主已确认，先展示底牌和地主信息，5秒后才出牌
                this.landlordUid = data.landlord_uid;
                // 设置玩家信息
                if (data.players && data.players.length > 0) {
                    UI.setPlayers(data.players, data.landlord_uid, this.myUid);
                }
                UI.updateRole(this.landlordUid === this.myUid ? '地主' : '农民');
                
                // 1. 先显示底牌（翻开展示）
                if (data.bottom_cards && data.bottom_cards.length > 0) {
                    UI.showBottomCards(data.bottom_cards, true); // 直接显示正面
                    UI.showMessage('game', `底牌: ${this.cardsToString(data.bottom_cards)}`, 'info');
                }
                
                console.log('[Game] Landlord confirmed, hiding call panel');
                UI.hideCallPanel();
                
                // 2. 显示地主信息
                UI.showMessage('game', `${data.players.find(p => p.uid === data.landlord_uid)?.nickname || '玩家'} 成为地主！`, 'success');
                
                // 3. 等待5秒，等待服务器发底牌给地主并开始出牌阶段（服务器会处理）
            } else if (data.uid !== undefined && data.action !== undefined) {
                // 处理叫地主结果广播（每个玩家叫地主/不叫都会触发）
                console.log(`[Game] Processing call landlord result: uid=${data.uid}, action=${data.action}, score=${data.score}, turn=${data.turn}`);
                let playerName = this.players[data.uid]?.name;
                console.log(`[Game] Player name from players map: ${playerName}`);
                if (!playerName) {
                    if (data.uid.includes('bot')) {
                        const botNum = data.uid.split('-').pop();
                        playerName = `机器人${botNum}`;
                    } else if (data.uid === this.myUid) {
                        playerName = '我';
                    } else {
                        playerName = data.uid.substring(0, 6);
                    }
                }
                console.log(`[Game] Final player name: ${playerName}`);
                if (data.action === 1 && data.score > 0) {
                    console.log(`[Game] Showing message: ${playerName} 叫地主 ${data.score}分！`);
                    UI.showMessage('game', `${playerName} 叫地主 ${data.score}分！`, 'info');
                } else {
                    console.log(`[Game] Showing message: ${playerName} 不叫`);
                    UI.showMessage('game', `${playerName} 不叫`, 'info');
                }
                
                // 如果同时包含 turn=true，表示这是机器人的叫地主结果，同时也是轮到下一个玩家的信号
                // 但机器人会自动叫地主，所以不需要额外显示"轮到谁"的消息
                // 只有当 turn=true 且 uid 是自己时，才需要显示叫地主面板
                if (data.turn === true && data.uid === this.myUid) {
                    console.log('[Game] It is my turn to call, showing call panel');
                    UI.showMessage('game', '轮到我叫地主', 'info');
                    UI.showCallPanel();
                }
            } else if (data.turn === true && data.uid !== undefined) {
                // 纯回合切换消息（没有 action，只是通知轮到谁）
                if (data.uid === this.myUid) {
                    console.log('[Game] It is my turn to call, showing call panel');
                    UI.showMessage('game', '轮到我叫地主', 'info');
                    UI.showCallPanel();
                } else {
                    let playerName = this.players[data.uid]?.name;
                    if (!playerName) {
                        if (data.uid.includes('bot')) {
                            const botNum = data.uid.split('-').pop();
                            playerName = `机器人${botNum}`;
                        } else {
                            playerName = data.uid.substring(0, 6);
                        }
                    }
                    UI.showMessage('game', `轮到${playerName}叫地主`, 'info');
                    UI.hideCallPanel();
                }
            } else {
                console.log('[Game] CALL_LANDLORD_NOTIFY received but no matching condition:', data);
            }
        });

        // 出牌通知
        WS.on(WS.MSG.PLAY_CARDS_NOTIFY, (data) => {
            console.log('[Game] Play cards:', data);
            
            // 进入打牌阶段，隐藏底牌
            UI.hideBottomCards();
            
            // 更新所有玩家的手牌数量（用于地主确认后的初始手牌数）
            if (data.card_counts) {
                for (const uid in data.card_counts) {
                    if (uid === this.myUid) {
                        UI.updateCardCount(data.card_counts[uid]);
                    } else {
                        UI.updateOpponentCardCount(uid, data.card_counts[uid]);
                    }
                }
            }
            
            // 更新玩家信息（用于地主确认）
            if (data.players && data.landlord_uid) {
                UI.setPlayers(data.players, data.landlord_uid, this.myUid);
            }
            
            if (data.uid === this.myUid && data.cards && data.cards.length > 0) {
                // 自己出的牌，更新手牌
                this.myCards = this.myCards.filter(c => 
                    !data.cards.some(played => played.value === c.value && played.suit === c.suit)
                );
                this.selectedCards = [];
                UI.renderMyCards(this.myCards);
                UI.updateCardCount(this.myCards.length);
            }
            
            // 在对应的玩家位置显示出牌
            if (data.uid && data.cards && data.cards.length > 0) {
                UI.showPlayerPlay(data.uid, data.cards);
                
                // 添加到历史记录
                const playerName = this.players[data.uid]?.name || data.uid;
                const cardStr = data.cards.map(c => this.getCardDisplay(c)).join(' ');
                this.historyRecords.push({
                    uid: data.uid,
                    playerName: playerName,
                    cards: data.cards,
                    cardStr: cardStr,
                    pattern: data.pattern || ''
                });
                this.totalPlayedCount += data.cards.length;
                
                // 更新历史记录UI
                UI.updateHistoryPanel(this.historyRecords, this.totalPlayedCount);
            }
            
            // 更新对手手牌数
            if (data.uid !== this.myUid && data.card_count !== undefined) {
                UI.updateOpponentCardCount(data.uid, data.card_count);
            }
            
            if (data.is_last) {
                UI.showMessage('game', `${data.uid === this.myUid ? '你' : '对手'}赢了！`, 'success');
            }
        });

        // Pass 通知
        WS.on(WS.MSG.PASS_NOTIFY, (data) => {
            console.log('[Game] Pass:', data);
            UI.showPlayerPlay(data.uid, '不出');
        });

        // 游戏结束通知
        WS.on(WS.MSG.GAME_END_NOTIFY, (data) => {
            console.log('[Game] Game ended:', data);
            const winner = data.winner_uid === this.myUid ? '你' : '对手';
            this.showSettlement(data, winner);
        });

        // 计时器通知
        WS.on(WS.MSG.TIMER_NOTIFY, (data) => {
            console.log('[Game] Timer:', data);
            this.currentTurnUid = data.current_turn_uid;
            const isMyTurn = data.current_turn_uid === this.myUid;
            
            // 更新所有玩家区域的定时器显示
            UI.showTimerForPlayer(data.current_turn_uid, data.remaining_seconds);
            
            UI.setActionButtons(isMyTurn);
            // 判断当前阶段：如果有地主uid说明是出牌阶段，否则是叫地主阶段
            const isPlayingPhase = !!this.landlordUid;
            UI.highlightCurrentPlayer(data.current_turn_uid, isPlayingPhase);
        });

        // 重连响应
        WS.on(WS.MSG.RECONNECT_RESP, (data) => {
            console.log('[Game] Reconnected:', data);
            if (data.success) {
                this.roomId = data.room_id;
                this.myCards = data.my_cards || [];
                this.landlordUid = data.landlord_uid;
                
                // 设置玩家信息
                if (data.players && data.players.length > 0) {
                    UI.setPlayers(data.players, data.landlord_uid, this.myUid);
                }
                
                // 更新角色
                if (this.landlordUid) {
                    UI.updateRole(this.landlordUid === this.myUid ? '地主' : '农民');
                }
                
                // 显示底牌（如果地主已确定）
                if (data.bottom_cards && data.bottom_cards.length > 0) {
                    UI.showBottomCards(data.bottom_cards, true);
                }
                
                // 更新当前回合
                if (data.current_turn_uid) {
                    this.currentTurnUid = data.current_turn_uid;
                    const isMyTurn = data.current_turn_uid === this.myUid;
                    UI.setActionButtons(isMyTurn);
                    UI.highlightCurrentPlayer(data.current_turn_uid, !!data.landlord_uid);
                }
                
                // 显示最后出牌
                if (data.last_played_uid && data.last_played_cards) {
                    const playerName = this.players[data.last_played_uid]?.name || data.last_played_uid.substring(0, 6);
                    UI.showLastPlay(playerName, data.last_played_cards);
                }
                
                UI.showScreen('game');
                UI.showMessage('game', '重连成功，游戏继续', 'success');
            } else {
                UI.showMessage('game', '重连失败：' + (data.message || '未知错误'), 'error');
                UI.showScreen('lobby');
            }
        });
        
        // 错误消息
        WS.on(WS.MSG.ERROR, (data) => {
            console.error('[Game] Error:', data);
            UI.showMessage('game', data.message, 'error');
        });
    },

    // 创建房间
    createRoom() {
        console.log('[Game] Creating room...');
        console.log('[Game] WS connected:', WS.connected);
        
        if (!WS.connected) {
            console.error('[Game] WebSocket not connected!');
            UI.showMessage('lobby', 'WebSocket 未连接', 'error');
            return;
        }
        
        WS.send(WS.MSG.CREATE_ROOM_REQ, {});
        console.log('[Game] Create room message sent!');
    },

    // 加入房间
    joinRoom(roomId) {
        WS.send(WS.MSG.JOIN_ROOM_REQ, { room_id: roomId });
    },

    // 准备
    ready() {
        WS.send(WS.MSG.PLAYER_READY_REQ, {});
    },

    // 叫地主
    callLandlord(score) {
        console.log(`[Game] callLandlord called: score=${score}, myUid=${this.myUid}, roomId=${this.roomId}`);
        WS.send(WS.MSG.CALL_LANDLORD_REQ, { action: 1, score: score });
        UI.hideCallPanel();
        console.log('[Game] callLandlord request sent');
    },

    // 不叫
    passCall() {
        console.log(`[Game] passCall called: myUid=${this.myUid}, roomId=${this.roomId}`);
        WS.send(WS.MSG.CALL_LANDLORD_REQ, { action: 0, score: 0 });
        UI.hideCallPanel();
        console.log('[Game] passCall request sent');
    },

    // 出牌
    playCards() {
        console.log('[Game] playCards called - myCards.length:', this.myCards.length, ', selectedCards.length:', this.selectedCards.length);
        console.log('[Game] selectedCards:', JSON.stringify(this.selectedCards));
        console.log('[Game] myCards:', JSON.stringify(this.myCards));
        
        // 如果只剩一张牌且没选，自动选中
        if (this.selectedCards.length === 0 && this.myCards.length === 1) {
            console.log('[Game] Auto-selecting last card');
            this.selectedCards = [this.myCards[0]];
            UI.renderMyCards(this.myCards, this.selectedCards);
        }
        
        if (this.selectedCards.length === 0) {
            console.log('[Game] No cards selected');
            UI.showMessage('game', '请选择要出的牌', 'error');
            return;
        }
        
        console.log('[Game] Sending play cards request:', JSON.stringify(this.selectedCards));
        WS.send(WS.MSG.PLAY_CARDS_REQ, { cards: this.selectedCards });
        
        console.log('[Game] Clearing selected cards');
        this.selectedCards = [];
        UI.renderMyCards(this.myCards);
    },

    // 不出
    passPlay() {
        WS.send(WS.MSG.PLAY_CARDS_REQ, { cards: [] });
    },

    // 选牌
    toggleCard(card) {
        // 如果当前被AI托管，点击选牌时取消托管
        if (this.isAIControlled) {
            console.log('[Game] Cancelling AI control on card selection');
            WS.send(WS.MSG.CANCEL_AI_CONTROL_REQ, {});
            this.isAIControlled = false;
        }
        
        // 支持字符串格式和对象格式的比较
        const index = this.selectedCards.findIndex(c => {
            if (typeof c === 'string' && typeof card === 'string') {
                return c === card;
            } else if (typeof c === 'object' && typeof card === 'object') {
                return c.value === card.value && c.suit === card.suit;
            } else {
                // 混合格式，比较格式化后的值和花色
                const fc = this.formatCard(c);
                const fcard = this.formatCard(card);
                return fc.value === fcard.value && fc.suit === fcard.suit;
            }
        });
        
        if (index >= 0) {
            this.selectedCards.splice(index, 1);
        } else {
            this.selectedCards.push(card);
        }
        
        UI.renderMyCards(this.myCards, this.selectedCards);
    },

    // 清空选牌
    clearSelection() {
        this.selectedCards = [];
        UI.renderMyCards(this.myCards);
    },

    // 获取卡牌显示字符串
    getCardDisplay(card) {
        const fc = this.formatCard(card);
        return fc.suit + fc.value;
    },
    
    // 将卡牌数组转换为字符串
    cardsToString(cards) {
        return cards.map(c => this.getCardDisplay(c)).join(', ');
    },
    
    // 格式化卡牌显示
    formatCard(card) {
        // 支持字符串格式（如 "S3", "H7", "JS", "JB"）和对象格式（{value, suit}）
        let value, suit, isRed;
        
        if (typeof card === 'string') {
            // 字符串格式: "S3", "H7", "JS", "JB"
            const suitMap = { 'S': 1, 'H': 2, 'C': 3, 'D': 4, 'J': 5, 'W': 6 };
            const suitCode = card[0];
            const valueCode = card.substring(1);
            
            suit = this.CARD_SUITS[suitMap[suitCode]] || '?';
            value = valueCode; // 已经是字符串格式了
            
            isRed = suitCode === 'H' || suitCode === 'D';
            // 大王特殊处理：字符串格式中大王通常表示为 JB 或 WB
            if (suitCode === 'J' && valueCode === 'B') {
                isRed = true; // 大王是红色
                suit = '🃏';
            } else if (suitCode === 'W') {
                // 另一种大王表示方式
                isRed = true; // 大王是红色
                suit = '🃏';
            } else if (suitCode === 'J' && valueCode === 'S') {
                // 小王
                suit = '🃏';
            }
        } else {
            // 对象格式: {value: number, suit: number}
            // suit: 1=黑桃, 2=红桃, 3=梅花, 4=方块, 5=王牌
            // value: 16=小王, 17=大王
            value = this.CARD_VALUES[card.value] || '?';
            suit = this.CARD_SUITS[card.suit] || '';
            // 红桃(2)、方块(4)、大王(value=17)是红色；黑桃(1)、梅花(3)、小王(value=16)是黑色
            isRed = card.suit === 2 || card.suit === 4 || card.value === 17;
        }
        
        return { value, suit, isRed };
    },

    // 显示结算界面
    showSettlement(data, winnerText) {
        console.log('[Game] Showing settlement:', data);
        
        // 显示结算界面
        UI.showScreen('settlement');
        
        // 设置赢家
        document.getElementById('winner-text').textContent = `${winnerText}赢了！`;
        
        // 设置结算详情
        document.getElementById('settlement-base-score').textContent = data.base_score || 1;
        document.getElementById('settlement-multiplier').textContent = data.multiplier || 1;
        document.getElementById('settlement-spring').textContent = data.is_spring ? '是' : '否';
        document.getElementById('settlement-counter-spring').textContent = data.is_counter_spring ? '是' : '否';
        
        // 显示玩家结果
        const resultsContainer = document.getElementById('player-results');
        resultsContainer.innerHTML = '<h3>玩家战绩</h3>';
        
        if (data.results && data.results.length > 0) {
            data.results.forEach(result => {
                const resultItem = document.createElement('div');
                resultItem.className = 'result-item' + (result.uid === data.winner_uid ? ' winner' : '');
                
                const playerName = result.is_landlord ? '地主' : '农民';
                const scoreClass = result.score_change >= 0 ? 'positive' : 'negative';
                const scoreText = result.score_change >= 0 ? `+${result.score_change}` : result.score_change;
                
                resultItem.innerHTML = `
                    <span class="player-name">${playerName}</span>
                    <span class="score-change ${scoreClass}">${scoreText}</span>
                `;
                
                resultsContainer.appendChild(resultItem);
            });
        }
    }
};

// UI 操作模块
const UI = {
    // 玩家信息存储
    players: {},

    CARD_SUITS: {
        1: '♠', 2: '♥', 3: '♣', 4: '♦', 5: '🃏', 6: '🃏'
    },
    
    CARD_VALUES: {
        3: '3', 4: '4', 5: '5', 6: '6', 7: '7', 8: '8', 9: '9', 10: '10',
        11: 'J', 12: 'Q', 13: 'K', 14: 'A', 15: '2', 16: 'X', 17: 'D'
    },
    
    formatCard(card) {
        let value, suit;
        if (typeof card === 'string') {
            const suitMap = { 'S': 1, 'H': 2, 'C': 3, 'D': 4, 'J': 5 };
            const suitCode = card[0];
            const valueCode = card.substring(1);
            suit = this.CARD_SUITS[suitMap[suitCode]] || '?';
            value = valueCode;
        } else {
            value = this.CARD_VALUES[card.value] || '?';
            suit = this.CARD_SUITS[card.suit] || '';
        }
        return { value, suit };
    },
    
    getCardDisplay(card) {
        const fc = this.formatCard(card);
        return fc.suit + fc.value;
    },
    
    // 切换界面
    showScreen(screenName) {
        document.querySelectorAll('.screen').forEach(s => s.classList.remove('active'));
        const screen = document.getElementById(`${screenName}-screen`);
        if (screen) screen.classList.add('active');
    },

    // 获取卡牌排序值（牌值）
    getCardSortValue(card) {
        // 字符串格式: "S3", "H7", "SJ", "HJ"
        // 对象格式: {value: number, suit: number}
        // 后端牌值定义: 3-10=3-10, 11=J, 12=Q, 13=K, 14=A, 15=2, 16=小王, 17=大王
        let value;
        if (typeof card === 'string') {
            value = card.substring(1);
        } else {
            // 对象格式: {value: number, suit: number}
            // 后端直接发送数字值
            const valueMap = { 3: '3', 4: '4', 5: '5', 6: '6', 7: '7', 8: '8', 9: '9', 10: '10',
                               11: 'J', 12: 'Q', 13: 'K', 14: 'A', 15: '2', 16: 'X', 17: 'D' };
            value = valueMap[card.value] || String(card.value);
        }
        
        // 排序值：3-10=3-10, J=11, Q=12, K=13, A=14, 2=15, 小王=16, 大王=17
        const sortValues = {
            '3': 3, '4': 4, '5': 5, '6': 6, '7': 7, '8': 8, '9': 9, '10': 10,
            'J': 11, 'Q': 12, 'K': 13, 'A': 14, '2': 15, 'X': 16, 'D': 17
        };
        return sortValues[value] || 0;
    },
    
    // 获取卡牌花色值
    getCardSuitValue(card) {
        // 字符串格式: "S3", "H7", "CK", "DJ"
        // 注意：J 在这里是 Joker（王），不是梅花
        // 梅花用 C 表示
        let suitCode;
        if (typeof card === 'string') {
            suitCode = card[0];
        } else {
            // 对象格式: {value: number, suit: number}
            // suit: 1=黑桃, 2=红桃, 3=梅花, 4=方块, 5=小王, 6=大王
            const suitMap = { 1: 'S', 2: 'H', 3: 'C', 4: 'D', 5: 'J', 6: 'J' };
            suitCode = suitMap[card.suit] || '?';
        }
        
        // 花色排序值：黑桃=4, 红桃=3, 梅花=2, 方块=1, 王=0
        const suitValues = { 'S': 4, 'H': 3, 'C': 2, 'D': 1, 'J': 0 };
        return suitValues[suitCode] || 0;
    },

    // 显示消息
    showMessage(screen, message, type) {
        const el = document.getElementById(`${screen}-message`);
        if (el) {
            el.textContent = message;
            el.className = `message ${type}`;
            setTimeout(() => { el.className = 'message'; }, 3000);
        }
    },

    // 渲染我的手牌
    renderMyCards(cards, selected = []) {
        const container = document.getElementById('my-cards');
        if (!container) return;
        
        container.innerHTML = '';
        
        // 排序手牌：按牌值排序（3最小，大小王最大），相同牌值按花色排序
        const sortedCards = [...cards].sort((a, b) => {
            const aVal = this.getCardSortValue(a);
            const bVal = this.getCardSortValue(b);
            if (aVal !== bVal) return aVal - bVal;
            // 牌值相同，按花色排序（黑桃 > 红桃 > 梅花 > 方块）
            const aSuit = this.getCardSuitValue(a);
            const bSuit = this.getCardSuitValue(b);
            return bSuit - aSuit;
        });
        
        sortedCards.forEach((card, index) => {
            const formatted = Game.formatCard(card);
            const cardEl = document.createElement('div');
            cardEl.className = `card ${formatted.isRed ? 'red' : 'black'}`;
            
            // 支持字符串格式和对象格式的选择判断
            const isSelected = selected.some(s => {
                if (typeof s === 'string' && typeof card === 'string') {
                    return s === card;
                } else if (typeof s === 'object' && typeof card === 'object') {
                    return s.value === card.value && s.suit === card.suit;
                } else {
                    return Game.formatCard(s).value === formatted.value && 
                           Game.formatCard(s).suit === formatted.suit;
                }
            });
            if (isSelected) {
                cardEl.classList.add('selected');
            }
            
            cardEl.innerHTML = `
                <span class="value">${formatted.value}</span>
                <span class="suit">${formatted.suit}</span>
            `;
            
            cardEl.addEventListener('click', () => {
                Game.toggleCard(card);
            });
            
            container.appendChild(cardEl);
        });
    },

    // 更新手牌数量
    updateCardCount(count) {
        const el = document.getElementById('my-card-count');
        if (el) el.textContent = count;
    },
    
    // 更新对手手牌数
    updateOpponentCardCount(uid, count) {
        if (UI.players[uid]) {
            const position = UI.players[uid].position;
            let cardCountEl = null;
            if (position === 'left') {
                cardCountEl = document.querySelector('#opponent-left .card-count span');
            } else if (position === 'right') {
                cardCountEl = document.querySelector('#opponent-right .card-count span');
            }
            if (cardCountEl) {
                cardCountEl.textContent = count;
            }
        }
    },

    // 更新角色显示
    updateRole(role) {
        const el = document.getElementById('my-role');
        if (el) el.textContent = role;
    },

    // 显示/隐藏叫地主面板
    showCallPanel() {
        const panel = document.getElementById('call-landlord-panel');
        if (panel) panel.classList.remove('hidden');
    },

    hideCallPanel() {
        const panel = document.getElementById('call-landlord-panel');
        if (panel) panel.classList.add('hidden');
    },
    
    // 显示取消托管按钮
    showCancelAIControlBtn() {
        const btn = document.getElementById('cancel-ai-btn');
        if (btn) btn.classList.remove('hidden');
    },
    
    // 隐藏取消托管按钮
    hideCancelAIControlBtn() {
        const btn = document.getElementById('cancel-ai-btn');
        if (btn) btn.classList.add('hidden');
    },

    // 计时器相关
    timerIntervals: {},
    
    // 显示指定玩家的计时器
    showTimerForPlayer(uid, seconds) {
        console.log('[Timer] Showing timer for:', uid, 'seconds:', seconds);
        
        // 隐藏所有定时器
        document.querySelectorAll('.timer-wrapper').forEach(wrapper => {
            wrapper.style.display = 'none';
        });
        
        // 清除所有定时器间隔
        Object.keys(this.timerIntervals).forEach(key => {
            clearInterval(this.timerIntervals[key]);
            delete this.timerIntervals[key];
        });
        
        // 根据玩家位置显示定时器
        let timerWrapper = null;
        let timerText = null;
        let timerCircle = null;
        
        if (uid === Game.myUid) {
            timerWrapper = document.getElementById('timer-me-wrapper');
            timerText = timerWrapper?.querySelector('.timer-text');
            timerCircle = timerWrapper?.querySelector('.circle');
        } else if (UI.players[uid] && UI.players[uid].position === 'left') {
            timerWrapper = document.getElementById('timer-left-wrapper');
            timerText = timerWrapper?.querySelector('.timer-text');
            timerCircle = timerWrapper?.querySelector('.circle');
        } else if (UI.players[uid] && UI.players[uid].position === 'right') {
            timerWrapper = document.getElementById('timer-right-wrapper');
            timerText = timerWrapper?.querySelector('.timer-text');
            timerCircle = timerWrapper?.querySelector('.circle');
        }
        
        console.log('[Timer] Found elements:', {timerWrapper: !!timerWrapper, timerText: !!timerText, timerCircle: !!timerCircle});
        
        if (timerWrapper && timerText && timerCircle) {
            timerWrapper.style.display = 'block';
            timerText.textContent = seconds;
            
            // 更新圆形进度条 - 已消耗时间百分比
            const percentage = ((15 - seconds) / 15) * 100;
            timerCircle.style.strokeDasharray = `${percentage}, 100`;
            
            // 更新定时器样式
            const timerClock = timerWrapper.querySelector('.timer-clock');
            this.updateTimerStyle(timerClock, seconds);
            
            // 启动倒计时
            const intervalId = setInterval(() => {
                seconds--;
                if (seconds >= 0) {
                    timerText.textContent = seconds;
                    
                    // 更新圆形进度条
                    const newPercentage = ((15 - seconds) / 15) * 100;
                    timerCircle.style.strokeDasharray = `${newPercentage}, 100`;
                    
                    this.updateTimerStyle(timerClock, seconds);
                }
                
                if (seconds <= 0) {
                    clearInterval(intervalId);
                    delete this.timerIntervals[uid];
                }
            }, 1000);
            
            this.timerIntervals[uid] = intervalId;
        }
    },
    
    // 更新计时器样式
    updateTimerStyle(el, remaining) {
        if (!el) return;
        if (remaining <= 5) {
            el.classList.add('warning');
            el.classList.remove('danger');
        } else {
            el.classList.remove('warning');
            el.classList.remove('danger');
        }
    },
    
    // 设置操作按钮状态
    setActionButtons(enabled) {
        const playBtn = document.getElementById('play-btn');
        const passBtn = document.getElementById('pass-btn');
        if (playBtn) playBtn.disabled = !enabled;
        if (passBtn) passBtn.disabled = !enabled;
    },

    // 高亮当前玩家
    highlightCurrentPlayer(uid, isPlayingPhase = false) {
        // 移除所有高亮
        document.querySelectorAll('.player-highlight').forEach(el => {
            el.classList.remove('player-highlight');
        });
        
        const isMyTurn = uid === Game.myUid;
        
        // 高亮当前玩家区域
        if (isMyTurn) {
            // 高亮自己的区域
            const myArea = document.querySelector('.my-area');
            if (myArea) myArea.classList.add('player-highlight');
        } else {
            // 只高亮当前出牌的那个对手
            if (UI.players[uid]) {
                const position = UI.players[uid].position;
                if (position === 'left') {
                    const leftOpponent = document.querySelector('#opponent-left');
                    if (leftOpponent) leftOpponent.classList.add('player-highlight');
                } else if (position === 'right') {
                    const rightOpponent = document.querySelector('#opponent-right');
                    if (rightOpponent) rightOpponent.classList.add('player-highlight');
                }
            } else {
                // 如果没有找到玩家位置信息，显示UID
                console.warn(`[UI] Player ${uid} not found in players map`);
            }
        }
        
        // 更新当前玩家文字提示
        const currentPlayerEl = document.getElementById('current-player');
        if (currentPlayerEl) {
            let playerName = '未知';
            if (isMyTurn) {
                playerName = '你';
            } else if (UI.players[uid]) {
                playerName = UI.players[uid].name;
            } else {
                playerName = uid.substring(0, 6);
            }
            if (isPlayingPhase) {
                currentPlayerEl.textContent = `轮到 ${playerName} 出牌`;
            } else {
                currentPlayerEl.textContent = `轮到 ${playerName} 叫地主`;
            }
        }
    },

    // 显示最后出牌
    showLastPlay(player, cards) {
        const display = document.getElementById('last-play-display');
        const playerEl = document.getElementById('last-player');
        const cardsEl = document.getElementById('last-cards');
        
        if (playerEl) playerEl.textContent = player;
        if (cardsEl) {
            if (cards === '不出') {
                cardsEl.innerHTML = '<span class="pass-text">不出</span>';
                cardsEl.className = '';
            } else if (Array.isArray(cards) && cards.length > 0) {
                // 生成可视化的牌
                const cardHtml = cards.map(c => {
                    const f = Game.formatCard(c);
                    const colorClass = f.isRed ? 'red' : 'black';
                    return `<span class="played-card ${colorClass}">${f.suit}${f.value}</span>`;
                }).join('');
                cardsEl.innerHTML = cardHtml;
                cardsEl.className = 'cards-display';
            } else {
                cardsEl.innerHTML = '-';
                cardsEl.className = '';
            }
        }
        
        // 显示出牌动画
        if (display && cards !== '不出' && Array.isArray(cards) && cards.length > 0) {
            display.classList.add('played');
            setTimeout(() => {
                display.classList.remove('played');
            }, 500);
        }
    },

    // 更新玩家数量
    updatePlayerCount(count) {
        // 可以在房间界面显示当前玩家数量
        console.log(`[UI] Players in room: ${count}`);
    },

    // 标记玩家准备
    markPlayerReady(uid) {
        const playerEl = document.querySelector(`.player-item[data-uid="${uid}"]`);
        if (playerEl) playerEl.classList.add('ready');
    },
    
    // 设置玩家信息
    setPlayers(playerData, landlordUid, myUid) {
        console.log('[UI] setPlayers called:', {playerData, landlordUid, myUid});
        this.players = {};
        const otherPlayers = [];
        
        if (Array.isArray(playerData)) {
            for (const p of playerData) {
                const uid = p.uid || p;
                const isLandlord = p.is_landlord || uid === landlordUid;
                const name = p.nickname || (p.is_bot ? '机器人' : '玩家');
                this.players[uid] = { 
                    position: uid === myUid ? 'my' : '', 
                    name: uid === myUid ? '我' : name, 
                    isLandlord: isLandlord 
                };
                console.log('[UI] Added player:', {uid, name, isLandlord, p});
                if (uid !== myUid) {
                    otherPlayers.push(uid);
                }
            }
        } else {
            const uids = Array.isArray(playerData) ? playerData : [];
            for (const uid of uids) {
                if (uid !== myUid) {
                    otherPlayers.push(uid);
                    this.players[uid] = { position: '', name: '机器人', isLandlord: uid === landlordUid };
                }
            }
            this.players[myUid] = { position: 'my', name: '我', isLandlord: myUid === landlordUid };
        }
        
        if (otherPlayers.length >= 1) {
            this.players[otherPlayers[0]].position = 'left';
        }
        if (otherPlayers.length >= 2) {
            this.players[otherPlayers[1]].position = 'right';
        }
        
        const leftPlayer = document.querySelector('#opponent-left .player-name');
        const rightPlayer = document.querySelector('#opponent-right .player-name');
        const leftRole = document.querySelector('#opponent-left .player-role');
        const rightRole = document.querySelector('#opponent-right .player-role');
        
        if (leftPlayer && this.players[otherPlayers[0]]) {
            leftPlayer.textContent = this.players[otherPlayers[0]].name;
        }
        if (rightPlayer && this.players[otherPlayers[1]]) {
            rightPlayer.textContent = this.players[otherPlayers[1]].name;
        }
        
        if (leftRole && this.players[otherPlayers[0]]) {
            leftRole.textContent = this.players[otherPlayers[0]].isLandlord ? '👑 地主' : '👤 农民';
            leftRole.className = 'player-role' + (this.players[otherPlayers[0]].isLandlord ? ' landlord' : ' peasant');
        }
        if (rightRole && this.players[otherPlayers[1]]) {
            rightRole.textContent = this.players[otherPlayers[1]].isLandlord ? '👑 地主' : '👤 农民';
            rightRole.className = 'player-role' + (this.players[otherPlayers[1]].isLandlord ? ' landlord' : ' peasant');
        }
        
        const myRole = document.getElementById('my-role');
        if (myRole) {
            if (myUid === landlordUid) {
                myRole.textContent = '👑 地主';
            } else {
                myRole.textContent = '👤 农民';
            }
        }
    },
    
    // 显示底牌
    showBottomCards(cards, isRevealed = true) {
        const bottomCardsArea = document.getElementById('bottom-cards-area');
        const bottomCardsDiv = document.getElementById('bottom-cards');
        if (!bottomCardsDiv) {
            return;
        }
        bottomCardsDiv.innerHTML = '';
        
        for (const card of cards) {
            const cardDiv = document.createElement('div');
            if (isRevealed) {
                const formatted = Game.formatCard(card);
                cardDiv.className = `card small ${formatted.isRed ? 'red' : 'black'}`;
                cardDiv.innerHTML = `<span class="value">${formatted.value}</span><span class="suit">${formatted.suit}</span>`;
            } else {
                cardDiv.className = 'card small card-back';
                cardDiv.textContent = '?';
            }
            bottomCardsDiv.appendChild(cardDiv);
        }
        bottomCardsArea.style.display = 'block';
    },
    
    // 隐藏底牌
    hideBottomCards() {
        const bottomCardsArea = document.getElementById('bottom-cards-area');
        if (bottomCardsArea) {
            bottomCardsArea.style.display = 'none';
        }
    },
    
    // 翻底牌（从背面翻到正面）
    flipBottomCards() {
        const bottomCardsDiv = document.getElementById('bottom-cards');
        if (!bottomCardsDiv) {
            return;
        }
        const cardBacks = bottomCardsDiv.querySelectorAll('.card-back');
        cardBacks.forEach((cardDiv, index) => {
            // 延迟翻牌效果
            setTimeout(() => {
                cardDiv.classList.remove('card-back');
                cardDiv.textContent = '';
            }, index * 200);
        });
    },
    
    // 显示玩家出牌
    showPlayerPlay(uid, cards) {
        let playArea = null;
        let playerName = '我';
        
        if (this.players[uid]) {
            if (this.players[uid].position === 'left') {
                playArea = document.getElementById('opponent-left-play');
                playerName = this.players[uid].name;
            } else if (this.players[uid].position === 'right') {
                playArea = document.getElementById('opponent-right-play');
                playerName = this.players[uid].name;
            } else {
                // 我的出牌显示在中间区域
                UI.showLastPlay('我', cards);
                return;
            }
        } else {
            // 未知玩家，显示在中间
            UI.showLastPlay(uid.substring(0, 4), cards);
            return;
        }
        
        if (!playArea) return;
        
        // 清空之前的
        playArea.innerHTML = '';
        
        if (cards === '不出') {
            playArea.innerHTML = '<span class="small-pass">不出</span>';
        } else if (Array.isArray(cards) && cards.length > 0) {
            const cardHtml = cards.map(c => {
                const f = Game.formatCard(c);
                const colorClass = f.isRed ? 'red' : 'black';
                return `<span class="small-played-card ${colorClass}">${f.suit}${f.value}</span>`;
            }).join('');
            playArea.innerHTML = cardHtml;
        }
        
        // 添加动画
        playArea.classList.add('play-active');
        setTimeout(() => {
            playArea.classList.remove('play-active');
        }, 500);
        
        // 中间区域也显示最后一次出牌
        UI.showLastPlay(playerName, cards);
    },
    
    // 更新历史出牌记录面板
    updateHistoryPanel(records, totalPlayed) {
        const historyList = document.getElementById('history-list');
        const totalCountEl = document.getElementById('total-played-count');
        
        if (totalCountEl) {
            totalCountEl.textContent = totalPlayed;
        }
        
        if (!historyList) return;
        
        historyList.innerHTML = '';
        
        records.forEach((record, index) => {
            const item = document.createElement('div');
            item.className = 'history-item';
            
            const playerSpan = document.createElement('span');
            playerSpan.className = 'history-player';
            playerSpan.textContent = record.playerName;
            
            const cardsSpan = document.createElement('span');
            cardsSpan.className = 'history-cards';
            cardsSpan.textContent = record.cardStr;
            
            let patternSpan = null;
            if (record.pattern && record.pattern !== 'single') {
                patternSpan = document.createElement('span');
                patternSpan.className = 'history-pattern';
                const patternNames = {
                    'pair': '对子',
                    'triple': '三张',
                    'triple_with_one': '三带一',
                    'triple_with_pair': '三带二',
                    'bomb': '炸弹',
                    'king_bomb': '王炸',
                    'straight': '顺子',
                    'double_straight': '连对',
                    'triple_straight': '飞机',
                    'triple_straight_with_one': '飞机带翅膀',
                    'triple_straight_with_pair': '飞机带对子'
                };
                patternSpan.textContent = patternNames[record.pattern] || record.pattern;
            }
            
            item.appendChild(playerSpan);
            item.appendChild(cardsSpan);
            if (patternSpan) {
                item.appendChild(patternSpan);
            }
            
            historyList.appendChild(item);
        });
        
        // 自动滚动到底部
        historyList.scrollTop = historyList.scrollHeight;
    },

    // 清空出牌历史
    clearPlayHistory() {
        const historyList = document.getElementById('history-list');
        const totalCountEl = document.getElementById('total-played-count');
        
        if (totalCountEl) {
            totalCountEl.textContent = '0';
        }
        
        if (historyList) {
            historyList.innerHTML = '';
        }
        
        // 重置Game对象的历史记录
        Game.historyRecords = [];
        Game.totalPlayedCount = 0;
    },

    // 清空最后出牌显示
    clearLastPlay() {
        const display = document.getElementById('last-play-display');
        const playerEl = document.getElementById('last-player');
        const cardsEl = document.getElementById('last-cards');
        
        if (playerEl) playerEl.textContent = '-';
        if (cardsEl) cardsEl.innerHTML = '-';
        
        // 清空对手的出牌显示
        document.getElementById('opponent-left-play').innerHTML = '';
        document.getElementById('opponent-right-play').innerHTML = '';
    },
    
    // 显示机器人加入倒计时
    showBotCountdown(seconds) {
        const countdownDiv = document.getElementById('bot-countdown');
        const secondsSpan = document.getElementById('countdown-seconds');
        if (countdownDiv && secondsSpan) {
            countdownDiv.classList.remove('hidden');
            secondsSpan.textContent = seconds;
        }
    },
    
    // 隐藏机器人加入倒计时
    hideBotCountdown() {
        const countdownDiv = document.getElementById('bot-countdown');
        if (countdownDiv) {
            countdownDiv.classList.add('hidden');
        }
    }
};
