package commands

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"context"
	"io/ioutil"
	"strconv"
	"time"

	"github.com/pyama86/slack-afk/go/presentation/blocks"
	"github.com/pyama86/slack-afk/go/store"
	"github.com/slack-go/slack"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"golang.org/x/oauth2/google"
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

	// 休憩終了時刻
	jst, _ := time.LoadLocation("Asia/Tokyo")
	now := time.Now().In(jst)

	// Redisから休憩開始時刻を取得
	lunchStartStr, err := c.redisClient.Get(uid+":lunch_start")
	if err == nil && lunchStartStr != "" {
		lunchStart, err := time.Parse(time.RFC3339, lunchStartStr)
		if err == nil {
			delta := int(now.Sub(lunchStart).Minutes())
			// スプレッドシートに休憩時間加算・備考追記・実働時間再計算
			spreadsheetID := os.Getenv("SPREADSHEET_ID")
			if spreadsheetID != "" {
				go func() {
					err := updateLunchEndToSheet(spreadsheetID, userName, now.Format("2006-01-02"), delta, now.Format("15:04:05"))
					if err != nil {
						slog.Error("Failed to update lunch end to sheet", slog.Any("error", err))
					}
				}()
			}
		}
	}

	return nil
}

// スプレッドシートの本日行の休憩時間加算・備考追記・実働時間再計算
func updateLunchEndToSheet(spreadsheetID, sheetName, dateStr string, addMinutes int, endTimeStr string) error {
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
	// 休憩時間加算（D列）
	dRange := fmt.Sprintf("%s!D%d", sheetName, rowIndex)
	dResp, err := srv.Spreadsheets.Values.Get(spreadsheetID, dRange).Do()
	if err != nil {
		return err
	}
	dOld := 0
	if len(dResp.Values) > 0 && len(dResp.Values[0]) > 0 {
		dOld, _ = strconv.Atoi(fmt.Sprintf("%v", dResp.Values[0][0]))
	}
	dNew := dOld + addMinutes
	_, err = srv.Spreadsheets.Values.Update(spreadsheetID, dRange, &sheets.ValueRange{Values: [][]interface{}{{dNew}}}).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		return err
	}
	// 実働時間再計算（E列）
	bRange := fmt.Sprintf("%s!B%d", sheetName, rowIndex)
	cRange := fmt.Sprintf("%s!C%d", sheetName, rowIndex)
	bResp, err := srv.Spreadsheets.Values.Get(spreadsheetID, bRange).Do()
	cResp, err2 := srv.Spreadsheets.Values.Get(spreadsheetID, cRange).Do()
	if err != nil || err2 != nil {
		return nil // 出勤・退勤がなければスキップ
	}
	bStr, cStr := "", ""
	if len(bResp.Values) > 0 && len(bResp.Values[0]) > 0 {
		bStr = fmt.Sprintf("%v", bResp.Values[0][0])
	}
	if len(cResp.Values) > 0 && len(cResp.Values[0]) > 0 {
		cStr = fmt.Sprintf("%v", cResp.Values[0][0])
	}
	if bStr != "" && cStr != "" {
		bTime, err1 := time.Parse("15:04:05", bStr)
		cTime, err2 := time.Parse("15:04:05", cStr)
		if err1 == nil && err2 == nil {
			workMin := int(cTime.Sub(bTime).Minutes()) - dNew
			if workMin < 0 {
				workMin = 0
			}
			hh := workMin / 60
			mm := workMin % 60
			eRange := fmt.Sprintf("%s!E%d", sheetName, rowIndex)
			_, _ = srv.Spreadsheets.Values.Update(spreadsheetID, eRange, &sheets.ValueRange{Values: [][]interface{}{{fmt.Sprintf("%02d:%02d", hh, mm)}}}).ValueInputOption("USER_ENTERED").Do()
		}
	}
	// 備考追記（F列）
	fRange := fmt.Sprintf("%s!F%d", sheetName, rowIndex)
	fResp, err := srv.Spreadsheets.Values.Get(spreadsheetID, fRange).Do()
	if err != nil {
		return err
	}
	fOld := ""
	if len(fResp.Values) > 0 && len(fResp.Values[0]) > 0 {
		fOld = fmt.Sprintf("%v", fResp.Values[0][0])
	}
	fNew := fOld
	if fOld != "" {
		fNew += ", "
	}
	fNew += fmt.Sprintf("休憩終了:%s", endTimeStr)
	_, err = srv.Spreadsheets.Values.Update(spreadsheetID, fRange, &sheets.ValueRange{Values: [][]interface{}{{fNew}}}).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		return err
	}
	return nil
}
