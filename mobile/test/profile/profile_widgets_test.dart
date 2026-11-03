import 'dart:convert';
import 'dart:typed_data';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tramas/core/network/api_client.dart';
import 'package:tramas/features/auth/models/session.dart';
import 'package:tramas/features/profile/services/image_selection.dart';
import 'package:tramas/main.dart';
import '../support/fakes.dart';
import 'profile_and_favorites_test.dart' show profileJson, playerJson;

class _ImagePicker implements ImageSelection {
  Object? error;
  @override
  Future<SelectedImage?> pick() async {
    if (error != null) throw error!;
    return SelectedImage.validate(
      base64Decode(
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
      ),
    );
  }
}

void main() {
  testWidgets(
    'profile editor saves text, uploads original bytes, deletes, and handles empty catalog',
    (tester) async {
      var nickname = '虎ファン', biography = '';
      Map<String, dynamic>? avatar;
      final picker = _ImagePicker();
      final t = FakeTransport((r) {
        if (r.path.endsWith('/auth/me')) return reply({'user': user()});
        if (r.path == '/api/v1/profile/standard-images') {
          return reply({'images': []});
        }
        if (r.path.startsWith('/api/v1/profile/images/')) {
          return ApiResponse(
            200,
            base64Decode(
              'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
            ),
          );
        }
        if (r.path.endsWith('/favorites')) return reply({'favorites': []});
        if (r.path.endsWith('/images/avatar')) {
          if (r.method == 'PUT') {
            expect(r.body, isA<Uint8List>());
            expect(r.contentType, 'image/png');
            avatar = {
              'id': '33333333-3333-4333-8333-333333333333',
              'content_type': 'image/png',
              'size': 68,
            };
          } else {
            avatar = null;
          }
          return reply(null, r.method == 'DELETE' ? 204 : 200);
        }
        if (r.method == 'PUT') {
          nickname = (r.body as Map)['nickname'] as String;
          biography = (r.body as Map)['biography'] as String;
        }
        return reply({
          'profile': {
            ...profileJson(nickname: nickname, biography: biography),
            'avatar': avatar,
          },
        });
      });
      final s = controller(t, MemoryVault(SessionRecord(pair())));
      addTearDown(s.dispose);
      await tester.pumpWidget(
        TramasApp(
          config: testConfig,
          session: s,
          identitySdk: FakeIdentitySdk(),
          imageSelection: picker,
        ),
      );
      await tester.pumpAndSettle();
      await tester.tap(find.byIcon(Icons.person_outline));
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.text('プロフィール編集'));
      await tester.tap(find.text('プロフィール編集'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextField).at(0), '編集後');
      await tester.enterText(find.byType(TextField).at(1), '自己紹介の内容');
      await tester.tap(find.text('プロフィールを保存'));
      await tester.pumpAndSettle();
      expect(nickname, '編集後');
      expect(biography, '自己紹介の内容');
      final choose = find.widgetWithText(TextButton, '画像を選択').first;
      await tester.ensureVisible(choose);
      await tester.tap(choose);
      await tester.pumpAndSettle();
      expect(avatar, isNotNull);
      final remove = find.widgetWithText(TextButton, '画像を削除').first;
      await tester.ensureVisible(remove);
      await tester.tap(remove);
      await tester.pumpAndSettle();
      expect(avatar, null);
      final standard = find.widgetWithText(TextButton, '標準画像').first;
      await tester.ensureVisible(standard);
      await tester.tap(standard);
      await tester.pumpAndSettle();
      expect(find.text('標準画像はまだ登録されていません。'), findsOneWidget);
      await tester.tap(find.text('閉じる'));
      await tester.pumpAndSettle();
      picker.error = const ApiException('image_permission_denied');
      await tester.ensureVisible(choose);
      await tester.tap(choose);
      await tester.pumpAndSettle();
      expect(find.textContaining('画像へのアクセスが許可されていません'), findsOneWidget);
      expect(tester.takeException(), null);
    },
  );
  testWidgets(
    'history loads owner-only cursor pages and preserves past player entries',
    (tester) async {
      final t = FakeTransport((r) {
        if (r.path.endsWith('/auth/me')) return reply({'user': user()});
        if (r.path == '/api/v1/players') return reply({'players': []});
        if (r.path.endsWith('/history')) {
          final page = r.uri.queryParameters['cursor'];
          return reply({
            'history': [
              {
                'id': page == null ? 'h1' : 'h2',
                'player': playerJson(page == null ? 1 : 2, status: 'retired'),
                'registered_at': '2026-01-01',
                'ended_at': '2026-02-01',
                'end_reason': 'ineligible',
              },
            ],
            'next_cursor': page == null ? 'cursor-1' : '',
          });
        }
        return reply({'favorites': []});
      });
      final s = controller(t, MemoryVault(SessionRecord(pair())));
      addTearDown(s.dispose);
      await tester.pumpWidget(
        TramasApp(
          config: testConfig,
          session: s,
          identitySdk: FakeIdentitySdk(),
        ),
      );
      await tester.pumpAndSettle();
      await tester.tap(find.byIcon(Icons.sports_baseball_outlined));
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.text('過去の推し履歴'));
      await tester.tap(find.text('過去の推し履歴'));
      await tester.pumpAndSettle();
      expect(find.text('テスト選手1'), findsOneWidget);
      await tester.ensureVisible(find.text('続きを読み込む'));
      await tester.tap(find.text('続きを読み込む'));
      await tester.pumpAndSettle();
      expect(find.text('テスト選手2'), findsOneWidget);
      expect(find.text('続きを読み込む'), findsNothing);
      expect(t.requests.last.uri.queryParameters['cursor'], 'cursor-1');
      expect(t.requests.last.path, '/api/v1/users/me/favorites/history');
    },
  );
  testWidgets(
    'reset request and token confirmation preserve non-disclosing response',
    (tester) async {
      final t = FakeTransport(
        (r) => reply(null, r.path.endsWith('/request') ? 202 : 204),
      );
      final s = controller(t, MemoryVault());
      addTearDown(s.dispose);
      await tester.pumpWidget(
        TramasApp(
          config: testConfig,
          session: s,
          identitySdk: FakeIdentitySdk(),
        ),
      );
      await tester.pumpAndSettle();
      await tester.ensureVisible(find.text('パスワードをお忘れの方'));
      await tester.tap(find.text('パスワードをお忘れの方'));
      await tester.pumpAndSettle();
      await tester.enterText(
        find.byType(TextField).at(0),
        'unknown@example.com',
      );
      await tester.tap(find.widgetWithText(ElevatedButton, '再設定メールを要求'));
      await tester.pumpAndSettle();
      expect(find.textContaining('送信要求を受け付けました'), findsOneWidget);
      await tester.enterText(find.byType(TextField).at(1), 'one-use-reset');
      await tester.enterText(
        find.byType(TextField).at(2),
        'new secure password',
      );
      await tester.enterText(
        find.byType(TextField).at(3),
        'new secure password',
      );
      await tester.ensureVisible(
        find.widgetWithText(ElevatedButton, 'パスワードを変更'),
      );
      await tester.tap(find.widgetWithText(ElevatedButton, 'パスワードを変更'));
      await tester.pumpAndSettle();
      expect(find.textContaining('パスワードを変更しました'), findsOneWidget);
      expect(t.requests.last.path, '/api/v1/auth/password/reset/complete');
      expect(s.user, null);
    },
  );
}
