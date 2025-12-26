tv-viewer
--

mirakurun (https://github.com/Chinachu/Mirakurun) を利用して TV 視聴ができる Web application。
基本機能
- mirakurun server 設定機能
    - mirakurun server は http://tuner:40772/ に準備した。openapi.yml は https://raw.githubusercontent.com/Chinachu/Mirakurun/refs/heads/master/api.yml を参照せよ。この mirakurun server は自由に利用して良い。
- 視聴 UI と機能
    - mirakurun から取得した MPEG2-TS をリアルタイムエンコードし、MP4 の HTTP Live Streaming でブラウザから視聴できる機能
        - この部分は ffmpeg をdocker 経由で利用する事を考えており、その image も作る必要がある。
    - MPEG2-TS から取得した ARIB 字幕データを画面上に表示する機能
        - doc/subtitle-system.md に詳細が記述されている。ffmpeg を使い ass format で取得するのがよいだろう
    - チャンネル変更機能
        - チャンネル変更時にエンコーダーの切り替えなども滑らかに行える必要がある。古いプロセスをゆっくり終了させ、新しいエンコーダープロセスを別途裏で立ち上げるような工夫が必要だろう
    - 番組表表示機能
        - mirakurun からデータをもらえるので、うまく利用する
        - チャンネル名クリックでそのチャンネルを選局する

サーバサイドでは golang を利用し、docker compose 一発でバックエンドとフロントエンドが立ち上がるようにしたい。
JS フレームワークとしては Svelte を利用してよい。typescript を使って良い。CSS は tailwindcss でよい。Server side rendering は行ってはいけない。
database は sqlite を利用する。番組情報などを一時的に保存してよいが、肥大しないように工夫せよ。
映像は画面の横幅いっぱいにしたいが、元映像よりも横長の画面の場合は縦幅いっぱいに表示するようにしたい。

