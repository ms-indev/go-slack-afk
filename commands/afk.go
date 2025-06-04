package commands

import (
	"fmt"
	"log/slog"

	"github.com/pyama86/slack-afk/go/presentation/blocks"
	"github.com/pyama86/slack-afk/go/store"
	"github.com/slack-go/slack"
	"github.com/pyama86/slack-afk/go/spreadsheet"
)

// AfkCommand handles the /afk command
type AfkCommand struct {
	client      *slack.Client
	redisClient *store.RedisClient
}

// NewAfkCommand creates a new AfkCommand
func NewAfkCommand(client *slack.Client, redisClient *store.RedisClient) *AfkCommand {
	return &AfkCommand{
		client:      client,
		redisClient: redisClient,
	}
}

// Execute handles the /afk command
func (c *AfkCommand) Execute(cmd slack.SlashCommand) error {
	uid := cmd.UserID
	text := cmd.Text
	userName := cmd.UserName
	channelID := cmd.ChannelID

	// Add user to registered list
	if err := c.redisClient.AddToList("registered", uid); err != nil {
		slog.Error("Failed to add user to registered list", slog.Any("error", err))
		return err
	}

	// Reset user's mention history
	userPresence, err := c.redisClient.GetUserPresence(uid)
	if err != nil {
		slog.Error("Failed to get user presence", slog.Any("error", err))
		return err
	}
	userPresence["mention_history"] = []interface{}{}
	if err := c.redisClient.SetUserPresence(uid, userPresence); err != nil {
		slog.Error("Failed to set user presence", slog.Any("error", err))
		return err
	}

	// Set message
	var message string
	if text != "" {
		message = fmt.Sprintf("%s は席を外しています。「%s」", userName, text)
		_, _, err := c.client.PostMessage(channelID, slack.MsgOptionBlocks(blocks.AfkBlocks(userName, text)...))
		if err != nil {
			slog.Error("Failed to post message", slog.Any("error", err))
			return err
		}
	} else {
		message = fmt.Sprintf("%s は席を外しています。反応が遅れるかもしれません。", userName)
		_, _, err := c.client.PostMessage(channelID, slack.MsgOptionBlocks(blocks.AfkBlocks(userName, "")...))
		if err != nil {
			slog.Error("Failed to post message", slog.Any("error", err))
			return err
		}
	}

	// Save to Redis
	if err := c.redisClient.Set(uid, message); err != nil {
		slog.Error("Failed to set message", slog.Any("error", err))
		return err
	}

	// Response message
	_, err = c.client.PostEphemeral(channelID, uid, slack.MsgOptionText("行ってらっしゃい!!1", false))
	if err != nil {
		slog.Error("Failed to post ephemeral message", slog.Any("error", err))
		return err
	}

	// 勤怠記録（エラーはログのみ）
	go func() {
		_, err := spreadsheet.AppendAttendanceRecord(c.client, uid, spreadsheet.TypeAfk, text)
		if err != nil {
			slog.Error("スプレッドシート勤怠記録失敗", slog.Any("error", err))
		}
	}()

	return nil
}
