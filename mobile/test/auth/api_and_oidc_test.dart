import 'dart:convert';
import 'dart:async';
import 'dart:io';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tramas/core/network/api_client.dart';
import 'package:tramas/core/network/error_messages.dart';
import 'package:tramas/features/auth/data/session_vault.dart';
import 'package:tramas/features/auth/models/session.dart';
import 'package:tramas/features/auth/services/oidc_service.dart';
import 'package:tramas/features/auth/services/register_service.dart';
import '../support/fakes.dart';

class _RealHttpOverrides extends HttpOverrides {}

class _DelayedSdk implements IdentitySdk {
  final result = Completer<String>();
  @override
  bool available(String provider) => true;
  @override
  Future<String> identityToken(String provider, String nonce) => result.future;
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'late OIDC link credentials cannot mutate an account that logged in afterward',
    () async {
      final sdk = _DelayedSdk();
      var newLogin = false;
      final t = FakeTransport((r) {
        if (r.path.endsWith('/auth/me')) {
          return reply({
            'user': {
              ...user(),
              'id': newLogin ? '22222222-2222-4222-8222-222222222222' : userId,
            },
          });
        }
        if (r.path.endsWith('/challenge')) {
          return reply({
            'challenge_token': 'old-bound-challenge',
            'nonce': 'old-user-nonce',
          });
        }
        if (r.path.endsWith('/logout')) return reply(null, 204);
        if (r.path.endsWith('/login')) {
          newLogin = true;
          return reply({'tokens': pair(suffix: 'other-user').toJson()});
        }
        throw StateError('stale link must never be sent');
      });
      final s = controller(t, MemoryVault(SessionRecord(pair())));
      addTearDown(s.dispose);
      await s.restore();
      final linking = OidcService(s, sdk).link('apple', 'reauth-proof');
      final rejected = expectLater(linking, throwsA(isA<ApiException>()));
      await Future<void>.delayed(Duration.zero);
      await s.logout();
      await s.login('other@example.com', 'other password');
      sdk.result.complete('old-user-provider-token');
      await rejected;
      expect(t.requests.where((r) => r.path.endsWith('/link')), isEmpty);
      expect(s.user!.id, '22222222-2222-4222-8222-222222222222');
    },
  );
  test(
    'registration requires HTTP 201 and preserves legacy duplicate status handling',
    () async {
      final unexpected = RegisterService(
        api: ApiClient(
          testConfig,
          transport: FakeTransport((_) => reply({}, 200)),
        ),
      );
      await expectLater(
        unexpected.registerDetailed(
          email: 'test@example.com',
          password: 'test password',
        ),
        throwsA(
          isA<ApiException>().having((e) => e.code, 'code', 'invalid_response'),
        ),
      );
      final duplicate = RegisterService(
        api: ApiClient(
          testConfig,
          transport: FakeTransport(
            (_) => failure(409, 'email_already_registered'),
          ),
        ),
      );
      expect(
        await duplicate.register(
          apiBaseUrl: testConfig.apiBaseUrl.toString(),
          email: 'test@example.com',
          password: 'test password',
        ),
        409,
      );
    },
  );
  test(
    'registration, verification, resend and password reset use public JSON routes',
    () async {
      final transport = FakeTransport(
        (r) => reply(null, r.path.endsWith('/register') ? 201 : 204),
      );
      final s = controller(transport, MemoryVault());
      addTearDown(s.dispose);
      await s.repository.register('name@example.com', 'long password');
      await s.repository.verify('verify-token');
      await s.repository.resend('name@example.com');
      await s.repository.requestReset('name@example.com');
      await s.repository.completeReset('reset-token', 'new password');
      expect(transport.requests.map((r) => r.path), [
        '/api/v1/auth/register',
        '/api/v1/auth/email/verify',
        '/api/v1/auth/email/resend',
        '/api/v1/auth/password/reset/request',
        '/api/v1/auth/password/reset/complete',
      ]);
      expect(
        transport.requests.every((r) => r.token == null && r.method == 'POST'),
        true,
      );
    },
  );
  test(
    'API error envelope and retry-after map to safe localized errors',
    () async {
      for (final status in [401, 403, 409, 422, 429, 503]) {
        final api = ApiClient(
          testConfig,
          transport: FakeTransport(
            (_) => failure(status, 'rate_limited', retryAfter: 20),
          ),
        );
        try {
          await api.send('POST', '/api/v1/auth/login');
          fail('must throw');
        } on ApiException catch (e) {
          expect(e.status, status);
          expect(e.retryAfter, 20);
          expect(errorMessage(e), contains('20'));
          expect(errorMessage(e), isNot(contains('server message')));
        }
      }
    },
  );
  test(
    'secure vault stores one whole operation and isolates API origins',
    () async {
      FlutterSecureStorage.setMockInitialValues({});
      final a = SecureSessionVault(Uri.parse('https://one.example'));
      final b = SecureSessionVault(Uri.parse('https://two.example'));
      final record = SessionRecord(
        pair(),
        pending: PendingRefresh('same-operation', testNow),
      );
      await a.write(record);
      expect((await a.read())!.pending!.requestId, 'same-operation');
      expect(await b.read(), null);
      await a.clear();
      expect(await a.read(), null);
    },
  );
  test(
    'OIDC login passes exact fresh server nonce, never fakes unconfigured success',
    () async {
      final sdk = FakeIdentitySdk();
      var challenges = 0;
      final transport = FakeTransport((r) {
        if (r.path.endsWith('/challenge')) {
          return reply({
            'nonce': 'nonce-${++challenges}',
            'challenge_token': 'challenge-$challenges',
          });
        }
        if (r.path.endsWith('/login')) {
          expect((r.body as Map)['id_token'], 'test-id-token');
          return reply({'tokens': pair().toJson()});
        }
        return reply({'user': user(complete: false)});
      });
      final s = controller(transport, MemoryVault());
      addTearDown(s.dispose);
      await s.restore();
      final oidc = OidcService(s, sdk);
      await expectLater(oidc.login('apple'), throwsA(isA<ApiException>()));
      expect(s.user, null);
      expect(transport.requests, isEmpty);
      sdk.configured = true;
      await oidc.login('apple');
      await oidc.login('google');
      expect(sdk.nonces, ['nonce-1', 'nonce-2']);
      expect(s.user!.profileCompleted, false);
    },
  );
  test(
    'OIDC explicit link requires password or provider proof and sends bearer',
    () async {
      final sdk = FakeIdentitySdk()..configured = true;
      final t = FakeTransport((r) {
        if (r.path.endsWith('/me')) return reply({'user': user()});
        if (r.path.endsWith('/reauthenticate')) {
          return reply({'reauth_token': 'password-proof'});
        }
        if (r.path.endsWith('/challenge')) {
          expect(r.token, 'access-a');
          return reply({
            'nonce': 'bound-nonce',
            'challenge_token': 'bound-challenge',
          });
        }
        if (r.path.endsWith('/reauth')) {
          return reply({'reauth_token': 'provider-proof'});
        }
        if (r.path.endsWith('/link')) {
          expect(r.token, 'access-a');
          return reply(null, 204);
        }
        throw StateError('unexpected route');
      });
      final s = controller(t, MemoryVault(SessionRecord(pair())));
      addTearDown(s.dispose);
      await s.restore();
      final oidc = OidcService(s, sdk);
      final passwordProof = await oidc.passwordProof('password');
      await oidc.link('apple', passwordProof);
      final providerProof = await oidc.providerProof('apple');
      await oidc.link('google', providerProof);
      expect(
        (t.requests.firstWhere((r) => r.path.endsWith('/challenge')).body
            as Map)['reauth_token'],
        'password-proof',
      );
    },
  );
  test(
    'real HTTP transport sends JSON/Bearer and refuses redirects',
    () => HttpOverrides.runZoned(() async {
      final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      addTearDown(() => server.close(force: true));
      final handled = server.listen((request) async {
        if (request.uri.path == '/redirect') {
          request.response.statusCode = 302;
          request.response.headers.set('location', '/leak');
        } else {
          expect(request.headers.value('authorization'), 'Bearer secret-test');
          expect(jsonDecode(await utf8.decoder.bind(request).join()), {
            'email': 'test@example.com',
          });
          request.response.headers.contentType = ContentType.json;
          request.response.write('{"ok":true}');
        }
        await request.response.close();
      });
      addTearDown(handled.cancel);
      final transport = IoApiTransport();
      final result = await transport.send(
        'POST',
        Uri.parse('http://127.0.0.1:${server.port}/json'),
        body: {'email': 'test@example.com'},
        token: 'secret-test',
      );
      expect(result.json['ok'], true);
      expect(
        (await transport.send(
          'GET',
          Uri.parse('http://127.0.0.1:${server.port}/redirect'),
          token: 'secret-test',
        )).status,
        302,
      );
    }, createHttpClient: _RealHttpOverrides().createHttpClient),
  );
}
