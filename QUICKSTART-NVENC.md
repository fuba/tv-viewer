# NVENC/HEVC クイックスタートガイド

GPU ハードウェアエンコーディングを使用して TV Viewer を高性能化するためのガイド

## 前提条件

- ✅ NVIDIA GPU (GTX 1000 シリーズ以降推奨)
- ✅ Docker & Docker Compose
- ✅ Mirakurun サーバー稼働中

## 1. NVIDIA Container Toolkit インストール

```bash
# パッケージリポジトリの設定
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg

curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
  sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
  sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list

# インストール
sudo apt-get update
sudo apt-get install -y nvidia-container-toolkit

# Docker設定と再起動
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker

# 動作確認
docker run --rm --gpus all nvidia/cuda:12.0.1-base-ubuntu22.04 nvidia-smi
```

## 2. TV Viewer セットアップ

```bash
# リポジトリクローン
git clone <your-repo-url>
cd tv-viewer

# GPU対応でビルド・起動
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d

# ブラウザでアクセス
open http://localhost:3001
```

## 3. NVENC 動作確認

```bash
# NVENC ステータス確認
curl http://localhost:18088/api/nvenc/status

# 期待される出力:
# {
#   "nvenc_available": true,
#   "encoders": ["h264_nvenc", "hevc_nvenc"]
# }

# GPU 使用状況モニタリング
nvidia-smi dmon -s pucvmet -c 10
```

## 4. エンコーディング設定

### H.264 使用 (デフォルト)
```bash
# 標準設定
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d
```

### H.265 使用 (高効率)
```bash
# docker-compose.gpu.yml を編集
# USE_HEVC=true に設定

# または環境変数で指定
export USE_HEVC=true
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d --force-recreate
```

### 品質設定
```bash
# 高品質 (デフォルト)
export ENCODING_QUALITY=high

# 中品質 (バランス重視)
export ENCODING_QUALITY=medium

# 低品質 (高速)
export ENCODING_QUALITY=low

docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d --force-recreate
```

## 5. パフォーマンス確認

### リアルタイム監視
```bash
# GPU 使用率
nvidia-smi -l 1

# CPU 使用率 (コンテナ内)
docker exec tv-viewer-backend-1 ps aux | grep ffmpeg

# ネットワーク使用量
docker stats tv-viewer-backend-1
```

### ログ確認
```bash
# NVENC 初期化ログ
docker logs tv-viewer-backend-1 | grep -i nvenc

# FFmpeg エンコーディングログ
curl http://localhost:18088/api/logs/20
```

## 6. トラブルシューティング

### GPU が検出されない
```bash
# ホストでGPU確認
nvidia-smi

# NVIDIA Container Toolkit 確認
docker run --rm --gpus all nvidia/cuda:12.0.1-base-ubuntu22.04 nvidia-smi

# Dockerデーモン再起動
sudo systemctl restart docker
```

### NVENC が利用できない
```bash
# FFmpeg のエンコーダー確認
docker exec tv-viewer-backend-1 /usr/local/bin/ffmpeg -encoders | grep nvenc

# 手動でGPU対応ビルド実行
docker compose -f docker-compose.yml -f docker-compose.gpu.yml build backend --no-cache
```

### パフォーマンスが改善しない
```bash
# CPU使用率確認 (10-12%程度になる想定)
docker exec tv-viewer-backend-1 ps aux | grep ffmpeg

# GPU使用率確認 (1-5%程度)
nvidia-smi

# エンコーダー確認
docker logs tv-viewer-backend-1 | grep "encoder.*nvenc"
```

## 7. 設定ファイル

### 環境変数一覧

| 変数名 | デフォルト | 説明 |
|--------|-----------|------|
| `USE_HEVC` | false | H.265エンコーディング有効化 |
| `ENCODING_QUALITY` | high | 品質設定 (high/medium/low) |
| `NVIDIA_VISIBLE_DEVICES` | all | 使用するGPU |
| `NVIDIA_DRIVER_CAPABILITIES` | compute,utility,video | GPU機能 |

### docker-compose.gpu.yml
GPU対応の設定ファイル。以下の機能を追加:
- NVIDIA Container Toolkit 統合
- CUDA ランタイムイメージ使用
- NVENC 対応 FFmpeg バイナリ
- 環境変数による設定制御

## 8. 期待されるパフォーマンス向上

| 項目 | CPU エンコード | NVENC H.264 | NVENC H.265 |
|------|---------------|-------------|-------------|
| CPU 使用率 | 15-30% | **10-12%** | **10-12%** |
| エンコード速度 | 1x | **1.2-1.5x** | **1.2-1.5x** |
| 並行処理能力 | 制限あり | **向上** | **向上** |
| 画質 | 高 | 高 | **最高** |
| ファイルサイズ | 基準 | 基準 | **-15~25%** |

NVENC を活用して、より効率的な TV ストリーミングをお楽しみください！