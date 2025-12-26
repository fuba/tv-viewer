# Docker で NVENC を使用する方法

## 現状

- ホストマシン: NVIDIA GeForce RTX 3080 搭載 ✓
- ホストFFmpeg: NVENC サポート有り ✓
- Docker内: NVIDIA Container Toolkit未インストールのためGPU非認識

## 推奨される解決方法

### 方法1: NVIDIA Container Toolkit をインストール（推奨）

```bash
# 1. リポジトリ設定
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
  sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
  sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list

# 2. インストール
sudo apt-get update
sudo apt-get install -y nvidia-container-toolkit

# 3. Docker設定
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker

# 4. 動作確認
docker run --rm --gpus all nvidia/cuda:12.0.1-base-ubuntu22.04 nvidia-smi

# 5. TV Viewer起動
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d
```

### 方法2: ホストでFFmpegを直接実行（暫定対応）

NVIDIA Container Toolkitをインストールできない場合の代替案：

1. **ホストでエンコーディングプロセスを実行**
```bash
# バックエンドはAPIのみ、FFmpegはホストで実行
# encoder.go を修正してホストのFFmpegを呼び出す
```

2. **開発用Docker設定（非推奨）**
```bash
# privileged モードでGPUデバイスを直接マウント
docker compose -f docker-compose.yml -f docker-compose.gpu-simple.yml up -d
```

### 方法3: FFmpeg静的バイナリ + ライブラリマウント（複雑）

NVIDIAライブラリを手動でマウントする方法（メンテナンスが困難）：
```yaml
volumes:
  - /lib/x86_64-linux-gnu/libnvidia-ml.so.1:/lib/x86_64-linux-gnu/libnvidia-ml.so.1:ro
  - /usr/lib/x86_64-linux-gnu/libnvidia-encode.so.1:/usr/lib/x86_64-linux-gnu/libnvidia-encode.so.1:ro
  # 他多数のライブラリ...
```

## 現在の制限事項

NVIDIA Container Toolkit未インストールのため、Dockerコンテナ内からGPUにアクセスできません。そのため：

- 現在はCPUエンコーディング（libx264）で動作
- NVENCを使用するにはNVIDIA Container Toolkitのインストールが必要

## パフォーマンス比較（予想値）

| エンコーダー | CPU使用率 | エンコード速度 | 品質 |
|------------|----------|--------------|-----|
| libx264 (CPU) | 15-30% | 1x | 高 |
| h264_nvenc (GPU) | 5-10% | 2-4x | 高 |

## まとめ

最も確実な方法は **NVIDIA Container Toolkit のインストール** です。これにより、Dockerコンテナから透過的にGPUを利用でき、NVENCによる高速エンコーディングが可能になります。