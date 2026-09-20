# Beast API

開発環境では`AUTH_MODE=static`と`AUTH_DEV_STATIC_TOKEN`を設定し、`Authorization: Bearer <AUTH_DEV_STATIC_TOKEN>`で認証します。利用者を分離する場合は`X-User-ID`も指定します。productionでは`AUTH_MODE=introspection`が必須で、`X-User-ID`は受け付けません。

Shared Keyの公開鍵をGraphQLの`registerSharedKey`で登録した後、`POST /api/videos/upload`へmultipartで`file`、`shared_key_id`、`encrypted_tags`を送信します。サーバーは登録済み公開鍵でEnvelope Encryptionを行い、暗号化メタデータと`objectKey`を返します。動画が`READY`になった後、返された動画IDの`/api/videos/:id/stream`を取得し、`X-Encryption-Chunk-Size`などのレスポンスメタデータを使ってクライアント側で1MiB単位のAES-GCMチャンクを復号します。エンコーダーへのジョブ発行は、暗号化済みS3 objectをエンコーダーが安全に取得・復号できる契約が整うまで有効化していません。

GraphQLの`createVideo`は、upload endpointで同じ所有者に登録済みの暗号化済み`objectKey`だけを受け付け、暗号化メタデータの一致を検証します。存在しないオブジェクトや他の所有者のオブジェクトからメタデータだけを作成することはできません。
