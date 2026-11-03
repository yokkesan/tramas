import 'package:flutter/material.dart';
import '../../../app/app_scope.dart';
import '../../../core/widgets/feature_widgets.dart';

class AccountLinkScreen extends StatefulWidget {
  const AccountLinkScreen({super.key});
  @override
  State<AccountLinkScreen> createState() => _AccountLinkScreenState();
}

class _AccountLinkScreenState extends State<AccountLinkScreen> {
  final _password = TextEditingController();
  String? _proof, _message;
  bool _busy = false;
  Object? _error;
  Future<void> _reauth(String? provider) async {
    if (_busy) return;
    final oidc = AppScope.of(context).oidc;
    setState(() {
      _busy = true;
      _error = null;
      _proof = null;
      _message = null;
    });
    try {
      final proof = provider == null
          ? await oidc.passwordProof(_password.text)
          : await oidc.providerProof(provider);
      if (mounted) {
        _password.clear();
        setState(() {
          _proof = proof;
          _message = '本人確認ができました。5分以内に連携先を選択してください。';
        });
      }
    } catch (e) {
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _link(String provider) async {
    if (_busy || _proof == null) return;
    final proof = _proof!, oidc = AppScope.of(context).oidc;
    setState(() {
      _busy = true;
      _error = null;
      _proof = null;
      _message = null;
    });
    try {
      await oidc.link(provider, proof);
      if (mounted) setState(() => _message = 'アカウントを連携しました。ユーザーIDは変更されません。');
    } catch (e) {
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  void dispose() {
    _password.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final sdk = AppScope.of(context).oidc.sdk;
    return FeatureScaffold(
      title: '外部アカウント連携',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('メールアドレスが同じでも自動統合しません。現在のアカウントで再認証してから、連携先を認証してください。'),
          const SizedBox(height: 16),
          FeatureCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text('1. 現在のアカウントで本人確認'),
                const SizedBox(height: 12),
                TextField(
                  controller: _password,
                  obscureText: true,
                  decoration: featureInput('現在のパスワード'),
                ),
                ActionButton(
                  'パスワードで本人確認',
                  busy: _busy,
                  onPressed: () => _reauth(null),
                ),
                TextButton(
                  onPressed: _busy || !sdk.available('apple')
                      ? null
                      : () => _reauth('apple'),
                  child: const Text('連携済みAppleで本人確認'),
                ),
                TextButton(
                  onPressed: _busy || !sdk.available('google')
                      ? null
                      : () => _reauth('google'),
                  child: const Text('連携済みGoogleで本人確認'),
                ),
              ],
            ),
          ),
          FeatureCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text('2. 新しく連携するアカウントを認証'),
                TextButton(
                  onPressed: _busy || _proof == null || !sdk.available('apple')
                      ? null
                      : () => _link('apple'),
                  child: const Text('Appleアカウントを連携'),
                ),
                TextButton(
                  onPressed: _busy || _proof == null || !sdk.available('google')
                      ? null
                      : () => _link('google'),
                  child: const Text('Googleアカウントを連携'),
                ),
                if (!sdk.available('apple') || !sdk.available('google'))
                  const Text(
                    '未設定の外部認証は利用できません。',
                    style: TextStyle(fontSize: 11),
                  ),
              ],
            ),
          ),
          ErrorText(_error),
          if (_message != null) Text(_message!),
        ],
      ),
    );
  }
}
