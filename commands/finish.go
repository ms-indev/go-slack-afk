package commands

import (
	"context"
	"fmt"
	"io/ioutil"
	"log/slog"
	"os"
	"time"

	"github.com/pyama86/slack-afk/go/presentation/blocks"
	"github.com/pyama86/slack-afk/go/store"
	"github.com/slack-go/slack"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"golang.org/x/oauth2/google"
)

// FinishCommand handles the /finish command
type FinishCommand struct {
	client      *slack.Client
	redisClient *store.RedisClient
}

// NewFinishCommand creates a new FinishCommand
func NewFinishCommand(client *slack.Client, redisClient *store.RedisClient) *FinishCommand {
	return &FinishCommand{
		client:      client,
		redisClient: redisClient,
	}
}

// Execute handles the /finish command
func (c *FinishCommand) Execute(cmd slack.SlashCommand) error {
	uid := cmd.UserID
	text := cmd.Text
	userName := cmd.UserName
	channelID := cmd.ChannelID

	// Add user to registered list
	if err := c.redisClient.AddToList("registered", uid); err != nil {
		slog.Error("Failed to add user to registered list", slog.Any("error", err))
		return err
	}

	// Set message
	var message string
	if text != "" {
		message = fmt.Sprintf("%s は退勤しました。「%s」", userName, text)
	} else {
		message = fmt.Sprintf("%s は退勤しました。反応が遅れるかもしれません。", userName)
	}

	// Save to Redis with expiration until tomorrow morning
	if err := c.redisClient.Set(uid, message); err != nil {
		slog.Error("Failed to set message", slog.Any("error", err))
		return err
	}

	// Calculate expiration time (until 9:00 AM tomorrow)
	jst, _ := time.LoadLocation("Asia/Tokyo")
	now := time.Now().In(jst)
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 9, 0, 0, 0, jst)
	expireDuration := tomorrow.Sub(now)
	if err := c.redisClient.Expire(uid, expireDuration); err != nil {
		slog.Error("Failed to set expiration", slog.Any("error", err))
		return err
	}

	// Get user presence
	userPresence, err := c.redisClient.GetUserPresence(uid)
	if err != nil {
		slog.Error("Failed to get user presence", slog.Any("error", err))
		return err
	}

	// Set today's end time
	userPresence["today_end"] = now.Format(time.RFC3339)
	if err := c.redisClient.SetUserPresence(uid, userPresence); err != nil {
		slog.Error("Failed to set user presence", slog.Any("error", err))
		return err
	}

	// Google Sheets 退勤打刻処理
	spreadsheetID := os.Getenv("SPREADSHEET_ID")
	if spreadsheetID == "" {
		slog.Error("SPREADSHEET_ID is not set in environment variables")
		return fmt.Errorf("SPREADSHEET_ID is not set in environment variables")
	}
	sheetName := userName
	dateStr := now.Format("2006-01-02")
	finishTimeStr := now.Format("15:04:05")
	remark := text

	go func() {
		err := writeFinishToSheet(spreadsheetID, sheetName, dateStr, finishTimeStr, remark)
		if err != nil {
			msg := fmt.Sprintf(":warning: スプレッドシートへの退勤打刻に失敗しました: %v", err)
			c.client.PostEphemeral(channelID, uid, slack.MsgOptionText(msg, false))
		}
	}()

	// Post message to channel
	_, _, err = c.client.PostMessage(channelID, slack.MsgOptionBlocks(blocks.FinishBlocks(userName, text)...))
	if err != nil {
		slog.Error("Failed to post message", slog.Any("error", err))
		return err
	}

	// Get finish message from environment variable or use default
	finishMessage := os.Getenv("AFK_FINISH_MESSAGE")
	if finishMessage == "" {
		finishMessage = "お疲れさまでした!!1"
	}

	// Add begin time if available
	beginTimeStr, ok := userPresence["today_begin"].(string)
	if ok {
		beginTime, err := time.Parse(time.RFC3339, beginTimeStr)
		if err == nil {
			finishMessage += fmt.Sprintf("\n始業時刻:%s", beginTime.Format("15:04"))
		}
	}

	// Add auto-disable time
	finishMessage += fmt.Sprintf("\n明日の%sに自動で解除します", tomorrow.Format("15:04"))

	// Response message
	_, err = c.client.PostEphemeral(channelID, uid, slack.MsgOptionText(finishMessage, false))
	if err != nil {
		slog.Error("Failed to post ephemeral message", slog.Any("error", err))
		return err
	}

	return nil
}

// Google Sheetsに退勤打刻を追記・上書きする関数
func writeFinishToSheet(spreadsheetID, sheetName, dateStr, finishTimeStr, remark string) error {
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
		headers := [][]interface{}{{"日付", "出勤時刻", "退勤時刻", "休憩時間", "備考"}}
		_, err = srv.Spreadsheets.Values.Append(spreadsheetID, sheetName+"!A1:E1", &sheets.ValueRange{Values: headers}).ValueInputOption("USER_ENTERED").Do()
		if err != nil {
			return fmt.Errorf("ヘッダー追加失敗: %w", err)
		}
	}

	// 既存の日付行があれば上書き、なければ追記
	readRange := fmt.Sprintf("%s!A:A", sheetName)
	resp, err := srv.Spreadsheets.Values.Get(spreadsheetID, readRange).Do()
	if err != nil {
		return fmt.Errorf("既存データ取得失敗: %w", err)
	}
	rowIndex := -1
	for i, row := range resp.Values {
		if len(row) > 0 && row[0] == dateStr {
			rowIndex = i + 1 // 1-indexed
			break
		}
	}
	// 既存行取得
	var row []interface{}
	if rowIndex > 0 {
		// 既存行を取得して退勤時刻・備考を上書き
		getRange := fmt.Sprintf("%s!A%d:E%d", sheetName, rowIndex, rowIndex)
		getResp, err := srv.Spreadsheets.Values.Get(spreadsheetID, getRange).Do()
		if err != nil || len(getResp.Values) == 0 {
			// 取得失敗時は新規作成
			row = []interface{}{dateStr, "", finishTimeStr, "", remark}
		} else {
			row = getResp.Values[0]
			// 必要な長さに調整
			for len(row) < 5 {
				row = append(row, "")
			}
			row[2] = finishTimeStr
			row[4] = remark
		}
		// 上書き
		updateRange := fmt.Sprintf("%s!A%d:E%d", sheetName, rowIndex, rowIndex)
		_, err = srv.Spreadsheets.Values.Update(spreadsheetID, updateRange, &sheets.ValueRange{Values: [][]interface{}{row}}).ValueInputOption("USER_ENTERED").Do()
		if err != nil {
			return fmt.Errorf("既存行上書き失敗: %w", err)
		}
	} else {
		// 追記
		row = []interface{}{dateStr, "", finishTimeStr, "", remark}
		appendRange := fmt.Sprintf("%s!A:E", sheetName)
		_, err = srv.Spreadsheets.Values.Append(spreadsheetID, appendRange, &sheets.ValueRange{Values: [][]interface{}{row}}).ValueInputOption("USER_ENTERED").Do()
		if err != nil {
			return fmt.Errorf("行追加失敗: %w", err)
		}
	}
	return nil
}
