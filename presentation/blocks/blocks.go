package blocks

import (
	"github.com/slack-go/slack"
)

// AfkBlocks creates blocks for afk command response
func AfkBlocks(userName string, text string) []slack.Block {
	var blocks []slack.Block

	if text != "" {
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", ":walking: *"+userName+"が離席しました*\n「"+text+"」", false, false),
			nil,
			nil,
		))
	} else {
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", ":walking: *"+userName+"が離席しました*\n代わりに不在をお伝えします", false, false),
			nil,
			nil,
		))
	}

	return blocks
}

// LunchBlocks creates blocks for lunch command response
func LunchBlocks(userName string, text string) []slack.Block {
	var blocks []slack.Block

	if text != "" {
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", ":bento: *"+userName+"がランチに行きました*\n「"+text+"」", false, false),
			nil,
			nil,
		))
	} else {
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", ":bento: *"+userName+"がランチに行きました*\n何食べるんでしょうね？", false, false),
			nil,
			nil,
		))
	}

	return blocks
}

// StartBlocks creates blocks for start command response
func StartBlocks(userName string) []slack.Block {
	return []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", ":sunrise: *"+userName+"が始業しました*", false, false),
			nil,
			nil,
		),
	}
}

// FinishBlocks creates blocks for finish command response
func FinishBlocks(userName string, text string) []slack.Block {
	var blocks []slack.Block

	if text != "" {
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", ":night_with_stars: *"+userName+"が退勤しました*\n「"+text+"」", false, false),
			nil,
			nil,
		))
	} else {
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", ":night_with_stars: *"+userName+"が退勤しました*\nお疲れさまでした！！１", false, false),
			nil,
			nil,
		))
	}

	return blocks
}

// ComebackBlocks creates blocks for comeback command response
func ComebackBlocks(userName string) []slack.Block {
	return []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", ":back: *"+userName+"が戻ってきました*\nI'll be back!!1", false, false),
			nil,
			nil,
		),
	}
}

// HelpBlocks creates blocks for help command response
func HelpBlocks() []slack.Block {
	helpText := "*使用可能なコマンド:*\n" +
		"• `/afk [メッセージ]` - 離席状態にする\n" +
		"• `/lunch [メッセージ]` - ランチ中の状態にする（1時間後に自動解除）\n" +
		"• `/start` - 始業状態にする\n" +
		"• `/finish [メッセージ]` - 退勤状態にする（翌日朝まで自動応答）\n" +
		"• `/comeback` - 離席状態を解除する"

	return []slack.Block{
		slack.NewHeaderBlock(
			slack.NewTextBlockObject("plain_text", "Slack-AFKヘルプ", false, false),
		),
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", helpText, false, false),
			nil,
			nil,
		),
	}
}
