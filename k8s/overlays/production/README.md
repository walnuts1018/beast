# 本番overlay

このoverlayはアプリケーションだけを配置し、PostgreSQL、RabbitMQ、SeaweedFSはkurumiの既存サービスを利用します。Secretの値はリポジトリに保存せず、既存のExternal Secrets Operatorの`onepassword`から`beast-runtime`へ同期します。

適用前に、kurumiのOnePassword vault `kurumi`にitem `beast`を作成し、次のfieldを用意してください。`onepassword` ClusterSecretStoreはinfraリポジトリで既にReadyであることを実測済みです。

- `database_url`、`database_user`、`database_password`、`database_name`
- `rabbitmq_url`
- `s3_access_key_id`、`s3_secret_access_key`
- `oidc_client_id`、`oidc_client_secret`

`database_url`の接続先は`postgresql-default-rw.databases.svc.cluster.local`、`rabbitmq_url`の接続先は`default.rabbitmq.svc.cluster.local`を指定してください。接続情報に含める認証情報はOnePassword itemだけで管理します。

実測した接続先は`postgresql-default-rw.databases.svc.cluster.local`、`default.rabbitmq.svc.cluster.local`、`seaweedfs-default-filer.seaweedfs.svc.cluster.local:8333`です。`beast.walnuts.dev`はHTTPRouteによりEnvoy Gatewayへ公開し、既存のwildcard証明書を利用します。DNS反映は既存のExternalDNS構成に依存します。

`beast` namespaceは本overlayが作成します。kurumiのOnePassword vault `kurumi`には`beast` itemを事前に作成してください。`beast-cluster-secret-store`や`s3.walnuts.dev`は参照していません。
