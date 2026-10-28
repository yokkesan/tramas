# 開発環境の構築・検証結果

確認日: 2026-10-28（日本時間）

## 作成内容

全ファイル一覧は [created-files.txt](created-files.txt) を参照してください。Flutter標準生成物を含み、今回以前から存在した22個の `.gitkeep` は除いています。

- ルート: README、Compose共通・開発・本番差分、環境変数サンプル、Git/Docker除外設定
- backend: Go Modules、API・Worker、共通DB接続・設定・ヘルスチェック、開発用管理画面、templ生成コード、固定版htmxとライセンス
- mobile: FlutterのiOS/Android標準構成、仮画面、API接続確認、環境別URL設定、依存ロック、SDKバージョン記録、Debug限定HTTP許可
- infra: API/WorkerのマルチステージDockerfile、非root用Caddy Dockerfile、Caddy設定、PostgreSQL初期化SQL
- scripts: 既存.envを上書きしない開発環境変数作成スクリプト
- docs: この報告書、機械可読な実測記録、作成ファイル一覧

開発専用 `.env` はGit除外・0600で作成しています。ビルド・SDKキャッシュ・IDEファイル等の除外対象生成物は一覧に含めていません。

## バージョン

| 技術 | 実際に使用したバージョン |
|---|---|
| Mac CPU | arm64 |
| Go（ホスト・Dockerビルド） | 1.26.5 |
| Echo | 5.4.2 |
| templ（runtime・CLI） | 0.3.1100 |
| pgx | 5.11.0 |
| htmx | 2.0.8（ローカル配信） |
| Flutter | 3.44.8 stable |
| Dart | 3.12.2 |
| PostgreSQL | 17.11 |
| Caddy | 2.11.7 |
| Docker | 29.5.2 |
| Docker Compose | 5.1.4 |
| Xcode | 26.6 |
| Android SDK | 36.1.0（doctorによる確認） |

Go依存は `go.mod` / `go.sum`、Flutter依存は `pubspec.lock`、Flutter SDKは `.fvmrc` に記録しました。Dockerベースイメージは取得したdigestに固定しています。将来のセキュリティ更新時には、バージョン・digestを更新して再検証してください。

Echoの導入方法は[公式Quickstart](https://echo.labstack.com/guide/quickstart/)、templ CLIの固定方法は[公式Installation](https://templ.guide/quick-start/installation/)、htmxのHTML応答方式は[公式ドキュメント](https://htmx.org/docs/)、Composeの環境別差分は[Docker公式ドキュメント](https://docs.docker.com/compose/how-tos/production/)を参照しました。

## 動作確認済み

| 検証 | 結果 |
|---|---|
| 開発Composeの設定検証 | 成功 |
| 本番差分Composeの設定検証 | 仮の検証値で成功。起動・デプロイなし |
| 4サービス | api / worker / postgres / caddy すべてhealthy |
| API | Caddy経由の `/health` が200、`status=ok, database=ok` |
| 管理画面HTTP | `/admin` が200、templ生成HTMLとhtmx属性を確認 |
| 管理画面のDB確認HTTP | `/admin/status` が200、接続成功のHTMLを確認 |
| htmx | 固定版JSのHTTP取得成功。SHA-256を記録 |
| Worker | 起動ログ・専用healthcheck・DB接続監視を確認。ジョブ未実装 |
| DB認証 | アプリ用ユーザー `tramas_app` のTCP接続成功 |
| DB権限 | アプリ用ユーザーは非superuser、DB作成・ロール作成権限なし |
| 業務テーブル | publicスキーマのテーブル数0 |
| 永続化 | 開発DBに検証用コメントを設定し、PostgreSQLコンテナ再作成後も保持。検証後にコメント削除 |
| 障害時 | DB停止中のAPI直アクセスで503を確認。DB再起動後、4サービスhealthyへ復旧 |
| DB新規初期化 | 別のネットワーク非接続・tmpfs上の一時PostgreSQLで最終SQLを検証。アプリ認証成功、テーブル数0。検証コンテナは停止・削除済み |
| 本番モードのルート制限 | 開発DBを使う一時APIで `APP_ENV=production` を検証。health正常、管理・静的アセットの各ルート404。一時APIは停止・削除済み |
| CaddyのHTTPS設定 | 検証用.invalidドメインで設定検証成功。証明書取得・外部HTTPS通信は未実施 |
| 非root | api/worker UID10001、postgres UID70、caddy UID1000 |
| 公開ポート | Caddyのみ127.0.0.1:8080。API/Worker/PostgreSQLはホスト公開なし |
| ログ上限 | 各サービスjson-file、10MB×3 |
| Go | templ生成、`go test ./...`、`go vet ./...`、`go mod verify` 成功。Goの個別テストケースはまだ未実装 |
| Flutter | pub get、ロックファイル固定での再解決、静的解析、2件のテスト成功 |
| iOS | Simulator用Debugビルド成功（署名なし）。生成アプリのDebug用HTTP許可設定も確認 |
| iOS設定 | Info.plist・Debug用plist・Xcodeプロジェクトの構文検証成功 |

実測の詳細は [runtime-verification.json](runtime-verification.json) に記録しています。

## リソース使用量

無負荷に近い状態での `docker compose stats --no-stream` の一回の測定です。性能試験・必要なVPS容量の見積もりではありません。

| サービス | CPU | メモリ使用量 | メモリ上限 | CPU上限 |
|---|---:|---:|---:|---:|
| api | 0.00% | 7.281 MiB | 256 MiB | 0.5 CPU |
| worker | 0.00% | 5.23 MiB | 256 MiB | 0.5 CPU |
| postgres | 0.04% | 21.13 MiB | 512 MiB | 1 CPU |
| caddy | 0.02% | 13.55 MiB | 128 MiB | 0.5 CPU |

## 未確認・未実施

- 管理ページのブラウザ描画・htmxボタン実操作。利用可能なブラウザ接続がなく、ChromeのComputer Useも許可されなかったため未確認。HTTP応答・JS取得確認と区別しています。
- iOS Simulator上でのアプリ起動、画面操作、Flutterから稼働APIへの実接続。Simulator向けビルドまで確認済みです。
- Androidネイティブビルド・Emulator起動・実機実行。doctorで一部ライセンス未承認と判明しており、承認操作は実施していません。
- 正式なアプリ識別子、署名・ストア配布、本番APIドメイン。生成された `com.example.tramas` は仮です。
- VPS接続、本番認証情報の作成、本番デプロイ、実ドメインのHTTPS・証明書更新検証。
- DB TLS・バックアップ復元・運用監視等の本番運用設計。
- 認証・認可・野球・SNS・AI・ジョブキュー・業務テーブル・マイグレーション方式。
- 設計書 `tramas_basic_design_v1.md` はワークスペース内に存在せず、整合性は未確認です。

## 発生した問題と対応

1. ComposeのtmpfsをYAMLフロー配列に記載した際、カンマが別項目として解釈され起動失敗。文字列全体を引用して修正しました。
2. PostgreSQLの初期化シェルが実行権限エラーとなり、アプリ用ロールが未作成になりました。初期化方式をSQLファイルに変更し、今回作成した開発DBへ不足するロールだけを追加しました。DBの削除は行っていません。最終SQLの新規初期化は一時DBで別途検証しました。
3. Caddy公式バイナリの低ポート用file capabilityがcapability全削除設定と競合して実行失敗。高ポートで待ち受ける構成のため、Dockerビルド時に不要なfile capabilityを除去しました。非root・cap_drop・no-new-privilegesを維持して起動できています。
4. 一時検証コマンドにPythonの引用記法エラーがありました。コマンドを修正して再実行し、検証は成功しています。
5. 上記のブラウザ利用制限とAndroidライセンス未承認は未解決です。

## 既存ファイル・Git・環境への影響

作業前の22個の `.gitkeep` は保持しています。既存ファイルとの衝突はありません。今回生成したFlutter標準ファイルは、仮画面と環境設定に合わせて編集しました。

Gitリポジトリは作業前後とも未初期化です。Git初期化・commit・push・ブランチ変更は行っていません。`.gitignore` の設定は追加しましたが、まだGit管理は開始していません。

MacのPATH、SDKバージョン、Androidライセンスは変更していません。各ツールの実行に伴うSDK・依存キャッシュとDockerイメージ・tramas専用ネットワーク・永続ボリュームは生成されています。既存の他プロジェクトのコンテナ・DBには操作していません。

完了時はtramasの4サービスを起動したままにしています。
