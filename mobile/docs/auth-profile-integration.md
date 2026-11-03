# 認証・プロフィールのFlutter連携

## 実装範囲

既存のログイン・新規登録画面の配色、ヘッダー、入力欄、ボタン構成を維持し、API処理、エラー、送信中表示を接続しました。デザイン参照元はリポジトリ内の `design/mobile/index.html` とデザインガイドです。`mobile-index.html` というファイルはありませんでした。

Go APIの契約は [authentication.md](../../docs/api/authentication.md) と [backend-features.md](../../docs/api/backend-features.md) を参照しています。バックエンド、開発DB、本番環境はこの作業では変更していません。

| 機能 | 接続先（`/api/v1` 以下） |
|---|---|
| 登録・ログイン | `POST /auth/register`, `POST /auth/login` |
| メール確認・再送 | `POST /auth/email/verify`, `POST /auth/email/resend` |
| セッション | `POST /auth/refresh`, `POST /auth/logout`, `GET /auth/me` |
| 初回ニックネーム | `PATCH /users/me/profile` |
| パスワード再設定 | `POST /auth/password/reset/request`, `POST /auth/password/reset/complete` |
| OIDC | `POST /auth/oidc/{apple,google}/challenge`, `/login`, `/reauth`, `/link` |
| パスワード再認証 | `POST /auth/reauthenticate` |
| 本人プロフィール | `GET`, `PUT /users/me/profile` |
| 公開プロフィール | `GET /users/{id}/profile` |
| 画像 | `PUT`, `DELETE /users/me/profile/images/{avatar,header}` |
| 標準画像 | `GET /profile/standard-images`, `PUT /users/me/profile/images/{kind}/standard` |
| アップロード済み画像表示 | `GET /profile/images/{id}` |
| 選手一覧・推し | `GET /players`, `GET /users/{me,id}/favorites` |
| 推し変更 | `POST /users/me/favorites`, `DELETE /users/me/favorites/{id}` |
| メイン変更 | `PUT /users/me/favorites/{id}/main`, `DELETE /users/me/favorites/main` |
| 本人限定の履歴 | `GET /users/me/favorites/history`, `GET /users/me/favorites/main-history` |

登録は従来どおりメール・パスワードを送信し、201を成功と扱います。登録後はメール確認案内へ進み、自動ログインしません。確認・再送・再設定は仕様書の成功ステータスとエラーコードに従い、存在確認につながる表示を避けています。401では更新または再ログイン、403では権限／初回設定、429では待機案内、503では一時利用不可を表示します。

## セッションと画面制御

- アクセス／更新トークン、更新処理の識別子、ログアウト保留状態は `flutter_secure_storage` の単一レコードに保存します。通常のSharedPreferences、ログ、平文ファイルへの保存はありません。API接続先ごとにキーを分離しています。
- APIの期限と `/auth/me` を基準に復元します。起動時の確認が失敗した場合は再試行画面となり、保存されたプロフィール状態だけで通常画面へ進めません。
- 更新要求を直列化し、送信前に保存した同じ `request_id` と更新トークンで通信失敗時に再試行します。再起動後も復元できます。バックエンドの60秒の再試行猶予に対して55秒で打ち切り、以後は再ログインを求めます。429は自動連続再試行しません。
- ログアウトは更新処理と調整してGo側の失効APIを呼びます。通信失敗時は失効保留として通常画面を閉じ、再試行できます。「端末の情報のみ削除」はリモート失効を保証しない旨を表示したうえで明示操作に限定します。
- `profile_completed=false` は初回ニックネーム設定に誘導し、設定後も `/auth/me` で確認します。文字数は `runes.length`、2〜20文字です。ASCIIスペースの前後除去、制御文字等の除外はGoの仕様に合わせています。
- 遅れて到着する通信／外部認証／画像選択の結果が別セッションへ混入しないように世代を検証します。ユーザー変更時には画像キャッシュと画面履歴を破棄します。

## プロフィール・画像・推し

本人のみ編集画面へ進めます。公開プロフィールは共有されたユーザーUUIDから表示し、メールアドレス・本人限定履歴・編集操作は表示しません。自己紹介に独自の文字数上限は追加していません。

画像は端末のファイル／写真選択機構から取得し、10MiB以下、JPEG・PNG・HEICのシグネチャを確認してバイナリPUTします。内容の最終検証はサーバー側で行います。アプリ独自のトリミングや再エンコードは行いません。標準画像カタログが空の場合も空状態を表示します。HEICの実機デコードやOSの写真アクセス許可はシミュレーター／実機で確認が必要です。バックエンドのデコード予算によっては10MiB以下でも拒否されます。

現役阪神選手だけを選択対象とし、最大3人、メイン最大1人を制御します。最終判断はAPIの応答に従います。選手0件では空状態を表示し、架空データを作成しません。本人限定履歴はカーソルで追加読込します。メイン履歴でAPIから選手名が得られない場合は選手IDを表示します。

## 設定待ち

設定値は `AppConfig` のDart define経由です。既存の `config/dev-ios.json`、`config/dev-android.json` と本番HTTPS検証を維持しています。クライアントIDは公開設定ですが、秘密鍵やクライアントシークレットをDart defineへ入れないでください。

| 対象 | 必要な設定／作業 |
|---|---|
| Apple iOS | 実際のApp ID、開発Team／署名、Sign in with Apple capability、Go側audience設定。その後 `APPLE_SIGN_IN_ENABLED=true` |
| Apple Android | 上記有効化に加え `APPLE_WEB_CLIENT_ID`、`APPLE_REDIRECT_URI`。Apple Service IDとHTTPSコールバック中継が必要。外部中継サービスは未提供のため未導入 |
| Google iOS | `GOOGLE_OIDC_CLIENT_ID`, `GOOGLE_OIDC_REDIRECT_URI`、対応するXcodeのURL scheme、Go側audience許可設定 |
| Google Android | `GOOGLE_ANDROID_SERVER_CLIENT_ID`、Google ConsoleのAndroid package／署名SHA登録、Go側audience許可設定 |
| メール | 外部配送サービス・リンク着地点・Universal Linkは未決定。画面はメール内トークンの手動入力に対応。開発用メール受信箱の管理キーをアプリへ埋め込まない |
| 画像素材・選手 | Go側の標準画像カタログと実選手データの整備待ち。空表示は実装済み |

Appleは `sign_in_with_apple`、Google iOSはAppAuth、Google Androidは公式Credential Manager／Google IDライブラリに接続しています。各試行でGoのchallengeを取得し、そのnonceをSDKへ渡してからIDトークンをGoへ送ります。Goによる検証完了後だけアプリのセッションを保存します。設定不足を成功扱いする処理はありません。

Googleの `google_sign_in` は初期化が一度だけでnonceも初期化時の設定となるため、試行ごとに変わるGoのnonceに合わせて上記構成としています。明示的なアカウント連携はパスワードまたは既存プロバイダーによる再認証後に行い、メール一致による統合は行いません。

SDK参照: [Apple](https://pub.dev/packages/sign_in_with_apple)、[AppAuth](https://pub.dev/packages/flutter_appauth)、[Google initialize](https://pub.dev/documentation/google_sign_in/latest/google_sign_in/GoogleSignIn/initialize.html)、[Android Credential Manager](https://developer.android.com/identity/sign-in/credential-manager-siwg-implementation)、[Google nonce](https://developers.google.com/identity/android-credential-manager/android/reference/com/google/android/libraries/identity/googleid/GetSignInWithGoogleOption.Builder)。

## 自動検証

2026-11-02、Flutter 3.44.8 / Dart 3.12.2で実施しました。

- `flutter analyze`: 指摘なし。
- `flutter test --machine`: **45テスト成功、失敗0、スキップ0**。ローダーのイベントは件数から除外しています。
- 対象: APIとエラー形式、登録201／既存サービス互換、ログイン／ログアウト、更新競合／応答消失／再起動／失効／保留失効、初回プロフィール遷移、ネイティブSDKのnonce受け渡し、再認証連携と遅延応答、画像・権限エラー、Unicode文字数、推し制約／履歴ページング。
- テストは偽transport、偽secure storage、SDK MethodChannelスタブ、一時的なローカルHTTPサーバーを利用しています。本番サービスやGo APIへの実通信テストではありません。
- `flutter build apk --debug --dart-define-from-file=config/dev-android.json`: 成功。`build/app/outputs/flutter-apk/app-debug.apk` を生成しました。初回ビルド時に既存SDK環境へAndroid Platform 35が自動追加されました。
- iOSビルド、シミュレーターでの操作、実Apple／Google認証は未実施です。

## iOSシミュレーターでの確認手順

Go開発APIを既存の手順で起動した状態で、次を実行してください。

```sh
cd mobile
fvm flutter pub get
fvm flutter run --dart-define-from-file=config/dev-ios.json
```

1. 既存のログイン／登録デザインとAPI接続先が維持されていることを確認します。
2. メール登録後に確認案内へ進むこと、開発用受信箱で得たトークンの入力・再送・使用済みトークンのエラーを確認します。
3. ログインし、未設定の場合はニックネーム画面へ進むことを確認します。1文字／21文字の拒否、2〜20文字の保存後の通常画面への遷移を確認します。
4. アプリを再起動し、設定済みなら再び初回設定を要求されないことを確認します。通信を切った復元時には再試行画面になることも確認します。
5. マイページでニックネーム・自己紹介、プロフィール／ヘッダー画像の選択・削除を確認します。JPEG／PNG／HEIC、10MiB超、写真アクセス拒否、標準画像の空状態を確認します。
6. 選手一覧0件の空状態、本人履歴、公開プロフィールのUUID検索を確認します。実データ整備後に3人上限・メイン変更・履歴の追加読込を確認します。
7. ログアウト、通信を切ったログアウトの保留／再試行、パスワード再設定の要求とトークン入力、新パスワードでのログインを確認します。
8. Apple／Googleの実設定が揃った後に、外部ログイン・キャンセル・再認証・明示連携を確認します。設定前は利用不可表示になるのが正しい挙動です。

Games／SNSの機能本体、未提供のメールリンク用Web画面、外部認証の本番設定、アカウント削除等は今回の実装対象外です。通常画面の未実装タブは準備中表示とし、架空の試合・投稿は表示しません。

## 変更ファイル

既存変更: `lib/main.dart`, `lib/config/app_config.dart`, `lib/features/auth/screens/{login_screen,register_screen}.dart`, `lib/features/auth/services/register_service.dart`, `pubspec.yaml`, `pubspec.lock`, `test/widget_test.dart`, `.gitignore`, `README.md`, `android/app/build.gradle.kts`, `android/app/src/main/AndroidManifest.xml`, `android/app/src/main/kotlin/com/example/tramas/MainActivity.kt`, `ios/Runner.xcodeproj/project.pbxproj`, `ios/Runner/{Info.plist,Info-Debug.plist}`。

追加:

- `lib/app/{app_scope,app_shell}.dart`
- `lib/core/network/{api_client,error_messages}.dart`, `lib/core/widgets/feature_widgets.dart`
- `lib/features/auth/data/{auth_repository,session_vault}.dart`, `models/session.dart`, `state/session_controller.dart`, `services/oidc_service.dart`
- `lib/features/auth/screens/{account_link_screen,password_reset_screen,verification_screen}.dart`
- `lib/features/profile/data/profile_repository.dart`, `models/profile.dart`, `services/image_selection.dart`, `widgets/profile_image_view.dart`
- `lib/features/profile/screens/{nickname_screen,profile_editor_screen,profile_screen}.dart`
- `lib/features/favorites/data/favorites_repository.dart`, `models/favorite.dart`, `state/favorites_controller.dart`, `screens/{favorites_screen,history_screen}.dart`
- `test/auth/{api_and_oidc_test,native_sdk_test,navigation_test,session_controller_test}.dart`
- `test/profile/{profile_and_favorites_test,profile_widgets_test}.dart`, `test/support/fakes.dart`
- `ios/Runner/Runner.entitlements`, 本書 `docs/auth-profile-integration.md`

`.gitignore` はルートの汎用 `data/` 除外がFlutterのRepositoryソースまで隠さないよう、mobile内だけ例外を追加しています。コミット・pushは行っていません。
