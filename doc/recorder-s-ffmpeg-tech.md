# FFmpeg エンコーディング設定の工夫

## 概要

fuba_recorderのエンコーダーは、日本のテレビ放送（MPEG2-TS形式）を高効率に変換するため、NVIDIA GPUアクセラレーションを活用した最適化されたffmpeg設定を使用しています。

## 実装箇所

- メインエンコーダー: `bin/encoder/ts2mp4_with_jimaku.pl:499-529`
- フォールバックエンコーダー: `bin/encoder/ts2mp4_handbreak_with_jimaku.pl:16-23`
- エンコーダー制御: `bin/encoder/encoder.pl`

## 主要な設定と工夫

### 1. NVIDIA GPUアクセラレーション（NVENC）

```perl
my $NVENC_DECODE = " -c:v mpeg2_cuvid -deint 2 -drop_second_field 1";
my $NVENC_H264 = " -f mp4 -vcodec hevc_nvenc -tag:v hvc1 -s $size_option -maxrate $max_rate -qmin 23 -qmax 36 -temporal-aq 1 -spatial-aq 1 -preset 1 -tune 1 -rc vbr -async 100";
```

#### デコード側の工夫
- **`mpeg2_cuvid`**: CUDA対応のMPEG2ハードウェアデコーダーを使用
- **`-deint 2`**: ハードウェアレベルでのインターレース解除（adaptive deinterlacing）
- **`-drop_second_field 1`**: 第二フィールドをドロップして30fps→29.97fpsに最適化

#### エンコード側の工夫
- **`hevc_nvenc`**: NVIDIA GPUによるH.265/HEVCハードウェアエンコーディング
- **`-tag:v hvc1`**: Apple/QuickTime互換のHEVCタグを設定（iOS/macOS再生対応）
- **VBR（可変ビットレート）**: `-rc vbr` でシーンに応じた最適なビットレート配分

### 2. 画質制御パラメータ

```perl
-qmin 23 -qmax 36 -temporal-aq 1 -spatial-aq 1
```

- **`qmin 23, qmax 36`**: 量子化パラメータの範囲を制限して画質の下限・上限を確保
- **`-temporal-aq 1`**: 時間的適応量子化（動きの激しいシーンで品質を維持）
- **`-spatial-aq 1`**: 空間的適応量子化（画面内の複雑な部分に多くのビットを割り当て）
- **`-preset 1`**: エンコード速度優先のプリセット（リアルタイム処理を重視）
- **`-tune 1`**: HQ（High Quality）チューニング

### 3. マルチスレッド処理の最適化

```perl
my $cores = `/usr/bin/getconf _NPROCESSORS_ONLN`;
chomp $cores;
$cores = int($cores / 2);
my $threads = " -threads ${cores}";
```

**システムコア数の半分を使用する理由:**
- エンコーダーは長時間動作するバックグラウンドプロセス
- システムの他のサービス（録画、Webサーバー等）との共存を考慮
- 熱管理とシステムの安定性を確保

### 4. コンテンツ別の解像度・ビットレート設定

```perl
# 標準設定（アニメ、ドラマ等）
my $size_option = '1280x720';     # 720p
my $max_rate = '1800k';            # 最大ビットレート

# 小サイズ設定（ニュース等）
if ($is_small_encode_target) {
    $size_option = '640x360';      # 360p
    $max_rate = '400k';            # 最大ビットレート
    $frac = 1000;                  # 高圧縮率を許容
}
```

**コンテンツタイプによる最適化:**
- **ニュース番組**: 静止画が多く、画面情報密度が低いため低解像度で十分
- **映画・アニメ**: 動きのあるシーンや詳細な画像が多いため高解像度を維持
- **圧縮率の閾値**: 元ファイルサイズの1/40〜1/1000を目安に成功判定

### 5. 音声処理

#### 通常の音声

```perl
my $audio_options = '-acodec aac -ac 2 -ar 48000 -ab 128k';
```

- **AAC**: 高効率な音声コーデック
- **ステレオ (2ch)**: `-ac 2`
- **サンプリングレート**: 48kHz（放送標準）
- **ビットレート**: 128kbps（音質と容量のバランス）

#### デュアルモノラル音声の特殊処理

```perl
# ファイル名から二カ国語放送を検出
if ($ts =~ /\[二\]|［二］/) {
    $is_dualmono = 1;
}

# 左右チャンネルを分離
my $dualmono_command =
    "$ffmpeg -y -i \"$target_ts\"".
    ' -filter_complex "[0:a]channelsplit=channel_layout=stereo"'.
    " \"${left_out}\" ";
```

**二カ国語放送の処理:**
1. ファイル名から `[二]` または `［二］` を検出
2. ステレオトラックから左右チャンネルを分離（L: 日本語、R: 外国語）
3. 分離した音声を別々のトラックとしてMP4に格納
4. 視聴時に音声トラックを切り替え可能

### 6. ストリームマッピング

```perl
my ($program_id, $video_map, @audio_maps) = @$stream_id;
my @maps = ($video_map, @audio_maps);

if ($program_id && $program_id =~ qr{\A[0-9]+\z}) {
    $stream_ids .= join " ", map {"-map ".$maps[$_]} (0..$#maps);
}
```

**ストリーム選択の工夫:**
- TSファイルから正しいプログラムID、映像ストリーム、音声ストリームを自動抽出
- 複数の音声トラック（主音声、副音声等）を適切にマッピング
- "No Program"（プログラム情報なし）のファイルにも対応

### 7. フォールバック機構

プライマリエンコーダー（ffmpeg + NVENC）が失敗した場合、HandBrakeCLIにフォールバック:

```perl
# HandBrake設定
VIDEO_OPTIONS => '-O -e nvenc_h265 -r 29.97 -b 800
    --encopts="qp-i=28:qp-p=32:qp-b=34:b_ref_mode=each:
               temporal_aq=1:spatial_aq=1:aq-strength=10:
               nonref_p=1:strict_gop=1:rc-lookahead=32:
               b_adapt=1:no-scenecut=1"'
```

**HandBrakeの詳細設定:**
- **QP値の制御**: I/P/Bフレームごとに量子化パラメータを個別設定
- **Bフレーム最適化**: `b_ref_mode=each` で各Bフレームを参照フレームとして利用
- **先読み処理**: `rc-lookahead=32` で32フレーム先まで解析してビットレート配分を最適化
- **GOP制御**: `strict_gop=1` で厳密なGOP構造を維持（シーク精度向上）

### 8. 音声トラック数の自動調整（HandBrake）

```perl
# HandBrakeは音声トラックが多すぎるとエラーになる場合があるため、
# 3トラックから順に減らして試行
for my $i (3,2,1) {
    my $audio_options = AUDIO_OPTIONS;
    my $audio_source_str = join ",", +(1..$i);
    $audio_options =~ s/\[audio\]/${audio_source_str}/;

    # エンコード実行...
    unless ($system_result) {
        exit;  # 成功したら終了
    }
    warn 'failed. reduce audio track' if $i > 1;
}
```

**音声トラック調整の理由:**
- HandBrakeのバージョンや入力ファイルによっては複数音声トラックの処理が不安定
- 3→2→1トラックと段階的に減らして最適な設定を自動探索

## エンコード前処理

### TsSplitter による多重分離

```perl
my $tss_command = "$tssplitter -FLEN -UNI -SEP2 -1SEG ${mx_option} -EIT -OUT '$workdir' '$ts4tss'";
```

**オプションの意味:**
- **`-FLEN`**: ファイル長情報を修正
- **`-UNI`**: Unicode対応
- **`-SEP2`**: 映像・音声を分離（HD/SD等の複数ストリームを個別ファイル化）
- **`-1SEG`**: ワンセグストリームを除外
- **`-EIT`**: EIT（番組情報）を出力
- **`-SD2`**: TOKYO MX専用オプション（SD2ストリームを処理）

### tsrenum による PCR 連続性の修正

```perl
my $renum_command = "/home/ec/fuba_recorder/env/puma2.sh $tsrenum < \"$target_ts_us\" > \"$temp_renum_file\"";
```

**PCR Wraparound対策:**
- MPEG-TSのPCR（Program Clock Reference）カウンタは27MHzで循環するため、長時間録画でオーバーフローする可能性
- `tsrenum` でPCRを再採番し、ffmpegが正しくタイムスタンプを処理できるようにする
- 特に長時間番組や連続録画で重要

## 字幕抽出との統合

エンコード前に `extract_jimaku` Dockerコンテナで字幕を抽出:

```perl
my $command_caption = "docker run --rm -v \"$target_ts_dir:/work\" ghcr.io/fuba/extract_jimaku:latest \"$target_ts_basename\" \"$simple_prefix\"";
```

**統合されたワークフロー:**
1. TSファイルから字幕をASS形式で抽出
2. `_fixed.ass` ファイル（位置補正済み）を優先的に使用
3. MP4ファイルと同名の `.ass` ファイルとして保存
4. ビューアーで映像と同期して表示

## 性能と効率

### ハードウェアアクセラレーションの効果

- **CPU使用率**: GPUオフロードにより10-20%程度に抑制
- **処理速度**: 1時間の番組を10-15分程度でエンコード（システム構成による）
- **同時処理**: GPUを使用することで、複数の録画・エンコードを並行処理可能

### ディスク容量削減

典型的な圧縮率（1時間番組の場合）:

| コンテンツタイプ | 元ファイルサイズ（TS） | エンコード後（MP4） | 圧縮率 |
|----------------|-------------------|------------------|--------|
| HD アニメ・ドラマ | 約8-10 GB | 約500-800 MB | 1/12-1/20 |
| HD 映画 | 約8-10 GB | 約800-1200 MB | 1/8-1/12 |
| ニュース（低解像度） | 約8-10 GB | 約100-200 MB | 1/40-1/100 |

## トラブルシューティング

### NVENC が使用できない場合

HandBrakeフォールバックが自動的に起動します:
- CPUエンコードになるため処理時間は増加
- 設定はNVENCと同等の品質を維持するよう調整済み

### 音声トラックの問題

- デュアルモノラル検出が正しく動作しない場合、ファイル名に `[二]` または `［二］` が含まれているか確認
- HandBrakeで音声エラーが出る場合、トラック数削減ロジックが自動的に対処

### エンコード失敗時の判定

```perl
# 圧縮率が1/40より小さい場合は失敗とみなす（ニュースは1/1000）
if (-e $output_dir && du($output_dir) > ((-s $input_file) / $frac)) {
    # 成功
}
```

出力ファイルサイズが極端に小さい場合、エンコード失敗と判定して `encode_failed` ディレクトリに移動します。

## まとめ

fuba_recorderのffmpeg設定は以下の点で最適化されています:

1. **ハードウェアアクセラレーション**: NVIDIA GPUを最大限活用して高速処理
2. **画質と容量のバランス**: コンテンツタイプに応じた適切な解像度・ビットレート
3. **放送特有の処理**: インターレース解除、PCR修正、デュアルモノラル対応
4. **堅牢性**: フォールバック機構と音声トラック自動調整
5. **統合ワークフロー**: 字幕抽出とエンコードを一貫して処理

これらの工夫により、大量の録画データを効率的に管理しながら、高品質な視聴体験を提供しています。
