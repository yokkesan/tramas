import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import '../../config/app_config.dart';

class ApiException implements Exception {
  const ApiException(
    this.code, {
    this.status = 0,
    this.retryAfter,
    this.fields = const {},
  });
  final String code;
  final int status;
  final int? retryAfter;
  final Map<String, dynamic> fields;
  bool get transient => status == 0 || status == 429 || status >= 500;
  @override
  String toString() => 'ApiException($status, $code)';
}

class ApiResponse {
  const ApiResponse(this.status, this.bytes, {this.retryAfter});
  final int status;
  final Uint8List bytes;
  final int? retryAfter;
  Map<String, dynamic> get json {
    if (bytes.isEmpty) return {};
    try {
      return jsonDecode(utf8.decode(bytes)) as Map<String, dynamic>;
    } catch (_) {
      throw const ApiException('invalid_response');
    }
  }
}

abstract interface class ApiTransport {
  Future<ApiResponse> send(
    String method,
    Uri uri, {
    Object? body,
    String? token,
    String? contentType,
  });
}

class IoApiTransport implements ApiTransport {
  @override
  Future<ApiResponse> send(
    String method,
    Uri uri, {
    Object? body,
    String? token,
    String? contentType,
  }) async {
    final client = HttpClient()..connectionTimeout = const Duration(seconds: 5);
    try {
      final request = await client
          .openUrl(method, uri)
          .timeout(const Duration(seconds: 10));
      request.followRedirects = false;
      request.headers.set(HttpHeaders.acceptHeader, 'application/json');
      if (token != null) {
        request.headers.set(HttpHeaders.authorizationHeader, 'Bearer $token');
      }
      if (body != null) {
        request.headers.set(
          HttpHeaders.contentTypeHeader,
          contentType ?? 'application/json',
        );
        if (body is Uint8List) {
          request.add(body);
        } else {
          request.add(utf8.encode(jsonEncode(body)));
        }
      }
      final response = await request.close().timeout(
        const Duration(seconds: 15),
      );
      final bytes = BytesBuilder(copy: false);
      await for (final chunk in response.timeout(const Duration(seconds: 15))) {
        if (bytes.length + chunk.length > 12 * 1024 * 1024) {
          throw const ApiException('invalid_response');
        }
        bytes.add(chunk);
      }
      return ApiResponse(
        response.statusCode,
        bytes.takeBytes(),
        retryAfter: int.tryParse(response.headers.value('retry-after') ?? ''),
      );
    } on ApiException {
      rethrow;
    } catch (_) {
      // Never retain or report raw socket/platform errors, request bodies or URLs.
      throw const ApiException('network_error');
    } finally {
      client.close(force: true);
    }
  }
}

class ApiClient {
  ApiClient(this.config, {ApiTransport? transport})
    : transport = transport ?? IoApiTransport();
  final AppConfig config;
  final ApiTransport transport;

  Future<ApiResponse> send(
    String method,
    String path, {
    Object? body,
    String? token,
    String? contentType,
  }) async {
    if (!path.startsWith('/api/v1/') || path.startsWith('//')) {
      throw const ApiException('invalid_request');
    }
    final result = await transport.send(
      method,
      config.apiBaseUrl.resolve(path),
      body: body,
      token: token,
      contentType: contentType,
    );
    if (result.status < 200 || result.status >= 300) {
      Map<String, dynamic> error = {};
      try {
        error = (result.json['error'] as Map?)?.cast<String, dynamic>() ?? {};
      } catch (_) {
        /* sanitized below */
      }
      throw ApiException(
        error['code'] is String ? error['code'] as String : 'request_error',
        status: result.status,
        retryAfter: result.retryAfter,
        fields: (error['fields'] as Map?)?.cast<String, dynamic>() ?? {},
      );
    }
    return result;
  }
}

abstract interface class AuthenticatedApi {
  Future<ApiResponse> request(
    String method,
    String path, {
    Object? body,
    String? contentType,
  });
}
