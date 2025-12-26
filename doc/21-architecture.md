# TV Viewer アーキテクチャドキュメント

## 概要

TV ViewerはMirakurunからのテレビストリームをWebブラウザで視聴できるようにするアプリケーションです。リアルタイムエンコーディング、GPUアクセラレーション、HLSストリーミングを特徴としています。

## システム構成

### コンポーネント構成

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Mirakurun     │────▶│    Backend      │────▶│     Nginx       │
│  (Tuner Server) │     │   (Go/Gin)      │     │    (Proxy)      │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                               │                         │
                               ▼                         ▼
                        ┌─────────────────┐     ┌─────────────────┐
                        │     FFmpeg      │     │    Frontend     │
                        │  (GPU/NVENC)    │     │    (Svelte)     │
                        └─────────────────┘     └─────────────────┘
```

### Docker構成

- **backend**: Goで実装されたAPIサーバー（NVIDIA CUDA対応）
- **frontend**: Svelteで実装されたWebフロントエンド
- **nginx**: リバースプロキシとストリーミング最適化

## バックエンド詳細

### 主要コンポーネント

1. **APIサーバー (Gin)**
   - ポート: 18088
   - エンドポイント:
     - `/api/channels` - チャンネル一覧
     - `/api/channels/:id/stream` - ストリーミング開始
     - `/api/stream/:channel/playlist.m3u8` - HLSプレイリスト
     - `/api/stream/:channel/:segment` - HLSセグメント
     - `/api/nvenc/status` - GPUエンコーディング状態

2. **エンコーダー管理**
   - 最大同時エンコーディング数: 1（一度に1チャンネルのみ視聴可能）
   - 自動的に古いセッションを停止（新しいチャンネルを視聴時）
   - リアルタイムログ収集
   - **自動ストリーム選択**: 最大解像度のビデオストリームを自動検出・選択

3. **GPUエンコーディング (NVENC)**
   - 対応コーデック: h264_nvenc, hevc_nvenc, av1_nvenc
   - 品質プロファイル:
     - **high**: CQ=19, preset=p5, 最大10Mbps
     - **medium**: CQ=22, preset=p4, 最大6Mbps
     - **low**: CQ=26, preset=p2, 最大4Mbps

### ストリーム自動選択

エンコーディング開始時に、最適なビデオストリームを自動的に選択します：

1. **ffprobeでストリーム情報を取得**
   - ストリームURLが利用可能な場合のみ実行
   - 全ビデオストリームの解像度を解析

2. **最大解像度のストリームを選択**
   - 解像度（幅×高さ）が最も大きいストリームを選択
   - 日本の地デジでは通常1440x1080 MPEG-2の1ストリームのみ
   - ストリームインデックスはチャンネルによって異なる（#0または#1）

3. **絶対インデックスでマッピング**
   - FFmpegの`-map`オプションに絶対インデックスを使用（例: `0:1`）
   - フロントエンドから送信されるインデックス値と一致

### FFmpegパイプライン

```bash
# 標準チャンネル (GR/BS)
ffmpeg -f mpegts \
  -fflags +genpts+discardcorrupt+igndts+ignidx \
  -analyzeduration 5000000 \
  -probesize 2000000 \
  -i pipe:0 \
  -map 0:1 \  # 自動選択されたビデオストリーム（絶対インデックス）
  -map 0:2 \  # 自動選択されたオーディオストリーム（絶対インデックス）
  -c:v h264_nvenc -preset p5 -tune hq -rc vbr -cq 19 \
  -c:a aac -b:a 128k \
  -f hls -hls_time 2 -hls_list_size 3 \
  playlist.m3u8

# CSチャンネル（問題のあるストリーム用）
ffmpeg -f mpegts \
  -analyzeduration 10000000 \
  -probesize 5000000 \
  -c copy \  # 再エンコードなし
  -f hls -hls_time 6 -hls_list_size 6 \
  playlist.m3u8
```

### データベース

- SQLite3を使用
- チャンネル情報とプログラム情報を管理
- `/app/data/tv-viewer.db`

## フロントエンド詳細

### 技術スタック

- **Svelte + TypeScript**: UIフレームワーク
- **HLS.js**: 動画プレイヤー
- **Tailwind CSS**: スタイリング
- **Vite**: ビルドツール

### HLS.js設定（最適化済み）

```javascript
{
  enableWorker: true,          // Webワーカーで処理
  lowLatencyMode: false,       // 低レイテンシモード無効（安定性重視）
  backBufferLength: 30,        // 30秒のバックバッファ
  maxBufferLength: 60,         // 最大60秒のバッファ
  maxMaxBufferLength: 120,     // 絶対最大120秒
  maxBufferSize: 60000000,     // 60MBのバッファサイズ
  startFragPrefetch: true,     // セグメントの先読み
  progressive: true            // プログレッシブダウンロード
}
```

### 主要コンポーネント

- **VideoPlayer.svelte**: メインの動画プレイヤー
- **ChannelList.svelte**: チャンネル選択UI
- **StreamSelector.svelte**: ストリーム選択（複数音声/映像）
- **ProgramInfo.svelte**: 番組情報表示
- **DebugLogs.svelte**: デバッグログ表示

## Nginxプロキシ設定

### 最適化ポイント

1. **バッファリング制御**
   ```nginx
   proxy_buffering off;         # プレイリストは無効
   proxy_buffer_size 1m;        # TSファイルは1MB
   proxy_buffers 8 1m;          # 8個のバッファ
   ```

2. **キャッシュ戦略**
   - プレイリスト(`.m3u8`): キャッシュ無効
   - セグメント(`.ts`): 1時間キャッシュ

3. **パフォーマンス最適化**
   - `sendfile on`: カーネルレベルのファイル転送
   - `tcp_nopush on`: TCPパケットの最適化
   - `worker_connections 4096`: 大量の同時接続

## ストリーミングフロー

1. **チャンネル選択**
   - ユーザーがチャンネルを選択
   - フロントエンドが`/api/channels/:id/stream`を呼び出し

2. **エンコーディング開始**
   - バックエンドがMirakurunからストリームを取得
   - FFmpegプロセスを起動（GPUエンコーディング）
   - HLSセグメントの生成開始

3. **プレイリスト配信**
   - HLS.jsが定期的にプレイリストを取得
   - 新しいセグメントを検出して自動ダウンロード

4. **セグメント配信**
   - Nginxがセグメントファイルを効率的に配信
   - 適切なバッファリングで安定した再生

## パフォーマンス特性

### GPUエンコーディング
- CPU使用率: 約5-10%（GPUエンコード時）
- GPU使用率: 約50-60%（エンコード時）
- メモリ使用量: 約300-400MB/ストリーム

### ネットワーク
- セグメントサイズ: 約1-2MB（2秒セグメント）
- 帯域幅: 4-10Mbps/ストリーム（品質設定による）

### レスポンスタイム
- 初回プレイリスト: 0.5-2秒
- チャンネル切替: 2-4秒
- セグメントダウンロード: 50-200ms

## セキュリティ考慮事項

1. **CORS設定**: 全オリジン許可（開発環境）
2. **入力検証**: チャンネルIDのサニタイズ
3. **リソース制限**: 最大1同時エンコーディング（リソース節約）

## 今後の改善点

1. **認証機能の実装**
2. **録画機能の追加**
3. **トランスコーディングプロファイルの追加**
4. **クラスタリング対応**
5. **メトリクス収集とモニタリング**

## トラブルシューティング

### 動画がカクカクする場合
- HLS.jsのバッファ設定を確認
- ネットワーク帯域幅を確認
- GPUエンコーディングが有効か確認

### ストリームが開始しない場合
- Mirakurunとの接続を確認
- FFmpegログを確認（`/api/logs/:channel`）
- ディスク容量を確認

### GPUが使用されない場合
- `nvidia-smi`でGPUを確認
- Dockerのデバイス設定を確認
- FFmpegのNVENCサポートを確認