import 'package:flutter/widgets.dart';
import '../features/auth/services/oidc_service.dart';
import '../features/auth/state/session_controller.dart';
import '../features/favorites/data/favorites_repository.dart';
import '../features/profile/data/profile_repository.dart';
import '../features/profile/services/image_selection.dart';

class AppScope extends InheritedWidget {
  const AppScope({
    super.key,
    required this.session,
    required this.oidc,
    required this.profiles,
    required this.favorites,
    required this.images,
    required super.child,
  });
  final SessionController session;
  final OidcService oidc;
  final ProfileRepository profiles;
  final FavoritesRepository favorites;
  final ImageSelection images;
  static AppScope of(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<AppScope>()!;
  @override
  bool updateShouldNotify(AppScope oldWidget) => session != oldWidget.session;
}
