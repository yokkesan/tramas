import 'package:flutter/material.dart';
import '../../../app/app_scope.dart';
import '../../../core/widgets/feature_widgets.dart';
import '../models/profile.dart';

class NicknameScreen extends StatefulWidget {
  const NicknameScreen({super.key});
  @override
  State<NicknameScreen> createState() => _NicknameScreenState();
}

class _NicknameScreenState extends State<NicknameScreen> {
  final _nickname = TextEditingController();
  bool _busy = false;
  Object? _error;
  String? _validation;
  Future<void> _save() async {
    if (_busy) return;
    if (!validNickname(_nickname.text)) {
      setState(() => _validation = 'ニックネームは2〜20文字で入力してください。');
      return;
    }
    final session = AppScope.of(context).session;
    setState(() {
      _busy = true;
      _error = null;
      _validation = null;
    });
    try {
      await session.request(
        'PATCH',
        '/api/v1/users/me/profile',
        body: {'nickname': normalizeNickname(_nickname.text)},
      );
      await session
          .reloadUser(); // Server profile_completed, not local completion, drives navigation.
    } catch (e) {
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  void dispose() {
    _nickname.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => PopScope(
    canPop: false,
    child: FeatureScaffold(
      title: '初回プロフィール設定',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('ニックネームを設定して、tramasの利用を始めましょう。画像・自己紹介・推し選手は後から設定できます。'),
          const SizedBox(height: 16),
          TextField(
            controller: _nickname,
            onChanged: (_) => setState(() {}),
            decoration: featureInput(
              'ニックネーム',
              helper: '2〜20文字・ほかのユーザーと重複できます',
              counter: '${normalizeNickname(_nickname.text).runes.length}/20',
            ),
          ),
          if (_validation != null) Text(_validation!),
          ErrorText(_error),
          ActionButton('設定して始める', busy: _busy, onPressed: _save),
          TextButton(
            onPressed: _busy
                ? null
                : () async {
                    try {
                      await AppScope.of(context).session.logout();
                    } catch (_) {
                      /* Gate renders durable logout error. */
                    }
                  },
            child: const Text('ログアウト'),
          ),
        ],
      ),
    ),
  );
}
