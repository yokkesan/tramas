import 'package:flutter/material.dart';
import '../../../app/app_scope.dart';
import '../../../core/widgets/feature_widgets.dart';
import '../../auth/screens/account_link_screen.dart';
import '../../auth/screens/password_reset_screen.dart';
import '../../favorites/models/favorite.dart';
import '../../favorites/screens/favorites_screen.dart';
import '../models/profile.dart';
import '../widgets/profile_image_view.dart';
import 'profile_editor_screen.dart';

class ProfileScreen extends StatefulWidget {
  const ProfileScreen({super.key, this.userId});
  final String? userId;
  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  int _revision = 0;
  @override
  Widget build(BuildContext context) {
    final scope = AppScope.of(context), own = widget.userId == null;
    return FeatureScaffold(
      title: own ? 'マイページ' : '公開プロフィール',
      child: AsyncContent<(UserProfile, List<Favorite>)>(
        key: ValueKey('${widget.userId}:$_revision'),
        load: () async {
          final profile = await scope.profiles.get(userId: widget.userId);
          final favorites = await scope.favorites.current(
            userId: widget.userId,
          );
          return (profile, favorites);
        },
        builder: (data, reload) {
          final (profile, favorites) = data;
          return Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              ProfileImageView(
                image: profile.header,
                standardId: profile.headerStandard,
                kind: 'header',
              ),
              const SizedBox(height: 12),
              ProfileImageView(
                image: profile.avatar,
                standardId: profile.avatarStandard,
                kind: 'avatar',
              ),
              const SizedBox(height: 12),
              Text(
                profile.nickname ?? '',
                style: const TextStyle(
                  fontSize: 23,
                  fontWeight: FontWeight.w900,
                ),
              ),
              const SizedBox(height: 12),
              Text(
                profile.biography.isEmpty ? '自己紹介はまだありません。' : profile.biography,
              ),
              if (own)
                SelectableText(
                  'ユーザーID: ${profile.id}',
                  style: const TextStyle(fontSize: 11, color: Colors.black54),
                ),
              const SizedBox(height: 16),
              FeatureCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text(
                      '現在の推し選手',
                      style: TextStyle(fontWeight: FontWeight.w800),
                    ),
                    if (favorites.isEmpty)
                      const Padding(
                        padding: EdgeInsets.only(top: 12),
                        child: Text('推し選手はまだ登録されていません。'),
                      ),
                    ...favorites.map(
                      (f) => ListTile(
                        contentPadding: EdgeInsets.zero,
                        title: Text(f.player.name),
                        trailing: f.main ? const Text('★ メイン') : null,
                      ),
                    ),
                    if (own)
                      TextButton(
                        onPressed: () => Navigator.of(context)
                            .push(
                              MaterialPageRoute<void>(
                                builder: (_) => const FavoritesScreen(),
                              ),
                            )
                            .then((_) {
                              if (mounted) reload();
                            }),
                        child: const Text('推し選手を編集'),
                      ),
                  ],
                ),
              ),
              if (own) ...[
                ActionButton(
                  'プロフィール編集',
                  onPressed: () async {
                    await Navigator.of(context).push(
                      MaterialPageRoute<void>(
                        builder: (_) => const ProfileEditorScreen(),
                      ),
                    );
                    if (mounted) setState(() => _revision++);
                  },
                ),
                TextButton(
                  onPressed: () => Navigator.of(context).push(
                    MaterialPageRoute<void>(
                      builder: (_) => const AccountLinkScreen(),
                    ),
                  ),
                  child: const Text('Apple・Googleアカウント連携'),
                ),
                TextButton(
                  onPressed: () => Navigator.of(context).push(
                    MaterialPageRoute<void>(
                      builder: (_) => const PasswordResetScreen(),
                    ),
                  ),
                  child: const Text('パスワード再設定'),
                ),
                OutlinedButton(
                  onPressed: () async {
                    try {
                      await scope.session.logout();
                    } catch (_) {
                      /* Root shows logout pending state. */
                    }
                  },
                  child: const Text('ログアウト'),
                ),
              ],
            ],
          );
        },
      ),
    );
  }
}

class PublicProfileLookupScreen extends StatefulWidget {
  const PublicProfileLookupScreen({super.key});
  @override
  State<PublicProfileLookupScreen> createState() =>
      _PublicProfileLookupScreenState();
}

class _PublicProfileLookupScreenState extends State<PublicProfileLookupScreen> {
  final _id = TextEditingController();
  @override
  void dispose() {
    _id.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => FeatureScaffold(
    title: '公開プロフィール',
    child: Column(
      children: [
        const Text('共有されたユーザーIDから公開プロフィールを表示できます。'),
        const SizedBox(height: 16),
        TextField(controller: _id, decoration: featureInput('ユーザーID')),
        ActionButton(
          'プロフィールを見る',
          onPressed: () => Navigator.of(context).push(
            MaterialPageRoute<void>(
              builder: (_) => ProfileScreen(userId: _id.text.trim()),
            ),
          ),
        ),
      ],
    ),
  );
}
