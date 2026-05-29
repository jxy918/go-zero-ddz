package ai

import (
	"math/rand"
	"time"

	"go-zero-ddz/pkg/cardutil"
)

// Bot AI 机器人
type Bot struct {
	UID        string
	Difficulty string
	Engine     *AIEngine
	Counter    *CardCounter
	rng        *rand.Rand
}

// NewBot 创建 AI 机器人
func NewBot(uid string, difficulty string) *Bot {
	config := getDifficultyConfig(difficulty)
	return &Bot{
		UID:        uid,
		Difficulty: difficulty,
		Engine:     NewAIEngine(config),
		Counter:    NewCardCounter(),
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// DecidePlay 机器人出牌决策
func (b *Bot) DecidePlay(ctx *AIContext) *PlayDecision {
	// 添加随机延迟
	delay := time.Duration(b.rng.Intn(b.Engine.config.DelayMsMax-b.Engine.config.DelayMsMin)+b.Engine.config.DelayMsMin) * time.Millisecond
	time.Sleep(delay)

	return b.Engine.DecidePlay(ctx)
}

// RecordOpponentPlay 记录对手出牌
func (b *Bot) RecordOpponentPlay(cards []cardutil.Card, playerUID string) {
	b.Counter.RecordPlayed(cards, playerUID, time.Now().UnixMilli())
}

// getDifficultyConfig 获取难度配置
func getDifficultyConfig(difficulty string) *AIConfig {
	switch difficulty {
	case "easy":
		return &AIConfig{
			RememberCards: false,
			UseBomb:       false,
			Strategy:      "random",
			ResponseRate:  0.5,
			DelayMsMin:    1000,
			DelayMsMax:    2000,
		}
	case "hard":
		return &AIConfig{
			RememberCards: true,
			UseBomb:       true,
			Strategy:      "optimal",
			ResponseRate:  1.0,
			DelayMsMin:    500,
			DelayMsMax:    1000,
			CardInference: true,
		}
	default: // normal
		return &AIConfig{
			RememberCards: false,
			UseBomb:       true,
			Strategy:      "minimum",
			ResponseRate:  0.8,
			DelayMsMin:    800,
			DelayMsMax:    1500,
		}
	}
}
