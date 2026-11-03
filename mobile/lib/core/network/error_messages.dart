import 'api_client.dart';

String errorMessage(Object error) {
  if (error is! ApiException) return '処理を完了できませんでした。もう一度お試しください。';
  final message = switch (error.code) {
    'network_error' => '通信できませんでした。接続状況を確認して再試行してください。',
    'secure_storage_error' => '端末の安全な保管領域にアクセスできません。端末のロックを解除して再試行してください。',
    'invalid_credentials' => 'メールアドレスまたはパスワードを確認してください。',
    'email_already_registered' => 'このメールアドレスはすでに登録されています。',
    'email_verification_required' => 'メールアドレスの本人確認を完了してください。',
    'invalid_token' => '認証情報の有効期限が切れているか、すでに使用されています。',
    'session_expired' => 'ログインの有効期限が切れました。もう一度ログインしてください。',
    'refresh_recovery_expired' => '更新結果を復元できる期限を過ぎました。もう一度ログインしてください。',
    'refresh_conflict' => '認証の更新が競合しました。保存済みの情報を確認して再試行してください。',
    'profile_required' => '通常利用の前にニックネームを設定してください。',
    'invalid_nickname' => 'ニックネームは2〜20文字で入力してください。',
    'invalid_password' => 'パスワードの条件を満たしていません。入力を確認してください。',
    'validation_error' => 'メールアドレスとパスワードの入力条件を確認してください。',
    'provider_unavailable' => 'この外部認証は設定待ちです。現在は利用できません。',
    'provider_cancelled' => '外部認証をキャンセルしました。',
    'external_auth_failed' => '外部認証を確認できませんでした。最初からやり直してください。',
    'identity_already_linked' => 'その外部アカウントはすでに連携されています。自動統合は行いません。',
    'favorite_limit' => '現役の推し選手は最大3人まで登録できます。',
    'ineligible_player' => '現在登録できる阪神の現役選手ではありません。一覧を更新してください。',
    'image_too_large' => '画像は10MB以下にしてください。容量内でもデコード時の安全上限を超える画像は登録できません。',
    'invalid_image' => 'JPEG・PNG・HEICの正常な画像を選択してください。',
    'image_permission_denied' => '画像へのアクセスが許可されていません。端末の設定で権限をご確認ください。',
    'image_selection_failed' => '画像を読み込めませんでした。ファイルへのアクセスと端末の設定をご確認ください。',
    'not_found' => '対象の情報が見つからないか、公開されていません。',
    'storage_unavailable' => '画像の保存先が利用できません。時間をおいて再試行してください。',
    'action_busy' => '処理中です。完了までお待ちください。',
    _ => switch (error.status) {
      401 => '認証を確認できません。もう一度ログインしてください。',
      403 => 'この情報を表示・変更する権限がありません。',
      429 => '試行回数が上限に達しました。時間をおいて再試行してください。',
      503 => '現在この機能は利用できません。時間をおいて再試行してください。',
      _ => '処理を完了できませんでした。入力と接続状況を確認してください。',
    },
  };
  return error.retryAfter != null
      ? '$message（再試行まで約${error.retryAfter}秒）'
      : message;
}
