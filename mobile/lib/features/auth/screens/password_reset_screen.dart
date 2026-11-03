import 'package:flutter/material.dart';
import '../../../app/app_scope.dart';
import '../../../core/widgets/feature_widgets.dart';

class PasswordResetScreen extends StatefulWidget {
  const PasswordResetScreen({super.key});
  @override
  State<PasswordResetScreen> createState() => _PasswordResetScreenState();
}

class _PasswordResetScreenState extends State<PasswordResetScreen> {
  final _email = TextEditingController(),
      _token = TextEditingController(),
      _password = TextEditingController(),
      _confirm = TextEditingController();
  bool _busy = false;
  Object? _error;
  String? _message;
  Future<void> _run(bool complete) async {
    if (_busy) return;
    if (complete &&
        (_password.text.isEmpty || _password.text != _confirm.text)) {
      setState(() => _message = 'パスワードと確認用入力を一致させてください。');
      return;
    }
    final session = AppScope.of(context).session;
    setState(() {
      _busy = true;
      _error = null;
      _message = null;
    });
    try {
      if (complete) {
        await session.repository.completeReset(
          _token.text.trim(),
          _password.text,
        );
        _token.clear();
        _password.clear();
        _confirm.clear();
        if (session.user != null) await session.forgetLocalSession();
      } else {
        await session.repository.requestReset(_email.text.trim());
      }
      if (mounted) {
        setState(
          () => _message = complete
              ? 'パスワードを変更しました。ログイン画面から再度ログインしてください。'
              : '送信要求を受け付けました。対象のアドレスへのメールをご確認ください。',
        );
      }
    } catch (e) {
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  void dispose() {
    _email.dispose();
    _token.dispose();
    _password.dispose();
    _confirm.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => FeatureScaffold(
    title: 'パスワード再設定',
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        FeatureCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text('再設定メールを要求'),
              const SizedBox(height: 12),
              TextField(
                controller: _email,
                keyboardType: TextInputType.emailAddress,
                decoration: featureInput('メールアドレス'),
              ),
              ActionButton(
                '再設定メールを要求',
                busy: _busy,
                onPressed: () => _run(false),
              ),
            ],
          ),
        ),
        FeatureCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text('メール内のトークンで変更'),
              const SizedBox(height: 12),
              TextField(
                controller: _token,
                obscureText: true,
                enableSuggestions: false,
                autocorrect: false,
                decoration: featureInput('再設定トークン'),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: _password,
                obscureText: true,
                autofillHints: const [AutofillHints.newPassword],
                decoration: featureInput('新しいパスワード'),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: _confirm,
                obscureText: true,
                decoration: featureInput('新しいパスワード（確認）'),
              ),
              ActionButton(
                'パスワードを変更',
                busy: _busy,
                onPressed: () => _run(true),
              ),
            ],
          ),
        ),
        ErrorText(_error),
        if (_message != null)
          Semantics(liveRegion: true, child: Text(_message!)),
      ],
    ),
  );
}
