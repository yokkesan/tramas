# tramas

阪神タイガースファン向けアプリの初期開発環境です。

## ローカル起動

Docker Desktopを起動し、リポジトリ直下で実行します。

```sh
python3 scripts/setup-dev-env.py
docker compose -f compose.yaml -f compose.dev.yaml up -d --build --wait
```

`.env` が既にある場合、作成スクリプトは上書きせず終了します。開発専用パスワードはランダム生成され、`.env` の権限は0600です。

- API: http://127.0.0.1:8080/health
- 管理確認ページ: http://127.0.0.1:8080/admin
- `DEV_HTTP_PORT` でホスト側ポートを変更可能。PostgreSQL・API・Workerのポートは直接公開しません。

```sh
docker compose -f compose.yaml -f compose.dev.yaml ps
docker compose -f compose.yaml -f compose.dev.yaml logs --tail=100 api worker
docker compose -f compose.yaml -f compose.dev.yaml stats --no-stream
docker compose -f compose.yaml -f compose.dev.yaml down
```

`down` 後も名前付きボリュームのDBデータは保持されます。`down -v` はデータを削除するため、通常の停止では使用しません。

## Go

```sh
cd backend
go tool templ generate
go test ./...
go vet ./...
```

ソース変更後はComposeを `up -d --build --wait` で再ビルドしてください。ホットリロード用の追加依存は導入していません。templ CLIも `go.mod` のtool directiveでバージョンを固定しています。

モジュール名は `tramas/backend` です。公開リポジトリは仮定していません。将来モジュールパスを変更する際は `go.mod` と内部importを書き換えます。

## Flutter

今回確認したMac側SDKはFlutter **3.44.8**（Dart **3.12.2**）です。`mobile/.fvmrc` に使用バージョンを記録し、`pubspec.lock` を保持します。SDKのインストール・更新、MacのPATH変更は行っていません。以下はこのMac上の実行例です。

```sh
cd mobile
/Users/yokke/fvm/versions/stable/bin/flutter pub get --enforce-lockfile
/Users/yokke/fvm/versions/stable/bin/flutter analyze
/Users/yokke/fvm/versions/stable/bin/flutter test
# iOS Simulator
/Users/yokke/fvm/versions/stable/bin/flutter run --dart-define-from-file=config/dev-ios.json
# Android Emulator
/Users/yokke/fvm/versions/stable/bin/flutter run --dart-define-from-file=config/dev-android.json
```

FlutterはホストのXcode・Android SDKを使います。実行前に端末またはSimulator/Emulatorを起動してください。Androidの未承認ライセンスは、内容を確認して別途承認する必要があります。

API接続先は `--dart-define-from-file` または `--dart-define=API_BASE_URL=...` で指定します。設定値はアプリに埋め込まれるため、秘密情報を入れないでください。実機接続は未構成です。ローカルComposeはloopback限定なので、実機向け公開が必要な場合はアクセス範囲を決めてから変更してください。

`config/production.example.json` の接続先は意図的に空です。正式なHTTPS接続先を決めて別ファイルで指定してください。releaseビルドでは `APP_ENV=production` とHTTPSを必須としています。iOSのHTTP許可はDebug専用plist、AndroidのHTTP許可はdebug manifestのみに設定しています。

Flutter生成時の `com.example.tramas` は仮のアプリ識別子です。正式なBundle ID・Application ID、署名・配布設定は未確定です。

## 構造と本番設定

- APIとWorkerは別エントリーポイント。DB接続・設定・ヘルスチェックは `backend/internal/platform` で共有します。
- WorkerはDB接続を監視する待機プロセスです。ジョブ・キュー・業務処理はまだありません。
- `/health` はDB接続失敗時に503を返します。Workerのhealthcheckは処理ループの生存とDB接続を確認します。
- 管理ページと静的アセットは `APP_ENV=development` だけに登録します。認証は未実装です。
- CORSは許可していません。モバイルネイティブ通信と同一originのhtmxが対象です。
- APIはリクエストID・リクエストログ・panic回復・ボディサイズ制限・HTTPタイムアウト・セキュリティヘッダーを設定しています。認証・業務API導入時の認可、入力検証、レート制限は後続工程で設計します。
- PostgreSQLの初回初期化で非superuserのアプリ用ユーザーを作成します。業務テーブル・マイグレーション方式は未導入です。`.env` の値を変えるだけでは既存DBの資格情報は変更されません。
- アプリ用ユーザーにはDB接続とpublicスキーマの参照権限だけを付与します。将来のテーブル権限・マイグレーション権限は別途設計が必要です。
- 全サービスを非rootで実行します。API・Worker・Caddyはread-only filesystem、capability削除を設定しています。ログは各サービス10MB×3、CPU・メモリには上限を設定しています。

本番向け差分は `compose.prod.yaml` です。開発用ファイルと本番用ファイルを同時指定しないでください。本番では別環境ファイルを明示します。

```sh
# 設定確認の形のみ。今回、本番への接続・起動は実施しません。
docker compose --env-file /secure/path/tramas-production.env -f compose.yaml -f compose.prod.yaml config --quiet
```

本番の環境ファイルには独立したDB名・ユーザー・URL-safeなパスワード（hex等）、`TRAMAS_DOMAIN` を設定します。開発用.envは再利用しません。Caddyが80/443を受けてHTTPSを提供する想定です。証明書・設定は名前付きボリュームで保持します。

現段階のDocker内DB接続は分離したinternalネットワーク上の平文です。VPS上の脅威モデルに応じたDB TLS、バックアップ・復元、ファイアウォール、秘密情報の配布・ローテーション、監視、イメージ更新・脆弱性確認、管理者認証は本番公開前の未完了事項です。今回の設定だけで本番公開可能とはしていません。

確認結果とファイル一覧は `docs/development-verification.md` と `docs/created-files.txt` に記録します。
