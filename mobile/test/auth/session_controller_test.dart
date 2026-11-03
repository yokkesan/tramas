import 'dart:async';
import 'dart:convert';
import 'package:flutter_test/flutter_test.dart';
import 'package:tramas/core/network/api_client.dart';
import 'package:tramas/features/auth/models/session.dart';
import 'package:tramas/features/auth/state/session_controller.dart';
import '../support/fakes.dart';

void main() {
  test(
    'refresh result not committed to secure storage can be recovered without a new operation',
    () async {
      final vault = MemoryVault(SessionRecord(pair(expired: true)))
        ..failAtWrite = 2;
      final ids = <String>[];
      final t = FakeTransport((r) {
        if (r.path.endsWith('/refresh')) {
          ids.add((r.body as Map)['request_id'] as String);
          return reply({'tokens': pair(suffix: 'committed').toJson()});
        }
        return reply({'user': user()});
      });
      final s = controller(t, vault);
      addTearDown(s.dispose);
      await s.restore();
      expect(s.phase, SessionPhase.offline);
      expect(vault.record!.pending, isNotNull);
      expect(vault.record!.tokens.refresh, 'refresh-a');
      vault.failAtWrite = 0;
      await s.restore();
      expect(s.phase, SessionPhase.ready);
      expect(ids.toSet().length, 1);
      expect(vault.record!.tokens.refresh, 'refresh-committed');
    },
  );
  test(
    'logout intent survives a failed in-flight refresh and restart',
    () async {
      final gate = Completer<void>();
      final vault = MemoryVault(SessionRecord(pair()));
      final t = FakeTransport((r) async {
        if (r.path.endsWith('/refresh')) {
          await gate.future;
          throw const ApiException('network_error');
        }
        return reply({'user': user()});
      });
      final s = controller(t, vault);
      addTearDown(s.dispose);
      await s.restore();
      final refreshing = s.refresh();
      final handled = expectLater(refreshing, throwsA(isA<ApiException>()));
      await Future<void>.delayed(Duration.zero);
      final loggingOut = s.logout();
      final logoutError = expectLater(loggingOut, throwsA(isA<ApiException>()));
      gate.complete();
      await handled;
      await logoutError;
      expect(s.phase, SessionPhase.logoutPending);
      expect(vault.record!.logoutPending, true);
      expect(vault.record!.pending, isNotNull);
      final resumed = controller(
        FakeTransport((_) => throw StateError('must remain blocked')),
        vault,
      );
      addTearDown(resumed.dispose);
      await resumed.restore();
      expect(resumed.phase, SessionPhase.logoutPending);
    },
  );
  test(
    'late protected responses are rejected after logout and never repopulate user state',
    () async {
      final wait = Completer<ApiResponse>();
      final vault = MemoryVault(SessionRecord(pair()));
      final t = FakeTransport((r) {
        if (r.path.endsWith('/logout')) return reply(null, 204);
        if (r.path == '/api/v1/players') return wait.future;
        return reply({'user': user()});
      });
      final s = controller(t, vault);
      addTearDown(s.dispose);
      await s.restore();
      final fetching = s.request('GET', '/api/v1/players');
      final rejected = expectLater(fetching, throwsA(isA<ApiException>()));
      await s.logout();
      wait.complete(reply({'players': []}));
      await rejected;
      expect(s.user, null);
      expect(s.phase, SessionPhase.signedOut);
    },
  );
  test(
    'login stores tokens and verifies server profile state; logout revokes before clearing',
    () async {
      final vault = MemoryVault();
      final transport = FakeTransport((r) {
        if (r.path.endsWith('/login')) {
          return reply({
            'tokens': pair().toJson(),
            'user': user(complete: false),
          });
        }
        if (r.path.endsWith('/me')) {
          return reply({'user': user(complete: false)});
        }
        if (r.path.endsWith('/logout')) {
          expect(vault.record?.logoutPending, true);
          expect((r.body as Map)['refresh_token'], 'refresh-a');
          return reply(null, 204);
        }
        throw StateError('unexpected route');
      });
      final s = controller(transport, vault);
      addTearDown(s.dispose);
      await s.restore();
      expect(s.phase, SessionPhase.signedOut);
      await s.login('test@example.com', 'long test password');
      expect(s.user!.profileCompleted, false);
      expect(vault.record!.tokens.refresh, 'refresh-a');
      expect(transport.requests.first.token, null);
      expect(transport.requests.last.token, 'access-a');
      await s.logout();
      expect(s.phase, SessionPhase.signedOut);
      expect(vault.record, null);
    },
  );

  test(
    'concurrent expired requests share one durable refresh; lost response retries same request ID',
    () async {
      final vault = MemoryVault(SessionRecord(pair(expired: true)));
      var calls = 0;
      final gate = Completer<void>();
      final ids = <String>[];
      final transport = FakeTransport((r) async {
        if (r.path.endsWith('/refresh')) {
          final id = (r.body as Map)['request_id'] as String;
          expect(vault.record?.pending?.requestId, id);
          ids.add(id);
          calls++;
          if (calls == 1) {
            await gate.future;
            throw const ApiException('network_error');
          }
          return reply({'tokens': pair(suffix: 'b').toJson()});
        }
        expect(r.token, 'access-b');
        return reply({'user': user()});
      });
      final s = controller(transport, vault);
      addTearDown(s.dispose);
      final restoring = s.restore();
      await Future<void>.delayed(Duration.zero);
      final requests = List.generate(
        4,
        (_) => s.request('GET', '/api/v1/auth/me'),
      );
      gate.complete();
      await restoring;
      await Future.wait(requests);
      expect(calls, 2);
      expect(ids.toSet().length, 1);
      expect(base64Url.decode('${ids.first}=').length, 32);
      expect(vault.record!.pending, null);
      expect(vault.record!.tokens.refresh, 'refresh-b');
    },
  );

  test(
    'restart recovers a pending refresh with the original operation ID',
    () async {
      final pending = PendingRefresh(
        'recover-request',
        testNow.subtract(const Duration(seconds: 10)),
      );
      final vault = MemoryVault(SessionRecord(pair(), pending: pending));
      final transport = FakeTransport((r) {
        if (r.path.endsWith('/refresh')) {
          expect((r.body as Map)['request_id'], 'recover-request');
          return reply({'tokens': pair(suffix: 'restored').toJson()});
        }
        return reply({'user': user()});
      });
      final s = controller(transport, vault);
      addTearDown(s.dispose);
      await s.restore();
      expect(s.phase, SessionPhase.ready);
      expect(vault.record!.pending, null);
    },
  );

  test(
    'pending refresh past recovery window requires login without replaying a new operation',
    () async {
      final vault = MemoryVault(
        SessionRecord(
          pair(),
          pending: PendingRefresh(
            'old-operation',
            testNow.subtract(const Duration(seconds: 56)),
          ),
        ),
      );
      final transport = FakeTransport((_) => throw StateError('must not send'));
      final s = controller(transport, vault);
      addTearDown(s.dispose);
      await s.restore();
      expect(s.phase, SessionPhase.signedOut);
      expect(s.problem!.code, 'refresh_recovery_expired');
      expect(vault.record, null);
    },
  );

  test(
    '401 recovery refreshes once and replays with replacement access token',
    () async {
      var first = true;
      final vault = MemoryVault(SessionRecord(pair()));
      final transport = FakeTransport((r) {
        if (r.path.endsWith('/refresh')) {
          return reply({'tokens': pair(suffix: 'new').toJson()});
        }
        if (first) {
          first = false;
          return failure(401, 'invalid_token');
        }
        expect(r.token, 'access-new');
        return reply({'user': user()});
      });
      final s = controller(transport, vault);
      addTearDown(s.dispose);
      await s.restore();
      expect(s.phase, SessionPhase.ready);
      expect(
        transport.requests.where((r) => r.path.endsWith('/refresh')).length,
        1,
      );
    },
  );

  test(
    'network failure preserves secure credentials and blocks restored normal screens',
    () async {
      final vault = MemoryVault(SessionRecord(pair()));
      final s = controller(
        FakeTransport((_) => throw const ApiException('network_error')),
        vault,
      );
      addTearDown(s.dispose);
      await s.restore();
      expect(s.phase, SessionPhase.offline);
      expect(vault.record, isNotNull);
    },
  );

  test(
    'invalid refresh expires session; 503 preserves durable recovery state',
    () async {
      for (final status in [401, 503]) {
        final vault = MemoryVault(SessionRecord(pair(expired: true)));
        final s = controller(
          FakeTransport(
            (_) => failure(
              status,
              status == 401 ? 'invalid_token' : 'internal_error',
            ),
          ),
          vault,
        );
        await s.restore();
        expect(
          s.phase,
          status == 401 ? SessionPhase.signedOut : SessionPhase.offline,
        );
        expect(vault.record != null, status == 503);
        s.dispose();
      }
    },
  );

  test(
    'refresh conflict adopts a newer saved pair without deleting it',
    () async {
      final vault = MemoryVault(SessionRecord(pair(expired: true)));
      final transport = FakeTransport((r) {
        if (r.path.endsWith('/refresh')) {
          vault.record = SessionRecord(pair(suffix: 'winner'));
          return failure(409, 'refresh_conflict');
        }
        expect(r.token, 'access-winner');
        return reply({'user': user()});
      });
      final s = controller(transport, vault);
      addTearDown(s.dispose);
      await s.restore();
      expect(s.phase, SessionPhase.ready);
      expect(vault.record!.tokens.access, 'access-winner');
    },
  );

  test(
    'failed logout is durable, blocks APIs, and resumes after process restart',
    () async {
      final vault = MemoryVault(SessionRecord(pair()));
      final transport = FakeTransport(
        (r) => r.path.endsWith('/logout')
            ? throw const ApiException('network_error')
            : reply({'user': user()}),
      );
      final s = controller(transport, vault);
      await s.restore();
      await expectLater(s.logout(), throwsA(isA<ApiException>()));
      expect(vault.record!.logoutPending, true);
      expect(s.phase, SessionPhase.logoutPending);
      await expectLater(
        s.request('GET', '/api/v1/auth/me'),
        throwsA(isA<ApiException>()),
      );
      s.dispose();
      final restored = controller(
        FakeTransport((r) {
          expect(r.path, '/api/v1/auth/logout');
          return reply(null, 204);
        }),
        vault,
      );
      addTearDown(restored.dispose);
      await restored.restore();
      expect(restored.phase, SessionPhase.logoutPending);
      await restored.logout();
      expect(vault.record, null);
      expect(restored.phase, SessionPhase.signedOut);
    },
  );

  test(
    'secure storage failure prevents login from entering a normal screen',
    () async {
      final vault = MemoryVault()..failWrite = true;
      final s = controller(
        FakeTransport((_) => reply({'tokens': pair().toJson()})),
        vault,
      );
      addTearDown(s.dispose);
      await s.restore();
      await expectLater(
        s.login('email', 'password'),
        throwsA(isA<ApiException>()),
      );
      expect(s.phase, SessionPhase.signedOut);
      expect(s.user, null);
    },
  );

  test(
    '403 profile_required returns to onboarding without replacing credentials',
    () async {
      var incomplete = false;
      final vault = MemoryVault(SessionRecord(pair()));
      final transport = FakeTransport(
        (r) => incomplete
            ? failure(403, 'profile_required')
            : reply({'user': user()}),
      );
      final s = controller(transport, vault);
      addTearDown(s.dispose);
      await s.restore();
      incomplete = true;
      await expectLater(
        s.request('GET', '/api/v1/players'),
        throwsA(isA<ApiException>()),
      );
      expect(s.user!.profileCompleted, false);
      expect(vault.record!.tokens.refresh, 'refresh-a');
    },
  );
}
