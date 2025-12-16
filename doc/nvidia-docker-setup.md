# NVIDIA Docker Setup Guide

DockerコンテナでNVENCを使用するためのセットアップガイド

## 前提条件

- NVIDIA GPUドライバーがインストール済み ✓ (確認済み: RTX 3080)
- Docker がインストール済み ✓

## NVIDIA Container Toolkit のインストール

### Ubuntu/Debian の場合

1. **リポジトリの設定**
```bash
distribution=$(. /etc/os-release;echo $ID$VERSION_ID)
curl -s -L https://nvidia.github.io/nvidia-docker/gpgkey | sudo apt-key add -
curl -s -L https://nvidia.github.io/nvidia-docker/$distribution/nvidia-docker.list | sudo tee /etc/apt/sources.list.d/nvidia-docker.list
```

2. **パッケージのインストール**
```bash
sudo apt-get update
sudo apt-get install -y nvidia-container-toolkit
```

3. **Docker デーモンの再起動**
```bash
sudo systemctl restart docker
```

### 別の方法（新しい手順）

```bash
# NVIDIA GPG キーの追加
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg

# リポジトリの追加
curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
  sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
  sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list

# インストール
sudo apt-get update
sudo apt-get install -y nvidia-container-toolkit

# Docker デーモンの設定
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker
```

## インストール確認

```bash
# GPU アクセステスト
docker run --rm --gpus all nvidia/cuda:12.0.1-base-ubuntu22.04 nvidia-smi
```

## TV Viewer での使用方法

1. **GPU付きでコンテナを起動**
```bash
# NVENC を有効にして起動
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d
```

2. **NVENC状態確認**
```bash
curl http://localhost:8082/api/nvenc/status
```

## トラブルシューティング

### エラー: "could not select device driver"
NVIDIA Container Toolkit がインストールされていません。上記の手順でインストールしてください。

### エラー: "no NVIDIA GPU device is present"
- `nvidia-smi` でGPUが認識されているか確認
- Docker デーモンが再起動されているか確認

### Docker Compose v1 の場合
古いバージョンでは `runtime: nvidia` を使用：
```yaml
services:
  backend:
    runtime: nvidia
    environment:
      - NVIDIA_VISIBLE_DEVICES=all
```