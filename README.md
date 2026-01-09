# TV Viewer

日本のデジタルテレビ（地上波・BS・CS）をWebブラウザで視聴できるリアルタイムストリーミングシステム

## 概要

このプロジェクトは、Mirakurunを使用して日本のデジタルテレビ放送を受信し、Webブラウザでリアルタイム視聴を可能にするシステムです。MPEG2-TSストリームをWebRTC経由で低遅延配信します。

## 主要機能

### 実装済み機能

- **📺 リアルタイム視聴**
  - 地上波デジタル放送対応
  - BSデジタル放送対応
  - CSデジタル放送対応
  - WebRTCによる低遅延ストリーミング
  - NVENC/H.264ハードウェアエンコーディング

- **🎮 ユーザーインターフェース**
  - モダンなWebUI（Svelte + Tailwind CSS）
  - レスポンシブデザイン（モバイル・タブレット対応）
  - **番組表（EPGグリッド）** - 新聞のラテ欄スタイル
  - タブ切り替えチャンネル選択（地上波・BS・CS）
  - オーバーレイパネルUI
  - チューナー状態表示

- **🔧 監視機能**
  - WebSocket接続状態監視
  - チューナーリーク防止（接続タイムアウト処理）
  - FFmpegリアルタイムログ表示
  - API健全性チェック

- **⚡ パフォーマンス**
  - WebRTCによる低遅延配信（2-3秒）
  - NVENC（NVIDIA GPU）ハードウェア加速
  - 効率的なH.264/Opusエンコーディング

## 技術スタック

### バックエンド
- **Go 1.21**: 高性能なAPIサーバー
- **Gin Framework**: RESTful API
- **Pion WebRTC**: WebRTCサーバー実装
- **FFmpeg**: H.264/Opus エンコーディング
- **SQLite**: 設定・データ管理

### フロントエンド
- **Svelte**: モダンなWebフレームワーク
- **TypeScript**: 型安全な開発
- **Tailwind CSS**: ユーティリティファーストCSS
- **WebRTC API**: 低遅延ビデオ再生

### インフラ
- **Docker & Docker Compose**: コンテナ化
- **NVIDIA Container Toolkit**: GPU コンテナサポート
- **Nginx**: リバースプロキシ
- **NVENC**: NVIDIA GPUハードウェアエンコーディング

## システム構成

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Mirakurun     │───▶│   TV Viewer      │───▶│  Web Browser    │
│  (TV Tuner)     │    │   Backend        │    │   (WebRTC)      │
│                 │    │                  │    │                 │
│ MPEG2-TS Stream │    │ Go + FFmpeg      │    │ Svelte Frontend │
└─────────────────┘    │ WebRTC Server    │    └─────────────────┘
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

# コンテナをビルド・起動（GPU対応）
docker compose up -d

# フロントエンドにアクセス
open http://localhost:18090
```

### 環境変数

```bash
# docker-compose.yml で設定
MIRAKURUN_URL=http://tuner:40772
PORT=18088
ENCODING_QUALITY=medium  # high/medium/low
```

## API仕様

### エンドポイント

| Method | Endpoint | 説明 |
|--------|----------|------|
| GET | `/api/health` | 健全性チェック |
| GET | `/api/channels` | チャンネル一覧取得 |
| GET | `/api/programs?serviceId={id}` | 番組情報取得 |
| GET | `/api/epg` | EPGデータ取得（全チャンネル） |
| GET | `/api/tuners` | チューナー状態取得 |
| GET | `/api/logs/{channel}` | FFmpegログ取得 |
| GET | `/api/sessions` | アクティブセッション一覧 |
| POST | `/api/sessions/stop-all` | 全セッション停止 |

### WebRTC

| Method | Endpoint | 説明 |
|--------|----------|------|
| WS | `/api/ws/webrtc/{channelId}` | WebRTCシグナリング |
| GET | `/api/webrtc/status` | WebRTC接続状態 |

### NVENC ハードウェアエンコーディング

| Method | Endpoint | 説明 |
|--------|----------|------|
| GET | `/api/nvenc/status` | NVENCステータス確認 |
| POST | `/api/nvenc/toggle` | NVENC有効/無効切替 |

## NVENC ハードウェアエンコーディング

### 機能
- **NVIDIA GPU 自動検出**: RTX/GTX シリーズで自動有効化
- **低遅延チューニング**: WebRTC向け最適化
- **CPU 負荷軽減**: ハードウェアエンコードによる効率化

### パフォーマンス

| 項目 | CPUエンコード | NVENC H.264 |
|------|------------------|-------------|
| CPU使用率 | 15-30% | 10-12% |
| GPU使用率 | 0% | 1-5% |
| 遅延 | 3-5秒 | 2-3秒 |

## 対応チャンネル

### 地上波デジタル放送
- NHK総合、NHK Eテレ
- 日本テレビ、テレビ朝日、TBS、テレビ東京、フジテレビ
- その他地域局

### BSデジタル放送
- NHK BS、NHK BS プレミアム
- BS朝日、BS-TBS、BSテレ東、BSフジ
- BS日テレ、その他専門チャンネル

### CSデジタル放送
- 各種専門チャンネル対応

## トラブルシューティング

### よくある問題

**問題**: WebRTC接続が確立しない
```bash
# WebSocket接続を確認
# ブラウザの開発者ツールでNetworkタブを確認
```

**問題**: 映像が表示されない
```bash
# FFmpegログを確認
docker logs tv-viewer-backend-1 -f
```

**問題**: チューナーがリークする
```bash
# WebSocket接続タイムアウト設定を確認
# 自動的に30秒後にチューナー解放
```

### ログ確認

```bash
# バックエンドログ
docker logs tv-viewer-backend-1 -f

# フロントエンドログ
docker logs tv-viewer-frontend-1 -f
```

## ライセンス

This project is licensed under CC0 (Creative Commons Zero).

## 謝辞

- [Mirakurun](https://github.com/Chinachu/Mirakurun) - デジタル放送受信
- [FFmpeg](https://ffmpeg.org/) - メディア変換・ストリーミング
- [Pion WebRTC](https://github.com/pion/webrtc) - Go WebRTC実装
