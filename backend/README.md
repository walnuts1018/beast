# Beast API

開発環境では`AUTH_MODE=static`と`AUTH_DEV_STATIC_TOKEN`を設定し、`Authorization: Bearer <AUTH_DEV_STATIC_TOKEN>`で認証します。利用者を分離する場合は`X-User-ID`も指定します。productionでは`AUTH_MODE=introspection`が必須で、`X-User-ID`は受け付けません。

Shared Keyの公開鍵をGraphQLの`registerSharedKey`で登録した後、`POST /api/videos/upload`へmultipartで`file`、`shared_key_id`、`encrypted_tags`を送信します。サーバーは`STAGING_ENCRYPTION_KEY`でstaging objectをチャンク暗号化してS3へ保存し、RabbitMQへ平文や秘密鍵を含まないowner-boundジョブを発行します。エンコーダーはstagingを復号してffmpegでDASH化し、各manifest/segmentをShared Key公開鍵でEnvelope EncryptionしてS3へ保存します。動画が`READY`になった後、`/api/videos/:id/dash/manifest.mpd`を取得し、レスポンスの暗号化メタデータでmanifestと各segmentをクライアント側で復号します。`STAGING_ENCRYPTION_KEY`はAPIとエンコーダーへ同じSecretから注入し、RabbitMQへ送信してはいけません。

GraphQLの`createVideo`は、upload endpointで同じ所有者に登録済みの暗号化済み`objectKey`だけを受け付け、暗号化メタデータの一致を検証します。存在しないオブジェクトや他の所有者のオブジェクトからメタデータだけを作成することはできません。

ジョブの契約は`contract_version`、`video_id`、`owner_id`、`source_object_key`、`output_prefix`、`public_key`、`shared_key_id`、`key_version`です。イベントのREADY時にはmanifestと全DASH artifactの暗号化メタデータが含まれ、APIは動画所有者・source鍵・動画ID prefix・Shared Key IDを検証してから状態を更新します。
