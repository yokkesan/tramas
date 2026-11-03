import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';
import 'package:tramas/config/app_config.dart';
import 'package:tramas/core/network/api_client.dart';
import 'package:tramas/features/auth/data/session_vault.dart';
import 'package:tramas/features/auth/models/session.dart';
import 'package:tramas/features/auth/services/oidc_service.dart';
import 'package:tramas/features/auth/state/session_controller.dart';

final testNow = DateTime.utc(2026, 11, 2, 3);
final testConfig = AppConfig.fromValues(
  environment: 'development',
  baseUrl: 'http://127.0.0.1:8080',
);
const userId = '11111111-1111-4111-8111-111111111111';
TokenPair pair({String suffix = 'a', bool expired = false}) => TokenPair(
  'access-$suffix',
  'refresh-$suffix',
  testNow.add(Duration(minutes: expired ? -1 : 15)),
  testNow.add(const Duration(days: 30)),
);
Map<String, dynamic> user({bool complete = true}) => {
  'id': userId,
  'email': 'test@example.com',
  'nickname': complete ? '虎ファン' : null,
  'email_verified': true,
  'profile_completed': complete,
};
ApiResponse reply([Object? body, int status = 200]) => ApiResponse(
  status,
  Uint8List.fromList(body == null ? [] : utf8.encode(jsonEncode(body))),
);
ApiResponse failure(int status, String code, {int? retryAfter}) => ApiResponse(
  status,
  Uint8List.fromList(
    utf8.encode(
      jsonEncode({
        'error': {'code': code, 'message': 'server message is not displayed'},
      }),
    ),
  ),
  retryAfter: retryAfter,
);

class SentRequest {
  const SentRequest(
    this.method,
    this.uri,
    this.body,
    this.token,
    this.contentType,
  );
  final String method;
  final Uri uri;
  final Object? body;
  final String? token, contentType;
  String get path => uri.path;
}

class FakeTransport implements ApiTransport {
  FakeTransport(this.handler);
  final FutureOr<ApiResponse> Function(SentRequest) handler;
  final List<SentRequest> requests = [];
  @override
  Future<ApiResponse> send(
    String method,
    Uri uri, {
    Object? body,
    String? token,
    String? contentType,
  }) async {
    final r = SentRequest(method, uri, body, token, contentType);
    requests.add(r);
    return await handler(r);
  }
}

class MemoryVault implements SessionVault {
  MemoryVault([this.record]);
  SessionRecord? record;
  bool failWrite = false;
  int failAtWrite = 0;
  final List<SessionRecord> writes = [];
  @override
  Future<SessionRecord?> read() async => record;
  @override
  Future<void> write(SessionRecord r) async {
    if (failWrite || writes.length + 1 == failAtWrite) {
      throw const ApiException('secure_storage_error');
    }
    writes.add(r);
    record = r;
  }

  @override
  Future<void> clear() async {
    record = null;
  }
}

class FakeIdentitySdk implements IdentitySdk {
  bool configured = false;
  final List<String> nonces = [];
  @override
  bool available(String provider) => configured;
  @override
  Future<String> identityToken(String provider, String nonce) async {
    nonces.add(nonce);
    return 'test-id-token';
  }
}

SessionController controller(
  FakeTransport transport,
  MemoryVault vault, {
  DateTime Function()? now,
}) => SessionController(
  api: ApiClient(testConfig, transport: transport),
  vault: vault,
  now: now ?? () => testNow,
  sleep: (_) async {},
);

class FakeGateway implements AuthenticatedApi {
  FakeGateway(this.handler);
  final FutureOr<ApiResponse> Function(SentRequest) handler;
  final List<SentRequest> requests = [];
  @override
  Future<ApiResponse> request(
    String method,
    String path, {
    Object? body,
    String? contentType,
  }) async {
    final r = SentRequest(
      method,
      testConfig.apiBaseUrl.resolve(path),
      body,
      'test-access',
      contentType,
    );
    requests.add(r);
    return await handler(r);
  }
}
