# 開発履歴・実装ノート

本ドキュメントでは、TV Viewerの開発過程で実装した機能、解決した問題、技術的な決定について詳しく記録しています。

## 開発フェーズ概要

### Phase 1: 基盤システム構築 ✅
- プロジェクト初期化とGitHub設定
- Docker Compose環境構築
- Go バックエンド + Svelte フロントエンド基盤

### Phase 2: コア機能実装 ✅
- Mirakurun API連携
- FFmpeg MPEG2-TS → HLS エンコーディング
- HLS.js ブラウザ再生機能

### Phase 3: 問題解決・最適化 ✅
- FFmpegプロセス管理問題の解決
- デッドロック問題の修正
- ストリーム解析パラメータの最適化

### Phase 4: UI/UX改善 ✅
- BSチャンネル表示の改善
- デバッグ機能の強化
- チャンネル名表示の最適化

### Phase 5: ハードウェアエンコーディング対応 ✅
- NVIDIA NVENCサポート追加
- GPU検出機能の実装
- エンコーダー動的切り替え

## 詳細実装記録

### 1. システム基盤構築

#### Docker構成の最適化
**問題**: 初期のbridge networkingでMirakurun接続に問題が発生
**解決策**: Host networkingモードに変更
```yaml
# docker-compose.yml
services:
  backend:
    network_mode: host  # 低レイテンシー通信
  frontend:
    network_mode: host
```

**効果**: Mirakurunとの通信レイテンシーが大幅改善

#### ポート設定の整理
- Backend: `localhost:8082`
- Frontend: `localhost:3001` 
- Mirakurun: `localhost:40772`

### 2. FFmpegエンコーディング最適化

#### 重大な問題: FFmpegプロセス起動失敗
**症状**: 
- APIレスポンスは正常だがFFmpegプロセスが起動しない
- ログが出力されない
- HLSセグメントが生成されない

**原因**: 
1. stderrパイプの処理でデッドロック発生
2. mutexの二重ロック
3. gorutine間の競合状態

**解決過程**:
```go
// 問題のあったコード（デッドロックの原因）
stderrPipe, _ := cmd.StderrPipe()
go func() {
    buf := make([]byte, 4096)
    for {
        n, err := stderrPipe.Read(buf)
        if err != nil { break }
        if n > 0 {
            // mutex ロックでデッドロック発生
            e.mu.Lock()
            e.logs[channelID] = append(e.logs[channelID], string(buf[:n]))
            e.mu.Unlock()
        }
    }
}()
```

```go
// 修正後（デッドロック回避）
stderrPipe, err := cmd.StderrPipe()
if err != nil { return nil, err }

go func() {
    defer stderrPipe.Close()
    buf := make([]byte, 4096)
    for {
        n, err := stderrPipe.Read(buf)
        if err != nil { break }
        if n > 0 {
            logMsg := string(buf[:n])
            log.Printf("FFmpeg [%s]: %s", channelID, logMsg)
            
            // 別のgorutineでmutex操作（デッドロック回避）
            go func(msg string) {
                e.mu.Lock()
                defer e.mu.Unlock()
                if e.logs[channelID] == nil {
                    e.logs[channelID] = []string{}
                }
                e.logs[channelID] = append(e.logs[channelID], msg)
                if len(e.logs[channelID]) > 1000 {
                    e.logs[channelID] = e.logs[channelID][len(e.logs[channelID])-1000:]
                }
            }(logMsg)
        }
    }
}()
```

#### ストリーム解析パラメータの最適化

**問題**: `Invalid frame dimensions 0x0` エラーが大量発生
**症状**: 日本のデジタル放送特有のマルチプログラム構造で解析に失敗

**解決策**: 
```go
// Before: 不十分な解析時間
"-analyzeduration", "1000000",  // 1秒
"-probesize", "500000",         // 500KB

// After: 放送形式に最適化
// 地上波（シンプル構造）
"-analyzeduration", "5000000",  // 5秒
"-probesize", "2000000",        // 2MB
"-thread_queue_size", "512",

// BS（マルチプログラム構造）  
"-analyzeduration", "10000000", // 10秒
"-probesize", "5000000",        // 5MB
"-thread_queue_size", "1024",
```

**効果**: フレーム検出成功率が大幅向上、エンコーディング安定性改善

#### HLSセグメント設定の最適化

**目標**: 低レイテンシーとブラウザ互換性の両立
```go
// 地上波設定（高速スタートアップ重視）
"-hls_time", "2",                    // 2秒セグメント
"-hls_list_size", "3",              // 3セグメント保持
"-hls_flags", "delete_segments+round_durations+independent_segments+omit_endlist",
"-hls_allow_cache", "0",            // キャッシュ無効化
"-hls_init_time", "0.5",           // 初期セグメント0.5秒
"-force_key_frames", "expr:gte(t,n_forced*1)", // キーフレーム間隔

// BS設定（品質重視）
"-hls_time", "2",
"-hls_list_size", "3", 
"-crf", "23",                       // 高品質設定
```

### 3. フロントエンド実装

#### デバッグ機能の強化

**要求**: コンソールではなく画面内でのデバッグ表示
**実装**:
```typescript
// VideoPlayer.svelte
let debugLogs: string[] = []
let ffmpegLogs: string[] = []

function addLog(message: string, type: 'info' | 'error' | 'success' = 'info') {
    const timestamp = new Date().toLocaleTimeString()
    const logEntry = `[${timestamp}] ${message}`
    debugLogs = [logEntry, ...debugLogs.slice(0, 99999)] // 100,000行対応
    console.log(message)
}

// FFmpegログ取得
async function fetchFFmpegLogs(channel: any) {
    try {
        const channelId = channel.channel || channel.id
        const encodedChannelId = encodeURIComponent(channelId)
        const response = await fetch(`/api/logs/${encodedChannelId}`)
        
        if (response.ok) {
            const data = await response.json()
            ffmpegLogs = data.logs || []
        }
    } catch (error) {
        addLog(`Failed to fetch FFmpeg logs: ${error}`, 'error')
    }
}
```

#### チャンネル管理の改善

**問題**: BSチャンネルの名前表示が不適切
- Before: `BS:BS01_0` 
- After: `ＢＳ朝日１`

**実装**:
```typescript
// ChannelList.svelte - サービス名からの表示名取得
<div class="font-medium">
    {#if channel.services && channel.services.length > 0}
        {channel.services[0].name}  // 実際の放送局名
    {:else}
        {channel.name}
    {/if}
</div>

// タイプ別カラーコーディング
{#if channel.type === 'GR'}
    <span class="text-green-400">地上波</span>
{:else if channel.type === 'BS'}
    <span class="text-blue-400">BS</span>
{/if}
```

**結果**: 28個のBSチャンネルが適切な放送局名で表示

### 4. エラー処理・信頼性向上

#### 重複リクエスト防止
**問題**: チャンネル切り替え時の重複リクエストでプロセス競合
**解決策**:
```typescript
let isStartingStream = false
let currentChannel: any = null

$: if (selectedChannel && videoElement) {
    // 重複リクエスト防止
    if (!isStartingStream && selectedChannel !== currentChannel) {
        startStream(selectedChannel)
    }
}
```

#### CSチャンネル無効化
**問題**: CSチャンネル処理時にセグメンテーション違反
**対応**: 安全のため一時的に無効化
```go
// encoder.go - CS channels safety disable
isCSChannel := strings.HasPrefix(channelID, "CS")
if isCSChannel {
    log.Printf("CS channel %s is currently not supported due to encoding issues", channelID)
    input.Close()
    return nil, fmt.Errorf("CS channels are temporarily unavailable")
}
```

### 5. パフォーマンス最適化

#### 同時エンコーディング制限
**理由**: リソース使用量とFFmpeg安定性のバランス
```go
type Encoder struct {
    mu            sync.Mutex
    sessions      map[string]*Session
    maxConcurrent int  // デフォルト: 1
    logs          map[string][]string
}

// 容量チェック・既存セッション停止
if len(e.sessions) >= e.maxConcurrent {
    log.Printf("Maximum concurrent encodings (%d) reached. Stopping all existing sessions.", e.maxConcurrent)
    for cid, session := range e.sessions {
        e.stopSession(session)
        delete(e.sessions, cid)
    }
}
```

#### HLS.jsの最適化設定
```typescript
hls = new Hls({
    debug: true,
    enableWorker: false,
    lowLatencyMode: true,
    backBufferLength: 6,      // 3 segments * 2 seconds
    maxBufferLength: 12,      // 6 segments * 2 seconds  
    manifestLoadingTimeOut: 10000,
    manifestLoadingRetryDelay: 500,
    manifestLoadingMaxRetry: 20,
    // キャッシュ問題回避
    xhrSetup: function(xhr: XMLHttpRequest, url: string) {
        const separator = url.includes('?') ? '&' : '?'
        const cacheParam = `_t=${Date.now()}&_r=${Math.random()}`
        xhr.open('GET', url + separator + cacheParam, true)
        xhr.setRequestHeader('Cache-Control', 'no-cache, no-store, must-revalidate')
    }
})
```

## 技術的な学習・発見

### 1. 日本のデジタル放送の特性
- **マルチプログラム構造**: 特にBSでは1つの周波数に複数番組
- **ARIB規格**: 独特な字幕・データ放送形式
- **初期化時間**: ストリーム開始時の解析時間が重要

### 2. FFmpegの最適化ポイント
- **プロービングパラメータ**: 放送形式に応じた調整が必須
- **スレッドキュー**: マルチプログラム対応に重要
- **HLS設定**: ブラウザ互換性と低レイテンシーの両立

### 3. Go並行処理のベストプラクティス
- **mutexの適切な使用**: デッドロック回避
- **gorutine管理**: リソースリーク防止
- **エラーハンドリング**: graceful shutdown

### 4. Docker最適化
- **Host networking**: 低レイテンシー用途では有効
- **静的バイナリ**: FFmpegの配布方法
- **ボリューム管理**: HLSセグメントの効率的な処理

## 今後の拡張可能性

### 短期的改善
- [ ] CS チャンネル対応（セグフォルト修正）
- [ ] 同時エンコーディング数の設定可能化
- [ ] 字幕表示の実装
- [ ] 番組表機能の改善

### 長期的機能
- [ ] 録画機能
- [ ] タイムシフト再生
- [ ] 複数チューナー対応
- [ ] 負荷分散機能

## パフォーマンス指標

### 現在の性能
- **エンコーディング遅延**: 
  - CPU (libx264): 2-4秒
  - GPU (NVENC): 1-2秒（予想値）
- **HLSセグメント長**: 2秒
- **同時視聴者数**: 1（同時エンコーディング制限により）
- **リソース使用量**: 
  - CPU (libx264): FFmpeg 1プロセスあたり15-30%
  - GPU (NVENC): FFmpeg 1プロセスあたり5-10% CPU + GPU使用
  - メモリ: 約200MB
  - ディスク: 一時的なHLSセグメント（自動削除）

### 測定例
```bash
# チャンネル切り替え時間測定
time curl -s http://localhost:8082/api/channels/16/stream
# 結果: 約1.3秒（FFmpeg起動 + 初期解析）

# セグメント生成間隔
ls -la /app/stream/16/ | grep segment
# 結果: 2秒間隔で安定生成
```

## トラブルシューティング履歴

### 解決済み問題一覧
1. **FFmpegプロセス起動失敗** → デッドロック修正
2. **`playlist not ready`エラー** → 解析パラメータ最適化  
3. **`Invalid frame dimensions`** → 日本放送特性対応
4. **重複ストリーム要求** → フロントエンド状態管理改善
5. **BSチャンネル名表示** → サービス名表示実装

これらの経験により、日本のデジタル放送システムに特化した安定したTV視聴システムが完成しました。