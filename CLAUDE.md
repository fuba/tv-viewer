# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## プロジェクト概要

日本のデジタルテレビ放送（地上波・BS・CS）をWebブラウザでリアルタイム視聴できるストリーミングシステム。Mirakurunから取得したMPEG2-TSストリームをFFmpegでHLS形式にエンコードし、HLS.jsを使ってブラウザで再生します。

## 技術スタック

- **バックエンド**: Go 1.21 + Gin Framework
- **フロントエンド**: Svelte 4 + TypeScript + Tailwind CSS
- **ストリーミング**: FFmpeg (NVENC GPU エンコーディング対応)
- **データベース**: SQLite
- **インフラ**: Docker Compose + NVIDIA Container Toolkit

## ビルド・開発コマンド

### Docker環境での開発

```bash
# 全サービスをビルド・起動
make build && make up

# GPU対応で起動 (NVENC使用)
docker compose -f docker-compose.yml up -d

# サービスの停止
make down

# ログ確認
make logs                # 全サービス
make logs-backend        # バックエンドのみ
make logs-frontend       # フロントエンドのみ
```

### ローカル開発環境

```bash
# 依存関係のインストール
make deps

# バックエンドとフロントエンドを同時に起動
make dev

# バックエンドのみ起動 (http://localhost:18088)
make dev-backend
# または
cd backend && go run cmd/server/main.go

# フロントエンドのみ起動 (http://localhost:18089)
make dev-frontend
# または
cd frontend && npm install && npm run dev
```

### テスト実行

```bash
# バックエンドのテスト全実行
make test
# または
cd backend && go test ./...

# 特定のパッケージのテスト
cd backend && go test ./internal/mirakurun -v

# フロントエンドの型チェック
cd frontend && npm run check
```

### ビルド・クリーンアップ

```bash
# 本番用ビルド
make prod-build
make prod

# クリーンアップ (データベース、ストリーム、ビルド成果物を削除)
make clean
```

## アーキテクチャ

### バックエンド構造 (Go)

```
backend/
├── cmd/
│   ├── server/main.go              # エントリーポイント (ポート18088)
│   └── test-mirakurun/main.go      # Mirakurun接続テストツール
├── internal/
│   ├── api/routes.go               # RESTful APIエンドポイント定義
│   ├── db/db.go                    # SQLiteデータベース初期化
│   ├── encoder/                    # FFmpegエンコーディング処理
│   │   ├── encoder.go              # メインエンコーダー・セッション管理
│   │   ├── nvenc.go                # NVIDIA NVENC GPU エンコーディング
│   │   ├── stream_info.go          # ストリーム情報解析 (ffprobe)
│   │   ├── ts_filter.go            # MPEG-TSパケットフィルタリング
│   │   └── log_parser.go           # FFmpegログリアルタイム解析
│   └── mirakurun/
│       ├── client.go               # Mirakurun APIクライアント
│       └── client_test.go
```

**主要な設計:**

- **セッション管理**: 各ストリームに対して独立したFFmpegプロセスを起動。最大同時エンコーディング数は1つ (encoder/encoder.go)
- **自動ストリーム選択**: 複数の映像・音声ストリームがある場合、最大解像度を自動選択 (SelectLargestVideoStream)
- **チャンネルタイプ別処理**:
  - **GR (地上波)**: CUDA HWデコーダー + デインターレース
  - **BS**: CUDA HWデコーダー + デインターレース (1080i対応)
  - **CS**: Copy mode (セグメンテーション違反回避)
- **HLS設定**: 2秒セグメント、最小3セグメント保持、1分以上前のセグメント自動削除
- **Service固有ストリーミング**: Full-seg (1080p) 優先、One-seg (320x180) 回避

### フロントエンド構造 (Svelte)

```
frontend/src/
├── main.ts                          # エントリーポイント
├── App.svelte                       # ルートコンポーネント
└── components/
    ├── VideoPlayer.svelte           # HLS.js統合・ビデオ再生
    ├── ChannelList.svelte           # チャンネル一覧 (GR/BS/CSタブ)
    ├── ProgramGuide.svelte          # 番組表表示
    ├── StreamSelector.svelte        # ストリーム選択UI
    ├── SubtitleRenderer.svelte      # ASS字幕レンダリング
    └── DebugLogs.svelte             # FFmpegログ表示
```

**主要な設計:**

- **HLS.js統合**: `VideoPlayer.svelte` で初期化、キャッシュ回避設定、エラー回復処理
- **状態管理**: Svelteのreactive declarations (`$:`) による宣言的な状態更新
- **字幕レンダリング**: ASS形式の字幕をCanvas APIで文字幅計測、リアルタイム同期表示
- **APIクライアント**: ネイティブfetch API (ライブラリ依存なし)、2秒ごとのFFmpegログポーリング
- **レスポンシブUI**: Tailwind CSSでモバイル・タブレット・デスクトップ対応

### 重要なデータフロー

```
1. チャンネル選択
   ChannelList → selectedChannel更新 → VideoPlayer.startStream()

2. ストリーミング開始
   /api/channels/{id}/stream → sessionId取得 → プレイリスト確認
   → HLS.js初期化 → /api/stream/{sessionId}/playlist.m3u8

3. FFmpegエンコーディング
   Mirakurunストリーム → TSフィルタリング (encoder/ts_filter.go)
   → FFmpeg (NVENC) → HLSセグメント出力 → Nginx静的配信

4. 字幕表示
   /api/stream/{sessionId}/subtitles.ass → ASS解析
   → video.timeupdate → 字幕レンダリング
```

## 開発時の重要なポイント

### FFmpegコマンド生成

- デフォルトは `pipe:0` (STDIN) 入力モード
- 環境変数 `USE_DIRECT_FFMPEG=true` で直接URL指定可能
- NVENC有効時は `mpeg2_cuvid` でハードウェアデコード + デインターレース (`-deint 2 -drop_second_field 1`)
- CS チャンネルは `-c copy` モード (トランスコードなし、6秒セグメント)

### MPEG-TSパケットフィルタリング

`encoder/ts_filter.go` で以下をフィルタリング:
- 同期バイト (0x47) チェック
- トランスポートエラーフラグ除外
- Nullパケット (PID 0x1FFF) 除外
- 無効なフレーム寸法 (0x0) 除外

環境変数 `SKIP_TS_FILTER=true` でフィルタリングをバイパス可能。

### Mirakurun API連携

- エンドポイント: 環境変数 `MIRAKURUN_URL` (デフォルト: `http://tuner:40772`)
- チャンネル取得: `/api/channels`
- サービス別ストリーミング: `/api/services/{serviceId}/stream` (Full-seg優先)
- 番組情報: `/api/programs?serviceId={id}`

### ASS字幕システム

`SubtitleRenderer.svelte` の実装詳細は `doc/30-subtitle-system.md` 参照。
- Canvas APIで文字幅計測 (WLMaru2004Emoji フォント使用)
- ASS カラー形式 (`&HAABBGGRR`) → RGB変換
- video.timeupdate イベントでリアルタイム同期

### 環境変数

| 変数 | デフォルト | 用途 |
|------|----------|------|
| `PORT` | 18088 | バックエンドAPIポート |
| `MIRAKURUN_URL` | http://tuner:40772 | Mirakurunエンドポイント |
| `ENCODING_QUALITY` | high | エンコーディング品質 (high/medium/low) |
| `USE_HEVC` | false | H.265/HEVC使用 |
| `SKIP_TS_FILTER` | false | TSフィルタリング無効化 |
| `USE_DIRECT_FFMPEG` | false | 直接URL入力モード |

### GPU / NVENC

- 初期化時に `nvidia-smi` でGPU検出
- `ffmpeg -encoders` で `h264_nvenc` / `hevc_nvenc` 対応確認
- `encoder/nvenc.go` で品質設定 (preset: p5/p4/p2, CQ値: 19/22/26)

### データベーススキーマ

`internal/db/db.go` で初期化される3テーブル:
1. **settings**: キー・バリュー形式の設定
2. **channels**: チャンネル情報キャッシュ
3. **programs**: 番組情報・スケジュール (インデックス: start_at, service_id)

## トラブルシューティング

### `playlist not ready` エラー
- FFmpeg解析時間 (`-analyzeduration`, `-probesize`) が不足している可能性
- 現在の設定: 地上波5秒、BS10秒 (encoder/encoder.go)

### `Invalid frame dimensions 0x0`
- 日本のデジタル放送の初期化時の一時的現象 (正常)
- 数秒後に正常なエンコーディングが開始される

### ポート競合
```bash
netstat -tlnp | grep :18088
netstat -tlnp | grep :18089
netstat -tlnp | grep :18090
```

### FFmpegログ確認
- ブラウザ内デバッグパネル (DebugLogs.svelte)
- または `/api/logs/{sessionId}` エンドポイント

## ドキュメント

プロジェクトの詳細ドキュメントは `doc/` ディレクトリにあります:
- `doc/01-tasks.md`: 開発タスク一覧
- `doc/20-specification.md`: 仕様書
- `doc/30-subtitle-system.md`: 字幕システム詳細
- `doc/12-mobile-access-guide.md`: モバイルアクセス設定
- `doc/41-nvenc-docker-guide.md`: NVENC Docker設定
- `doc/44-quality-tuning.md`: エンコーディング品質調整
- `doc/45-h264-vs-h265.md`: H.264 vs H.265 比較
