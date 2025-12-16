# TV Viewer

日本のデジタルテレビ（地上波・BS）をWebブラウザで視聴できるリアルタイムストリーミングシステム

## 概要

このプロジェクトは、Mirakurunを使用して日本のデジタルテレビ放送を受信し、Webブラウザでリアルタイム視聴を可能にするシステムです。MPEG2-TSストリームをHLS（HTTP Live Streaming）形式に変換し、HLS.jsを使用してブラウザで再生します。

## 主要機能

### ✅ 実装済み機能

- **📺 リアルタイム視聴**
  - 地上波デジタル放送対応
  - BSデジタル放送対応（28チャンネル）
  - リアルタイムMPEG2-TS → HLS変換

- **🎮 ユーザーインターフェース**
  - モダンなWebUI（Svelte + Tailwind CSS）
  - 直感的なチャンネル選択
  - 放送局名の正確な表示（BS朝日、BS-TBS など）
  - タイプ別カラーコーディング（地上波：緑、BS：青）

- **🔧 デバッグ・監視機能**
  - 画面内デバッグログ表示（100,000行対応）
  - FFmpegリアルタイムログ表示
  - エンコーディングプロセス監視
  - API健全性チェック

- **⚡ パフォーマンス**
  - 最適化されたFFmpeg設定
  - 効率的なHLSセグメント管理（2秒セグメント）
  - デッドロック回避設計
  - 同時エンコーディング制限（リソース管理）

## 技術スタック

### バックエンド
- **Go 1.21**: 高性能なAPIサーバー
- **Gin Framework**: RESTful API
- **FFmpeg**: MPEG2-TS → HLS エンコーディング
- **SQLite**: 設定・データ管理

### フロントエンド
- **Svelte**: モダンなWebフレームワーク
- **TypeScript**: 型安全な開発
- **Tailwind CSS**: ユーティリティファーストCSS
- **HLS.js**: ブラウザHLS再生

### インフラ
- **Docker & Docker Compose**: コンテナ化
- **NVIDIA Container Toolkit**: GPU コンテナサポート
- **Nginx**: リバースプロキシ
- **Host Networking**: 低レイテンシー通信
- **NVENC**: NVIDIA GPUハードウェアエンコーディング

## システム構成

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Mirakurun     │───▶│   TV Viewer      │───▶│  Web Browser    │
│  (TV Tuner)     │    │   Backend        │    │   (HLS.js)      │
│                 │    │                  │    │                 │
│ MPEG2-TS Stream │    │ Go + FFmpeg      │    │ Svelte Frontend │
└─────────────────┘    │ HLS Transcoding  │    └─────────────────┘
                       └──────────────────┘
```

## クイックスタート

### 前提条件
- Docker & Docker Compose
- Mirakurunサーバーが稼働中（`localhost:40772`）
- （オプション）NVIDIA GPU + NVIDIA Container Toolkit（NVENC使用時）

### 起動方法

```bash
# リポジトリをクローン
git clone <your-repo-url>
cd tv-viewer

# コンテナをビルド・起動
docker compose up -d

# GPU搭載システムでNVENCを使用する場合
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d

# フロントエンドにアクセス
open http://localhost:3001
```

### 環境変数

```bash
# docker-compose.yml で設定済み
MIRAKURUN_URL=http://localhost:40772
PORT=8082
```

## API仕様

### エンドポイント

| Method | Endpoint | 説明 |
|--------|----------|------|
| GET | `/api/health` | 健全性チェック |
| GET | `/api/channels` | チャンネル一覧取得 |
| GET | `/api/channels/{id}/stream` | ストリーミング開始 |
| GET | `/api/logs/{channel}` | FFmpegログ取得 |
| GET | `/api/sessions` | アクティブセッション一覧 |
| POST | `/api/sessions/stop-all` | 全セッション停止 |

### NVENC ハードウェアエンコーディング

| Method | Endpoint | 説明 |
|--------|----------|------|
| GET | `/api/nvenc/status` | NVENCステータス確認 |
| POST | `/api/nvenc/toggle` | NVENC有効/無効切替 |

### HLSストリーミング

| Method | Endpoint | 説明 |
|--------|----------|------|
| GET | `/api/stream/{channel}/playlist.m3u8` | HLSプレイリスト |
| GET | `/api/stream/{channel}/segment{n}.ts` | HLSセグメント |
| GET | `/api/stream/{channel}/subtitles.ass` | 字幕ファイル |

## NVENC ハードウェアエンコーディング

### 機能
- **NVIDIA GPU 自動検出**: RTX/GTX シリーズで自動有効化
- **コーデック選択**: H.264 または H.265(HEVC)
- **品質設定**: High/Medium/Low プリセット
- **CPU 負荷減**: 15-30% → 10-12% にCPU使用率削減

### 使用方法

```bash
# GPU対応で起動
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d

# NVENC ステータス確認
curl http://localhost:8082/api/nvenc/status

# H.265 を有効にする場合
export USE_HEVC=true
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d
```

### パフォーマンス

| 項目 | CPUエンコード | NVENC H.264 | NVENC H.265 |
|------|------------------|-------------|-------------|
| CPU使用率 | 15-30% | 10-12% | 10-12% |
| GPU使用率 | 0% | 1-5% | 1-5% |
| GPUメモリ | 0MB | 272MB | 330MB |
| 圧縮効率 | 基準 | 基準 | +15-25% |
| 画質 | 高 | 高 | 最高 |

## 対応チャンネル

### 地上波デジタル放送
- NHK総合、NHK Eテレ
- 日本テレビ、テレビ朝日、TBS、テレビ東京、フジテレビ
- その他地域局

### BSデジタル放送（28チャンネル）
- NHK BS、NHK BS プレミアム
- BS朝日、BS-TBS、BSテレ東、BSフジ
- BS日テレ、その他専門チャンネル

### 制限事項
- CSチャンネルは現在無効化（セグメンテーション違反回避）
- 同時エンコーディング数制限（デフォルト：1）

## トラブルシューティング

### よくある問題

**問題**: `playlist not ready` エラー
```bash
# FFmpeg解析時間の調整で解決済み
# analyzeduration: 地上波5秒、BS10秒に最適化
```

**問題**: `Invalid frame dimensions 0x0`
```bash
# 正常動作: 日本のデジタル放送の初期化時の一時的現象
# 数秒後に正常なエンコーディングが開始されます
```

**問題**: Docker コンテナが起動しない
```bash
# ポート競合確認
docker compose logs
netstat -tlnp | grep :3001
```

### ログ確認

```bash
# バックエンドログ
docker logs tv-viewer-backend-1 -f

# フロントエンドログ  
docker logs tv-viewer-frontend-1 -f

# FFmpegログ（ブラウザ内）
# デバッグモードを有効にして確認
```

## 開発履歴

詳細な開発履歴と実装ノートは [DEVELOPMENT.md](DEVELOPMENT.md) を参照してください。

## ライセンス

This project is licensed under CC0 (Creative Commons Zero).

## 謝辞

- [Mirakurun](https://github.com/Chinachu/Mirakurun) - デジタル放送受信
- [FFmpeg](https://ffmpeg.org/) - メディア変換・ストリーミング
- [HLS.js](https://github.com/video-dev/hls.js/) - ブラウザHLS再生