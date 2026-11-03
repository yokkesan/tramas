import 'package:flutter/foundation.dart';
import '../../../core/network/api_client.dart';
import '../data/favorites_repository.dart';
import '../models/favorite.dart';

class FavoritesController extends ChangeNotifier {
  FavoritesController(this.repository);
  final FavoritesRepository repository;
  List<Player> players = [];
  List<Favorite> favorites = [];
  bool loading = false, busy = false, loaded = false, _disposed = false;
  Object? error;
  void _emit() {
    if (!_disposed) notifyListeners();
  }

  Future<void> load() async {
    if (loading) return;
    loading = true;
    loaded = false;
    error = null;
    _emit();
    try {
      players = await repository.players();
      favorites = await repository.current();
      loaded = true;
    } catch (e) {
      error = e;
    } finally {
      loading = false;
      _emit();
    }
  }

  Future<void> toggle(Player p) async {
    if (busy || !loaded || loading) return;
    final selected = favorites.any((f) => f.player.id == p.id);
    if (!selected && (!p.eligible || favorites.length >= 3)) {
      error = ApiException(
        p.eligible ? 'favorite_limit' : 'ineligible_player',
        status: 409,
      );
      _emit();
      return;
    }
    await _mutate(
      () => selected ? repository.remove(p.id) : repository.add(p.id),
    );
  }

  Future<void> setMain(String? id) async {
    if (busy || !loaded || loading) return;
    if (id != null &&
        !favorites.any((f) => f.player.id == id && f.player.eligible)) {
      error = const ApiException('ineligible_player', status: 422);
      _emit();
      return;
    }
    await _mutate(() => repository.main(id));
  }

  Future<void> _mutate(Future<void> Function() action) async {
    busy = true;
    error = null;
    _emit();
    try {
      await action();
      favorites = await repository.current();
    } catch (e) {
      error = e;
    } finally {
      busy = false;
      _emit();
    }
  }

  @override
  void dispose() {
    _disposed = true;
    super.dispose();
  }
}
