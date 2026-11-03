package com.example.tramas

import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import android.os.CancellationSignal
import androidx.credentials.CredentialManager
import androidx.credentials.CredentialManagerCallback
import androidx.credentials.CustomCredential
import androidx.credentials.GetCredentialRequest
import androidx.credentials.GetCredentialResponse
import androidx.credentials.exceptions.GetCredentialException
import androidx.credentials.exceptions.GetCredentialCancellationException
import com.google.android.libraries.identity.googleid.GetSignInWithGoogleOption
import com.google.android.libraries.identity.googleid.GoogleIdTokenCredential
import java.util.concurrent.Executor

class MainActivity : FlutterActivity() {
    private var pending: MethodChannel.Result? = null
    private var cancellation: CancellationSignal? = null

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, "tramas/oidc").setMethodCallHandler { call, result ->
            if (call.method != "googleSignIn") {
                result.notImplemented()
                return@setMethodCallHandler
            }
            val clientId = call.argument<String>("client_id")
            val nonce = call.argument<String>("nonce")
            if (pending != null) {
                result.error("action_busy", "Authentication in progress", null)
                return@setMethodCallHandler
            }
            if (clientId.isNullOrBlank() || nonce == null || !Regex("^[A-Za-z0-9_-]{43}$").matches(nonce)) {
                result.error("external_auth_failed", "Invalid configuration", null)
                return@setMethodCallHandler
            }
            pending = result
            val signal = CancellationSignal()
            cancellation = signal
            try {
                val option = GetSignInWithGoogleOption.Builder(clientId).setNonce(nonce).build()
                val request = GetCredentialRequest.Builder().addCredentialOption(option).build()
                CredentialManager.create(this).getCredentialAsync(this, request, signal,
                    Executor { task -> runOnUiThread(task) },
                    object : CredentialManagerCallback<GetCredentialResponse, GetCredentialException> {
                        override fun onResult(response: GetCredentialResponse) {
                            if (pending !== result) return
                            try {
                                val credential = response.credential
                                require(credential is CustomCredential && credential.type == GoogleIdTokenCredential.TYPE_GOOGLE_ID_TOKEN_CREDENTIAL)
                                val token = GoogleIdTokenCredential.createFrom(credential.data).idToken
                                pending = null
                                cancellation = null
                                result.success(token)
                            } catch (_: Exception) { fail(result, "external_auth_failed") }
                        }
                        override fun onError(error: GetCredentialException) {
                            fail(result, if (error is GetCredentialCancellationException) "provider_cancelled" else "external_auth_failed")
                        }
                    })
            } catch (_: Exception) { fail(result, "external_auth_failed") }
        }
    }

    private fun fail(result: MethodChannel.Result, code: String) {
        if (pending !== result) return
        pending = null
        cancellation = null
        result.error(code, "Authentication could not be completed", null)
    }

    override fun onDestroy() {
        val result = pending
        pending = null
        cancellation?.cancel()
        cancellation = null
        result?.error("provider_cancelled", "Authentication cancelled", null)
        super.onDestroy()
    }
}
