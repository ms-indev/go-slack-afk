package commands

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/pyama86/slack-afk/go/presentation/blocks"
	"github.com/pyama86/slack-afk/go/spreadsheet"
	"github.com/pyama86/slack-afk/go/store"
	"github.com/slack-go/slack"
)

// ComebackCommand handles the /comeback command
type ComebackCommand struct {
	client      *slack.Client
	redisClient *store.RedisClient
}

// NewComebackCommand creates a new ComebackCommand
func NewComebackCommand(client *slack.Client, redisClient *store.RedisClient) *ComebackCommand {
	return &ComebackCommand{
		client:      client,
		redisClient: redisClient,
	}
}

// Execute handles the /comeback command
func (c *ComebackCommand) Execute(cmd slack.SlashCommand) error {
	uid := cmd.UserID
	userName := cmd.UserName
	channelID := cmd.ChannelID

	// Get user presence
	userPresence, err := c.redisClient.GetUserPresence(uid)
	if err != nil {
		slog.Error("Failed to get user presence", slog.Any("error", err))
		return err
	}

	// Post message to channel
	_, _, err = c.client.PostMessage(channelID, slack.MsgOptionBlocks(blocks.ComebackBlocks(userName)...))
	if err != nil {
		slog.Error("Failed to post message", slog.Any("error", err))
		return err
	}

	// Remove user from Redis
	if err := c.redisClient.Delete(uid); err != nil {
		slog.Error("Failed to delete user from Redis", slog.Any("error", err))
		return err
	}

	// Remove user from registered list
	if err := c.redisClient.RemoveFromList("registered", uid); err != nil {
		slog.Error("Failed to remove user from registered list", slog.Any("error", err))
		return err
	}

	// Check if there are any mentions
	var mentionHistory []interface{}
	if history, ok := userPresence["mention_history"].([]interface{}); ok {
		mentionHistory = history
	}

	// Prepare response message
	var responseMessage string
	if len(mentionHistory) == 0 {
		responseMessage = "おかえりなさい!!1特にいない間にメンションは飛んでこなかったみたいです。"
	} else {
		responseMessage = "おかえりなさい!!1\nいない間に飛んできたメンションです\n"

		slackDomain := os.Getenv("SLACK_DOMAIN")
		if slackDomain == "" {
			slackDomain = "slack.com"
		}

		for _, mention := range mentionHistory {
			if m, ok := mention.(map[string]interface{}); ok {
				user, _ := m["user"].(string)
				channel, _ := m["channel"].(string)
				text, _ := m["text"].(string)
				eventTS, _ := m["event_ts"].(string)

				// Format timestamp for link
				linkTS := eventTS
				if linkTS != "" {
					linkTS = strings.ReplaceAll(linkTS, ".", "")
				}

				mentionText := fmt.Sprintf("<@%s>: <https://%s/archives/%s/p%s|Link>\n内容: %s\n",
					user, slackDomain, channel, linkTS, text)
				responseMessage += mentionText
			}
		}
	}

	// Response message
	_, err = c.client.PostEphemeral(channelID, uid, slack.MsgOptionText(responseMessage, false))
	if err != nil {
		slog.Error("Failed to post ephemeral message", slog.Any("error", err))
		return err
	}

	// 勤怠記録（エラーはログのみ）
	go func() {
		err := spreadsheet.AppendAttendanceRecord(c.client, uid, spreadsheet.TypeComeback, "")
		if err != nil {
			slog.Error("スプレッドシート勤怠記録失敗", slog.Any("error", err))
		}
	}()

	return nil
}
