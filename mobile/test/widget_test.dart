import 'package:flutter_test/flutter_test.dart';
import 'package:tramas/config/app_config.dart';
import 'package:tramas/main.dart';
import 'support/fakes.dart';

void main() {
  testWidgets('初期画面を表示する', (tester) async {
    final session = controller(
      FakeTransport(
        (_) => throw StateError('signed-out startup must not call API'),
      ),
      MemoryVault(),
    );
    addTearDown(session.dispose);
    await tester.pumpWidget(
      TramasApp(
        session: session,
        identitySdk: FakeIdentitySdk(),
        config: AppConfig.fromValues(
          environment: 'development',
          baseUrl: 'http://127.0.0.1:8080',
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('tramas.'), findsOneWidget);
    expect(find.text('ログイン'), findsNWidgets(3));
    expect(find.text('新規登録へ'), findsOneWidget);
    expect(find.textContaining('設定待ち'), findsOneWidget);
  });

  test('release requires a production HTTPS URL', () {
    expect(
      () => AppConfig.fromValues(
        environment: 'production',
        baseUrl: 'http://localhost',
        release: true,
      ),
      throwsArgumentError,
    );
    expect(
      () => AppConfig.fromValues(
        environment: 'development',
        baseUrl: 'https://localhost',
        release: true,
      ),
      throwsArgumentError,
    );
    expect(
      AppConfig.fromValues(
        environment: 'production',
        baseUrl: 'https://localhost',
        release: true,
      ).apiBaseUrl.scheme,
      'https',
    );
  });
}
