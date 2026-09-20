# エンコーダー

エンコーダーはRabbitMQの`RABBITMQ_ENCODE_JOB_QUEUE`を購読し、S3互換ストレージからstaging objectを取得してHLS(fMP4)の単一品質へ変換します。staging objectは`STAGING_ENCRYPTION_KEY`でチャンク暗号化され、出力artifactは`MEDIA_ENCRYPTION_KEY`でサーバー側のAES-256-GCMチャンク暗号化を行って保存されます。復号した入力はencoder Pod内の一時領域だけに置かれます。

ffprobeで入力のvideo codecがH.264、HEVC、AV1のいずれか、audio codecがAACであることを確認できた場合は`-c copy`で再エンコードせずHLS(fMP4)へ変換します。対応外のcodecまたはcopy変換に失敗した場合だけH.264/AACへ再エンコードし、`force_key_frames`でセグメント境界のシーク性能を確保します。

ジョブは次のJSONです。

```json
{"contract_version":"v2","video_id":"video-id","owner_id":"owner-id","source_object_key":"staging/source-id","output_prefix":"videos/video-id/hls"}
```

処理中は`RABBITMQ_ENCODE_EVENT_QUEUE`へ`ENCODING`イベントを進捗率付きで発行し、成功時は`READY`とmanifestおよびartifactごとの暗号化メタデータ、失敗時は`FAILED`と`error`を発行します。暗号化メタデータにはチャンクサイズ、nonce、ラップ済みDEK、平文サイズが含まれます。APIは認証済みユーザーへだけartifactをHTTP Rangeでチャンク復号して返します。

`S3_ENDPOINT`、`S3_BUCKET`、S3認証情報、`RABBITMQ_URL`、`STAGING_ENCRYPTION_KEY`、`MEDIA_ENCRYPTION_KEY`が必要です。2つの暗号鍵は32バイトのbase64または16進文字列で、RabbitMQへ送信しません。HLSセグメント長は`ENCODER_HLS_SEGMENT_SECONDS`で設定します。
