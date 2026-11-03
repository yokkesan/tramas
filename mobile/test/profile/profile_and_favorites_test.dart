import 'dart:convert';
import 'dart:typed_data';
import 'package:flutter_test/flutter_test.dart';
import 'package:tramas/core/network/api_client.dart';
import 'package:tramas/features/profile/data/profile_repository.dart';
import 'package:tramas/features/profile/models/profile.dart';
import 'package:tramas/features/profile/services/image_selection.dart';
import 'package:tramas/features/favorites/data/favorites_repository.dart';
import 'package:tramas/features/favorites/state/favorites_controller.dart';
import '../support/fakes.dart';

Map<String, dynamic> playerJson(
  int i, {
  String status = 'active',
  String? team = 'hanshin_tigers',
}) => {'id': 'player-$i', 'name': 'テスト選手$i', 'team': team, 'status': status};
Map<String, dynamic> favoriteJson(int i, {bool main = false, String? ended}) =>
    {
      'id': 'favorite-$i',
      'player': playerJson(i),
      'main': main,
      'registered_at': '2026-11-02T00:00:00Z',
      'ended_at': ?ended,
    };
Map<String, dynamic> profileJson({
  String id = userId,
  String nickname = '虎ファン',
  String biography = '',
}) => {
  'id': id,
  'nickname': nickname,
  'biography': biography,
  'avatar': null,
  'header': null,
  'avatar_standard_id': null,
  'header_standard_id': null,
};

void main() {
  test('nickname matches Go Unicode scalar and ASCII-space policy', () {
    final cases = <String, bool>{
      'ab': true,
      ' 😀😀 ': true,
      'e\u0301': true,
      'é': false,
      '😀': false,
      'a': false,
      '　　': false,
      '\t虎': false,
      'a\ufffd': false,
      '虎' * 20: true,
      '虎' * 21: false,
      '\u2003\u2003': false,
    };
    for (final e in cases.entries) {
      expect(validNickname(e.key), e.value, reason: 'Unicode nickname policy');
    }
    expect(normalizeNickname(' 虎太郎 '), '虎太郎');
    expect(normalizeNickname('　虎　'), '　虎　');
  });
  test(
    'optional biography is sent intact and public profile uses only public route',
    () async {
      final gateway = FakeGateway(
        (r) => reply({'profile': profileJson(biography: '自己紹介')}),
      );
      final repo = ProfileRepository(gateway);
      final bio = '自己紹介' * 3000;
      await repo.update(nickname: ' 虎ファン ', biography: bio);
      expect((gateway.requests.first.body as Map)['biography'], bio);
      expect((gateway.requests.first.body as Map)['nickname'], '虎ファン');
      await repo.get(userId: '22222222-2222-4222-8222-222222222222');
      expect(
        gateway.requests.last.path,
        '/api/v1/users/22222222-2222-4222-8222-222222222222/profile',
      );
      await expectLater(
        repo.update(nickname: 'a', biography: ''),
        throwsA(isA<ApiException>()),
      );
      expect(gateway.requests.length, 2);
      await expectLater(
        repo.get(userId: '../me'),
        throwsA(isA<ApiException>()),
      );
      expect(gateway.requests.length, 2);
    },
  );
  test(
    'image signature detection accepts JPEG PNG HEIC and rejects other types or over 10MiB',
    () {
      final png = Uint8List.fromList([137, 80, 78, 71, 13, 10, 26, 10, 1]);
      expect(SelectedImage.validate(png).contentType, 'image/png');
      expect(
        SelectedImage.validate(
          Uint8List.fromList([255, 216, 255, 1]),
        ).contentType,
        'image/jpeg',
      );
      final heic = Uint8List(24);
      ByteData.sublistView(heic).setUint32(0, 24);
      heic.setRange(4, 8, ascii.encode('ftyp'));
      heic.setRange(8, 12, ascii.encode('heic'));
      expect(SelectedImage.validate(heic).contentType, 'image/heic');
      for (final bad in [
        Uint8List(0),
        Uint8List.fromList(utf8.encode('<svg/>')),
        Uint8List.fromList(ascii.encode('GIF89a')),
      ]) {
        expect(() => SelectedImage.validate(bad), throwsA(isA<ApiException>()));
      }
      expect(
        () => SelectedImage.validate(Uint8List(10 * 1024 * 1024 + 1)),
        throwsA(isA<ApiException>().having((e) => e.status, 'status', 413)),
      );
    },
  );
  test(
    'image upload is raw bytes with MIME, deletes and standard selection cannot specify owner',
    () async {
      final gateway = FakeGateway((r) => reply(null, 204));
      final repo = ProfileRepository(gateway);
      final bytes = Uint8List.fromList([137, 80, 78, 71, 13, 10, 26, 10]);
      await repo.upload('avatar', bytes, 'image/png');
      expect(identical(gateway.requests.first.body, bytes), true);
      expect(gateway.requests.first.contentType, 'image/png');
      await repo.remove('header');
      await repo.selectStandard('avatar', 'catalog-item');
      expect(gateway.requests.map((r) => r.method), ['PUT', 'DELETE', 'PUT']);
      expect(
        gateway.requests.every(
          (r) => r.path.startsWith('/api/v1/users/me/profile/images/'),
        ),
        true,
      );
      await expectLater(
        repo.upload('../other', bytes, 'image/png'),
        throwsA(isA<ApiException>()),
      );
      await expectLater(
        repo.upload('avatar', Uint8List(10 * 1024 * 1024 + 1), 'image/png'),
        throwsA(isA<ApiException>()),
      );
      expect(gateway.requests.length, 3);
    },
  );
  test(
    'eligible list excludes transferred retired and other-team players; empty source stays empty',
    () async {
      final gateway = FakeGateway(
        (_) => reply({
          'players': [
            playerJson(1),
            playerJson(2, status: 'retired'),
            playerJson(3, team: 'other'),
            playerJson(4, status: 'transferred'),
            playerJson(5, team: null),
          ],
        }),
      );
      final players = await FavoritesRepository(gateway).players();
      expect(players.map((p) => p.id), ['player-1']);
      expect(
        await FavoritesRepository(
          FakeGateway((_) => reply({'players': []})),
        ).players(),
        isEmpty,
      );
    },
  );
  test(
    'favorites enforce 3 active choices and send only one main ID without optimistic duplicates',
    () async {
      final selected = <int>[];
      int? main;
      final gateway = FakeGateway((r) {
        if (r.path == '/api/v1/players') {
          return reply({'players': List.generate(4, (i) => playerJson(i))});
        }
        if (r.method == 'GET') {
          return reply({
            'favorites': selected
                .map((i) => favoriteJson(i, main: i == main))
                .toList(),
          });
        }
        if (r.method == 'POST') {
          selected.add(
            int.parse((r.body as Map)['player_id'].toString().split('-').last),
          );
          return reply(null, 204);
        }
        if (r.path.endsWith('/main')) {
          main = r.method == 'DELETE'
              ? null
              : int.parse(r.path.split('/')[6].split('-').last);
          return reply(null, 204);
        }
        selected.remove(int.parse(r.path.split('/').last.split('-').last));
        return reply(null, 204);
      });
      final c = FavoritesController(FavoritesRepository(gateway));
      addTearDown(c.dispose);
      await c.load();
      for (var i = 0; i < 3; i++) {
        await c.toggle(c.players[i]);
      }
      expect(c.favorites.length, 3);
      final before = gateway.requests.length;
      await c.toggle(c.players[3]);
      expect(gateway.requests.length, before);
      expect((c.error as ApiException).code, 'favorite_limit');
      await c.setMain('player-0');
      await c.setMain('player-1');
      expect(c.favorites.where((f) => f.main).length, 1);
      expect(c.favorites.singleWhere((f) => f.main).player.id, 'player-1');
      await c.setMain(null);
      expect(c.favorites.where((f) => f.main), isEmpty);
      await c.toggle(c.players[0]);
      await c.toggle(c.players[3]);
      expect(c.favorites.length, 3);
    },
  );
  test(
    'history pagination preserves cursor and always uses owner-only me route',
    () async {
      final gateway = FakeGateway(
        (r) => reply({
          'history': [
            {'id': 'entry'},
          ],
          'next_cursor': 'next-cursor',
        }),
      );
      final repo = FavoritesRepository(gateway);
      final page = await repo.history(cursor: 'opaque+/= ?&user_id=other');
      expect(page.nextCursor, 'next-cursor');
      expect(
        gateway.requests.last.uri.queryParameters['cursor'],
        'opaque+/= ?&user_id=other',
      );
      expect(gateway.requests.last.path, '/api/v1/users/me/favorites/history');
      await repo.history(main: true, cursor: page.nextCursor);
      expect(
        gateway.requests.last.path,
        '/api/v1/users/me/favorites/main-history',
      );
    },
  );
  test(
    'mutation network errors are displayed without inventing successful favorites',
    () async {
      final gateway = FakeGateway((r) {
        if (r.path == '/api/v1/players') {
          return reply({
            'players': [playerJson(1)],
          });
        }
        if (r.method == 'GET') return reply({'favorites': []});
        throw const ApiException('network_error');
      });
      final c = FavoritesController(FavoritesRepository(gateway));
      addTearDown(c.dispose);
      await c.load();
      await c.toggle(c.players.first);
      expect(c.favorites, isEmpty);
      expect(c.error, isA<ApiException>());
      expect(c.busy, false);
    },
  );
}
