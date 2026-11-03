import 'package:flutter/material.dart';

import 'app/app_scope.dart';
import 'app/app_shell.dart';
import 'config/app_config.dart';
import 'core/network/api_client.dart';
import 'core/network/error_messages.dart';
import 'core/widgets/feature_widgets.dart';
import 'features/auth/data/session_vault.dart';
import 'features/auth/screens/login_screen.dart';
import 'features/auth/services/oidc_service.dart';
import 'features/auth/state/session_controller.dart';
import 'features/profile/data/profile_repository.dart';
import 'features/profile/screens/nickname_screen.dart';
import 'features/profile/services/image_selection.dart';
import 'features/favorites/data/favorites_repository.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(TramasApp(config: AppConfig.fromEnvironment()));
}

class TramasApp extends StatefulWidget {
  const TramasApp({
    super.key,
    required this.config,
    this.session,
    this.identitySdk,
    this.imageSelection,
  });
  final AppConfig config;
  final SessionController? session;
  final IdentitySdk? identitySdk;
  final ImageSelection? imageSelection;
  @override
  State<TramasApp> createState() => _TramasAppState();
}

class _TramasAppState extends State<TramasApp> {
  late final SessionController _session;
  final _navigator = GlobalKey<NavigatorState>();
  String? _previousId;
  bool? _previousCompleted;
  SessionPhase? _previousPhase;
  @override
  void initState() {
    super.initState();
    _session =
        widget.session ??
        SessionController(
          api: ApiClient(widget.config),
          vault: SecureSessionVault(widget.config.apiBaseUrl),
        );
    _session.addListener(_changed);
    Future.microtask(_session.restore);
  }

  void _changed() {
    final id = _session.user?.id, complete = _session.user?.profileCompleted;
    if (id != _previousId ||
        complete != _previousCompleted ||
        _previousPhase != _session.phase) {
      if (id != _previousId) {
        PaintingBinding.instance.imageCache.clear();
        PaintingBinding.instance.imageCache.clearLiveImages();
      }
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) _navigator.currentState?.popUntil((r) => r.isFirst);
      });
    }
    _previousId = id;
    _previousCompleted = complete;
    _previousPhase = _session.phase;
  }

  @override
  void dispose() {
    _session.removeListener(_changed);
    if (widget.session == null) _session.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AppScope(
    session: _session,
    oidc: OidcService(
      _session,
      widget.identitySdk ?? NativeIdentitySdk(widget.config),
    ),
    profiles: ProfileRepository(_session),
    favorites: FavoritesRepository(_session),
    images: widget.imageSelection ?? NativeImageSelection(),
    child: MaterialApp(
      navigatorKey: _navigator,
      title: 'tramas',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        useMaterial3: true,
        colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFFFFDA2D)),
      ),
      home: AnimatedBuilder(
        animation: _session,
        builder: (context, _) => switch (_session.phase) {
          SessionPhase.restoring => const FeatureScaffold(
            title: 'tramas',
            child: Center(child: CircularProgressIndicator()),
          ),
          SessionPhase.signedOut => LoginScreen(
            apiBaseUrl: widget.config.apiBaseUrl.toString(),
          ),
          SessionPhase.ready =>
            _session.user?.profileCompleted == true
                ? AppShell(
                    key: ValueKey('${_session.epoch}:${_session.user!.id}'),
                  )
                : const NicknameScreen(),
          SessionPhase.offline => _RecoveryScreen(
            session: _session,
            logout: false,
          ),
          SessionPhase.logoutPending => _RecoveryScreen(
            session: _session,
            logout: true,
          ),
        },
      ),
    ),
  );
}

class _RecoveryScreen extends StatelessWidget {
  const _RecoveryScreen({required this.session, required this.logout});
  final SessionController session;
  final bool logout;
  Future<void> _forget(BuildContext context) async {
    final accepted = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('端末のログイン情報を削除'),
        content: const Text('サーバー側のセッション失効は確認できていません。端末の情報だけを削除してログイン画面に戻ります。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('キャンセル'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('端末から削除'),
          ),
        ],
      ),
    );
    if (accepted != true) return;
    try {
      await session.forgetLocalSession();
    } catch (e) {
      if (context.mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text(errorMessage(e))));
      }
    }
  }

  @override
  Widget build(BuildContext context) => FeatureScaffold(
    title: logout ? 'ログアウトを確認中' : '接続の確認',
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          logout
              ? 'サーバーでのログアウトが完了するまで、通常機能へのアクセスを停止しています。'
              : '認証状態をサーバーで確認できませんでした。保存済みの情報は安全な保管領域に保持しています。',
        ),
        ErrorText(session.problem),
        ActionButton(
          '再試行',
          busy: session.authBusy,
          onPressed: () async {
            try {
              if (logout) {
                await session.logout();
              } else {
                await session.restore();
              }
            } catch (_) {
              /* State preserves the error. */
            }
          },
        ),
        TextButton(
          onPressed: session.authBusy ? null : () => _forget(context),
          child: const Text('端末のログイン情報だけを削除'),
        ),
      ],
    ),
  );
}
