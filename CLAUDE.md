# Beastの実装方針

詳細な作業規約は`AGENTS.md`を参照し、実装・コメント・コミットメッセージは日本語で行う。未リリースのため、読みやすさ・安全性・性能を優先して設計する。

## 運用

- ブランチを切らず`main`へ直接コミットしてよい。
- 本番Kubernetesの変更は`../infra`へJsonnetまたはTerraformとしてコミットし、BeastリポジトリへArgo CD Applicationを追加しない。
- ツールはmiseで管理し、Goの開発ツールは`go tool`で実行する。
- 対応環境はWindows 11、macOS 26、Android 16、iOS 26と、それぞれの最新Chrome、Safari、Edge、Firefoxに限定する。

## サービス

所有者だけが動画へアクセスできる個人動画ライブラリとして、タグ、5段階評価、再生回数、最終再生日時、おすすめ表示を提供する。認証はZITADELのOIDCを使い、複数端末では同じOIDCアカウントで利用する。

動画とタグはサーバー側のAES-256-GCMで保存時暗号化する。動画は固定長チャンク単位で暗号化し、認証済みのHLSリクエストに必要な範囲だけを復号する。端末鍵、共有鍵、クライアント側復号は使用しない。

## API

- Go 1.27を使用する。
- DBアクセスはBob v0.50.0のコード生成を使い、sqlcは使用しない。
- HTTPはecho/v5、GraphQLはgqlgenを使う。
- OAuth 2.1の認可コード+PKCEとIdPのToken Introspectionで認証・認可する。
- 生成ツールと静的解析は`go tool`で実行する。ツール依存は`backend/tools.mod`へ分離する。
- レイヤーはGraphQL Resolver、ユースケース、ドメイン、インフラの依存方向を守り、ドメインへ外部I/Oを持ち込まない。

## 動画処理

- 配信形式はHLS(fMP4)とする。
- 入力がH.264、HEVC、AV1の動画とAAC音声の組み合わせなら再エンコードせずストリームコピーする。
- それ以外はH.264とAACへフォールバックし、2〜4秒間隔のキーフレームを設定する。
- エンコードはアップロード後にRabbitMQ経由のEncoderサービスで非同期に行い、進捗イベントを発行する。
- 複数画質は生成せず、再生時のシーク性能と不要なCPU負荷の削減を優先する。

## クライアント

- Webは認証済みHLSをhls.jsまたはブラウザのネイティブHLSで再生する。
- AndroidはKotlin、Jetpack Compose、Media3 HLSを使い、HTTP Rangeで必要なセグメントだけを取得する。
- YouTubeを参考に、片手操作、ダブルタップによる10秒シーク、長押し中の倍速再生を優先する。

## ローカル開発

Skaffoldのdevモードで変更を反映し、ローカル開発でも共有Kubernetesクラスタのnamespaceを分離して利用する。定期的な検証コマンドは`mise.toml`のtaskとして定義する。
