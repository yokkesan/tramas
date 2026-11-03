# Mobile

## Technology Stack

| Component | Technology |
|---|---|
| Framework | Flutter 3.44.8 |
| Language | Dart 3.12.2 |
| Platforms | iOS / Android |
| API Client | Dart `HttpClient` |
| Secure Session Storage | Platform Keychain / encrypted Android storage |
| Identity | Sign in with Apple, AppAuth (iOS Google), Credential Manager (Android Google) |
| Configuration | Dart compile-time environment variables |

## Configuration

| File | Purpose |
|---|---|
| `.fvmrc` | Flutter SDK version |
| `pubspec.yaml` / `pubspec.lock` | Dependencies and locked versions |
| `config/dev-ios.json` | Local API endpoint for iOS Simulator |
| `config/dev-android.json` | Local API endpoint for Android Emulator |
| `config/production.example.json` | Production API configuration template |
| `lib/config/app_config.dart` | Environment and API URL validation |

## Directory Structure

```text
.
├── android/
├── ios/
├── lib/
│   ├── api/
│   ├── app/
│   ├── config/
│   ├── core/
│   ├── features/
│   │   ├── auth/
│   │   ├── favorites/
│   │   └── profile/
│   └── main.dart
├── config/
├── test/
├── .fvmrc
├── analysis_options.yaml
├── pubspec.yaml
└── pubspec.lock
```

See [authentication and profile integration](docs/auth-profile-integration.md) for API coverage, provider configuration, verification results, and simulator checks.
