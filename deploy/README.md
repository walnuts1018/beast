# Beastのデプロイ

ローカルではkindとSkaffoldを使います。Docker、kind、miseが利用できる状態で次を実行してください。

```sh
kind create cluster --config k8s/kind.yaml
mise exec -- skaffold dev
```

ブラウザから確認する場合は、別の端末で`kubectl -n beast port-forward service/beast-frontend 8080:8080`を実行して`http://localhost:8080`を開きます。ローカル認証のBearer tokenは`local-development-token`で、利用者を分ける場合は`X-User-ID`も指定します。ローカルのPostgreSQL、RabbitMQ、SeaweedFSは`emptyDir`を使うため、kindクラスターを削除するとデータも削除されます。

本番相当のレンダリングはExternal Secrets Operatorと既存のPostgreSQL、RabbitMQ、SeaweedFSを利用します。SecretStoreとリモートSecretを用意したうえで、対象contextを確認してから次を実行してください。

```sh
skaffold render --profile production-render
```

本番overlayのSecret契約と、DNS、Envoy Gateway、HTTPRouteの前提は`k8s/overlays/production/README.md`に記載しています。本番のデプロイはinfraリポジトリの`k8s/apps/beast`にあるJsonnetをArgo CDが同期します。ApplicationSetのsourceはinfraリポジトリに固定されており、Beastリポジトリのmanifestは参照しません。
