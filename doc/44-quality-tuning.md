# 画質調整ガイド

## 現在の高品質設定

- **CQ (Constant Quality)**: 19（0-51、低いほど高品質）
- **最大ビットレート**: 10Mbps
- **プリセット**: p5（高品質・低速）
- **プロファイル**: High
- **Temporal AQ/Spatial AQ**: 有効

## さらに高画質にする場合

### 1. 環境変数で調整
```bash
# 最高画質（CQ 15、最大20Mbps）
docker compose -f docker-compose.yml -f docker-compose.gpu.yml \
  -e ENCODING_QUALITY=high \
  up -d
```

### 2. nvenc.go を直接編集

```go
// 超高品質設定例
case "ultra":
    return []string{
        "-c:v", "h264_nvenc",
        "-preset", "p6", // p6 or p7 for best quality
        "-tune", "hq",
        "-rc", "vbr",
        "-cq", "15", // さらに低い値
        "-b:v", "0",
        "-maxrate", "20M", // 20Mbps
        "-bufsize", "40M",
        "-profile:v", "high",
        "-level", "4.2",
        "-b_ref_mode", "2",
        "-temporal-aq", "1",
        "-spatial-aq", "1",
        "-lookahead", "32", // より良い先読み
        "-aq-strength", "15", // AQ強度
    }
```

## ビットレートモード

### VBR（現在使用中）
- 品質優先
- ファイルサイズ変動

### CBR（固定ビットレート）
```go
"-rc", "cbr",
"-b:v", "8M", // 8Mbps固定
```

## 推奨設定

| 用途 | CQ値 | 最大ビットレート | プリセット |
|-----|------|-----------------|-----------|
| 高画質視聴 | 19 | 10M | p5 |
| 通常視聴 | 22 | 6M | p4 |
| モバイル | 26 | 4M | p2 |

## GPU負荷との バランス

- p5/p6: GPU使用率 10-15%
- p7: GPU使用率 15-20%
- CQ値を下げると、ビットレートとGPUメモリ使用量が増加