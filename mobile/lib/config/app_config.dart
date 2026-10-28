import 'package:flutter/foundation.dart';

class AppConfig {
  const AppConfig._(this.environment, this.apiBaseUrl);

  final String environment;
  final Uri apiBaseUrl;

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
  );

  factory AppConfig.fromValues({
    required String environment,
    required String baseUrl,
    bool release = false,
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
    return AppConfig._(environment, uri);
  }
}
