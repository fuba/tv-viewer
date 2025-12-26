# ストリーム自動選択機能の実装

**実装日**: 2024-12-25
**カテゴリ**: エンコーディング最適化

## 概要

エンコーディング開始時に、最も解像度の高いビデオストリームを自動的に検出・選択する機能を実装しました。これにより、複数のビデオストリームを持つチャンネルでも、常に最高品質のストリームでエンコーディングが行われます。

## 背景

### 問題点

1. **ストリームインデックスの不統一**
   - 地デジチャンネルでビデオストリームのインデックスが異なる
   - 一部のチャンネルは`#0`、一部は`#1`を使用
   - 固定インデックス（`0:v:0`）では一部のチャンネルで失敗する可能性

2. **複数解像度への対応不足**
   - BS/CSチャンネルでは複数のビデオストリームが存在する可能性
   - 手動選択なしでは最適なストリームが選べない

3. **ストリームマッピングの問題**
   - FFmpegの相対インデックス（`0:v:0`）とAPI/フロントエンドの絶対インデックスが不一致
   - ユーザーが選択したストリームと実際にエンコードされるストリームが異なる可能性

## 調査結果

### 地デジ全チャンネルの分析

主要な地デジチャンネルのストリーム構成を調査：

| チャンネル | ビデオStream# | 解像度 | コーデック |
|-----------|-------------|--------|-----------|
| NHK総合 | #0 | 1440x1080 | MPEG-2 |
| NHK Eテレ | #0 | 1440x1080 | MPEG-2 |
| 日テレ | #1 | 1440x1080 | MPEG-2 |
| TBS | #1 | 1440x1080 | MPEG-2 |
| フジテレビ | #1 | 1440x1080 | MPEG-2 |
| テレビ朝日 | #0 | 1440x1080 | MPEG-2 |
| テレビ東京 | #1 | 1440x1080 | MPEG-2 |
| TOKYO MX | #0 | 1440x1080 | MPEG-2 |

**重要な発見**:
- すべてのチャンネルで解像度は1440x1080で統一
- ビデオストリームは1つのみ
- **ストリームインデックスがチャンネルによって異なる**（#0または#1）

### Mirakurun API

- サービスIDは組み合わせID（networkId + serviceId）を使用する必要がある
- 例: テレビ東京は`1072`ではなく`3274201072`

## 実装内容

### 1. 自動選択ロジック (`encoder.go`)

```go
// Auto-select largest video stream if not specified and streamURL is available
if videoStreamIndex == -1 && streamURL != "" {
    streamInfo, err := GetStreamInfo(streamURL)
    if err != nil {
        log.Printf("Failed to get stream info for auto-selection: %v, using default", err)
    } else {
        selectedIndex := SelectLargestVideoStream(streamInfo)
        if selectedIndex >= 0 {
            videoStreamIndex = selectedIndex
            log.Printf("Auto-selected video stream %d (largest resolution) for channel %s", videoStreamIndex, channelID)
        }
    }
}
```

### 2. 最大解像度ストリーム選択関数 (`stream_info.go`)

```go
// SelectLargestVideoStream returns the index of the video stream with the largest resolution
func SelectLargestVideoStream(info *StreamInfo) int {
    if info == nil {
        return -1
    }

    videoStreams := FilterStreamsByType(info.Streams, "video")
    if len(videoStreams) == 0 {
        return -1
    }

    // Find the stream with the largest resolution (width * height)
    largestIndex := -1
    largestResolution := 0

    for _, stream := range videoStreams {
        resolution := stream.Width * stream.Height
        if resolution > largestResolution {
            largestResolution = resolution
            largestIndex = stream.Index
        }
    }

    return largestIndex
}
```

### 3. 絶対インデックスマッピング

FFmpegの`-map`オプションを相対インデックスから絶対インデックスに変更：

**変更前**:
```go
cmd.Args = append(cmd.Args, "-map", fmt.Sprintf("0:v:%d", videoStreamIndex))
```

**変更後**:
```go
cmd.Args = append(cmd.Args, "-map", fmt.Sprintf("0:%d", videoStreamIndex))
```

これにより、API/フロントエンドから送信される絶対インデックス値と一致するようになります。

### 4. 最大同時エンコーディング数の調整

リソース節約のため、最大同時エンコーディング数を3から1に変更：

```go
maxConcurrent: 1, // Allow 1 concurrent encoding
```

## 動作フロー

1. **エンコーディング開始リクエスト**
   - `videoStreamIndex == -1`（自動選択モード）
   - `streamURL`が提供されている場合

2. **ストリーム情報の取得**
   - ffprobeを使用してストリームURLを解析
   - 全ビデオストリームの解像度を取得

3. **最大解像度ストリームの選択**
   - 解像度（幅×高さ）を計算
   - 最も大きい解像度のストリームのインデックスを取得

4. **FFmpegコマンド生成**
   - 選択されたインデックスを使用して`-map`オプションを設定
   - 絶対インデックスでマッピング

5. **ログ出力**
   - 選択されたストリームのインデックスと解像度
   - 全ビデオストリームの情報（デバッグ用）

## テスト結果

- ✅ 全地デジチャンネルで正しいビデオストリームが選択される
- ✅ ストリームインデックス#0と#1の両方のチャンネルで動作確認
- ✅ ログに選択されたストリーム情報が正しく出力される
- ✅ GPUエンコーディングが正常に動作
- ✅ リソース使用量が適切（CPU 12%, メモリ 136MB）

## 影響範囲

### 変更されたファイル

- `backend/internal/encoder/encoder.go`: 自動選択ロジックとコメント追加
- `backend/internal/encoder/stream_info.go`: SelectLargestVideoStream関数追加
- `doc/architecture.md`: ストリーム自動選択セクション追加

### 互換性

- 既存の手動ストリーム選択機能は引き続き動作
- 自動選択は`videoStreamIndex == -1`の場合のみ実行
- フロントエンドの変更は不要

## 今後の改善案

1. **オーディオストリームの自動選択**
   - 現在はビデオのみ対応
   - 複数音声トラック（副音声など）への対応

2. **品質プリファレンス**
   - 解像度だけでなく、ビットレート、フレームレートも考慮
   - ユーザー設定による優先順位

3. **エラーハンドリングの強化**
   - ffprobeタイムアウト時のフォールバック
   - ストリーム情報取得失敗時の詳細ログ

4. **パフォーマンス最適化**
   - ストリーム情報のキャッシュ
   - 並列処理の最適化

## 参考情報

- FFmpeg `-map` documentation: https://ffmpeg.org/ffmpeg.html#Stream-specifiers
- Mirakurun API: https://github.com/Chinachu/Mirakurun
- 日本の地デジ規格: ARIB STD-B10, ARIB STD-B25
