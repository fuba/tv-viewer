# Mobile Access Instructions

## AndroidスマホからTV Viewerにアクセスする方法

### 前提条件
- AndroidスマホとこのPCが同じWi-Fiネットワークに接続されていること

### アクセスURL
```
http://192.168.10.20:3001
```

### 手順
1. AndroidスマホのChromeブラウザを開く
2. アドレスバーに上記URLを入力
3. TV Viewerの画面が表示される

### トラブルシューティング

#### 接続できない場合
1. **ネットワーク確認**
   - スマホとPCが同じWi-Fiに接続されているか確認
   - PCのIPアドレスが変わっていないか確認

2. **ファイアウォール設定**
   - PCのファイアウォールでポート3001が許可されているか確認
   - Windowsの場合: Windows Defender ファイアウォール
   - Linuxの場合: ufw, firewalld等

3. **別のブラウザで試す**
   - Firefox
   - Opera
   - Brave

### 動画再生の注意事項
- HLS形式の動画はChromeで再生可能
- データ通信量に注意（Wi-Fi推奨）
- 画質は自動調整されない（固定ビットレート）

### API直接アクセス（デバッグ用）
- チャンネル一覧: http://192.168.10.20:3001/api/channels
- ヘルスチェック: http://192.168.10.20:3001/api/health