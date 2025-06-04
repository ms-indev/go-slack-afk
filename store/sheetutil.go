package store

import (
	"context"
	"fmt"
	"io/ioutil"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"golang.org/x/oauth2/google"
)

// AppendKintaiRow: Googleスプレッドシートに1行追記（簿記型）
func AppendKintaiRow(spreadsheetID, date, userName, kind, timeStr, note string) error {
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
		headers := [][]interface{}{{"日付", "ユーザー名", "打刻種別", "時刻", "備考"}}
		_, err = srv.Spreadsheets.Values.Append(spreadsheetID, sheetName+"!A1:E1", &sheets.ValueRange{Values: headers}).ValueInputOption("USER_ENTERED").Do()
		if err != nil {
			return fmt.Errorf("ヘッダー追加失敗: %w", err)
		}
	}

	row := []interface{}{date, userName, kind, timeStr, note}
	appendRange := fmt.Sprintf("%s!A:E", sheetName)
	_, err = srv.Spreadsheets.Values.Append(spreadsheetID, appendRange, &sheets.ValueRange{Values: [][]interface{}{row}}).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		return fmt.Errorf("行追加失敗: %w", err)
	}
	return nil
}
