# doh-lookup

公開リゾルバ（**Cloudflare** または **Google**）へ **DoH（DNS over HTTPS）**で
問い合わせ、ドメインの DNS レコードを収集する CLI 兼ローカル MCP サーバー。

`dig` は OS/設定のリゾルバへ UDP/53 で問い合わせるため、調査クエリが通常のトラフィックに
埋没し、組織リゾルバのキャッシュやログを汚染する。**doh-lookup** は公開リゾルバへ
**帯域外・HTTPS/443** で問い合わせ、すべての結果に**どのリゾルバ／エンドポイントが応答したか**と
**DNSSEC の AD フラグ**を明記する。これにより調査は通常 DNS と明示的に区別可能となり、監査証跡が
残り、自組織の DNS 基盤には一切触れない。

[`asn-lookup`](https://github.com/nlink-jp/asn-lookup)（帰属）・
[`abuse-lookup`](https://github.com/nlink-jp/abuse-lookup)（評判）・
[`whois-lookup`](https://github.com/nlink-jp/whois-lookup)（登録情報）・
[`tor-exit-lookup`](https://github.com/nlink-jp/tor-exit-lookup)（Tor exit 判定）の
**DNS 解決版**の姉妹ツール。**credential ゼロ・外部依存ゼロ。**

## インストール

```bash
# Homebrew（nlink-jp tap; prebuilt・Developer ID 署名 + notarized・arm64 macOS）
brew install nlink-jp/tap/doh-lookup

# ソースから（Go 1.25+）
make build      # → dist/doh-lookup
```

## 使い方

```bash
# 正引き — 既定プロファイル（A/AAAA/MX/TXT/NS/SOA/CAA）を一括取得
doh-lookup lookup example.com

# レコード種別を絞る
doh-lookup lookup --type A,MX example.com

# 逆引き（PTR）— IP を渡す
doh-lookup lookup 8.8.8.8

# リゾルバを選択（既定: cloudflare）
doh-lookup lookup --provider google example.com

# JSON 出力（複数ターゲットは JSONL）。bulk は引数・--input・stdin で
doh-lookup lookup --json example.com
printf 'example.com\ncloudflare.com\n' | doh-lookup lookup --type A --json
doh-lookup lookup --input targets.txt

# リゾルバ生 JSON を含める／キャッシュをバイパス
doh-lookup lookup --raw example.com
doh-lookup lookup --refresh example.com

# キャッシュ管理
doh-lookup cache status
doh-lookup cache clear
```

実行例:

```
$ doh-lookup lookup example.com
example.com  [forward, NOERROR, DNSSEC:validated]  via cloudflare (https://cloudflare-dns.com/dns-query)
  A      example.com                    201    104.20.23.154
  AAAA   example.com                    43     2606:4700:10::6814:179a
  MX     example.com                    300    0 .
  TXT    example.com                    300    "v=spf1 -all"
  NS     example.com                    86400  hera.ns.cloudflare.com.
  SOA    example.com                    1800   elliott.ns.cloudflare.com. dns.cloudflare.com. ...
```

### 終了コード（`lookup`）

| コード | 意味 |
|------|---------|
| `0`  | 1 つ以上のターゲットが解決 |
| `1`  | 全ターゲットが NXDOMAIN（存在しない） |
| `2`  | エラー（不正入力・ネットワーク障害 …） |

## MCP サーバー

```bash
doh-lookup mcp   # stdio JSON-RPC 2.0
```

ツール: `lookup`（ドメインまたは IP）・`cache_status`・`get_usage`。まず
`get_usage` を呼んで全リファレンスとエラー回復表を取得すること。エラーは構造化
JSON（`{code, message}`）。登録例（Claude Code）:

```json
{
  "mcpServers": {
    "doh-lookup": { "command": "doh-lookup", "args": ["mcp"] }
  }
}
```

## 設定

任意。[`config.example.toml`](config.example.toml) を
`~/.config/doh-lookup/config.toml` にコピーする。優先順位は
**フラグ > 環境変数 > ファイル > 既定**。credential 不要。

| 設定 | TOML | 環境変数 | 既定 |
|---------|------|-----|---------|
| プロバイダ | `[provider] default` | `DOH_LOOKUP_PROVIDER` | `cloudflare` |
| Cloudflare エンドポイント | `[provider] cloudflare_url` | `DOH_LOOKUP_CLOUDFLARE_URL` | `https://cloudflare-dns.com/dns-query` |
| Google エンドポイント | `[provider] google_url` | `DOH_LOOKUP_GOOGLE_URL` | `https://dns.google/resolve` |
| 既定プロファイル | `[query] profile` | `DOH_LOOKUP_PROFILE` | `A,AAAA,MX,TXT,NS,SOA,CAA` |
| ECS 抑止 | `[query] suppress_ecs` | `DOH_LOOKUP_SUPPRESS_ECS` | `true` |
| キャッシュ TTL フロア | `[cache] ttl_floor_seconds` | `DOH_LOOKUP_CACHE_TTL_FLOOR_SECONDS` | `60` |
| キャッシュディレクトリ | `[cache] dir` | `DOH_LOOKUP_CACHE_DIR` | `~/.cache/doh-lookup` |
| ネットワークタイムアウト | `[network] timeout_seconds` | `DOH_LOOKUP_TIMEOUT_SECONDS` | `10` |

## 「見分けがつく」を保つ仕組み

- **帯域外・HTTPS 経由。** クエリはローカルリゾルバではなく `1.1.1.1` / `8.8.8.8` の
  443 へ向かい、ネットワーク層でブラウジング DNS と分離可能。
- **すべての結果に来歴。** リゾルバ名とエンドポイント URL を常に報告し、調査を
  匿名でなく監査可能にする。
- **DNSSEC AD を報告。** `authenticated` フラグはリゾルバが応答チェーンを検証したかを
  示す（AD を意味あるものにするため DO ビットを要求する。署名自体の検証は行わない）。
- **ECS を既定で抑止。** リゾルバに EDNS Client Subnet を転送しないよう要求し、
  調査元ネットワークを権威サーバーへ漏らさない。

## 注記

- **DNSSEC:** `authenticated: true` は「リゾルバがチェーンを検証した」の意で、
  doh-lookup が署名を検証したわけではない。独立検証（RFC 8484 wireformat + 暗号）は
  v2 候補。
- **レート制限:** 公開 DoH には IP 単位のソフト制限がある。応答は DNS TTL を尊重して
  （フロア付きで）キャッシュされ、bulk は礼儀正しくペーシングする（積極リトライなし）。

## 開発

```bash
make build      # → dist/doh-lookup（`go build` を直接使わない）
make test       # オフラインの単体/結合テスト（HTTP はモック）
make e2e        # 実 Cloudflare/Google DoH への live E2E（ネットワーク必須）
make check      # lint + test + build-all
```

Go 1.25+・標準ライブラリのみ。アーキテクチャは [AGENTS.md](AGENTS.md) を参照。

## ライセンス

MIT — [LICENSE](LICENSE) を参照。
