# Beastのデプロイ

ローカルではkindとSkaffoldを使います。Docker、kind、miseが利用できる状態で次を実行してください。

```sh
kind create cluster --config k8s/kind.yaml
mise exec -- skaffold dev
```

ブラウザから確認する場合は、別の端末で`kubectl -n beast port-forward service/beast-frontend 8080:8080`を実行して`http://localhost:8080`を開きます。ローカル認証のBearer tokenは`local-development-token`です。ローカルのPostgreSQL、RabbitMQ、SeaweedFSは`emptyDir`を使うため、kindクラスターを削除するとデータも削除されます。

本番相当のマニフェストはExternal Secrets Operatorと既存のPostgreSQL、RabbitMQ、SeaweedFSを利用します。SecretStoreとリモートSecretを用意したうえで、対象contextを確認してから次を実行してください。

```sh
skaffold run --profile production
```

本番overlayのSecret契約と、DNS、Ingress、Issuerの前提は`k8s/overlays/production/README.md`に記載しています。Skaffoldのproduction profileは自動選択されないため、`skaffold run`だけで本番へ適用されることはありません。
