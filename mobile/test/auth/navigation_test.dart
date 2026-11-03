import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tramas/core/network/api_client.dart';
import 'package:tramas/features/auth/models/session.dart';
import 'package:tramas/features/auth/state/session_controller.dart';
import 'package:tramas/main.dart';
import '../support/fakes.dart';

Future<void> mount(WidgetTester tester, SessionController session) async {
  await tester.pumpWidget(
    TramasApp(
      key: UniqueKey(),
      config: testConfig,
      session: session,
      identitySdk: FakeIdentitySdk(),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('login uses preserved form and navigates only after server me', (
    tester,
  ) async {
    final t = FakeTransport(
      (r) => r.path.endsWith('/login')
          ? reply({'tokens': pair().toJson()})
          : reply({'user': user()}),
    );
    final s = controller(t, MemoryVault());
    addTearDown(s.dispose);
    await mount(tester, s);
    expect(find.text('tramas.'), findsOneWidget);
    await tester.enterText(find.byType(TextField).at(0), 'test@example.com');
    await tester.enterText(find.byType(TextField).at(1), 'long password');
    await tester.tap(find.widgetWithText(ElevatedButton, 'ログイン'));
    await tester.pumpAndSettle();
    expect(find.text('今日も、虎とともに。'), findsOneWidget);
    expect(find.text('初回プロフィール設定'), findsNothing);
    expect(tester.takeException(), null);
  });
  testWidgets(
    'incomplete profile cannot access normal UI; me is authoritative after save and restart',
    (tester) async {
      var completed = false;
      final t = FakeTransport(
        (r) => r.method == 'PATCH'
            ? reply({'user': user(complete: true)})
            : reply({'user': user(complete: completed)}),
      );
      final vault = MemoryVault(SessionRecord(pair()));
      final s = controller(t, vault);
      addTearDown(s.dispose);
      await mount(tester, s);
      expect(find.text('初回プロフィール設定'), findsOneWidget);
      expect(find.text('ホーム'), findsNothing);
      await tester.tap(find.widgetWithText(ElevatedButton, '設定して始める'));
      await tester.pump();
      expect(find.text('ニックネームは2〜20文字で入力してください。'), findsOneWidget);
      await tester.enterText(find.byType(TextField), '😀😀');
      await tester.tap(find.widgetWithText(ElevatedButton, '設定して始める'));
      await tester.pumpAndSettle();
      expect(find.text('初回プロフィール設定'), findsOneWidget);
      completed = true;
      await tester.tap(find.widgetWithText(ElevatedButton, '設定して始める'));
      await tester.pumpAndSettle();
      expect(find.text('ホーム'), findsOneWidget);
      final restored = controller(t, vault);
      addTearDown(restored.dispose);
      await mount(tester, restored);
      expect(find.text('初回プロフィール設定'), findsNothing);
      expect(find.text('ホーム'), findsOneWidget);
    },
  );
  testWidgets(
    'registration is not auto-login, prevents duplicate submit, and shows verification',
    (tester) async {
      final result = Completer<ApiResponse>();
      final t = FakeTransport((r) => result.future);
      final s = controller(t, MemoryVault());
      addTearDown(s.dispose);
      await mount(tester, s);
      await tester.ensureVisible(find.text('新規登録へ'));
      await tester.tap(find.text('新規登録へ'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextField).at(0), 'new@example.com');
      await tester.enterText(
        find.byType(TextField).at(1),
        'a long unique password',
      );
      await tester.enterText(
        find.byType(TextField).at(2),
        'a long unique password',
      );
      final button = find.widgetWithText(ElevatedButton, '登録');
      await tester.tap(button);
      await tester.tap(button);
      await tester.pump();
      expect(t.requests.length, 1);
      expect(t.requests.single.path, '/api/v1/auth/register');
      expect(t.requests.single.token, null);
      result.complete(
        reply({
          'user': {'id': userId},
        }, 201),
      );
      await tester.pumpAndSettle();
      expect(find.text('メール本人確認'), findsOneWidget);
      expect(s.phase, SessionPhase.signedOut);
    },
  );
  testWidgets(
    'unverified login offers confirmation and resend without logging in',
    (tester) async {
      final t = FakeTransport((r) {
        if (r.path.endsWith('/login')) {
          return failure(403, 'email_verification_required');
        }
        return reply(null, r.path.endsWith('/resend') ? 202 : 204);
      });
      final s = controller(t, MemoryVault());
      addTearDown(s.dispose);
      await mount(tester, s);
      await tester.enterText(find.byType(TextField).at(0), 'test@example.com');
      await tester.enterText(find.byType(TextField).at(1), 'password');
      await tester.tap(find.widgetWithText(ElevatedButton, 'ログイン'));
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.text('メール本人確認へ'));
      await tester.tap(find.text('メール本人確認へ'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextField).at(1), 'verify-token');
      await tester.tap(find.widgetWithText(ElevatedButton, '本人確認を完了'));
      await tester.pumpAndSettle();
      expect(find.text('本人確認が完了しました。ログインしてください。'), findsOneWidget);
      expect(s.user, null);
    },
  );
  testWidgets(
    'network restore failure offers retry and does not expose normal screens',
    (tester) async {
      final s = controller(
        FakeTransport((_) => throw const ApiException('network_error')),
        MemoryVault(SessionRecord(pair())),
      );
      addTearDown(s.dispose);
      await mount(tester, s);
      expect(find.text('接続の確認'), findsOneWidget);
      expect(find.text('再試行'), findsOneWidget);
      expect(find.text('マイページ'), findsNothing);
    },
  );
  testWidgets(
    'empty player source displays empty state without fictional players',
    (tester) async {
      final t = FakeTransport((r) {
        if (r.path.endsWith('/me')) return reply({'user': user()});
        if (r.path == '/api/v1/players') return reply({'players': []});
        return reply({'favorites': []});
      });
      final s = controller(t, MemoryVault(SessionRecord(pair())));
      addTearDown(s.dispose);
      await mount(tester, s);
      await tester.tap(find.byIcon(Icons.sports_baseball_outlined));
      await tester.pumpAndSettle();
      expect(find.text('現在、登録できる選手データはありません。データの公開をお待ちください。'), findsOneWidget);
      expect(find.text('テスト選手'), findsNothing);
    },
  );
}
