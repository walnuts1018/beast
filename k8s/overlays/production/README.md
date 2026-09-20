# 本番overlay

このoverlayはアプリケーションだけを配置し、PostgreSQL、RabbitMQ、SeaweedFSは既存のクラスタサービスを利用します。Secretの値はリポジトリに保存せず、External Secrets Operatorの`beast-cluster-secret-store`から`beast-runtime`へ同期します。

適用前に、External Secrets Operatorの`ClusterSecretStore`と次のリモートSecretを用意してください。

- `beast/postgres`: `username`、`password`、`database`
- `beast/rabbitmq`: `url`
- `beast/seaweedfs`: `accessKey`、`secretKey`
- `beast/oidc`: `clientId`、`clientSecret`

クラスタ内サービス名やS3エンドポイントが異なる場合は、`kustomization.yaml`の`configMapGenerator`を環境固有のoverlayから上書きしてください。`beast.walnuts.dev`のDNS、Ingress Controller、`letsencrypt-prod`のIssuerも事前に必要です。
