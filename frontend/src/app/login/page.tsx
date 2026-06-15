'use client';

import React, { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import Script from 'next/script';
import { HardDrive, ShieldAlert, Cpu } from 'lucide-react';
import { useAuthStore } from '../../store/authStore';

export default function LoginPage() {
  const router = useRouter();
  const setAuth = useAuthStore((state) => state.setAuth);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);

  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const googleClientId = process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID;

  // If already logged in, redirect to dashboard
  useEffect(() => {
    if (isAuthenticated) {
      router.push('/dashboard');
    }
  }, [isAuthenticated, router]);

  const initializeGoogleSignIn = () => {
    const google = (window as any).google;
    if (!google || !googleClientId) return;

    try {
      google.accounts.id.initialize({
        client_id: googleClientId,
        callback: handleGoogleCredentialResponse,
      });

      google.accounts.id.renderButton(
        document.getElementById('google-signin-button'),
        { 
          theme: 'filled_blue', 
          size: 'large', 
          width: 320, 
          shape: 'pill',
          text: 'signin_with'
        }
      );

      google.accounts.id.prompt();
    } catch (err) {
      console.error('Failed to initialize Google Sign-In:', err);
    }
  };

  useEffect(() => {
    // If the script was already loaded and Client ID is available, initialize it
    if (googleClientId && typeof window !== 'undefined' && (window as any).google) {
      initializeGoogleSignIn();
    }
  }, [googleClientId]);

  const handleGoogleCredentialResponse = async (response: any) => {
    setError(null);
    setLoading(true);
    try {
      const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8081';
      const res = await fetch(`${API_URL}/api/auth/google`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ credential: response.credential }),
      });

      const data = await res.json();

      if (!res.ok) {
        throw new Error(data.error || 'Google Authentication failed');
      }

      setAuth(data.user, data.access_token, data.refresh_token);
      router.push('/dashboard');
    } catch (err: any) {
      setError(err.message || 'An error occurred during Google Sign-In');
    } finally {
      setLoading(false);
    }
  };

  const handleDevBypass = async () => {
    setError(null);
    setLoading(true);
    try {
      const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8081';
      const res = await fetch(`${API_URL}/api/auth/google`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ credential: 'dev-bypass-token' }),
      });

      const data = await res.json();

      if (!res.ok) {
        throw new Error(data.error || 'Developer Bypass failed');
      }

      setAuth(data.user, data.access_token, data.refresh_token);
      router.push('/dashboard');
    } catch (err: any) {
      setError(err.message || 'An error occurred during Developer Bypass');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-slate-950 px-4 py-12 sm:px-6 lg:px-8 relative overflow-hidden">
      {googleClientId && (
        <Script
          src="https://accounts.google.com/gsi/client"
          onLoad={initializeGoogleSignIn}
          onError={() => console.error('Failed to load Google Sign-In script')}
          strategy="afterInteractive"
        />
      )}

      {/* Decorative background glows */}
      <div className="absolute top-1/4 right-1/4 translate-x-1/2 -translate-y-1/2 w-80 h-80 bg-indigo-500/10 rounded-full blur-[100px] pointer-events-none"></div>
      <div className="absolute bottom-1/4 left-1/4 -translate-x-1/2 translate-y-1/2 w-80 h-80 bg-emerald-500/10 rounded-full blur-[100px] pointer-events-none"></div>

      <div className="w-full max-w-md space-y-8 bg-slate-900/60 backdrop-blur-xl border border-slate-800 p-8 rounded-2xl shadow-2xl relative z-10 text-center">
        <div>
          <div className="inline-flex h-12 w-12 items-center justify-center rounded-xl bg-indigo-600/10 text-indigo-400 mb-4 border border-indigo-500/20">
            <HardDrive className="h-6 w-6" />
          </div>
          <h2 className="text-3xl font-bold tracking-tight text-white">
            CloudStore Portal
          </h2>
          <p className="mt-2 text-sm text-slate-400">
            Secure, decentralized storage powered by local AI
          </p>
        </div>

        {error && (
          <div className="rounded-lg bg-red-500/10 border border-red-500/30 p-4 text-sm text-red-400 animate-shake">
            {error}
          </div>
        )}

        <div className="mt-8 space-y-6 flex flex-col items-center justify-center">
          {googleClientId ? (
            <div className="space-y-4 w-full flex flex-col items-center">
              <div className="text-xs text-slate-500 uppercase tracking-widest">
                Secure Sign-In
              </div>
              <div id="google-signin-button" className="flex justify-center min-h-[50px] w-full max-w-[320px]">
                {loading && (
                  <div className="flex items-center space-x-2 text-slate-400 text-sm">
                    <div className="h-4 w-4 animate-spin rounded-full border-2 border-indigo-500 border-t-transparent"></div>
                    <span>Authenticating...</span>
                  </div>
                )}
              </div>
              <div className="text-xs text-slate-500">
                Accounts are automatically provisioned upon sign-in.
              </div>
            </div>
          ) : (
            <div className="space-y-4 w-full text-left bg-slate-950/40 p-5 rounded-xl border border-slate-800/80">
              <div className="flex items-start space-x-3 text-amber-500/90">
                <ShieldAlert className="h-5 w-5 mt-0.5 flex-shrink-0" />
                <div>
                  <h4 className="text-sm font-semibold text-amber-400">Google OAuth Unconfigured</h4>
                  <p className="text-xs text-slate-400 mt-1">
                    To enable Google Auth, set the <code className="text-slate-200">NEXT_PUBLIC_GOOGLE_CLIENT_ID</code> variable in <code className="text-slate-200">docker-compose.yml</code>.
                  </p>
                </div>
              </div>
            </div>
          )}

          {/* Developer Bypass Button */}
          <div className="w-full pt-4 border-t border-slate-800/60 mt-4">
            <button
              onClick={handleDevBypass}
              disabled={loading}
              className="group relative flex w-full justify-center items-center gap-2 rounded-xl bg-slate-800/80 border border-slate-700/50 hover:bg-slate-700/80 px-4 py-3 text-sm font-medium text-slate-200 hover:text-white transition-all duration-200 shadow-md focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-offset-slate-900 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <Cpu className="h-4 w-4 text-indigo-400 group-hover:scale-110 transition-transform" />
              {loading ? 'Processing...' : 'Sign In as Developer (Offline)'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
