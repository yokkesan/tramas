import 'package:flutter/foundation.dart';

class AppConfig {
  const AppConfig._(
    this.environment,
    this.apiBaseUrl,
    this.appleEnabled,
    this.googleClientId,
    this.googleRedirectUri,
    this.appleWebClientId,
    this.appleRedirectUri,
    this.googleAndroidServerClientId,
  );

  final String environment;
  final Uri apiBaseUrl;
  final bool appleEnabled;
  final String googleClientId;
  final String googleRedirectUri;
  final String appleWebClientId, appleRedirectUri;
  final String googleAndroidServerClientId;
  bool get googleEnabled =>
      googleClientId.isNotEmpty && googleRedirectUri.isNotEmpty;

  factory AppConfig.fromEnvironment() => AppConfig.fromValues(
    environment: const String.fromEnvironment(
      'APP_ENV',
      defaultValue: 'development',
    ),
    baseUrl: const String.fromEnvironment(
      'API_BASE_URL',
      defaultValue: 'http://127.0.0.1:8080',
    ),
    release: kReleaseMode,
    appleEnabled: const bool.fromEnvironment('APPLE_SIGN_IN_ENABLED'),
    googleClientId: const String.fromEnvironment('GOOGLE_OIDC_CLIENT_ID'),
    googleRedirectUri: const String.fromEnvironment('GOOGLE_OIDC_REDIRECT_URI'),
    appleWebClientId: const String.fromEnvironment('APPLE_WEB_CLIENT_ID'),
    appleRedirectUri: const String.fromEnvironment('APPLE_REDIRECT_URI'),
    googleAndroidServerClientId: const String.fromEnvironment(
      'GOOGLE_ANDROID_SERVER_CLIENT_ID',
    ),
  );

  factory AppConfig.fromValues({
    required String environment,
    required String baseUrl,
    bool release = false,
    bool appleEnabled = false,
    String googleClientId = '',
    String googleRedirectUri = '',
    String appleWebClientId = '',
    String appleRedirectUri = '',
    String googleAndroidServerClientId = '',
  }) {
    final uri = Uri.tryParse(baseUrl);
    if (!['development', 'staging', 'production'].contains(environment) ||
        uri == null ||
        uri.host.isEmpty ||
        !['http', 'https'].contains(uri.scheme) ||
        uri.userInfo.isNotEmpty ||
        uri.hasQuery ||
        uri.hasFragment ||
        (uri.path.isNotEmpty && uri.path != '/')) {
      throw ArgumentError('APP_ENV or API_BASE_URL is invalid.');
    }
    if ((release || environment != 'development') && uri.scheme != 'https') {
      throw ArgumentError(
        'HTTPS is required outside development debug builds.',
      );
    }
    if (release && environment != 'production') {
      throw ArgumentError('Release builds require APP_ENV=production.');
    }
    if (googleRedirectUri.isNotEmpty) {
      final callback = Uri.tryParse(googleRedirectUri);
      if (callback == null ||
          callback.scheme.isEmpty ||
          callback.scheme == 'http' ||
          callback.scheme != 'https' && !callback.scheme.contains('.') ||
          callback.userInfo.isNotEmpty ||
          callback.hasFragment) {
        throw ArgumentError('Invalid Google OIDC redirect URI.');
      }
    }
    if (appleRedirectUri.isNotEmpty) {
      final callback = Uri.tryParse(appleRedirectUri);
      if (callback == null ||
          callback.scheme != 'https' ||
          callback.host.isEmpty ||
          callback.userInfo.isNotEmpty ||
          callback.hasFragment) {
        throw ArgumentError('Invalid Apple redirect URI.');
      }
    }
    return AppConfig._(
      environment,
      uri,
      appleEnabled,
      googleClientId,
      googleRedirectUri,
      appleWebClientId,
      appleRedirectUri,
      googleAndroidServerClientId,
    );
  }
}
