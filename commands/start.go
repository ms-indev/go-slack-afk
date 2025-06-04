package commands

import (
	"log/slog"
	"os"
	"time"

	"github.com/pyama86/slack-afk/go/presentation/blocks"
	"github.com/pyama86/slack-afk/go/store"
	"github.com/slack-go/slack"
	"context"
	"fmt"
	"io/ioutil"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"golang.org/x/oauth2/google"
)

// StartCommand handles the /start command
type StartCommand struct {
	client      *slack.Client
	redisClient *store.RedisClient
}

// NewStartCommand creates a new StartCommand
func NewStartCommand(client *slack.Client, redisClient *store.RedisClient) *StartCommand {
	return &StartCommand{
		client:      client,
		redisClient: redisClient,
	}
}

// Execute handles the /start command
func (c *StartCommand) Execute(cmd slack.SlashCommand) error {
	uid := cmd.UserID
	userName := cmd.UserName
	channelID := cmd.ChannelID

	// Remove user from registered list
	if err := c.redisClient.RemoveFromList("registered", uid); err != nil {
		slog.Error("Failed to remove user from registered list", slog.Any("error", err))
		return err
	}

	// Get user presence
	userPresence, err := c.redisClient.GetUserPresence(uid)
	if err != nil {
		slog.Error("Failed to get user presence", slog.Any("error", err))
		return err
	}

	// Set today's begin time
	jst, _ := time.LoadLocation("Asia/Tokyo")
	now := time.Now().In(jst)
	userPresence["today_begin"] = now.Format(time.RFC3339)
	if err := c.redisClient.SetUserPresence(uid, userPresence); err != nil {
		slog.Error("Failed to set user presence", slog.Any("error", err))
		return err
	}

	// Google Sheets 打刻処理（簿記型）
	spreadsheetID := os.Getenv("SPREADSHEET_ID")
	if spreadsheetID == "" {
		slog.Error("SPREADSHEET_ID is not set in environment variables")
		return fmt.Errorf("SPREADSHEET_ID is not set in environment variables")
	}
	dateStr := now.Format("2006-01-02")
	startTimeStr := now.Format("15:04:05")
	go func() {
		err := writeStartToSheet(spreadsheetID, userName, dateStr, startTimeStr)
		if err != nil {
			msg := fmt.Sprintf(":warning: スプレッドシートへの打刻に失敗しました: %v", err)
			c.client.PostEphemeral(channelID, uid, slack.MsgOptionText(msg, false))
		}
	}()

	// Post message to channel
	_, _, err = c.client.PostMessage(channelID, slack.MsgOptionBlocks(blocks.StartBlocks(userName)...))
	if err != nil {
		slog.Error("Failed to post message", slog.Any("error", err))
		return err
	}

	// Get start message from environment variable or use default
	startMessage := os.Getenv("AFK_START_MESSAGE")
	if startMessage == "" {
		startMessage = "おはようございます、今日も自分史上最高の日にしましょう!!1"
	}

	// Response message
	_, err = c.client.PostEphemeral(channelID, uid, slack.MsgOptionText(startMessage, false))
	if err != nil {
		slog.Error("Failed to post ephemeral message", slog.Any("error", err))
		return err
	}

	return nil
}

// Google Sheetsに出勤打刻を1行追記する関数（簿記型）
func writeStartToSheet(spreadsheetID, userName, dateStr, timeStr string) error {
	ctx := context.Background()
	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		return fmt.Errorf("credentials.jsonの読み込みに失敗: %w", err)
	}
	config, err := google.JWTConfigFromJSON(b, sheets.SpreadsheetsScope)
	if err != nil {
		return fmt.Errorf("JWTConfigFromJSON失敗: %w", err)
	}
	srv, err := sheets.NewService(ctx, option.WithHTTPClient(config.Client(ctx)))
	if err != nil {
		return fmt.Errorf("Sheetsサービス作成失敗: %w", err)
	}

	sheetName := "打刻記録"
	// シート存在確認＆なければ作成
	spreadsheet, err := srv.Spreadsheets.Get(spreadsheetID).Do()
	if err != nil {
		return fmt.Errorf("スプレッドシート取得失敗: %w", err)
	}
	sheetExists := false
	for _, s := range spreadsheet.Sheets {
		if s.Properties.Title == sheetName {
			sheetExists = true
			break
		}
	}
	if !sheetExists {
		addSheetReq := &sheets.Request{
			AddSheet: &sheets.AddSheetRequest{
				Properties: &sheets.SheetProperties{Title: sheetName},
			},
		}
		_, err := srv.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{addSheetReq},
		}).Do()
		if err != nil {
			return fmt.Errorf("シート作成失敗: %w", err)
		}
		// ヘッダー行を追加
		headers := [][]interface{}{{"日付", "ユーザー名", "打刻種別", "時刻", "備考"}}
		_, err = srv.Spreadsheets.Values.Append(spreadsheetID, sheetName+"!A1:E1", &sheets.ValueRange{Values: headers}).ValueInputOption("USER_ENTERED").Do()
		if err != nil {
			return fmt.Errorf("ヘッダー追加失敗: %w", err)
		}
	}

	// 1打刻1行で追記
	row := []interface{}{dateStr, userName, "出勤", timeStr, ""}
	appendRange := fmt.Sprintf("%s!A:E", sheetName)
	_, err = srv.Spreadsheets.Values.Append(spreadsheetID, appendRange, &sheets.ValueRange{Values: [][]interface{}{row}}).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		return fmt.Errorf("行追加失敗: %w", err)
	}
	return nil
}
