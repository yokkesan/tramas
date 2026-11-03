import '../../../core/network/api_client.dart';
import '../models/favorite.dart';

class FavoritesRepository {
  FavoritesRepository(this.api);
  final AuthenticatedApi api;
  Future<List<Player>> players() async {
    final r = await api.request('GET', '/api/v1/players');
    return (r.json['players'] as List)
        .map((v) => Player.fromJson((v as Map).cast<String, dynamic>()))
        .where((p) => p.eligible)
        .toList();
  }

  Future<List<Favorite>> current({String? userId}) async {
    final r = await api.request(
      'GET',
      '/api/v1/users/${Uri.encodeComponent(userId ?? 'me')}/favorites',
    );
    return (r.json['favorites'] as List)
        .map((v) => Favorite.fromJson((v as Map).cast<String, dynamic>()))
        .where((f) => f.endedAt == null && f.player.eligible)
        .toList();
  }

  Future<void> add(String id) async {
    await api.request(
      'POST',
      '/api/v1/users/me/favorites',
      body: {'player_id': id},
    );
  }

  Future<void> remove(String id) async {
    await api.request(
      'DELETE',
      '/api/v1/users/me/favorites/${Uri.encodeComponent(id)}',
    );
  }

  Future<void> main(String? id) async {
    await api.request(
      id == null ? 'DELETE' : 'PUT',
      id == null
          ? '/api/v1/users/me/favorites/main'
          : '/api/v1/users/me/favorites/${Uri.encodeComponent(id)}/main',
    );
  }

  Future<HistoryPage> history({bool main = false, String cursor = ''}) async {
    final path =
        '/api/v1/users/me/favorites/${main ? 'main-history' : 'history'}';
    final r = await api.request(
      'GET',
      cursor.isEmpty
          ? path
          : '$path?cursor=${Uri.encodeQueryComponent(cursor)}',
    );
    return HistoryPage(
      (r.json['history'] as List)
          .map((e) => (e as Map).cast<String, dynamic>())
          .toList(),
      r.json['next_cursor'] as String,
    );
  }
}
