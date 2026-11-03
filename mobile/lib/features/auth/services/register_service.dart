import '../../../config/app_config.dart';
import '../../../core/network/api_client.dart';
import 'package:flutter/foundation.dart';

class RegisterService {
  RegisterService({this.api});
  final ApiClient? api;
  Future<void> registerDetailed({
    required String email,
    required String password,
  }) async {
    final response = await api!.send(
      'POST',
      '/api/v1/auth/register',
      body: {'email': email, 'password': password},
    );
    if (response.status != 201) {
      throw const ApiException('invalid_response');
    }
  }

  Future<int> register({
    required String apiBaseUrl,
    required String email,
    required String password,
  }) async {
    final client =
        api ??
        ApiClient(
          AppConfig.fromValues(
            environment: const String.fromEnvironment(
              'APP_ENV',
              defaultValue: 'development',
            ),
            baseUrl: apiBaseUrl,
            release: kReleaseMode,
          ),
        );
    try {
      final response = await client.send(
        'POST',
        '/api/v1/auth/register',
        body: {'email': email, 'password': password},
      );
      return response.status;
    } on ApiException catch (e) {
      if (e.status != 0) return e.status;
      rethrow;
    }
  }
}
