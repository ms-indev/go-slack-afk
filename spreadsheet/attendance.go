package spreadsheet

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"golang.org/x/oauth2/google"
	"github.com/slack-go/slack"
)

const (
	spreadsheetIDEnv = "ATTENDANCE_SPREADSHEET_ID" // スプレッドシートIDは環境変数で指定
)

// 勤怠種別
const (
	TypeStart  = "出勤"
	TypeFinish = "退勤"
	TypeLunch  = "外出"
	TypeAfk    = "離席"
)

// 勤怠レコード
// messageは任意
func AppendAttendanceRecord(slackClient *slack.Client, userID, recordType, message string) error {
	ctx := context.Background()

	spreadsheetID := os.Getenv(spreadsheetIDEnv)
	if spreadsheetID == "" {
		return fmt.Errorf("環境変数 %s が未設定です", spreadsheetIDEnv)
	}

	// Google Sheets API認証
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		return fmt.Errorf("credentials.jsonの読み込みに失敗: %w", err)
	}
	config, err := google.JWTConfigFromJSON(b, sheets.SpreadsheetsScope)
	if err != nil {
		return fmt.Errorf("Google認証情報のパースに失敗: %w", err)
	}
	ts := config.TokenSource(ctx)
	srv, err := sheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("Sheets APIクライアント生成失敗: %w", err)
	}

	// Slackユーザー情報取得
	profile, err := slackClient.GetUserProfile(&slack.GetUserProfileParameters{UserID: userID})
	if err != nil {
		return fmt.Errorf("Slackユーザープロフィール取得失敗: %w", err)
	}
	sheetName := profile.FirstName + profile.LastName
	if sheetName == "" {
		sheetName = profile.RealName // fallback
	}
	if sheetName == "" {
		sheetName = userID // fallback
	}

	// 日付・時刻
	jst, _ := time.LoadLocation("Asia/Tokyo")
	now := time.Now().In(jst)
	dateStr := now.Format("2006-01-02")
	timeStr := now.Format("15:04:05")

	// シート存在確認＆なければ作成
	exists, err := sheetExists(srv, spreadsheetID, sheetName)
	if err != nil {
		return err
	}
	if !exists {
		if err := createSheet(srv, spreadsheetID, sheetName); err != nil {
			return err
		}
		// ヘッダー行追加
		header := []interface{}{ "日付", "時刻", "種別", "メッセージ" }
		vr := &sheets.ValueRange{ Values: [][]interface{}{ header } }
		_, err := srv.Spreadsheets.Values.Append(spreadsheetID, sheetName+"!A1", vr).ValueInputOption("RAW").Do()
		if err != nil {
			return fmt.Errorf("ヘッダー追加失敗: %w", err)
		}
	}

	// 勤怠レコード追加
	row := []interface{}{ dateStr, timeStr, recordType, message }
	vr := &sheets.ValueRange{ Values: [][]interface{}{ row } }
	_, err = srv.Spreadsheets.Values.Append(spreadsheetID, sheetName+"!A1", vr).ValueInputOption("RAW").Do()
	if err != nil {
		return fmt.Errorf("勤怠レコード追加失敗: %w", err)
	}
	return nil
}

// シート存在確認
func sheetExists(srv *sheets.Service, spreadsheetID, sheetName string) (bool, error) {
	ss, err := srv.Spreadsheets.Get(spreadsheetID).Do()
	if err != nil {
		return false, fmt.Errorf("スプレッドシート取得失敗: %w", err)
	}
	for _, s := range ss.Sheets {
		if s.Properties.Title == sheetName {
			return true, nil
		}
	}
	return false, nil
}

// シート作成
func createSheet(srv *sheets.Service, spreadsheetID, sheetName string) error {
	rq := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				AddSheet: &sheets.AddSheetRequest{
					Properties: &sheets.SheetProperties{
						Title: sheetName,
					},
				},
			},
		},
	}
	_, err := srv.Spreadsheets.BatchUpdate(spreadsheetID, rq).Do()
	if err != nil {
		return fmt.Errorf("シート作成失敗: %w", err)
	}
	return nil
}
