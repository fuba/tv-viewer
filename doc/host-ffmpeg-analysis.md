# ホストFFmpeg使用の分析

## なぜホストのFFmpegを直接使うのが難しいか

### 1. **実行環境の違い**
- コンテナ: Debian bookworm-slim
- ホスト: Ubuntu (おそらく異なるバージョン)
- libc等の基本ライブラリのバージョン不一致

### 2. **パスの違い**
- コンテナ内: `/app/stream/`
- ホスト: `/home/ec/tv-viewer/stream/`
- FFmpegの引数でパスを変換する必要がある

### 3. **プロセス管理**
- Goプログラムはコンテナ内で実行
- FFmpegプロセスをホストで実行すると、プロセス管理が複雑

## 実用的な解決策

### 解決策1: SSH経由でホストFFmpegを実行
```go
// コンテナからSSH経由でホストのFFmpegを実行
cmd := exec.Command("ssh", "host", "/usr/bin/ffmpeg", args...)
```
**問題**: SSH設定、認証、レイテンシー

### 解決策2: 共有ソケット/Named Pipe
```bash
# ホストでFFmpegサービスを実行
mkfifo /tmp/ffmpeg-pipe
/usr/bin/ffmpeg -i /tmp/ffmpeg-pipe ...
```
**問題**: 複雑な実装、エラーハンドリング

### 解決策3: NVENC対応の静的FFmpegバイナリ（推奨）
```dockerfile
# NVIDIA公式のFFmpegビルドを使用
RUN wget https://developer.nvidia.com/ffmpeg-static-nvenc.tar.gz
```
**利点**: シンプル、ポータブル、依存関係なし

### 解決策4: NVIDIA Container Toolkit（最良）
```bash
sudo apt-get install nvidia-container-toolkit
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d
```
**利点**: 標準的、メンテナンスが容易、Dockerネイティブ

## 現在の制限

1. **johnvansickle版FFmpeg**: GPUサポートなし（CPUのみ）
2. **ライブラリ依存**: ホストFFmpegは動的リンクで多数の依存関係
3. **デバイスアクセス**: `/dev/nvidia*`へのアクセス権限

## 結論

ホストのFFmpegを使うことは技術的には可能ですが、以下の理由で推奨されません：

1. **複雑性**: パス変換、ライブラリ依存、プロセス管理
2. **保守性**: ホスト環境に依存、ポータビリティなし
3. **標準から逸脱**: Docker のベストプラクティスに反する

**推奨**: NVIDIA Container Toolkitをインストールして標準的な方法でGPUを使用する