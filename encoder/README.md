# エンコーダー

エンコーダーはRabbitMQの`RABBITMQ_ENCODE_JOB_QUEUE`を購読し、S3互換ストレージからstaging objectを取得して動画をMPEG-DASHの単一品質へ変換します。staging objectは`STAGING_ENCRYPTION_KEY`でチャンク暗号化されており、復号した入力はencoder Pod内の一時領域だけに置かれます。入力ストリームを最初に`-c copy`でコンテナ変換し、入力がDASH互換でない場合だけ`libx264`とAACで再エンコードします。出力artifactはShared Key公開鍵で個別にEnvelope EncryptionしてS3へ保存し、成功イベントの発行後にstaging objectを削除します。

ジョブは次のJSONです。

```json
{"contract_version":"v1","video_id":"video-id","owner_id":"owner-id","source_object_key":"staging/source-id","output_prefix":"videos/video-id/dash","public_key":"-----BEGIN PUBLIC KEY-----...","shared_key_id":"shared-key-id","key_version":"1"}
```

処理中は`RABBITMQ_ENCODE_EVENT_QUEUE`へ`ENCODING`イベントを進捗率付きで発行し、成功時は`READY`とmanifestおよびartifactごとの暗号化メタデータ、失敗時は`FAILED`と`error`を発行します。API側はowner・source鍵・動画ID prefix・Shared Key IDを検証し、`ENCODING`、`READY`、`FAILED`の状態と進捗を永続化します。イベントは次の形式です。

```json
{"contract_version":"v1","video_id":"video-id","owner_id":"owner-id","source_object_key":"staging/source-id","status":"READY","progress":1,"manifest":{"object_key":"videos/video-id/dash/manifest.mpd","encryption":{"algorithm":"AES-256-GCM-CHUNKED-RSA-OAEP-SHA256","chunk_size":1048576,"key_version":"1","nonce":"...","encrypted_data_key":"...","shared_key_id":"shared-key-id"}},"artifacts":{"chunk-stream0-00001.m4s":{"object_key":"videos/video-id/dash/chunk-stream0-00001.m4s","encryption":{"algorithm":"AES-256-GCM-CHUNKED-RSA-OAEP-SHA256","chunk_size":1048576,"key_version":"1","nonce":"...","encrypted_data_key":"...","shared_key_id":"shared-key-id"}}},"occurred_at":"2026-09-20T00:00:00Z"}
```

S3接続、RabbitMQ、`STAGING_ENCRYPTION_KEY`が必須です。`STAGING_ENCRYPTION_KEY`は32バイトのbase64または16進文字列で、平文や秘密鍵をRabbitMQへ送信しません。
