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
	TypeComeback = "復帰"
	TypeCancel = "取消"
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
		header := []interface{}{ "日付", "時刻", "種別", "メッセージ", "実働時間（h:mm）" }
		vr := &sheets.ValueRange{ Values: [][]interface{}{ header } }
		_, err := srv.Spreadsheets.Values.Append(spreadsheetID, sheetName+"!A1", vr).ValueInputOption("RAW").Do()
		if err != nil {
			return fmt.Errorf("ヘッダー追加失敗: %w", err)
		}
	}

	// 勤怠レコード追加
	row := []interface{}{ dateStr, timeStr, recordType, message, "" }
	vr := &sheets.ValueRange{ Values: [][]interface{}{ row } }
	_, err = srv.Spreadsheets.Values.Append(spreadsheetID, sheetName+"!A1", vr).ValueInputOption("RAW").Do()
	if err != nil {
		return fmt.Errorf("勤怠レコード追加失敗: %w", err)
	}
	return nil
}

// 直近の有効な記録を取消し、取消履歴を残す
// 戻り値: 取消したか, 元の種別, 元のメッセージ, エラー
func CancelLastRecord(slackClient *slack.Client, userID string) (bool, string, string, error) {
	ctx := context.Background()
	spreadsheetID := os.Getenv(spreadsheetIDEnv)
	if spreadsheetID == "" {
		return false, "", "", fmt.Errorf("環境変数 %s が未設定です", spreadsheetIDEnv)
	}
	b, err := os.ReadFile("credentials.json")
	if err != nil {
		return false, "", "", fmt.Errorf("credentials.jsonの読み込みに失敗: %w", err)
	}
	config, err := google.JWTConfigFromJSON(b, sheets.SpreadsheetsScope)
	if err != nil {
		return false, "", "", fmt.Errorf("Google認証情報のパースに失敗: %w", err)
	}
	ts := config.TokenSource(ctx)
	srv, err := sheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return false, "", "", fmt.Errorf("Sheets APIクライアント生成失敗: %w", err)
	}
	profile, err := slackClient.GetUserProfile(&slack.GetUserProfileParameters{UserID: userID})
	if err != nil {
		return false, "", "", fmt.Errorf("Slackユーザープロフィール取得失敗: %w", err)
	}
	sheetName := profile.FirstName + profile.LastName
	if sheetName == "" {
		sheetName = profile.RealName
	}
	if sheetName == "" {
		sheetName = userID
	}

	// シート全データ取得
	resp, err := srv.Spreadsheets.Values.Get(spreadsheetID, sheetName+"!A:E").Do()
	if err != nil {
		return false, "", "", fmt.Errorf("シートデータ取得失敗: %w", err)
	}
	if len(resp.Values) < 2 {
		return false, "", "", nil // データなし
	}
	// 直近の有効な記録（取消以外）を探す
	for i := len(resp.Values) - 1; i >= 1; i-- {
		row := resp.Values[i]
		if len(row) < 3 { continue }
		typ := fmt.Sprint(row[2])
		if typ == TypeCancel { continue }
		// 既に取消済みかチェック
		cancelled := false
		for j := i + 1; j < len(resp.Values); j++ {
			if len(resp.Values[j]) < 3 { continue }
			if fmt.Sprint(resp.Values[j][2]) == TypeCancel && fmt.Sprint(resp.Values[j][3]) == fmt.Sprintf("%d", i+1) {
				cancelled = true
				break
			}
		}
		if cancelled { continue }
		// 取消
		origType := typ
		origMsg := ""
		if len(row) > 3 { origMsg = fmt.Sprint(row[3]) }
		// 取消履歴として「取消」種別＋取消対象行番号をメッセージ欄に記録
		jst, _ := time.LoadLocation("Asia/Tokyo")
		now := time.Now().In(jst)
		dateStr := now.Format("2006-01-02")
		timeStr := now.Format("15:04:05")
		cancelRow := []interface{}{dateStr, timeStr, TypeCancel, fmt.Sprintf("%d", i+1), ""}
		vr := &sheets.ValueRange{Values: [][]interface{}{cancelRow}}
		_, err := srv.Spreadsheets.Values.Append(spreadsheetID, sheetName+"!A1", vr).ValueInputOption("RAW").Do()
		if err != nil {
			return false, "", "", fmt.Errorf("取消履歴追加失敗: %w", err)
		}
		return true, origType, origMsg, nil
	}
	return false, "", "", nil // 取消できる記録なし
}

// /finish時に実働時間を計算して記入
// 取消履歴・取消対象は無視して有効な記録のみで計算
func UpdateActualWorkTime(slackClient *slack.Client, userID string) error {
	ctx := context.Background()
	spreadsheetID := os.Getenv(spreadsheetIDEnv)
	if spreadsheetID == "" {
		return fmt.Errorf("環境変数 %s が未設定です", spreadsheetIDEnv)
	}
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
	profile, err := slackClient.GetUserProfile(&slack.GetUserProfileParameters{UserID: userID})
	if err != nil {
		return fmt.Errorf("Slackユーザープロフィール取得失敗: %w", err)
	}
	sheetName := profile.FirstName + profile.LastName
	if sheetName == "" {
		sheetName = profile.RealName
	}
	if sheetName == "" {
		sheetName = userID
	}

	// シート全データ取得
	resp, err := srv.Spreadsheets.Values.Get(spreadsheetID, sheetName+"!A:E").Do()
	if err != nil {
		return fmt.Errorf("シートデータ取得失敗: %w", err)
	}
	if len(resp.Values) < 2 {
		return nil // データなし
	}
	// 取消行・取消対象行を無視して有効な記録だけを抽出
	cancelledRows := map[int]bool{}
	for _, row := range resp.Values[1:] {
		if len(row) < 3 { continue }
		if fmt.Sprint(row[2]) == TypeCancel {
			// メッセージ欄に取消対象行番号が入っている
			if len(row) > 3 {
				var idx int
				fmt.Sscanf(fmt.Sprint(row[3]), "%d", &idx)
				if idx > 0 {
					cancelledRows[idx-1] = true // 0-indexed
				}
			}
		}
	}
	type rowWithIndex struct {
	row     []interface{}
	origIdx int
}
var validRows []rowWithIndex
for i, row := range resp.Values[1:] {
	if cancelledRows[i] { continue }
	if len(row) < 3 { continue }
	if fmt.Sprint(row[2]) == TypeCancel { continue }
	validRows = append(validRows, rowWithIndex{row, i+1}) // +1: ヘッダー分
}
	// 日付ごとに最新の退勤行を探す
	dateToRows := map[string][]int{}
	for i, rw := range validRows {
		row := rw.row
		if len(row) < 3 { continue }
		date, typ := fmt.Sprint(row[0]), fmt.Sprint(row[2])
		if typ == TypeFinish {
			dateToRows[date] = append(dateToRows[date], i)
		}
	}
	// validRowsの一番下から上に向かって最初の退勤行だけに実働時間を書き込む
for i := len(validRows) - 1; i >= 0; i-- {
	row := validRows[i].row
	if len(row) < 3 { continue }
	if fmt.Sprint(row[2]) != TypeFinish { continue }
	date := fmt.Sprint(row[0])
	// その日分の全行を抽出
	var (
		startTime, finishTime time.Time
		breaks [][2]time.Time
	)
	for _, rw := range validRows {
		r := rw.row
		if len(r) < 3 { continue }
		rowDate, t := fmt.Sprint(r[0]), fmt.Sprint(r[2])
		if rowDate != date { continue }
		ts, _ := time.ParseInLocation("15:04:05", fmt.Sprint(r[1]), time.Local)
		ts = time.Date(0,1,1,ts.Hour(),ts.Minute(),ts.Second(),0,time.Local)
		switch t {
		case TypeStart:
			startTime = ts
		case TypeFinish:
			finishTime = ts
		case TypeLunch, TypeAfk:
			breaks = append(breaks, [2]time.Time{ts, {}})
		case TypeComeback:
			if len(breaks) > 0 && breaks[len(breaks)-1][1].IsZero() {
				breaks[len(breaks)-1][1] = ts
			}
		}
	}
	if startTime.IsZero() || finishTime.IsZero() { break }
	var breakDur time.Duration
	for _, b := range breaks {
		if !b[0].IsZero() && !b[1].IsZero() {
			breakDur += b[1].Sub(b[0])
		}
	}
	workDur := finishTime.Sub(startTime) - breakDur
	if workDur < 0 { workDur = 0 }
	workStr := fmt.Sprintf("%d:%02d", int(workDur.Hours()), int(workDur.Minutes())%60)
	cell := fmt.Sprintf("E%d", validRows[i].origIdx+1) // +1: 1-indexed
	_, err := srv.Spreadsheets.Values.Update(spreadsheetID, sheetName+"!"+cell, &sheets.ValueRange{Values: [][]interface{}{{workStr}}}).ValueInputOption("RAW").Do()
	if err != nil {
		return fmt.Errorf("実働時間書き込み失敗: %w", err)
	}
	break // 1件だけ
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
