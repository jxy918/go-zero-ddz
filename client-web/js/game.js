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
                // 清理上一局的游戏状态
                this.myCards = [];
                this.selectedCards = [];
                this.landlordUid = null;
                this.currentTurnUid = null;
                this.players = {};
                this.bottomCards = [];
                console.log('[Game] Game state cleared for new room');
                
                this.roomId = data.room_id;
                console.log(`[Game] Room created successfully: roomId=${this.roomId}, myUid=${this.myUid}`);
                UI.showScreen('room');
                document.getElementById('current-room-id').textContent = this.roomId;
                UI.showMessage('room', '房间创建成功', 'success');
                
                // 清理 UI 状态
                UI.clearPlayHistory();
                UI.clearLastPlay();
                UI.hideBottomCards();
                
                // 清理 UI 玩家数据（关键修复：清空上一局的机器人数据）
                UI.players = {};
                console.log('[Game] UI.players cleared for new room');
                
                // 清理房间玩家列表（关键修复：清空上一局的机器人数据）
                const playersList = document.getElementById('players-list');
                if (playersList) {
                    playersList.innerHTML = '';
                    console.log('[Game] Players list cleared for new room');
                }
                
                // 重置玩家计数
                UI.updatePlayerCount(1);
                
                // 重置对手手牌显示
                const leftCardCount = document.querySelector('#opponent-left .card-count span');
                const rightCardCount = document.querySelector('#opponent-right .card-count span');
                if (leftCardCount) leftCardCount.textContent = '0';
                if (rightCardCount) rightCardCount.textContent = '0';
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
                UI.hideBotCountdown();
                // 更新玩家列表UI
                if (data.players && data.players.length > 0) {
                    UI.renderRoomPlayerList(data.players, this.myUid);
                } else if (data.uid) {
                    // 如果没有players数组，使用当前单个玩家数据
                    const playerData = {
                        uid: data.uid,
                        nickname: data.nickname,
                        is_bot: data.is_bot,
                        is_ready: data.is_ready
                    };
                    // 更新房间玩家列表（追加模式）
                    const playersList = document.getElementById('players-list');
                    if (playersList) {
                        const existingItem = playersList.querySelector(`[data-uid="${data.uid}"]`);
                        if (!existingItem) {
                            console.log(`[Game] Adding player to room list: ${data.uid}`);
                            UI.addRoomPlayer(playerData, this.myUid);
                        } else {
                            console.log(`[Game] Player already in list: ${data.uid}`);
                        }
                    } else {
                        console.log('[Game] players-list element not found');
                    }
                }
            } else if (data.event === 'player_ready') {
                UI.markPlayerReady(data.uid);
            } else if (data.event === 'bot_join_countdown') {
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
            
            // 显示底牌：发牌阶段显示盖着的，地主确认后显示翻开的
            if (data.bottom_cards && data.bottom_cards.length > 0) {
                // 保存底牌供后续使用
                this.bottomCards = data.bottom_cards;

                // 仅在"无 landlord_uid"或"landlord_uid === ''"（即叫地主阶段）时盖住底牌
                // 当地主已确认（landlord_uid 存在且非空）时不要再次重置底牌显示，
                // 因为底牌已在 CALL_LANDLORD_NOTIFY 中翻开展示。
                if (!data.landlord_uid) {
                    console.log('[Game] SHOWING BOTTOM CARDS during DEAL_CARDS_NOTIFY (call phase), keeping hidden');
                    UI.showBottomCards(data.bottom_cards, false);
                } else {
                    console.log('[Game] Landlord already confirmed, skip resetting bottom cards display');
                }
            }
            
            // 设置玩家信息（包含地主标记）- 必须在 counts 处理之前
            if (data.players && data.players.length > 0) {
                console.log('[Game] Setting players:', data.players, 'landlord:', data.landlord_uid);
                // 如果地主UID为空，使用已保存的地主UID
                const effectiveLandlordUid = data.landlord_uid || this.landlordUid || "";
                console.log('[Game] Effective landlord_uid:', effectiveLandlordUid);
                UI.setPlayers(data.players, effectiveLandlordUid, this.myUid);
                // 同步更新 Game.players 对象
                for (const p of data.players) {
                    const uid = p.uid;
                    if (!uid) {
                        console.warn('[Game] Player missing uid:', p);
                        continue;
                    }
                    const isBot = p.is_bot || uid.includes('bot');
                    let name = p.nickname;
                    if (!name) {
                        if (isBot) {
                            const botMatch = uid.match(/-(\d+)$/);
                            name = botMatch ? `机器人${botMatch[1]}` : uid;
                        } else {
                            name = uid.substring(0, 6);
                        }
                    }
                    this.players[uid] = { name: uid === this.myUid ? '我' : name };
                }
            }
            
            // 更新所有玩家手牌数量 - 必须在 setPlayers 之后，因为 updateOpponentCardCount 需要 UI.players
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
            
            // 首次发牌或地主确认后都切换到游戏界面
            UI.showScreen('game');
            
            if (data.your_cards === undefined && data.my_cards !== undefined) {
                UI.showBroadcastMessage('🃏 游戏开始！');
            } else if (data.is_landlord) {
                UI.showBroadcastMessage('👑 你成为地主！');
            }
        });

        // 叫地主通知
        WS.on(WS.MSG.CALL_LANDLORD_NOTIFY, (data) => {
            console.log('[Game] Call landlord:', data);
            console.log('[Game] myUid:', this.myUid);
            console.log('[Game] players:', this.players);
            
            // 确保在叫地主阶段切换到游戏界面
            UI.showScreen('game');
            
            // 优先处理"轮到叫地主"消息（turn=true）- 只显示在中间广播区，不在上方蓝条重复显示
            if (data.turn === true && data.uid !== undefined) {
                console.log('[Game] It is someone turn to call');
                if (data.uid === this.myUid) {
                    console.log('[Game] It is my turn to call');
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
                    // 中间广播区显示"轮到叫地主"
                    UI.showBroadcastMessage(`🎤 轮到${playerName}叫地主`);
                }
                // 只高亮玩家区域，不更新上方蓝条的文字（避免重复显示"轮到xxx叫地主"）
                UI.highlightCurrentPlayerWithoutText(data.uid);
                return;
            }
            
            // 处理叫地主结果（包含 action 和 score）- 显示在上方蓝条
            if (data.uid !== undefined && data.action !== undefined) {
                console.log(`[Game] Processing call landlord result: uid=${data.uid}, action=${data.action}, score=${data.score}, turn=${data.turn}, landlord_uid=${data.landlord_uid}`);
                
                // 如果包含 landlord_uid，说明是地主确认消息
                if (data.landlord_uid !== undefined && data.landlord_uid !== '') {
                    console.log('[Game] This is a landlord confirmation message with call result');
                    // 处理地主确认
                    this.landlordUid = data.landlord_uid;
                    if (data.players && data.players.length > 0) {
                        UI.setPlayers(data.players, data.landlord_uid, this.myUid);
                    }
                    UI.updateRole(this.landlordUid === this.myUid ? '地主' : '农民');
                    
                    // 先显示地主确认消息
                    const landlordName = data.players?.find(p => p.uid === data.landlord_uid)?.nickname || '玩家';
                    UI.showBroadcastMessage(`👑 ${landlordName} 成为地主！`);
                    
                    // 显示底牌（翻开展示）
                    if (data.bottom_cards && data.bottom_cards.length > 0) {
                        UI.showBottomCards(data.bottom_cards, true);
                        // 延迟显示底牌广播，避免被地主确认消息覆盖
                        setTimeout(() => {
                            UI.showBroadcastMessage(`📦 底牌: ${this.cardsToString(data.bottom_cards)}`);
                        }, 1500);
                    }
                    
                    console.log('[Game] Landlord confirmed, hiding call panel');
                    UI.hideCallPanel();
                    
                    // 设置当前回合和按钮状态
                    if (data.current_turn_uid) {
                        this.currentTurnUid = data.current_turn_uid;
                        const isMyTurn = data.current_turn_uid === this.myUid;
                        console.log(`[Game] Current turn uid: ${data.current_turn_uid}, myUid: ${this.myUid}, isMyTurn: ${isMyTurn}`);
                        UI.setActionButtons(isMyTurn);
                        UI.highlightCurrentPlayer(data.current_turn_uid, true);
                    }
                    return;
                }
                
                // 显示叫地主结果（叫地主或不叫）- 在上方蓝条显示
                let playerName = this.players[data.uid]?.name;
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
                
                // 在上方蓝条显示叫地主结果
                const currentPlayerEl = document.getElementById('current-player');
                if (currentPlayerEl) {
                    if (data.action === 1 && data.score > 0) {
                        currentPlayerEl.textContent = `${playerName} 叫地主 ${data.score}分`;
                    } else {
                        currentPlayerEl.textContent = `${playerName} 不叫`;
                    }
                }
                
                // 如果是自己叫地主，隐藏叫地主面板
                if (data.uid === this.myUid) {
                    UI.hideCallPanel();
                }
                
            } else {
                console.log('[Game] CALL_LANDLORD_NOTIFY received but no matching condition:', data);
            }
        });

        // 出牌通知
        WS.on(WS.MSG.PLAY_CARDS_NOTIFY, (data) => {
            console.log('[Game] Play cards:', data);
            
            // 进入打牌阶段，延迟隐藏底牌（让用户有时间看到底牌）
            // 只有当有实际出牌时才隐藏底牌（data.uid不为空）
            if (data.uid && data.uid !== '') {
                UI.hideBottomCards();
            }
            
            // 如果消息包含 landlord_uid，设置 this.landlordUid（确保后续 TIMER_NOTIFY 能正确判断阶段）
            if (data.landlord_uid) {
                this.landlordUid = data.landlord_uid;
                console.log('[Game] Set landlordUid from PLAY_CARDS_NOTIFY:', data.landlord_uid);
            }
            
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
                
                // 获取玩家昵称（统一处理机器人）
                let playerName = this.players[data.uid]?.name;
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
                
                // 显示最后出牌
                UI.showLastPlay(playerName, data.cards);
                
                // 添加到历史记录
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
            // 优先从结果中解析玩家名称（保证赢家显示为正确名称）
            let winnerText = '对手';
            if (data.winner_uid === this.myUid) {
                winnerText = '你';
            } else if (data.winner_uid && data.winner_uid.includes('bot')) {
                const botMatch = data.winner_uid.match(/-(\d+)$/);
                winnerText = botMatch ? `机器人${botMatch[1]}` : data.winner_uid.substring(0, 6);
            } else if (data.results && data.results.length > 0) {
                const winner = data.results.find(r => r.uid === data.winner_uid);
                if (winner && winner.is_landlord !== undefined) {
                    // 地主/农民身份标记
                    winnerText = winner.is_landlord ? '地主' : '农民';
                }
            }
            this.showSettlement(data, winnerText);
        });

        // 计时器通知
        WS.on(WS.MSG.TIMER_NOTIFY, (data) => {
            console.log('[Game] Timer:', data);
            this.currentTurnUid = data.current_turn_uid;
            const isMyTurn = data.current_turn_uid === this.myUid;
            
            // 处理宽限期警告（warning=true, grace=true）
            // 使用宽松判断：data.warning === true 或 data.warning === "true"
            const isGraceWarning = (data.warning === true || data.warning === "true") && 
                                   (data.grace === true || data.grace === "true");
            if (isGraceWarning && isMyTurn) {
                console.log('[Game] Grace period warning: AI will take over in', data.remaining_seconds, 'seconds');
                // 在玩家出牌区域显示警告，而不是广播区域
                UI.showGraceWarning(data.remaining_seconds);
                UI.showTimerForPlayer(data.current_turn_uid, data.remaining_seconds);
                return;
            }
            
            // 更新所有玩家区域的定时器显示
            UI.showTimerForPlayer(data.current_turn_uid, data.remaining_seconds);
            
            UI.setActionButtons(isMyTurn);
            
            // 判断当前阶段：多种方式判断是否是打牌阶段
            // 1. 如果 this.landlordUid 已设置
            // 2. 如果消息中包含 landlord_uid（后端可能发送）
            // 3. 如果 UI.players 中有地主标记
            // 4. 如果当前玩家是地主（打牌阶段地主先出牌）
            let isPlayingPhase = !!this.landlordUid;
            
            // 兜底检查：如果 landlordUid 未设置，尝试从其他来源获取
            if (!isPlayingPhase) {
                // 检查消息是否包含 landlord_uid
                if (data.landlord_uid) {
                    this.landlordUid = data.landlord_uid;
                    isPlayingPhase = true;
                    console.log('[Game] Set landlordUid from TIMER_NOTIFY:', data.landlord_uid);
                }
                // 检查 UI.players 中是否有地主
                else if (UI.players) {
                    for (const uid in UI.players) {
                        if (UI.players[uid].isLandlord) {
                            this.landlordUid = uid;
                            isPlayingPhase = true;
                            console.log('[Game] Set landlordUid from UI.players:', uid);
                            break;
                        }
                    }
                }
            }
            
            // 通用的玩家名称解析（优先级：Game.players > UI.players > 结构化解析）
            let playerName = this.players[data.current_turn_uid]?.name
                || (UI.players && UI.players[data.current_turn_uid]?.name);
            if (!playerName) {
                if (data.current_turn_uid === this.myUid) {
                    playerName = '我';
                } else if (data.current_turn_uid.includes('bot')) {
                    const botMatch = data.current_turn_uid.match(/-(\d+)$/);
                    playerName = botMatch ? `机器人${botMatch[1]}` : data.current_turn_uid.substring(0, 6);
                } else {
                    playerName = data.current_turn_uid.substring(0, 6);
                }
            }
            
            // 叫地主阶段：只高亮玩家区域，同时在中间广播"轮到xxx叫地主"
            // 打牌阶段：高亮玩家区域 + 上方蓝条 + 中间广播"轮到xxx出牌"
            if (isPlayingPhase) {
                UI.highlightCurrentPlayer(data.current_turn_uid, true);
                UI.showBroadcastMessage(`🎴 轮到${playerName}出牌`);
            } else {
                UI.highlightCurrentPlayerWithoutText(data.current_turn_uid);
            }
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
                    UI.showBroadcastMessage(`📦 底牌: ${this.cardsToString(data.bottom_cards)}`);
                }
                
                // 如果之前显示过未翻开的底牌，现在翻开
                if (data.landlord_uid && this.bottomCards && this.bottomCards.length > 0) {
                    UI.showBottomCards(this.bottomCards, true);
                    UI.showBroadcastMessage(`📦 底牌: ${this.cardsToString(this.bottomCards)}`);
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
        
        // 玩家主动出牌，隐藏宽限期警告
        UI.hideGraceWarning();
        
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
        // 玩家主动不出，隐藏宽限期警告
        UI.hideGraceWarning();
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
                
                // 优先从 Game.players 取名称，否则根据 uid 推导
                let displayName = Game.players[result.uid]?.name;
                if (!displayName) {
                    if (result.uid === Game.myUid) {
                        displayName = '我';
                    } else if (result.uid.includes('bot')) {
                        const botMatch = result.uid.match(/-(\d+)$/);
                        displayName = botMatch ? `机器人${botMatch[1]}` : result.uid.substring(0, 6);
                    } else {
                        displayName = result.uid.substring(0, 6);
                    }
                }
                // 带地主/农民标记
                const roleTag = result.is_landlord ? '（地主）' : '（农民）';
                const scoreClass = result.score_change >= 0 ? 'positive' : 'negative';
                const scoreText = result.score_change >= 0 ? `+${result.score_change}` : result.score_change;
                
                resultItem.innerHTML = `
                    <span class="player-name">${displayName}${roleTag}</span>
                    <span class="score-change ${scoreClass}">${scoreText}</span>
                `;
                
                resultsContainer.appendChild(resultItem);
            });
        } else {
            // 当后端没有传 results 时，显示默认提示（而不是空白）
            const emptyTip = document.createElement('div');
            emptyTip.className = 'result-item';
            emptyTip.innerHTML = `<span class="player-name">（暂无战绩数据）</span><span class="score-change">0</span>`;
            resultsContainer.appendChild(emptyTip);
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
        console.log(`[UI] showScreen called with: ${screenName}`);
        document.querySelectorAll('.screen').forEach(s => {
            const wasActive = s.classList.contains('active');
            s.classList.remove('active');
            if (wasActive) {
                console.log(`[UI] Screen ${s.id} deactivated`);
            }
        });
        const screen = document.getElementById(`${screenName}-screen`);
        if (screen) {
            screen.classList.add('active');
            console.log(`[UI] Screen ${screen.id} activated`);
        } else {
            console.error(`[UI] Screen ${screenName}-screen not found!`);
        }
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

    // 显示广播消息（游戏中间区域）
    showBroadcastMessage(message) {
        const el = document.getElementById('broadcast-message');
        if (el) {
            el.textContent = message;
            el.style.display = 'block';
            // 3秒后隐藏
            setTimeout(() => {
                el.style.display = 'none';
            }, 3000);
        }
    },

    // 显示宽限期警告（在玩家出牌区域）
    showGraceWarning(seconds) {
        console.log('[UI] showGraceWarning called, seconds:', seconds);
        // 查找.my-area元素（底部我的手牌区域）
        const myArea = document.querySelector('.my-area');
        if (myArea) {
            // 移除已有的警告
            const existingWarning = myArea.querySelector('.grace-warning');
            if (existingWarning) existingWarning.remove();

            // 创建警告元素
            const warningDiv = document.createElement('div');
            warningDiv.className = 'grace-warning';
            warningDiv.innerHTML = `<span class="warning-icon">⚠️</span> 即将托管！请在 <span class="warning-seconds">${seconds}</span> 秒内出牌`;
            warningDiv.style.cssText = `
                background: linear-gradient(135deg, #ff6b6b, #ee5a24);
                color: white;
                padding: 10px 15px;
                border-radius: 8px;
                font-size: 14px;
                font-weight: bold;
                text-align: center;
                margin-top: 10px;
                animation: pulse 1s infinite;
                box-shadow: 0 4px 15px rgba(238, 90, 36, 0.4);
            `;
            myArea.appendChild(warningDiv);
            console.log('[UI] Grace warning displayed in .my-area');
        } else {
            console.error('[UI] .my-area not found');
        }
    },

    // 隐藏宽限期警告
    hideGraceWarning() {
        console.log('[UI] hideGraceWarning called');
        const warningDivs = document.querySelectorAll('.grace-warning');
        warningDivs.forEach(div => div.remove());
        console.log('[UI] Grace warning hidden, removed', warningDivs.length, 'elements');
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
        console.log('[UI] updateOpponentCardCount called:', {uid, count, players: this.players});
        if (UI.players[uid]) {
            const position = UI.players[uid].position;
            console.log('[UI] Found player position:', position);
            let cardCountEl = null;
            if (position === 'left') {
                cardCountEl = document.querySelector('#opponent-left .card-count span');
            } else if (position === 'right') {
                cardCountEl = document.querySelector('#opponent-right .card-count span');
            }
            if (cardCountEl) {
                cardCountEl.textContent = count;
                console.log('[UI] Updated card count for', position, 'to', count);
            } else {
                console.log('[UI] Card count element not found for position:', position);
            }
        } else {
            console.log('[UI] Player not found in UI.players:', uid);
        }
    },

    // 更新角色显示
    updateRole(role) {
        const el = document.getElementById('my-role');
        if (el) el.textContent = role;
    },

    // 显示/隐藏叫地主面板
    showCallPanel() {
        const overlay = document.getElementById('call-landlord-overlay');
        if (overlay) overlay.classList.remove('hidden');
    },

    hideCallPanel() {
        const overlay = document.getElementById('call-landlord-overlay');
        if (overlay) overlay.classList.add('hidden');
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
    
    // 显示宽限期警告（在玩家出牌区域）
    showGraceWarning(seconds) {
        // 在玩家出牌区域显示警告
        const myPlayArea = document.querySelector('#my-area .play-area');
        if (myPlayArea) {
            // 清除之前的警告
            const existingWarning = myPlayArea.querySelector('.grace-warning');
            if (existingWarning) existingWarning.remove();
            
            // 创建警告元素
            const warningDiv = document.createElement('div');
            warningDiv.className = 'grace-warning';
            warningDiv.innerHTML = `<span class="warning-icon">⚠️</span> 即将托管！请在 <span class="warning-seconds">${seconds}</span> 秒内出牌`;
            warningDiv.style.cssText = `
                background: linear-gradient(135deg, #ff6b6b, #ee5a24);
                color: white;
                padding: 10px 15px;
                border-radius: 8px;
                font-size: 14px;
                font-weight: bold;
                text-align: center;
                margin-top: 10px;
                animation: pulse 1s infinite;
                box-shadow: 0 4px 15px rgba(238, 90, 36, 0.4);
            `;
            myPlayArea.appendChild(warningDiv);
            
            // 添加动画样式
            if (!document.getElementById('grace-warning-style')) {
                const style = document.createElement('style');
                style.id = 'grace-warning-style';
                style.textContent = `
                    @keyframes pulse {
                        0%, 100% { transform: scale(1); }
                        50% { transform: scale(1.05); }
                    }
                `;
                document.head.appendChild(style);
            }
        }
    },
    
    // 隐藏宽限期警告
    hideGraceWarning() {
        const warningDivs = document.querySelectorAll('.grace-warning');
        warningDivs.forEach(div => div.remove());
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
    // 只高亮玩家区域，不更新文字（用于避免重复显示"轮到xxx叫地主"）
    highlightCurrentPlayerWithoutText(uid) {
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
                console.warn(`[UI] Player ${uid} not found in players map`);
            }
        }
        // 注意：不更新 current-player 的文本内容
    },
    
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
        console.log(`[UI] Players in room: ${count}`);
    },
    
    // 渲染房间玩家列表
    renderRoomPlayerList(players, myUid) {
        const playersList = document.getElementById('players-list');
        if (!playersList) return;
        
        playersList.innerHTML = '';
        
        for (const p of players) {
            const uid = p.uid || p;
            const isBot = p.is_bot || uid.includes('bot');
            const isReady = p.is_ready || false;
            const isMe = uid === myUid;
            
            let name = p.nickname;
            if (!name) {
                if (isBot) {
                    const botMatch = uid.match(/bot-\w+-(\d+)/);
                    const botNum = botMatch ? botMatch[1] : '?';
                    name = `机器人${botNum}`;
                } else {
                    name = '玩家';
                }
            }
            
            const playerItem = document.createElement('div');
            playerItem.className = `player-item ${isMe ? 'me' : ''} ${isReady ? 'ready' : ''}`;
            playerItem.setAttribute('data-uid', uid);
            
            const playerIcon = document.createElement('div');
            playerIcon.className = `player-icon ${isBot ? 'bot' : ''}`;
            playerIcon.textContent = isBot ? '🤖' : '👤';
            
            const playerName = document.createElement('div');
            playerName.className = 'player-name';
            playerName.textContent = isMe ? '我' : name;
            
            const playerStatus = document.createElement('div');
            playerStatus.className = 'player-status';
            playerStatus.textContent = isReady ? '✓ 已准备' : '○ 等待中';
            
            playerItem.appendChild(playerIcon);
            playerItem.appendChild(playerName);
            playerItem.appendChild(playerStatus);
            
            playersList.appendChild(playerItem);
        }
    },

    // 添加单个玩家到房间列表
    addRoomPlayer(player, myUid) {
        const playersList = document.getElementById('players-list');
        if (!playersList) return;

        const uid = player.uid;
        const isBot = player.is_bot || uid.includes('bot');
        const isReady = player.is_ready || false;
        const isMe = uid === myUid;

        let name = player.nickname;
        if (!name) {
            if (isBot) {
                const botMatch = uid.match(/bot-\w+-(\d+)/);
                const botNum = botMatch ? botMatch[1] : '?';
                name = `机器人${botNum}`;
            } else {
                name = '玩家';
            }
        }

        const playerItem = document.createElement('div');
        playerItem.className = `player-item ${isMe ? 'me' : ''} ${isReady ? 'ready' : ''}`;
        playerItem.setAttribute('data-uid', uid);

        const playerIcon = document.createElement('div');
        playerIcon.className = `player-icon ${isBot ? 'bot' : ''}`;
        playerIcon.textContent = isBot ? '🤖' : '👤';

        const playerName = document.createElement('div');
        playerName.className = 'player-name';
        playerName.textContent = isMe ? '我' : name;

        const playerStatus = document.createElement('div');
        playerStatus.className = 'player-status';
        playerStatus.textContent = isReady ? '✓ 已准备' : '○ 等待中';

        playerItem.appendChild(playerIcon);
        playerItem.appendChild(playerName);
        playerItem.appendChild(playerStatus);

        playersList.appendChild(playerItem);
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
                let name = p.nickname;
                if (!name) {
                    if (p.is_bot || uid.includes('bot')) {
                        const botMatch = uid.match(/-(\d+)$/);
                        const botNum = botMatch ? botMatch[1] : '?';
                        name = `机器人${botNum}`;
                    } else {
                        name = '玩家';
                    }
                }
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
                    let name = '玩家';
                    if (uid.includes('bot')) {
                        const botMatch = uid.match(/-(\d+)$/);
                        const botNum = botMatch ? botMatch[1] : '?';
                        name = `机器人${botNum}`;
                    }
                    this.players[uid] = { position: '', name: name, isLandlord: uid === landlordUid };
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
            leftRole.classList.remove('landlord', 'peasant');
            leftRole.classList.add(this.players[otherPlayers[0]].isLandlord ? 'landlord' : 'peasant');
        }
        if (rightRole && this.players[otherPlayers[1]]) {
            rightRole.textContent = this.players[otherPlayers[1]].isLandlord ? '👑 地主' : '👤 农民';
            rightRole.classList.remove('landlord', 'peasant');
            rightRole.classList.add(this.players[otherPlayers[1]].isLandlord ? 'landlord' : 'peasant');
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
        if (!bottomCardsDiv || !bottomCardsArea) {
            console.log('[UI] bottom-cards-area or bottom-cards element not found');
            return;
        }
        bottomCardsDiv.innerHTML = '';
        
        console.log(`[UI] showBottomCards: ${cards.length} cards, isRevealed=${isRevealed}`);
        
        for (const card of cards) {
            const cardDiv = document.createElement('div');
            if (isRevealed) {
                const formatted = Game.formatCard(card);
                cardDiv.className = `card small ${formatted.isRed ? 'red' : 'black'}`;
                cardDiv.innerHTML = `<span class="value">${formatted.value}</span><span class="suit">${formatted.suit}</span>`;
            } else {
                // 未翻开的底牌，显示牌背
                cardDiv.className = 'card small card-back';
                cardDiv.innerHTML = '<span class="card-back-icon">🂠</span>';
                // 确保牌背样式正确应用
                cardDiv.style.background = '#1a472a';
                cardDiv.style.border = '2px solid #d4af37';
                cardDiv.style.borderRadius = '4px';
                cardDiv.style.width = '40px';
                cardDiv.style.height = '56px';
                cardDiv.style.display = 'flex';
                cardDiv.style.alignItems = 'center';
                cardDiv.style.justifyContent = 'center';
                cardDiv.style.color = '#d4af37';
                cardDiv.style.fontSize = '24px';
            }
            bottomCardsDiv.appendChild(cardDiv);
        }
        
        // 确保底牌区域显示
        bottomCardsArea.classList.remove('hidden');
        bottomCardsArea.style.display = 'block';
        console.log('[UI] bottom-cards-area displayed');
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
        let playerName = '未知';
        
        if (this.players[uid]) {
            playerName = this.players[uid].name;
            if (this.players[uid].position === 'left') {
                playArea = document.getElementById('opponent-left-play');
            } else if (this.players[uid].position === 'right') {
                playArea = document.getElementById('opponent-right-play');
            } else {
                // 我的出牌显示在中间区域（不要清空对手已出的牌，他们可能还在回合中）
                UI.showLastPlay('我', cards);
                return;
            }
        } else {
            // 未知玩家，尝试从UID解析昵称
            if (uid.includes('bot')) {
                const botMatch = uid.match(/bot-\w+-(\d+)/);
                const botNum = botMatch ? botMatch[1] : '?';
                playerName = `机器人${botNum}`;
            } else {
                playerName = uid.substring(0, 6);
            }
            // 未知位置的玩家，显示在中间
            UI.showLastPlay(playerName, cards);
            return;
        }
        
        if (!playArea) {
            UI.showLastPlay(playerName, cards);
            return;
        }
        
        // 清空之前的出牌
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
        
        // 中间区域显示最后一次出牌（带昵称）
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
