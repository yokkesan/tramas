import 'dart:convert';
import 'dart:io';

class RegisterService {
  Future<int> register({
    required String apiBaseUrl,
    required String email,
    required String password,
  }) async {
    final client = HttpClient()..connectionTimeout = const Duration(seconds: 5);

    try {
      final uri = Uri.parse('$apiBaseUrl/api/v1/auth/register');

      final request = await client
          .postUrl(uri)
          .timeout(const Duration(seconds: 5));

      request.headers.contentType = ContentType.json;

      request.write(jsonEncode({'email': email, 'password': password}));

      final response = await request.close().timeout(
        const Duration(seconds: 5),
      );

      await response.drain<void>();

      return response.statusCode;
    } finally {
      client.close(force: true);
    }
  }
}
