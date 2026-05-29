export const MSG_ID = {
    HEARTBEAT_REQ: 0x0001,
    HEARTBEAT_RESP: 0x0002,
    ERROR_RESP: 0x0003,
    
    LOGIN_REQ: 0x0101,
    LOGIN_RESP: 0x0102,
    
    CREATE_ROOM_REQ: 0x0201,
    CREATE_ROOM_RESP: 0x0202,
    JOIN_ROOM_REQ: 0x0203,
    JOIN_ROOM_RESP: 0x0204,
    ROOM_STATE_NOTIFY: 0x0206,
    PLAYER_READY_REQ: 0x0207,
    
    MATCH_START_REQ: 0x0301,
    MATCH_CANCEL_REQ: 0x0302,
    MATCH_SUCCESS_NOTIFY: 0x0303,
    
    DEAL_CARDS_NOTIFY: 0x0401,
    CALL_LANDLORD_REQ: 0x0402,
    CALL_LANDLORD_NOTIFY: 0x0403,
    PLAY_CARDS_REQ: 0x0404,
    PLAY_CARDS_NOTIFY: 0x0405,
    PASS_NOTIFY: 0x0406,
    GAME_END_NOTIFY: 0x0407,
    TIMER_NOTIFY: 0x0408,
    
    RECONNECT_REQ: 0x0501,
    RECONNECT_RESP: 0x0502,
};

export const CARD_VALUE = {
    3: '3', 4: '4', 5: '5', 6: '6', 7: '7', 8: '8', 9: '9',
    10: '10', 11: 'J', 12: 'Q', 13: 'K', 14: 'A', 15: '2',
    16: '小王', 17: '大王'
};

export const CARD_SUIT = {
    1: '♠', 2: '♥', 3: '♣', 4: '♦', 5: '🃏'
};

export const ROOM_STATE = {
    WAITING: 'waiting',
    PLAYING: 'playing',
    SETTLEMENT: 'settlement'
};

export const GAME_STATE = {
    DEALING: 'dealing',
    CALLING: 'calling',
    PLAYING: 'playing',
    SETTLEMENT: 'settlement'
};

export const PLAYER_ROLE = {
    PEASANT: 'peasant',
    LANDLORD: 'landlord'
};

export const MAX_CARDS_PER_PLAYER = 17;
export const BOTTOM_CARDS_COUNT = 3;
export const PLAY_TIMEOUT = 15;
export const MAX_PLAYERS = 3;