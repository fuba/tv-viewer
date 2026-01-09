# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## プロジェクト概要

日本のデジタルテレビ放送（地上波・BS・CS）をWebブラウザでリアルタイム視聴できるストリーミングシステム。

**v2 (WebRTC版)**: Mirakurunから取得したMPEG2-TSストリームをFFmpegでH.264にエンコードし、Pion WebRTCでRTP化してブラウザに低遅延配信（目標200-500ms）。

## 技術スタック

- **バックエンド**: Go 1.21 + Gin Framework + Pion WebRTC
- **フロントエンド**: Svelte 4 + TypeScript + Tailwind CSS
- **ストリーミング**: FFmpeg (NVENC GPU エンコーディング対応) → WebRTC
- **データベース**: SQLite
- **インフラ**: Docker Compose + NVIDIA Container Toolkit

## アーキテクチャ (v2 WebRTC)

```
【WebRTC低遅延ストリーミング】
Mirakurun (MPEG2-TS)
    ↓ io.ReadCloser
FFmpeg (NVENC H.264 to stdout)
    ↓ NAL Units
Pion WebRTC (RTP Packetization)
    ↓ WebRTC PeerConnection
ブラウザ (RTCPeerConnection → video.srcObject)

遅延目標: 200-500ms (HLS比 約10倍改善)
```

### バックエンド構造 (Go)

```
backend/
├── cmd/
│   └── server/main.go              # エントリーポイント (ポート18088)
├── internal/
│   ├── api/
│   │   ├── routes.go               # RESTful APIエンドポイント
│   │   └── webrtc_routes.go        # WebRTCシグナリングエンドポイント
│   ├── encoder/
│   │   ├── encoder.go              # エンコーダー基盤（NVENC管理等）
│   │   ├── webrtc_encoder.go       # WebRTC用エンコーダー
│   │   ├── nvenc.go                # NVIDIA NVENC GPU エンコーディング
│   │   ├── stream_info.go          # ストリーム情報解析 (ffprobe)
│   │   ├── ts_filter.go            # MPEG-TSパケットフィルタリング
│   │   └── log_parser.go           # FFmpegログリアルタイム解析
│   ├── webrtc/                     # WebRTC v2 コアモジュール
│   │   ├── peer.go                 # PeerConnection管理
│   │   ├── h264_parser.go          # H.264 NALユニット解析
│   │   ├── rtp_packetizer.go       # RTPパケット化
│   │   └── signaling.go            # シグナリングメッセージ定義
│   ├── mirakurun/
│   │   └── client.go               # Mirakurun APIクライアント
│   └── wsmonitor/                  # WebSocketセッション監視 (レガシー)
│       └── hub.go                  # 中央Hub管理
```

### フロントエンド構造 (Svelte)

```
frontend/src/
├── main.ts                          # エントリーポイント
├── App.svelte                       # ルートコンポーネント
├── lib/
│   ├── webrtc/                      # WebRTC クライアント
│   │   ├── RTCClient.ts             # WebRTC接続管理
│   │   └── types.ts                 # 型定義
│   └── types/
│       └── epg.ts                   # EPG型定義
└── components/
    ├── VideoPlayer.svelte           # WebRTC再生
    ├── ChannelList.svelte           # チャンネル一覧 (GR/BS/CSタブ)
    ├── EPGGrid.svelte               # 番組表グリッド (ラテ欄)
    ├── ProgramGuide.svelte          # 番組表表示
    ├── MetaBar.svelte               # 接続状態表示バー
    ├── OverlayPanel.svelte          # オーバーレイパネルUI
    ├── SettingsPanel.svelte         # 設定パネル
    └── TunerStatus.svelte           # チューナー状態表示
```

## WebRTC v2 データフロー

```
1. チャンネル選択
   ChannelList → selectedChannel更新 → VideoPlayer.startStream()

2. WebRTC接続開始
   RTCClient.connect() → WebSocket接続 (/api/ws/webrtc/{channelId})
   → 'stream-start' メッセージ送信

3. サーバー側処理
   WebSocketハンドラー → Mirakurunストリーム取得
   → StartWebRTCEncoding() → FFmpeg (H.264 stdout)
   → PeerConnection作成 → SDP Offer送信

4. シグナリング
   クライアント: Answer送信 → ICE候補交換
   サーバー: ICE候補送信 → 接続確立

5. メディアストリーミング
   FFmpeg stdout → H264Parser → RTPPacketizer
   → VideoTrack.WriteRTP() → ブラウザで再生

6. 字幕 (DataChannel)
   FFmpeg字幕抽出 → DataChannel送信 → ブラウザで描画
```

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
```

### ローカル開発環境

```bash
# バックエンドのみ起動 (http://localhost:18088)
cd backend && go run cmd/server/main.go

# フロントエンドのみ起動 (http://localhost:18089)
cd frontend && npm install && npm run dev
```

### テスト実行

```bash
# バックエンドのテスト
cd backend && go test ./...

# フロントエンドの型チェック
cd frontend && npm run check
```

## WebRTC APIエンドポイント

| エンドポイント | メソッド | 説明 |
|--------------|---------|------|
| `/api/ws/webrtc/:channelId` | WS | WebSocketシグナリング |
| `/api/webrtc/channels/:id/stream` | POST | WebRTCストリーム開始 |
| `/api/webrtc/offer` | POST | SDP Offer処理 |
| `/api/webrtc/ice-candidate` | POST | ICE候補追加 |
| `/api/webrtc/peer/:peerId` | DELETE | ピア切断 |
| `/api/webrtc/status` | GET | WebRTC状態確認 |

## 環境変数

| 変数 | デフォルト | 用途 |
|------|----------|------|
| `PORT` | 18088 | バックエンドAPIポート |
| `MIRAKURUN_URL` | http://tuner:40772 | Mirakurunエンドポイント |
| `ENCODING_QUALITY` | high | エンコーディング品質 (high/medium/low) |
| `SKIP_TS_FILTER` | false | TSフィルタリング無効化 |

## 開発時の重要なポイント

### WebRTC FFmpegコマンド (webrtc_encoder.go)

```bash
ffmpeg -f mpegts -i pipe:0 \
  -c:v h264_nvenc -preset p4 -tune ll -rc cbr \
  -b:v 4M -bufsize 500k -g 30 -bf 0 \
  -profile:v baseline -level 4.0 \
  -f h264 -bsf:v h264_mp4toannexb pipe:1
```

- **低遅延設定**: `-tune ll`, `-bf 0` (Bフレームなし), 小さいバッファ
- **Baseline Profile**: WebRTC互換性のため
- **Annex B形式**: RTPパケット化に必要

### Pion WebRTC実装

- `PeerManager`: ピア接続の作成・管理
- `H264Parser`: FFmpeg stdoutからNALユニット抽出
- `RTPPacketizer`: NALユニットをRTPパケットに変換
- `TrackLocalStaticRTP`: ビデオトラックへのRTP書き込み

### フロントエンド RTCClient

```typescript
const client = new RTCClient({
  channelId: 'GR01',
  onTrack: (track, stream) => {
    videoElement.srcObject = stream;
  },
  onConnectionStateChange: (state) => {
    console.log('Connection:', state);
  }
});
await client.connect();
```

## トラブルシューティング

### WebRTC接続が確立しない
- ICE候補の交換を確認
- ローカルネットワーク内ではSTUN/TURN不要
- ブラウザのWebRTC設定を確認

### 映像が表示されない
- FFmpegのH.264出力を確認 (`/api/logs/{channelId}`)
- NALユニットの解析ログを確認
- RTPパケット送信エラーを確認

### 遅延が大きい
- FFmpegバッファサイズを確認 (`-bufsize`)
- GOP間隔を確認 (`-g`)
- ネットワーク帯域を確認

## ドキュメント

- `doc/01-tasks.md`: 開発タスク一覧
- `doc/20-specification.md`: 仕様書
- `doc/41-nvenc-docker-guide.md`: NVENC Docker設定
