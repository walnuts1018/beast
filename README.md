# beast

自分で撮影した動画を暗号化して保存し、スマートフォンから安全に再生するためのサービスです。動画の所有者だけが動画とタグへアクセスでき、再生回数、再生日時、五段階評価を使ったおすすめを提供します。

## 構成

- `backend`: Go 1.27、Echo v5、gqlgen、Bob、pgxによるGraphQL APIです。
- `encoder`: RabbitMQのジョブを受け取り、ffmpegでMPEG-DASHへ変換するワーカーです。まずストリームコピーを試し、失敗した場合だけ軽量なH.264/AACへ再エンコードします。
- `frontend`: Chrome向けのモバイル優先Web UIです。GraphQL APIへBearer tokenを付けて接続します。
- `android`: Kotlin、Jetpack Compose、Media3によるスマートフォンUIです。
- `k8s`: kindで使うローカルoverlayと、kurumiの既存サービスを使うproduction overlayです。

動画本体はランダムなAES-256-GCM Data Keyでチャンク暗号化し、Data KeyをShared KeyのRSA公開鍵でRSA-OAEP-SHA256暗号化します。Shared Key秘密鍵とDevice Key秘密鍵はクライアントだけが保持し、APIやオブジェクトストレージには保存しません。タグも暗号化済みの値だけをAPIへ渡します。

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

production overlayはアプリケーションだけを配置し、PostgreSQL、RabbitMQ、SeaweedFS、External Secrets Operator、Envoy Gatewayをkurumiの既存リソースへ接続します。Secretのfield契約、DNS、Argo CDアプリケーション登録は`k8s/overlays/production/README.md`と`infra`リポジトリ側で管理します。

```sh
skaffold run --profile production
```

通常の本番反映は`infra`リポジトリへArgo CD Applicationをコミットし、Argo CDの同期結果、各PodのReady状態、`/livez`、`/readyz`、GraphQL認証を確認します。イメージはレジストリへpushした不変タグを指定してください。

Androidの実機コンパイルにはAndroid SDKが必要です。SDKがある環境では`mise exec gradle@8.10.2 -- gradle -p android :app:compileDebugKotlin`を実行します。
