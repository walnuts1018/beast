# beast

自分で撮影した動画を暗号化して保存し、スマートフォンから安全に再生するためのサービスです。動画の所有者だけが動画とタグへアクセスでき、再生回数、再生日時、五段階評価を使ったおすすめを提供します。

## 構成

- `backend`: Go 1.27、Echo v5、gqlgen、Bob、pgxによるGraphQL APIです。
- `encoder`: RabbitMQのジョブを受け取り、ffmpegでHLS(fMP4)へ変換するワーカーです。H.264、HEVC、AV1とAACの入力はストリームコピーし、それ以外だけH.264/AACへ再エンコードします。
- `frontend`: 最新のChrome、Safari、Edge、Firefoxを対象にしたモバイル優先Web UIです。hls.jsで認証済みHLSを再生します。
- `android`: Kotlin、Jetpack Compose、Media3によるスマートフォンUIです。OIDCログインはサーバー側PKCEブローカーを経由し、アクセストークンはAndroid Keystoreで暗号化して保持します。
- `k8s`: kindで使うローカルoverlayと、本番相当のレンダリングを確認するproduction overlayです。本番のデプロイマニフェストはinfraリポジトリで管理します。

動画本体はサーバー側でランダムなAES-256-GCM Data Keyを使って固定長チャンク暗号化し、Data Keyをサーバーの鍵でラップして保存します。APIは認証済みリクエストに対して必要なHLSマニフェストやセグメントの範囲だけを復号して返すため、端末鍵の登録やクライアント側復号を必要とせず、複数デバイスで再生できます。タグはサーバー側で暗号化して保存し、GraphQLでは認証済み所有者にだけ平文で返します。

## 開発

ツールはmiseで揃えます。バックエンドのコード生成はBobを使い、SQLスキーマを変更した場合は生成物を更新します。

```sh
mise install
mise run backend-generate
mise run backend-test
mise run encoder-test
mise run frontend-lint
mise run frontend-build
mise run manifest-render
```

GraphQL APIを単体で起動する場合は、`backend`ディレクトリで`go run ./cmd/api`を実行します。開発モードのHTTP APIは`X-User-ID`、staticモードは`Authorization: Bearer <AUTH_DEV_STATIC_TOKEN>`を使います。本番モードではZITADELのToken Introspectionが必須です。

kindで依存サービスを含めて起動する場合は、次を実行します。

```sh
kind create cluster --config k8s/kind.yaml
skaffold dev
```

ブラウザ確認は`kubectl -n beast port-forward service/beast-frontend 8080:8080`の後に`http://localhost:8080`を開きます。ローカルoverlayのPostgreSQL、RabbitMQ、SeaweedFSは`emptyDir`のため、kindクラスターを削除するとデータも削除されます。

## コード生成

Bobの設定は`backend/bobgen.yaml`です。`backend/internal/store/migrations/001_initial.sql`を変更したら、次を実行します。

```sh
mise run backend-generate
```

GraphQLスキーマの生成も同じ`go generate ./...`で行われます。`sqlc`の設定、依存、クエリは使用しません。

## デプロイ

production overlayはアプリケーションだけを配置し、PostgreSQL、RabbitMQ、SeaweedFS、External Secrets Operator、Envoy Gatewayをkurumiの既存リソースへ接続する本番相当のレンダリング用です。Secretのfield契約は`k8s/overlays/production/README.md`で確認できます。

```sh
skaffold run --profile production
```

通常の本番反映は`infra/k8s/apps/beast`へJsonnetマニフェストをコミットしてArgo CDへ同期させます。infraのApplicationSetが`app.json5`を検出してApplicationを生成し、Applicationのsourceは常にinfraリポジトリの`k8s/apps/beast`です。Argo CDがこのリポジトリのmanifestや`k8s/overlays/production`を参照することはありません。同期後は各PodのReady状態、`/livez`、`/readyz`、GraphQL認証を確認します。イメージはレジストリへpushした不変タグを指定してください。

Androidの実機コンパイルにはAndroid SDKが必要です。SDKがある環境では`mise exec gradle@8.10.2 -- gradle -p android :app:compileDebugKotlin`を実行します。
