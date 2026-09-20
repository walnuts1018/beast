# Beast API

開発環境では`AUTH_MODE=static`と`AUTH_DEV_STATIC_TOKEN`を設定し、`Authorization: Bearer <AUTH_DEV_STATIC_TOKEN>`で認証します。利用者を分離する場合は`X-User-ID`も指定します。productionでは`AUTH_MODE=introspection`が必須で、`X-User-ID`は受け付けません。

Shared Keyの公開鍵をGraphQLの`registerSharedKey`で登録した後、`POST /api/videos/upload`へmultipartで`file`、`shared_key_id`、`encrypted_tags`を送信します。サーバーは登録済み公開鍵でEnvelope Encryptionを行い、暗号化メタデータと`objectKey`を返します。動画の再生では返された動画IDの`/api/videos/:id/stream`を取得し、レスポンスの暗号化メタデータを使ってクライアント側で1MiB単位のAES-GCMチャンクを復号します。

GraphQLの`createVideo`は、既に暗号化済みオブジェクトが存在する`objectKey`だけを受け付けます。存在しないオブジェクトからメタデータだけを作成することはできません。
