---
name: react-conventions
description: React Hookの正しい使用方法と、SSR/CSRの使い分けに関する規約
when_to_use: frontend配下でReactコンポーネントやHookを追加・変更する時
---

## SSR/CSRの使い分け

- 全てのページ・データ取得をSSRにすればよいわけではありません。ユーザー体験（初期表示速度、インタラクティブ性、SEOの要否など）を踏まえて、SSRとCSRを適切に使い分けてください。
- **SSR（サーバーサイド処理）を活用する領域**:
  - **ルートシェル & 初期認証セッション**: `routes/__root.tsx` の `beforeLoad` / `loader` で `getAuthSession` を実行し、初期 HTML と認証状態をサーバー側で確立する。
  - **認証ガード & 即時リダイレクト**: 認証必須ルート（`/albums`, `/vault`, `/upload` 等）は、ルートの `beforeLoad` で `requireAuthenticated(context.session)` を呼び出し、未認証時にサーバー側で即座にログイン画面へリダイレクト（HTTP 302/307）する。
  - **動的タイトル & メタデータ / OGP**: 各ルートの `head` 関数を用いて、ページタイトルやメタタグを SSR 時に HTML `<head>` に出力する。
- **CSR（クライアント専用 `ssr: false`）を維持する領域**:
  - **大量データの仮想化グリッド**: `_library.tsx`（写真一覧）のように、`ResizeObserver` による要素幅計測や `useWindowVirtualizer`、ドラッグ＆ドロップ、矩形選択などブラウザ DOM に強く依存する画面。
  - **ブラウザ専用エンジン**: `edit.$mediaStackId.$mediaId.tsx`（画像エディタ）のように、WebAssembly や WebGPU、Canvas 描画などブラウザ環境でしか動作しないリッチな操作画面。
