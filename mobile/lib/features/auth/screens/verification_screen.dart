import 'package:flutter/material.dart';
import '../../../app/app_scope.dart';
import '../../../core/widgets/feature_widgets.dart';

class VerificationScreen extends StatefulWidget {
  const VerificationScreen({super.key, this.email = ''});
  final String email;
  @override
  State<VerificationScreen> createState() => _VerificationScreenState();
}

class _VerificationScreenState extends State<VerificationScreen> {
  late final _email = TextEditingController(text: widget.email);
  final _token = TextEditingController();
  bool _busy = false, _verified = false;
  Object? _error;
  String? _message;
  Future<void> _run(bool verify) async {
    if (_busy) return;
    setState(() {
      _busy = true;
      _error = null;
      _message = null;
    });
    try {
      final auth = AppScope.of(context).session.repository;
      if (verify) {
        await auth.verify(_token.text.trim());
      } else {
        await auth.resend(_email.text.trim());
      }
      if (!mounted) return;
      setState(() {
        _verified = verify;
        _message = verify
            ? '本人確認が完了しました。ログインしてください。'
            : '送信要求を受け付けました。対象のアドレスへのメールをご確認ください。';
      });
      if (verify) _token.clear();
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
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => FeatureScaffold(
    title: 'メール本人確認',
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text('登録後はメールアドレスの本人確認が必要です。メール内の確認トークンを入力してください。'),
        const SizedBox(height: 16),
        TextField(
          controller: _email,
          keyboardType: TextInputType.emailAddress,
          decoration: featureInput('メールアドレス'),
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _token,
          obscureText: true,
          enableSuggestions: false,
          autocorrect: false,
          decoration: featureInput('確認トークン'),
        ),
        ErrorText(_error),
        if (_message != null)
          Semantics(liveRegion: true, child: Text(_message!)),
        Wrap(
          spacing: 8,
          children: [
            ActionButton(
              '本人確認を完了',
              busy: _busy,
              onPressed: _verified ? null : () => _run(true),
            ),
            OutlinedButton(
              onPressed: _busy ? null : () => _run(false),
              child: const Text('確認メールを再送'),
            ),
          ],
        ),
        TextButton(
          onPressed: _busy
              ? null
              : () => Navigator.of(context).popUntil((r) => r.isFirst),
          child: const Text('ログインへ戻る'),
        ),
      ],
    ),
  );
}
