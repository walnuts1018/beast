---
name: general-coding-style
description: 言語設定、コメント規約、Go実装時の安全なコーディング規約などの基本スタイル
---

## コメント規約

- 実装の意図や「なぜそう実装したか」がわかりやすいコード・命名を心がけ、不要なコメントは避けてください。
- セキュリティに関する処理（時間制限、シングルショット配信など）には意図が分かるコメントを付与してください。
- 一時的な実装には、理由と解消条件を記した改善事項コメントを必ず残してください。
- コメントは日本語で書いてください。

## ドメインモデリング

- 関数型ドメインモデリング（DMMF）に沿って実装してください。Goでの型付けや直和型エラーの表現方法など、具体的な指針は[ADR 0009](../../../docs/adr/0009-functional-domain-modeling.md)（およびその前身の[ADR 0003](../../../docs/adr/0003-dmmf-typed-models.md)）を参照してください。
- 原著（dmmf.epub）はF#前提の内容なので、Go言語での実装に適した形に変換して適用してください。

## GraphQLスキーマ設計

- GraphQL SchemaのMutationは、1つのMutationフィールドと、そのMutationだけで使うInput、Result union、Success、UserError、UserErrorCodeなどを1つの`*_mutation.graphql`ファイルへまとめてください。複数のMutationを同じSchemaファイルへ集約しないでください。
- Mutation以外のGraphQL型は基本的に1type1ファイルとします。ただし、EdgeとConnection、ある型だけで使うenumやunionのvariant、同じprojectionを構成する親子型など、単独では意味を持たず特定の型と強く結びつく型は同じファイルへまとめてください。機械的な行数や型数ではなく、既存Schemaと同程度の責務単位を基準にしてください。
- Schemaのファイル構成だけを検証するテストは追加しないでください。GraphQL生成と既存の型検査で契約の整合性を確認してください。
- Mutationのユーザーエラー表現は[ADR 0011](../../../docs/adr/0011-graphql-user-error.md)のResult union(`XxxResult = XxxSuccess | XxxUserError`)パターンに統一してください。
- クエリ側でも、実際に意味を持つ組み合わせしか出現しないフィールドの集まりは、nullableフィールドの寄せ集めではなくunion/interfaceで型として表現してください(例: [ADR 0013](../../../docs/adr/0013-graphql-media-kind-union.md)のMedia.state)。
- 対応するドメイン実装が存在しない機能(型は残すが未実装)は、`@unimplemented(reason: "docs/tasks/xxxxx-yyy.md")`ディレクティブをQuery/Mutationフィールドに付与し、実行時は共通ハンドラ(`router/handler/graphql.go`のDirectiveRoot.Unimplemented)がUNIMPLEMENTEDエラーコードを返す形に統一してください。個々のresolverに固定のUserErrorスタブを手書きしないでください。型・フィールドのdescriptionコメントには`[未実装]`prefixを付け、実装予定タスクのパスを明記してください。

## Goの安全なコーディング規約

- `defer`関数がエラーを返す場合は、握りつぶさずログとして出力してください。
- URLを文字列結合で組み立てるのは禁止です。`net/url`など適切なパッケージを利用して組み立ててください。
- リダイレクト先URLを外部入力から決定する処理では、オープンリダイレクタにならないよう許可されたURL/パスのみに制限してください。
