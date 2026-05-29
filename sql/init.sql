-- ============================================================
-- 斗地主游戏数据库初始化脚本
-- 数据库: MySQL 8.0+
-- 字符集: utf8mb4
-- ============================================================

-- 创建数据库
CREATE DATABASE IF NOT EXISTS ddz DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE ddz;

-- ============================================================
-- 用户表
-- ============================================================
CREATE TABLE IF NOT EXISTS users (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    uid VARCHAR(64) NOT NULL UNIQUE COMMENT '用户唯一标识 (UUID)',
    username VARCHAR(32) NOT NULL UNIQUE COMMENT '登录用户名',
    password VARCHAR(128) NOT NULL DEFAULT '' COMMENT '密码哈希 (SHA256)',
    nickname VARCHAR(64) NOT NULL DEFAULT '' COMMENT '显示昵称',
    avatar_id INT UNSIGNED NOT NULL DEFAULT 1 COMMENT '头像ID',
    elo INT NOT NULL DEFAULT 1000 COMMENT 'ELO积分',
    tier VARCHAR(32) NOT NULL DEFAULT 'Bronze I' COMMENT '段位',
    gold INT NOT NULL DEFAULT 5000 COMMENT '金币',
    wins INT NOT NULL DEFAULT 0 COMMENT '胜场',
    losses INT NOT NULL DEFAULT 0 COMMENT '负场',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_uid (uid),
    INDEX idx_username (username),
    INDEX idx_elo (elo),
    INDEX idx_tier (tier)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户表';

-- ============================================================
-- 对局记录表
-- ============================================================
CREATE TABLE IF NOT EXISTS game_records (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    room_id VARCHAR(64) NOT NULL COMMENT '房间ID',
    players JSON NOT NULL COMMENT '玩家列表 [{"uid":"xxx","is_landlord":true},...]',
    winner_uid VARCHAR(64) NOT NULL COMMENT '赢家UID',
    winner_side TINYINT NOT NULL DEFAULT 0 COMMENT '获胜方 0=地主 1=农民',
    results JSON NOT NULL COMMENT '结算结果 [{"uid":"xxx","score_change":100,"new_elo":1100},...]',
    base_score INT NOT NULL DEFAULT 1 COMMENT '基础分',
    multiplier INT NOT NULL DEFAULT 1 COMMENT '倍数',
    is_spring TINYINT NOT NULL DEFAULT 0 COMMENT '是否春天',
    is_counter_spring TINYINT NOT NULL DEFAULT 0 COMMENT '是否反春',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_room_id (room_id),
    INDEX idx_winner_uid (winner_uid),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='对局记录表';

-- ============================================================
-- 玩家对局结果表
-- ============================================================
CREATE TABLE IF NOT EXISTS player_results (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    room_id VARCHAR(64) NOT NULL COMMENT '房间ID',
    uid VARCHAR(64) NOT NULL COMMENT '玩家UID',
    is_landlord TINYINT NOT NULL DEFAULT 0 COMMENT '是否是地主',
    score_change INT NOT NULL DEFAULT 0 COMMENT '分数变化',
    new_elo INT NOT NULL DEFAULT 1000 COMMENT '新的ELO积分',
    new_tier VARCHAR(32) NOT NULL DEFAULT 'Bronze I' COMMENT '新的段位',
    is_promoted TINYINT NOT NULL DEFAULT 0 COMMENT '是否晋级',
    is_demoted TINYINT NOT NULL DEFAULT 0 COMMENT '是否降级',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_room_id (room_id),
    INDEX idx_uid (uid),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='玩家对局结果表';

-- ============================================================
-- 插入测试数据
-- ============================================================
-- 密码均为 "123" 的 SHA256 哈希: a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3
INSERT INTO users (uid, username, password, nickname, avatar_id, elo, tier, gold, wins, losses) VALUES
('test-uid-001', 'test', 'a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3', 'TestPlayer', 1, 1000, 'Bronze I', 5000, 0, 0),
('user-002', 'zhangsan', 'a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3', '张三', 2, 1200, 'Silver II', 15000, 10, 5),
('user-003', 'lisi', 'a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3', '李四', 3, 1500, 'Gold I', 20000, 20, 10),
('user-004', 'wangwu', 'a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3', '王五', 4, 1800, 'Platinum III', 30000, 35, 15);
