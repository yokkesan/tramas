import 'package:flutter/material.dart';

import '../../../core/theme/app_colors.dart';
import '../services/register_service.dart';

class RegisterScreen extends StatefulWidget {
  const RegisterScreen({super.key, required this.apiBaseUrl});

  final String apiBaseUrl;

  @override
  State<RegisterScreen> createState() => _RegisterScreenState();
}

class _RegisterScreenState extends State<RegisterScreen> {
  final _emailController = TextEditingController();
  final _passwordController = TextEditingController();
  final _passwordConfirmController = TextEditingController();

  final _registerService = RegisterService();

  bool _isLoading = false;
  String? _message;

  @override
  void dispose() {
    _emailController.dispose();
    _passwordController.dispose();
    _passwordConfirmController.dispose();
    super.dispose();
  }

  Future<void> _register() async {
    if (_isLoading) return;

    final email = _emailController.text.trim();
    final password = _passwordController.text;
    final passwordConfirm = _passwordConfirmController.text;

    if (email.isEmpty || password.isEmpty) {
      setState(() {
        _message = 'メールアドレスとパスワードを入力してください。';
      });
      return;
    }

    if (password != passwordConfirm) {
      setState(() {
        _message = 'パスワードが一致しません。';
      });
      return;
    }

    setState(() {
      _isLoading = true;
      _message = null;
    });

    try {
      final statusCode = await _registerService.register(
        apiBaseUrl: widget.apiBaseUrl,
        email: email,
        password: password,
      );

      if (!mounted) return;

      setState(() {
        switch (statusCode) {
          case 201:
            _message = 'ユーザー登録が完了しました。';
          case 409:
            _message = 'このメールアドレスはすでに登録されています。';
          case 422:
            _message = '入力内容を確認してください。';
          case 429:
            _message = 'アクセスが集中しています。時間をおいて再度お試しください。';
          default:
            _message = '登録に失敗しました。もう一度お試しください。';
        }
      });
    } catch (_) {
      if (!mounted) return;

      setState(() {
        _message = '通信エラーが発生しました。接続状況をご確認ください。';
      });
    } finally {
      if (mounted) {
        setState(() {
          _isLoading = false;
        });
      }
    }
  }

  InputDecoration _inputDecoration(String hint) {
    return InputDecoration(
      hintText: hint,
      hintStyle: const TextStyle(fontSize: 13, color: Color(0xFF7E8289)),
      filled: true,
      fillColor: AppColors.white,
      contentPadding: const EdgeInsets.all(12),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(10),
        borderSide: const BorderSide(color: Color(0xFFDDDDDD)),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(10),
        borderSide: const BorderSide(color: AppColors.primary),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.background,
      appBar: AppBar(
        backgroundColor: AppColors.header,
        foregroundColor: AppColors.white,
        title: const Text(
          '新規登録',
          style: TextStyle(fontSize: 27, fontWeight: FontWeight.w900),
        ),
      ),
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text(
                '新規登録',
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w800,
                  color: AppColors.textPrimary,
                ),
              ),
              const SizedBox(height: 10),

              TextField(
                controller: _emailController,
                keyboardType: TextInputType.emailAddress,
                autofillHints: const [AutofillHints.email],
                decoration: _inputDecoration('メールアドレス'),
              ),
              const SizedBox(height: 9),

              TextField(
                controller: _passwordController,
                obscureText: true,
                autofillHints: const [AutofillHints.newPassword],
                decoration: _inputDecoration('パスワード'),
              ),
              const SizedBox(height: 9),

              TextField(
                controller: _passwordConfirmController,
                obscureText: true,
                decoration: _inputDecoration('パスワード確認'),
              ),
              const SizedBox(height: 9),

              ElevatedButton(
                onPressed: _isLoading ? null : _register,
                style: ElevatedButton.styleFrom(
                  backgroundColor: AppColors.primary,
                  foregroundColor: AppColors.textPrimary,
                  disabledBackgroundColor: AppColors.primary,
                  disabledForegroundColor: AppColors.textPrimary,
                  elevation: 0,
                  padding: const EdgeInsets.symmetric(
                    horizontal: 13,
                    vertical: 10,
                  ),
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(9),
                  ),
                ),
                child: Text(
                  _isLoading ? '登録中...' : '登録',
                  style: const TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w800,
                  ),
                ),
              ),

              if (_message != null) ...[
                const SizedBox(height: 12),
                Semantics(
                  liveRegion: true,
                  child: Text(
                    _message!,
                    style: const TextStyle(
                      fontSize: 12,
                      color: AppColors.textPrimary,
                    ),
                  ),
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}
