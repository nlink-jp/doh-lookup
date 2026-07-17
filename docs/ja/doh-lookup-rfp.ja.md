# RFP: doh-lookup

> Generated: 2026-07-17
> Status: Draft

## 1. Problem Statement

`dig` をはじめとする通常の DNS 問い合わせは、OS/設定のリゾルバへ UDP/53 で送られる。
そのため CTI/IR 実務者が不審ドメインを調査する際、その調査クエリは (a) 通常のアプリ由来
DNS トラフィックに埋没して SOC の DNS ログでノイズになり、(b) 組織リゾルバのキャッシュ/ログを
汚染して「誰かが実際にその不審ドメインを踏んだ」ように見えてしまう。**doh-lookup は Google /
Cloudflare の DoH（DNS over HTTPS）エンドポイントへ HTTPS/443 で問い合わせることで、調査クエリを
組織 DNS 基盤から明確に分離し、「意図的・帯域外の調査」として明示的に見分けがつく状態で
DNS 情報を収集する**ための CLI 兼ローカル MCP サーバーである。対象ユーザーは、不審ドメインの
DNS 属性を安全な OpSec で確認したい CTI/IR 実務者。`asn-lookup`（帰属）・`abuse-lookup`（評判）・
`tor-exit-lookup`（Tor exit 判定）・`whois-lookup`（登録情報）・`icloud-relay-lookup`（Private Relay 判定）
に並ぶ、**DNS 解決**にフォーカスした cybersecurity-series の姉妹品。

## 2. Functional Specification

### Commands / API Surface

**CLI サブコマンド**（姉妹ツール規約に準拠）:

- `doh-lookup lookup <target...>` — 主操作。`<target>` はドメインまたは IP（複数指定可）
  - `--type A,AAAA,MX,...` — 取得レコード種別（カンマ区切り）。省略時はドメインプロファイル束
  - `--provider cloudflare|google` — DoH プロバイダ（既定 cloudflare）
  - `--json` — 構造化出力（bulk 時は JSONL、1 ターゲット 1 行）
  - `--raw` — リゾルバ生 JSON レスポンスをそのまま出力
  - `--no-dnssec` / `--cd` — checking disabled（DNSSEC 検証失敗でもレコードを返させる）
  - stdin / `--input <file>` — 改行区切りのターゲットリストを bulk 入力
- `doh-lookup cache <status|clear>` — キャッシュの状態表示 / クリア
- `doh-lookup mcp` — ローカル MCP サーバーを起動（stdio）
- `doh-lookup --version`

**MCP ツール**（whois-lookup と同一構成）:

- `lookup` — `{ name, types?, provider? }` → 正規化レコード + メタ（resolver / endpoint / AD / RCODE）
- `cache_status` — キャッシュ統計
- `get_usage` — ツールリファレンスとエラー回復表

### Input / Output

- **入力分類**: ドメイン → 正引き（プロファイル束または `--type` 指定）、IP → 自動 PTR 逆引き。
  非 IP 入力は RFC ホスト名検証ゲート（≤253 総長 / label 1–63 LDH / dot 必須 / 制御文字・CRLF 拒否）を
  通過しないと拒否（CLI exit 2、MCP `{code:"invalid_input"}`）。
- **既定レコード（プロファイル束）**: `A / AAAA / MX / TXT / NS / SOA / CAA` を 1 回で取得。
- **出力メタ（識別性の実装本体）**: すべての結果に **使用リゾルバ名 / エンドポイント URL /
  DNSSEC AD フラグ / RCODE / 応答時刻** を明記。これにより「どの帯域外リゾルバへ意図的に投げたか」が
  監査可能になる。
- **正規化**: CNAME 連鎖・ワイルドカード・NXDOMAIN（名前なし）と NODATA（NOERROR かつ空応答）の
  区別を engine で一元的に正規化。
- **出力形式**: 人間可読（既定、bulk はターゲットごとにセクション）/ `--json`（bulk は JSONL）/ `--raw`。
- **Exit code 契約（lookup）**: `0` 成功 / `1` NXDOMAIN（名前なし＝存在しないという成功回答）/ `2` エラー。

### Configuration

`~/.config/doh-lookup/config.toml`（sectioned TOML、任意）。`DOH_LOOKUP_*` 環境変数が上書き。
precedence は **フラグ > env > config > 既定**。

```toml
[provider]
# default = "cloudflare"          # cloudflare | google
# cloudflare_url = "https://cloudflare-dns.com/dns-query"
# google_url = "https://dns.google/resolve"

[query]
# profile = ["A","AAAA","MX","TXT","NS","SOA","CAA"]  # --type 省略時の束
# suppress_ecs = true             # edns_client_subnet=0.0.0.0/0 で調査元を漏らさない

[cache]
# ttl_floor_seconds = 60          # DNS TTL 尊重。ただし下限フロア
# dir = "~/.cache/doh-lookup"

[network]
# timeout_seconds = 10
```

### External Dependencies

- **なし**（Go 標準ライブラリのみ）。DoH は `net/http` + `encoding/json` で完結。`miekg/dns` 不使用。
- 外部サービスは Google / Cloudflare の公開 DoH エンドポイントのみ。**credential・API キー不要**。

## 3. Design Decisions

- **言語 = Go、外部依存ゼロ。** シリーズ標準（asn/abuse/tor/whois/icloud-relay と同一）。DoH JSON API は
  標準ライブラリだけで扱え、単一署名バイナリで配布できる。
- **DoH-to-公開リゾルバという設計自体が識別性の本体。** 追加の User-Agent マーカーや EDNS0 調査タグは
  付けない。代わりに **出力へ使用リゾルバ/エンドポイントを常に明記**し、監査可能性で担保する
  （whois-lookup が出力に `rdap|whois` の情報源を明記するのと同じ思想）。
- **engine は CLI / MCP で共有**し挙動を分岐させない。HTTP クライアントは注入インターフェイスで
  テスト時にモックする（設計でのテスト容易性）。
- **ネットワーク I/O 前の検証ゲート**を必須化。CRLF/制御文字混入・レート浪費・キャッシュ汚染を封じる。
- **ECS 抑止をデフォルト**（`edns_client_subnet=0.0.0.0/0`）。調査元ネットワークをリゾルバへ漏らさない
  OpSec 上の既定。
- **姉妹ツールとの関係**: `asn-lookup`（IP→AS）・`abuse-lookup`（IP 評判）・`whois-lookup`（登録情報）・
  `tor-exit-lookup` / `icloud-relay-lookup`（出口 IP 判定）の DNS 解決版。返却 IP の enrichment は
  各姉妹ツールへ委譲（UNIX 哲学）。
- **スコープ外（意図的）**:
  - IP enrichment（AS / 評判 / geo）— 姉妹ツールに委譲
  - RFC 8484 wireformat（`application/dns-message`）対応 — v2 候補
  - DNSSEC 署名の独立検証 — AD フラグ報告のみ。独立検証は wireformat + 暗号が要るため v2 候補
  - ゾーン転送 / 権威サーバー直接問い合わせ / パッシブ DNS 履歴

## 4. Development Plan

### Phase 1: Core（CLI）— 独立レビュー可

- `internal/query`: 入力分類（domain / IP）+ RFC ホスト名検証ゲート、IDN punycode
  （whois-lookup の RFC 3492 実装を移植）
- `internal/config`: sectioned TOML + `DOH_LOOKUP_*` env + フラグ（precedence 適用）
- `internal/cache`: DNS TTL 尊重キャッシュ（min-TTL フロア設定可）、atomic write
- `internal/doh`: DoH クライアント（Cloudflare / Google JSON API、HTTP 注入インターフェイス、
  ECS 抑止、`do=1` で AD 取得）
- `internal/engine`: validate → cache → resolve → normalize（CNAME 連鎖・NXDOMAIN vs NODATA・
  RCODE 判定）
- `internal/app`: `lookup` / `cache` サブコマンド、`--type`/`--json`/`--raw`/`--provider`、
  プロファイル束既定、PTR 逆引き、複数ターゲット + stdin、出力メタ明記、exit code 契約
- モック HTTP でのテーブル駆動テスト一式

### Phase 2: Features（MCP）— 独立レビュー可

- `internal/mcp`: zero-dep stdio JSON-RPC 2.0、ツール `lookup` / `cache_status` / `get_usage`、
  構造化エラー `{code, message, details}`
- 大量 bulk 結果は workspace ファイル経由（abuse-lookup `get_reports` / asn-lookup `prefixes_file` 方式）

### Phase 3: Release

- README.md / README.ja.md / CHANGELOG / AGENTS.md / config.example.toml / docs/{en,ja}
- Makefile + scripts（codesign / notarize / brew）、build-all（linux amd64/arm64・darwin arm64・
  windows amd64）、darwin 署名 + notarize、homebrew-tap formula
- submodule 統合 → org profile + web-site catalog 同期 → check-org.sh

## 5. Required API Scopes / Permissions

**None.** すべての DoH エンドポイントは公開。認証・API キー・OAuth スコープ・IAM ロールは一切不要。

## 6. Series Placement

Series: **cybersecurity-series**
Reason: 不審ドメインの DNS 属性を OpSec 分離した状態で収集する CTI/IR 支援ツールであり、
`asn-lookup` / `abuse-lookup` / `tor-exit-lookup` / `whois-lookup` / `icloud-relay-lookup` と同じ
「CLI 兼 MCP・credential ゼロ・外部依存ゼロの調査ルックアップ」ファミリーに属する。

## 7. External Platform Constraints

- **DoH JSON API のスキーマ drift**: Google `dns.google/resolve` は独自 IF、Cloudflare は文書化 IF だが
  いずれも RFC ではない。レスポンス JSON は lenient decode し、一箇所（`internal/doh`）で正規化する
  （whois-lookup の RDAP dialect 対策と同じ思想）。
- **ソフトレート制限**: 公開 DoH は AbuseIPDB のような明示日次クォータは無いが、乱用ボリュームは
  throttle / block される。TTL キャッシュを尊重し、bulk は礼儀正しくペーシング、積極リトライ禁止。
- **content type**: Cloudflare は `Accept: application/dns-json` 必須。Google は `dns.google/resolve` が
  JSON を返す。
- **DNSSEC**: AD フラグはリゾルバが検証する場合のみ意味を持つ（Google / Cloudflare は検証する）。
  本ツールは AD を報告するのみで、署名を独立検証しない。
- **応答サイズ**: DoH は HTTPS 上で 512 バイト UDP 制限が無く、大きな TXT / 多数 Answer も扱える。

---

## Discussion Log

- **発端**: Google / Cloudflare の DoH でドメイン情報を収集する abuse-lookup / tor-exit-lookup の姉妹品案。
  `dig` は通常 DNS と見分けがつかないため、**明示的に見分けがつく状態**で問い合わせることが主目的。
- **識別性の解釈を確認・合意**: `dig`（OS リゾルバへ UDP/53）は調査クエリが通常 DNS に埋没し組織リゾルバの
  キャッシュ/ログを汚染する。DoH で公開リゾルバへ HTTPS/443 投げれば「意図的・帯域外の調査」として分離・
  識別可能になる、という OpSec 分離が主目的、で合意。
- **識別性の実現レベル**: 「DoH 設計で分離」を選択。追加の User-Agent マーカー / EDNS0 タグは付けず、
  出力に使用リゾルバ/エンドポイントを明記して監査可能にする方針。
- **スコープ**: 「純 DoH ルックアップに集中」を選択。返却 IP の enrichment は asn/abuse 等の姉妹ツールへ
  委譲（UNIX 哲学）。
- **実装規約の実物確認**: cybersecurity-series の whois-lookup（ドメイン対象・credential ゼロで最も近い）を
  テンプレートに採用。`main.go` + `internal/{query,engine,cache,config,app,mcp}` 層構成、CLI サブコマンド
  `lookup`/`cache`/`mcp` + `--type`/`--json`/`--raw`、MCP ツール `lookup`/`cache_status`/`get_usage`、
  `~/.config/<tool>/config.toml` + `<TOOL>_*` env、検証ゲート、exit code 0/1/2、出力が情報源を明記、という
  共通規約を確認。
- **機能仕様の決定（4 点）**:
  - 既定レコード = **ドメインプロファイル束**（A/AAAA/MX/TXT/NS/SOA/CAA、`--type` で絞り込み）
  - **PTR 逆引き対応**（IP を渡すと自動 PTR）
  - **bulk 対応を v1 に含める**（複数位置引数 + stdin/`--input`、pipe-friendly）
  - 既定プロバイダ = **Cloudflare (1.1.1.1)**（プライバシー方針が明確。`--provider`/config で Google へ切替）
- **Development Plan**: Phase1 Core CLI → Phase2 MCP → Phase3 Release。Phase1・2 は独立レビュー可。
- **補足の設計判断**: ECS 抑止（`edns_client_subnet=0.0.0.0/0`）をデフォルトにし調査元ネットワークを
  漏らさない。DNSSEC は AD フラグ報告のみ（署名検証は v2 の wireformat 対応時に検討）。
