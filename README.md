# Slack-AFK (Go 版)

Slack-AFK は Slack で離席状態を管理するためのボットです。このリポジトリは Ruby 版の Slack-AFK を Golang に移植したものです。

## 機能

- `/afk [メッセージ]` - 離席状態にする
- `/lunch [メッセージ]` - ランチ中の状態にする（1 時間後に自動解除）
- `/start` - 始業状態にする
- `/finish [メッセージ]` - 退勤状態にする（翌日朝まで自動応答）
- `/comeback` - 離席状態を解除する
- `@bot-name ping` - ping に対して「pong」と応答
- `@bot-name help` - ヘルプを表示

## 特徴

- Socket Mode で動作
- リッチな応答（絵文字やブロックを使用）
- メンションの代理応答機能
- メンション履歴の記録と表示

## 必要条件

- Go 1.23.3 以上
- Redis

## 環境変数

以下の環境変数を設定する必要があります：

- `SLACK_BOT_TOKEN` - Slack ボットの OAuth トークン（`xoxb-`で始まる）
- `SLACK_APP_TOKEN` - Slack アプリのトークン（`xapp-`で始まる）
- `REDIS_URL` - Redis の URL（例：`redis://localhost:6379`）
- `SLACK_DOMAIN` - Slack のドメイン（オプション、デフォルトは `slack.com`）

オプションの環境変数：

- `AFK_START_MESSAGE` - 始業時のカスタムメッセージ
- `AFK_FINISH_MESSAGE` - 退勤時のカスタムメッセージ

環境変数は直接設定するか、`.env`ファイルを使用して設定できます：

```bash
# .env ファイルの例
SLACK_BOT_TOKEN=xoxb-your-token
SLACK_APP_TOKEN=xapp-your-token
REDIS_URL=redis://localhost:6379
```

`.env.sample`ファイルをコピーして`.env`ファイルを作成することもできます：

```bash
cp .env.sample .env
# 編集して適切な値を設定
```

## ビルド方法

```bash
cd go
go build -o slack-afk
```

## 実行方法

```bash
./slack-afk
```

または、環境変数を指定して実行：

```bash
SLACK_BOT_TOKEN=xoxb-xxx SLACK_APP_TOKEN=xapp-xxx REDIS_URL=redis://localhost:6379 ./slack-afk
```

## 実装の概要

- **メインパッケージ**: アプリケーションのエントリーポイント
- **Slack パッケージ**: ソケットモードの処理
- **ハンドラーパッケージ**: コマンドとイベントの処理
- **コマンドパッケージ**: 各コマンドの実装
- **ストアパッケージ**: Redis との連携
- **プレゼンテーションパッケージ**: リッチな応答の構築
