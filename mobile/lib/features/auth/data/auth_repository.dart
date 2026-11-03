import '../../../core/network/api_client.dart';
import '../models/session.dart';
import '../services/register_service.dart';

class AuthRepository {
  AuthRepository(this.api);
  final ApiClient api;
  Future<void> register(String email, String password) => RegisterService(
    api: api,
  ).registerDetailed(email: email, password: password);
  Future<void> verify(String token) async {
    await api.send('POST', '/api/v1/auth/email/verify', body: {'token': token});
  }

  Future<void> resend(String email) async {
    await api.send('POST', '/api/v1/auth/email/resend', body: {'email': email});
  }

  Future<TokenPair> login(String email, String password) async {
    final r = await api.send(
      'POST',
      '/api/v1/auth/login',
      body: {'email': email, 'password': password},
    );
    return TokenPair.fromJson(
      (r.json['tokens'] as Map).cast<String, dynamic>(),
    );
  }

  Future<TokenPair> refresh(String token, String requestId) async {
    final r = await api.send(
      'POST',
      '/api/v1/auth/refresh',
      body: {'refresh_token': token, 'request_id': requestId},
    );
    return TokenPair.fromJson(
      (r.json['tokens'] as Map).cast<String, dynamic>(),
    );
  }

  Future<void> logout(String token) async {
    await api.send(
      'POST',
      '/api/v1/auth/logout',
      body: {'refresh_token': token},
    );
  }

  Future<void> requestReset(String email) async {
    await api.send(
      'POST',
      '/api/v1/auth/password/reset/request',
      body: {'email': email},
    );
  }

  Future<void> completeReset(String token, String password) async {
    await api.send(
      'POST',
      '/api/v1/auth/password/reset/complete',
      body: {'token': token, 'password': password},
    );
  }
}
