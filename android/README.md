# beast Android

KotlinとJetpack Composeによるスマートフォン向けクライアントです。

## 構成

- `ui`: Material 3のライブラリ、推薦レール、タグ編集、Media3プレーヤー
- `MainViewModel`: 読み込み、評価、再生履歴、タグ変更の状態管理
- `data/api`: Bearerトークンを付けたGraphQL over HTTPの型付き境界
- `data/repository`: GraphQL実装と、debugビルドだけで使うローカルプレビュー実装
- `media`: OAuth 2.1 Bearer認証付きHLSをMedia3へストリーミングするRange対応DataSource

`debug`ビルドはAPI未起動でも操作確認できる`PreviewVideoRepository`を使います。releaseビルドはブラウザで`/api/auth/mobile/login`を開くPKCEログインを使い、コールバックで受け取ったアクセストークンをAndroid Keystoreで暗号化して保存します。動画はサーバー側で保存時に暗号化され、再生時に認証済みのHLSとしてRange配信されるため、複数端末で同じOIDCアカウントを利用できます。

## 静的確認

Gradleはmise経由で次のように実行できます。

```sh
mise exec gradle@8.10.2 -- gradle -p android help
```

Android SDKが入った環境では次でコンパイルします。

```sh
mise exec gradle@8.10.2 -- gradle -p android :app:compileDebugKotlin
```
