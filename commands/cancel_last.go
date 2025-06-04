package commands

import (
	"log/slog"
	"github.com/pyama86/slack-afk/go/store"
	"github.com/slack-go/slack"
	"github.com/pyama86/slack-afk/go/spreadsheet"
)

// CancelLastCommand handles the /cancel_last command
// 直近の有効な勤怠記録をキャンセルし、取消履歴を残す
// 取消は「取消」種別で記録、実働時間計算からは除外される
// 連続で/cancel_lastした場合、どんどん過去に遡る

type CancelLastCommand struct {
	client      *slack.Client
	redisClient *store.RedisClient
}

func NewCancelLastCommand(client *slack.Client, redisClient *store.RedisClient) *CancelLastCommand {
	return &CancelLastCommand{
		client:      client,
		redisClient: redisClient,
	}
}

func (c *CancelLastCommand) Execute(cmd slack.SlashCommand) error {
	uid := cmd.UserID
	channelID := cmd.ChannelID

	// 勤怠記録取消
	cancelled, origType, origMsg, err := spreadsheet.CancelLastRecord(c.client, uid)
	if err != nil {
		slog.Error("勤怠記録取消失敗", slog.Any("error", err))
		_, _ = c.client.PostEphemeral(channelID, uid, slack.MsgOptionText("取消に失敗しました: "+err.Error(), false))
		return err
	}
	if !cancelled {
		_, _ = c.client.PostEphemeral(channelID, uid, slack.MsgOptionText("取消できる記録がありません。", false))
		return nil
	}
	msg := "直近の記録（" + origType + ": " + origMsg + "）を取消しました。"
	_, _ = c.client.PostEphemeral(channelID, uid, slack.MsgOptionText(msg, false))
	return nil
}
