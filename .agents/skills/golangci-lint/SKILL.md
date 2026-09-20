---
name: golangci-lint
description: golangci-lintを用いた静的解析とコード修正
when_to_use: Goコードの変更を伴う作業が終わった時
---

## golangci-lint の実行

Goコードの変更を伴う作業が終わった時は、`mise run golangci-lint run --fix` を実行して静的解析を行ってください。

## エラーの修正方針

- エラーが出た場合は、指摘内容を理解した上でコードを修正してください。
- 警告を無視するための `//nolint:...` コメントは、正当な理由がない限り使用しないでください。使用する場合は、必ず理由を併記してください。
