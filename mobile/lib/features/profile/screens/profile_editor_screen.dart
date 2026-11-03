import 'package:flutter/material.dart';
import '../../../app/app_scope.dart';
import '../../../core/widgets/feature_widgets.dart';
import '../models/profile.dart';
import '../widgets/profile_image_view.dart';

class ProfileEditorScreen extends StatelessWidget {
  const ProfileEditorScreen({super.key});
  @override
  Widget build(BuildContext context) => FeatureScaffold(
    title: 'プロフィール編集',
    child: AsyncContent<UserProfile>(
      load: () => AppScope.of(context).profiles.get(),
      builder: (profile, _) => _EditorForm(profile: profile),
    ),
  );
}

class _EditorForm extends StatefulWidget {
  const _EditorForm({required this.profile});
  final UserProfile profile;
  @override
  State<_EditorForm> createState() => _EditorFormState();
}

class _EditorFormState extends State<_EditorForm> {
  late UserProfile _profile = widget.profile;
  late final _nickname = TextEditingController(text: _profile.nickname ?? '');
  late final _bio = TextEditingController(text: _profile.biography);
  bool _busy = false;
  Object? _error;
  Future<void> _action(Future<void> Function(AppScope scope) action) async {
    if (_busy) return;
    final scope = AppScope.of(context);
    final generation = scope.session.epoch;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await action(scope);
      if (!mounted || scope.session.epoch != generation) return;
      final updated = await scope.profiles.get();
      if (mounted) setState(() => _profile = updated);
    } catch (e) {
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _save() => _action((scope) async {
    await scope.profiles.update(nickname: _nickname.text, biography: _bio.text);
    await scope.session.reloadUser();
    if (mounted) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(const SnackBar(content: Text('プロフィールを保存しました。')));
    }
  });
  Future<void> _choose(String kind) => _action((scope) async {
    final generation = scope.session.epoch;
    final selection = await scope.images.pick();
    if (!mounted || scope.session.epoch != generation) return;
    if (selection != null) {
      await scope.profiles.upload(kind, selection.bytes, selection.contentType);
    }
  });
  Future<void> _standard(String kind) => _action((scope) async {
    final generation = scope.session.epoch;
    final catalog = (await scope.profiles.standards())
        .where((v) => v.kind == kind)
        .toList();
    if (!mounted) return;
    final selected = await showModalBottomSheet<String>(
      context: context,
      builder: (context) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Text('標準画像を選択'),
              if (catalog.isEmpty)
                const Padding(
                  padding: EdgeInsets.all(20),
                  child: Text('標準画像はまだ登録されていません。'),
                ),
              Flexible(
                child: ListView(
                  shrinkWrap: true,
                  children: catalog
                      .map(
                        (v) => ListTile(
                          leading:
                              standardImageUri(
                                    scope.session.api.config.apiBaseUrl,
                                    v.url,
                                  ) ==
                                  null
                              ? const Icon(Icons.image_outlined)
                              : SizedBox(
                                  width: 48,
                                  height: 48,
                                  child: Image.network(
                                    standardImageUri(
                                      scope.session.api.config.apiBaseUrl,
                                      v.url,
                                    )!.toString(),
                                    fit: BoxFit.cover,
                                    errorBuilder: (_, _, _) =>
                                        const Icon(Icons.broken_image_outlined),
                                  ),
                                ),
                          title: Text(v.id),
                          onTap: () => Navigator.pop(context, v.id),
                        ),
                      )
                      .toList(),
                ),
              ),
              TextButton(
                onPressed: () => Navigator.pop(context),
                child: const Text('閉じる'),
              ),
            ],
          ),
        ),
      ),
    );
    if (selected != null && mounted && scope.session.epoch == generation) {
      await scope.profiles.selectStandard(kind, selected);
    }
  });
  Widget _imageControls(String kind) => FeatureCard(
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(kind == 'avatar' ? 'プロフィール画像（任意）' : 'ヘッダー画像（任意）'),
        const SizedBox(height: 12),
        ProfileImageView(
          image: kind == 'avatar' ? _profile.avatar : _profile.header,
          standardId: kind == 'avatar'
              ? _profile.avatarStandard
              : _profile.headerStandard,
          kind: kind,
        ),
        Wrap(
          spacing: 8,
          children: [
            TextButton(
              onPressed: _busy ? null : () => _choose(kind),
              child: const Text('画像を選択'),
            ),
            TextButton(
              onPressed: _busy ? null : () => _standard(kind),
              child: const Text('標準画像'),
            ),
            TextButton(
              onPressed: _busy
                  ? null
                  : () => _action((s) => s.profiles.remove(kind)),
              child: const Text('画像を削除'),
            ),
          ],
        ),
        const Text(
          'JPEG・PNG・HEIC／10MB以下。原本を送信します。',
          style: TextStyle(fontSize: 11, color: Colors.black54),
        ),
      ],
    ),
  );
  @override
  void dispose() {
    _nickname.dispose();
    _bio.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      TextField(
        controller: _nickname,
        onChanged: (_) => setState(() {}),
        decoration: featureInput(
          'ニックネーム',
          counter: '${normalizeNickname(_nickname.text).runes.length}/20',
          helper: '2〜20文字・重複可能',
        ),
      ),
      const SizedBox(height: 12),
      TextField(
        controller: _bio,
        minLines: 3,
        maxLines: 8,
        decoration: featureInput('自己紹介（任意）'),
      ),
      const SizedBox(height: 12),
      ActionButton('プロフィールを保存', busy: _busy, onPressed: _save),
      ErrorText(_error),
      const SizedBox(height: 12),
      _imageControls('avatar'),
      _imageControls('header'),
    ],
  );
}
