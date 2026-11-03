import 'package:flutter/material.dart';

import '../../../core/theme/app_colors.dart';
import '../../../app/app_scope.dart';
import '../../../core/network/api_client.dart';
import '../../../core/widgets/feature_widgets.dart';
import 'verification_screen.dart';
import 'password_reset_screen.dart';
import 'register_screen.dart';

class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key, required this.apiBaseUrl});

  final String apiBaseUrl;

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _emailController = TextEditingController();
  final _passwordController = TextEditingController();
  bool _busy = false;
  Object? _error;

  Future<void> _login([String? provider]) async {
    if (_busy) return;
    final scope = AppScope.of(context);
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      if (provider == null) {
        await scope.session.login(
          _emailController.text.trim(),
          _passwordController.text,
        );
      } else {
        await scope.oidc.login(provider);
      }
      if (mounted) _passwordController.clear();
    } catch (e) {
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  void dispose() {
    _emailController.dispose();
    _passwordController.dispose();
    super.dispose();
  }

  InputDecoration _inputDecoration(String hint) {
    return InputDecoration(
      hintText: hint,
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
          'ログイン',
          style: TextStyle(fontSize: 27, fontWeight: FontWeight.w900),
        ),
      ),
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // カンプのヒーローエリア
              Container(
                width: double.infinity,
                height: 180,
                padding: const EdgeInsets.all(18),
                decoration: BoxDecoration(
                  gradient: const LinearGradient(
                    begin: Alignment.topLeft,
                    end: Alignment.bottomRight,
                    colors: [Color(0xFF111111), Color(0xFF413A23)],
                  ),
                  borderRadius: BorderRadius.circular(14),
                ),
                child: const Column(
                  mainAxisAlignment: MainAxisAlignment.end,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'tramas.',
                      style: TextStyle(
                        color: AppColors.primary,
                        fontSize: 38,
                        fontWeight: FontWeight.w900,
                      ),
                    ),
                    Text(
                      '阪神をもっと楽しむ、ファンの居場所。',
                      style: TextStyle(color: AppColors.white, fontSize: 13),
                    ),
                  ],
                ),
              ),

              const SizedBox(height: 17),

              const Text(
                'ログイン',
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
                autofillHints: const [AutofillHints.password],
                decoration: _inputDecoration('パスワード'),
              ),

              const SizedBox(height: 9),

              ElevatedButton(
                onPressed: _busy ? null : _login,
                style: ElevatedButton.styleFrom(
                  backgroundColor: AppColors.primary,
                  foregroundColor: AppColors.textPrimary,
                  disabledBackgroundColor: AppColors.primary,
                  disabledForegroundColor: AppColors.textPrimary,
                ),
                child: Text(_busy ? 'ログイン中...' : 'ログイン'),
              ),

              const SizedBox(height: 12),

              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  ElevatedButton(
                    onPressed:
                        _busy ||
                            !AppScope.of(context).oidc.sdk.available('apple')
                        ? null
                        : () => _login('apple'),
                    style: ElevatedButton.styleFrom(
                      backgroundColor: AppColors.header,
                      foregroundColor: AppColors.white,
                      disabledBackgroundColor: AppColors.header,
                      disabledForegroundColor: AppColors.white,
                    ),
                    child: const Text('Appleでログイン'),
                  ),
                  ElevatedButton(
                    onPressed:
                        _busy ||
                            !AppScope.of(context).oidc.sdk.available('google')
                        ? null
                        : () => _login('google'),
                    child: const Text('Googleでログイン'),
                  ),
                ],
              ),

              const SizedBox(height: 12),

              if (!AppScope.of(context).oidc.sdk.available('apple') ||
                  !AppScope.of(context).oidc.sdk.available('google'))
                Text(
                  '${!AppScope.of(context).oidc.sdk.available('apple') ? 'Apple認証は設定待ち。' : ''}${!AppScope.of(context).oidc.sdk.available('google') ? 'Google認証は設定待ち。' : ''}',
                  style: const TextStyle(fontSize: 11),
                ),
              ErrorText(_error),
              if (_error == null)
                ErrorText(AppScope.of(context).session.problem),
              if (_error is ApiException &&
                  (_error as ApiException).code ==
                      'email_verification_required')
                TextButton(
                  onPressed: _busy
                      ? null
                      : () => Navigator.of(context).push(
                          MaterialPageRoute<void>(
                            builder: (_) => VerificationScreen(
                              email: _emailController.text.trim(),
                            ),
                          ),
                        ),
                  child: const Text('メール本人確認へ'),
                ),

              OutlinedButton(
                onPressed: _busy
                    ? null
                    : () {
                        Navigator.of(context).push(
                          MaterialPageRoute<void>(
                            builder: (context) =>
                                RegisterScreen(apiBaseUrl: widget.apiBaseUrl),
                          ),
                        );
                      },
                child: const Text('新規登録へ'),
              ),
              TextButton(
                onPressed: _busy
                    ? null
                    : () => Navigator.of(context).push(
                        MaterialPageRoute<void>(
                          builder: (_) => VerificationScreen(
                            email: _emailController.text.trim(),
                          ),
                        ),
                      ),
                child: const Text('メール本人確認・再送'),
              ),
              TextButton(
                onPressed: _busy
                    ? null
                    : () => Navigator.of(context).push(
                        MaterialPageRoute<void>(
                          builder: (_) => const PasswordResetScreen(),
                        ),
                      ),
                child: const Text('パスワードをお忘れの方'),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
