package commands

import (
	"fmt"
	"log/slog"
	"time"
	"os"
	"context"
	"io/ioutil"

	"github.com/pyama86/slack-afk/go/presentation/blocks"
	"github.com/pyama86/slack-afk/go/store"
	"github.com/slack-go/slack"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"golang.org/x/oauth2/google"
)

// LunchCommand handles the /lunch command
type LunchCommand struct {
	client      *slack.Client
	redisClient *store.RedisClient
}

// NewLunchCommand creates a new LunchCommand
func NewLunchCommand(client *slack.Client, redisClient *store.RedisClient) *LunchCommand {
	return &LunchCommand{
		client:      client,
		redisClient: redisClient,
	}
}

// Execute handles the /lunch command
func (c *LunchCommand) Execute(cmd slack.SlashCommand) error {
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
		message = fmt.Sprintf("%s はランチに行っています。「%s」", userName, text)
	} else {
		message = fmt.Sprintf("%s はランチに行っています。反応が遅れるかもしれません。", userName)
	}

	// Save to Redis with 1 hour expiration
	if err := c.redisClient.Set(uid, message); err != nil {
		slog.Error("Failed to set message", slog.Any("error", err))
		return err
	}
	if err := c.redisClient.Expire(uid, 1*time.Hour); err != nil {
		slog.Error("Failed to set expiration", slog.Any("error", err))
		return err
	}

	// Post message to channel
	_, _, err = c.client.PostMessage(channelID, slack.MsgOptionBlocks(blocks.LunchBlocks(userName, text)...))
	if err != nil {
		slog.Error("Failed to post message", slog.Any("error", err))
		return err
	}

	// Get JST time
	jst, _ := time.LoadLocation("Asia/Tokyo")
	now := time.Now().In(jst)

	// Save last lunch date
	userPresence["last_lunch_date"] = now.Format(time.RFC3339)
	if err := c.redisClient.SetUserPresence(uid, userPresence); err != nil {
		slog.Error("Failed to set user presence", slog.Any("error", err))
		return err
	}

	// Redisに休憩開始時刻を保存
	if err := c.redisClient.Set(uid+":lunch_start", now.Format(time.RFC3339)); err != nil {
		slog.Error("Failed to set lunch start in Redis", slog.Any("error", err))
	}

	// スプレッドシートに休憩開始を記録
	spreadsheetID := os.Getenv("SPREADSHEET_ID")
	if spreadsheetID != "" {
		go func() {
			err := appendLunchNoteToSheet(spreadsheetID, userName, now.Format("2006-01-02"), fmt.Sprintf("休憩開始:%s", now.Format("15:04:05")))
			if err != nil {
				slog.Error("Failed to append lunch note to sheet", slog.Any("error", err))
			}
		}()
	}

	// Response message
	returnTime := now.Add(1 * time.Hour).Format("15:04")
	_, err = c.client.PostEphemeral(channelID, uid, slack.MsgOptionText(fmt.Sprintf("行ってらっしゃい!!1 %sに自動で解除します", returnTime), false))
	if err != nil {
		slog.Error("Failed to post ephemeral message", slog.Any("error", err))
		return err
	}

	return nil
}

// スプレッドシートの本日行の備考欄に追記
func appendLunchNoteToSheet(spreadsheetID, sheetName, dateStr, note string) error {
	ctx := context.Background()
	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		return err
	}
	config, err := google.JWTConfigFromJSON(b, sheets.SpreadsheetsScope)
	if err != nil {
		return err
	}
	srv, err := sheets.NewService(ctx, option.WithHTTPClient(config.Client(ctx)))
	if err != nil {
		return err
	}
	readRange := fmt.Sprintf("%s!A:A", sheetName)
	resp, err := srv.Spreadsheets.Values.Get(spreadsheetID, readRange).Do()
	if err != nil {
		return err
	}
	rowIndex := -1
	for i, row := range resp.Values {
		if len(row) > 0 && row[0] == dateStr {
			rowIndex = i + 1
			break
		}
	}
	if rowIndex < 0 {
		return fmt.Errorf("本日行が見つかりません")
	}
	getRange := fmt.Sprintf("%s!E%d", sheetName, rowIndex)
	getResp, err := srv.Spreadsheets.Values.Get(spreadsheetID, getRange).Do()
	if err != nil {
		return err
	}
	oldNote := ""
	if len(getResp.Values) > 0 && len(getResp.Values[0]) > 0 {
		oldNote = fmt.Sprintf("%v", getResp.Values[0][0])
	}
	newNote := oldNote
	if oldNote != "" {
		newNote += ", "
	}
	newNote += note
	updateRange := fmt.Sprintf("%s!E%d", sheetName, rowIndex)
	_, err = srv.Spreadsheets.Values.Update(spreadsheetID, updateRange, &sheets.ValueRange{Values: [][]interface{}{{newNote}}}).ValueInputOption("USER_ENTERED").Do()
	return err
}
