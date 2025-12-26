# CS放送対応技術ガイド

## 概要

CS（Communication Satellite）放送は衛星経由で配信される有料放送サービスです。本システムでは50個のCSサービスに対応しています。

## システム構成

### CS放送の特徴

1. **マルチプログラム構造**: 1つの物理チャンネルに複数のサービス（番組）が含まれる
2. **複雑なストリーム**: 地上波/BSより分析に時間がかかる
3. **サービスID**: 各放送局に固有のID（296, 299, 339など）

### 対応チャンネル

| 物理CH | サービス数 | 主なサービス例 |
|-------|----------|--------------|
| CS2   | 4        | TBSチャンネル1, テレ朝チャンネル1/2, ディズニージュニア |
| CS4   | 4        | 時代劇専門ch, スカイA, エンタメ～テレ, MTV |
| CS6   | 8        | ホームドラマCH, カートゥーン, アニマルプラネット等 |
| CS8   | 2        | 日テレNEWS24, 東映チャンネル |
| CS10  | 3        | スポーツライブ+, 衛星劇場, スカチャン1 |
| CS12  | 4        | GAORA, ナショジオ, エムオン！, キッズステーション |
| CS14  | 4        | スーパー！ドラマTV, ヒストリーch, ザ・シネマ等 |
| CS16  | 6        | アクションch, SKY STAGE, MusicJapan等 |
| CS18  | 4        | ゴルフネットワーク, チャンネル銀河等 |
| CS20  | 4        | フジテレビONE/TWO/NEXT, スペースシャワーTV |
| CS22  | 3        | TBSチャンネル2, TBS NEWS, Dlife |
| CS24  | 4        | 日テレジータス, MONDO TV, 日テレプラス等 |

**総計**: 50サービス

## 技術実装

### 1. エンコード設定

#### 高速化のアプローチ
```go
// 従来（20秒分析）→ 改善後（0.5秒分析）
"-analyzeduration", "500000",    // 0.5秒
"-probesize", "32768",          // 32KB
```

#### CS専用FFmpegオプション
```bash
ffmpeg \
  -f mpegts \
  -fflags +genpts+discardcorrupt \
  -analyzeduration 500000 \
  -probesize 32768 \
  -avoid_negative_ts make_zero \
  -thread_queue_size 512 \
  -i pipe:0 \
  -map 0:v:0? -map 0:a:0? \
  -c:v h264_nvenc -preset p5 -tune hq \
  -c:a aac -b:a 128k \
  -f hls \
  -hls_time 2 \
  -hls_list_size 5 \
  playlist.m3u8
```

### 2. フロントエンド対応

#### サービス展開ロジック
```typescript
// 物理チャンネル → 個別サービス展開
allChannels.forEach((ch) => {
  if (ch.type === 'CS') {
    ch.services.forEach((service) => {
      processedChannels.push({
        ...ch,
        serviceId: service.serviceId,
        serviceName: service.name,
        displayName: service.name,
        originalChannel: ch.channel,
        isService: true
      })
    })
  }
})
```

#### UI表示
- **物理チャンネル**: CS2, CS4, CS6...（12個）
- **個別サービス**: TBSチャンネル1, テレ朝チャンネル1...（50個）

### 3. API設計

#### 現在の実装
```bash
GET /api/channels/:id/stream
# CS2 → CS2全体のストリーム（最初のサービス）
```

#### 理想的な実装（今後の改善）
```bash
GET /api/services/:serviceId/stream  
# 296 → TBSチャンネル1のみ
# 299 → テレ朝チャンネル2のみ
```

## 運用ガイド

### CS放送の視聴手順

1. **チャンネル一覧でCSタブを選択**
2. **50個のサービスから選択**（例：TBSチャンネル1）
3. **ストリーミング開始**（自動でCS2チャンネル使用）
4. **HLS再生開始**（2秒セグメント）

### トラブルシューティング

#### エンコード開始が遅い場合
```bash
# FFmpegログ確認
curl http://localhost:18088/api/logs/CS2

# セッション状況確認  
curl http://localhost:18088/api/sessions
```

#### よくある問題

1. **分析タイムアウト**
   - `analyzeduration`を増加（500000 → 1000000）
   - `probesize`を増加（32768 → 65536）

2. **ストリーム選択失敗**
   - マッピングを必須に変更（`0:v:0?` → `0:v:0`）
   - 特定サービス指定（今後の改善）

3. **音声同期問題**
   - `aresample=async=1`フィルター追加済み
   - より長いバッファサイズ設定

## パフォーマンス指標

### エンコード起動時間
- **CS8**: 約20秒で初回セグメント生成 ✅
- **CS2**: 分析中（改善の余地あり）
- **CS6**: 分析中（改善の余地あり）

### リソース使用量
- **CPU使用率**: 約20-30%（NVENC使用時）
- **メモリ使用量**: 約500MB-1GB
- **同時チャンネル数**: 最大3チャンネル

### ネットワーク
- **セグメントサイズ**: 約2-3MB（2秒）
- **ビットレート**: 約10Mbps（H.264/AAC）

## 開発者向け情報

### ファイル構造
```
backend/internal/encoder/
├── encoder.go              # CS対応エンコーダー
├── nvenc.go                # NVENC設定
└── quality_config.go       # 品質設定

frontend/src/components/
├── ChannelList.svelte      # CSサービス展開
├── VideoPlayer.svelte      # HLS再生
└── DebugLogs.svelte        # デバッグ表示
```

### 設定例

#### 開発環境
```bash
# 開発サーバー起動
docker compose -f docker-compose.dev.yml up

# CS2チャンネルテスト
curl http://localhost:18088/api/channels/CS2/stream
```

#### 本番環境
```bash
# GPU対応起動
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d

# アクセス
# Frontend: http://localhost:3001
# Backend: http://localhost:18088
```

### ログ分析

#### 成功パターン
```
Stream mapping:
  Stream #0:1 -> #0:0 (mpeg2video (native) -> h264 (h264_nvenc))
  Stream #0:2 -> #0:1 (aac (native) -> aac (native))
```

#### 失敗パターン  
```
No video/audio streams found
Could not find video stream
Segmentation fault (core dumped)
```

## 今後の改善計画

### Phase 1: サービス単位ストリーミング
- サービスID指定でのストリーミングAPI
- 番組表のサービス単位取得

### Phase 2: CS特化機能
- EPG（電子番組表）対応
- 字幕/多重音声対応
- 録画機能

### Phase 3: UX改善
- お気に入りCS局設定
- 視聴履歴
- レコメンデーション機能