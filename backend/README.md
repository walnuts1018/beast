# APIサーバー

APIサーバーはEchoとgqlgenで認証済みユーザーの動画メタデータを提供し、`POST /api/videos/upload`でmultipartの`file`とJSON配列文字列の`tags`を受け付けます。タグはPostgreSQLへ保存する前に`MEDIA_ENCRYPTION_KEY`でAES-GCM暗号化します。動画のHLS(fMP4)成果物も同じ鍵から生成したオブジェクト単位DEKで暗号化して保存します。

再生時は`/api/videos/:id/hls/manifest.m3u8`と各`init.mp4`、`segment_*.m4s`を認証済みのHTTPリクエストへだけ返します。`Range`を受け取った場合は必要な暗号化チャンクだけをS3から取得し、API内で復号して返すため、端末へ鍵を渡さず複数デバイスで再生できます。レスポンスは`Accept-Ranges`と`Content-Range`を付け、シーク時の再取得量を抑えます。

staging objectの一時保存だけは`STAGING_ENCRYPTION_KEY`で暗号化し、エンコーダーが入力を取得して削除します。`MEDIA_ENCRYPTION_KEY`と`STAGING_ENCRYPTION_KEY`は分離し、どちらも平文や鍵をRabbitMQへ送信しません。DBアクセスはBob v0.50.0を使用し、sqlcは使用しません。
