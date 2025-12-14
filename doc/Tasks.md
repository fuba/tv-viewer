# TV Viewer Development Tasks

## Overview
Mirakurunを利用したWebベースのTV視聴アプリケーション開発

## Task List

### Phase 1: 基盤構築
- [x] Git初期化とタスクリスト作成
- [ ] GitHub privateリポジトリ作成と初期コミット
- [ ] プロジェクト構造の作成
  - Golang バックエンド (API server)
  - Svelte フロントエンド (SPA)
  - Docker構成 (golang, ffmpeg, frontend)
  - SQLite データベース設定

### Phase 2: Mirakurun連携
- [ ] Mirakurun API接続テストの実装
  - チャンネル一覧取得
  - ストリーム取得 
  - 番組情報取得
- [ ] Mirakurun設定機能の実装
  - サーバーURL設定
  - 接続テスト機能

### Phase 3: ストリーミング機能
- [ ] FFmpegを使用したMPEG2-TS→HLSエンコーディング機能の実装
  - Dockerイメージ作成
  - エンコーディングパラメータ最適化
  - セグメント管理
- [ ] ARIB字幕抽出・ASS形式変換機能の実装
  - FFmpegによる字幕抽出
  - ASS形式での出力
  - リアルタイム処理

### Phase 4: チャンネル管理
- [ ] チャンネル切り替え機能の実装
  - エンコーダープロセス管理
  - 滑らかな切り替え処理
  - リソース管理

### Phase 5: UI実装
- [ ] 番組表表示機能の実装
  - 番組データ取得・キャッシュ
  - UI実装
  - チャンネル選択連携
- [ ] フロントエンドUI実装
  - HLS動画プレイヤー
  - ASS字幕レンダリング
  - チャンネル選択UI
  - レスポンシブ対応

### Phase 6: 統合・最適化
- [ ] Docker Compose設定
- [ ] パフォーマンス最適化
- [ ] エラーハンドリング強化
- [ ] ログシステム実装

## Technical Stack
- Backend: Go
- Frontend: Svelte + TypeScript + Tailwind CSS
- Streaming: FFmpeg (Docker)
- Database: SQLite
- Container: Docker Compose