import 'dart:convert';
import 'dart:io';

class HealthClient {
  Future<bool> check(Uri baseUrl) async {
    final client = HttpClient()..connectionTimeout = const Duration(seconds: 5);
    try {
      final request = await client
          .getUrl(baseUrl.resolve('/health'))
          .timeout(const Duration(seconds: 5));
      request.followRedirects = false;
      final response = await request.close().timeout(
        const Duration(seconds: 5),
      );
      final body = await response
          .transform(utf8.decoder)
          .join()
          .timeout(const Duration(seconds: 5));
      if (response.statusCode != HttpStatus.ok) return false;
      final value = jsonDecode(body);
      return value is Map &&
          value['status'] == 'ok' &&
          value['database'] == 'ok';
    } finally {
      client.close(force: true);
    }
  }
}
