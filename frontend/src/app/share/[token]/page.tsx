'use client';

import React, { useEffect, useState, use } from 'react';
import { HardDrive, File, Folder, Download, Lock, Key, Loader2, AlertTriangle, ShieldCheck } from 'lucide-react';

interface SharedFileItem {
  id: string;
  name: string;
  size: number;
  mime_type: string;
  created_at: string;
}

interface ShareResponse {
  type: 'file' | 'folder';
  name: string;
  size?: number;
  download_url?: string;
  files?: SharedFileItem[];
}

export default function SharePage({ params }: { params: Promise<{ token: string }> }) {
  // Unwrap params using React.use
  const { token } = use(params);

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [shareData, setShareData] = useState<ShareResponse | null>(null);

  // Password-lock States
  const [passwordRequired, setPasswordRequired] = useState(false);
  const [passwordInput, setPasswordInput] = useState('');
  const [unlockError, setUnlockError] = useState<string | null>(null);

  useEffect(() => {
    fetchSharedResource();
  }, [token]);

  const fetchSharedResource = async (password: string = '') => {
    setLoading(true);
    setError(null);
    setUnlockError(null);
    try {
      const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8081';
      let url = `${API_URL}/api/shares/public/${token}`;
      if (password) {
        url += `?password=${encodeURIComponent(password)}`;
      }

      const response = await fetch(url);
      const data = await response.json();

      if (response.status === 403 && data.password_required) {
        setPasswordRequired(true);
        setLoading(false);
        return;
      }

      if (!response.ok) {
        if (password) {
          setUnlockError(data.error || 'Incorrect password');
        } else {
          setError(data.error || 'Share link is invalid, expired, or download limit reached');
        }
        return;
      }

      // Success!
      setShareData(data);
      setPasswordRequired(false);
    } catch (err: any) {
      setError('A connection error occurred trying to load the shared item.');
    } finally {
      setLoading(false);
    }
  };

  const handleUnlock = (e: React.FormEvent) => {
    e.preventDefault();
    if (!passwordInput.trim()) return;
    fetchSharedResource(passwordInput);
  };

  const handleGuestDownload = async (fileId: string, filename: string) => {
    try {
      const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8081';
      let url = `${API_URL}/api/shares/public/${token}/download/${fileId}`;
      if (passwordInput) {
        url += `?password=${encodeURIComponent(passwordInput)}`;
      }

      const response = await fetch(url);
      const data = await response.json();

      if (!response.ok) {
        throw new Error(data.error || 'Failed to generate guest download url');
      }

      const link = document.createElement('a');
      link.href = data.download_url;
      link.setAttribute('download', filename);
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
    } catch (err: any) {
      alert(err.message);
    }
  };

  const formatBytes = (bytes: number, decimals = 2) => {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
  };

  return (
    <div className="flex min-h-screen flex-col bg-slate-950 text-slate-100 overflow-hidden relative">
      {/* Decorative glows */}
      <div className="absolute inset-0 bg-[linear-gradient(to_right,#0f172a_1px,transparent_1px),linear-gradient(to_bottom,#0f172a_1px,transparent_1px)] bg-[size:4rem_4rem] [mask-image:radial-gradient(ellipse_60%_50%_at_50%_0%,#000_70%,transparent_100%)] pointer-events-none"></div>
      <div className="absolute top-1/4 left-1/2 -translate-x-1/2 -translate-y-1/2 w-80 h-80 bg-indigo-500/10 rounded-full blur-[120px] pointer-events-none"></div>

      {/* Header */}
      <header className="relative z-10 border-b border-slate-900 bg-slate-950/50 backdrop-blur-md px-6 py-4 flex items-center justify-between max-w-7xl mx-auto w-full">
        <div className="flex items-center gap-2 font-bold text-xl tracking-tight text-white">
          <HardDrive className="h-6 w-6 text-indigo-500" />
          <span>CloudStore</span>
        </div>
        <div className="text-xs text-slate-500 font-semibold tracking-wider uppercase flex items-center gap-1.5 bg-slate-900 px-3 py-1 rounded-full border border-slate-800">
          <ShieldCheck className="h-3.5 w-3.5 text-indigo-400" />
          Secure Share Link
        </div>
      </header>

      {/* Main container */}
      <main className="flex-1 flex items-center justify-center p-6 relative z-10">
        {loading && (
          <div className="flex flex-col items-center gap-4">
            <Loader2 className="h-8 w-8 animate-spin text-indigo-500" />
            <p className="text-sm text-slate-400">Loading shared resource details...</p>
          </div>
        )}

        {!loading && error && (
          <div className="w-full max-w-md bg-slate-900/60 backdrop-blur-xl border border-slate-800/80 p-8 rounded-2xl shadow-2xl text-center">
            <div className="inline-flex h-12 w-12 items-center justify-center rounded-xl bg-red-500/10 text-red-400 mb-4 border border-red-500/20">
              <AlertTriangle className="h-6 w-6" />
            </div>
            <h2 className="text-xl font-bold text-white mb-2">Access Expired or Invalid</h2>
            <p className="text-sm text-slate-400 leading-relaxed">
              {error}
            </p>
          </div>
        )}

        {!loading && passwordRequired && (
          <div className="w-full max-w-md bg-slate-900/60 backdrop-blur-xl border border-slate-800/80 p-8 rounded-2xl shadow-2xl">
            <div className="text-center">
              <div className="inline-flex h-12 w-12 items-center justify-center rounded-xl bg-indigo-600/10 text-indigo-400 mb-4 border border-indigo-500/20">
                <Lock className="h-6 w-6" />
              </div>
              <h2 className="text-xl font-bold text-white mb-2">Password Protected</h2>
              <p className="text-sm text-slate-400 leading-normal mb-6">
                This link requires a password to unlock. Please enter the password below.
              </p>
            </div>

            {unlockError && (
              <div className="mb-4 rounded-lg bg-red-500/10 border border-red-500/30 p-3.5 text-xs text-red-400 text-center font-medium">
                {unlockError}
              </div>
            )}

            <form onSubmit={handleUnlock} className="space-y-4">
              <div>
                <input
                  type="password"
                  required
                  value={passwordInput}
                  onChange={(e) => setPasswordInput(e.target.value)}
                  className="block w-full rounded-lg bg-slate-950 border border-slate-800 px-3.5 py-2.5 text-white placeholder-slate-600 focus:border-indigo-500 focus:outline-none text-sm transition-colors text-center font-medium"
                  placeholder="Enter link password"
                  autoFocus
                />
              </div>
              <button
                type="submit"
                className="w-full flex justify-center items-center gap-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 px-4 py-2.5 text-sm font-semibold text-white transition-all shadow-lg shadow-indigo-500/15 cursor-pointer"
              >
                <Key className="h-4 w-4" />
                Unlock Resource
              </button>
            </form>
          </div>
        )}

        {!loading && shareData && (
          <div className="w-full max-w-2xl bg-slate-900/60 backdrop-blur-xl border border-slate-800/80 p-8 rounded-2xl shadow-2xl">
            
            {/* Share details for single FILE */}
            {shareData.type === 'file' && (
              <div className="text-center">
                <div className="inline-flex h-14 w-14 items-center justify-center rounded-2xl bg-indigo-600/10 text-indigo-400 mb-5 border border-indigo-500/20 shadow-xl">
                  <File className="h-7 w-7" />
                </div>
                <h2 className="text-2xl font-bold text-white truncate max-w-md mx-auto mb-1">
                  {shareData.name}
                </h2>
                <p className="text-sm text-slate-400 mb-8 font-medium">
                  Size: {formatBytes(shareData.size || 0)}
                </p>

                <a
                  href={shareData.download_url}
                  download={shareData.name}
                  className="inline-flex items-center justify-center gap-2 bg-indigo-600 hover:bg-indigo-500 text-white font-semibold py-3 px-8 rounded-xl transition-all shadow-lg shadow-indigo-500/20 cursor-pointer"
                >
                  <Download className="h-4 w-4" />
                  Download File
                </a>
              </div>
            )}

            {/* Share details for FOLDER */}
            {shareData.type === 'folder' && (
              <div className="space-y-6">
                <div className="flex items-center gap-4 border-b border-slate-800 pb-5">
                  <div className="h-12 w-12 bg-indigo-600/10 rounded-xl flex items-center justify-center text-indigo-400 border border-indigo-500/20 shadow-lg flex-shrink-0">
                    <Folder className="h-6 w-6" />
                  </div>
                  <div>
                    <h2 className="text-xl font-bold text-white truncate max-w-sm">
                      {shareData.name}
                    </h2>
                    <p className="text-xs text-slate-400 mt-0.5">Shared Folder Contents (Guest Access)</p>
                  </div>
                </div>

                {shareData.files && shareData.files.length > 0 ? (
                  <div className="space-y-2 max-h-96 overflow-y-auto pr-1">
                    {shareData.files.map((file) => (
                      <div 
                        key={file.id}
                        className="flex items-center justify-between p-3.5 bg-slate-950/40 hover:bg-slate-900/60 border border-slate-900 hover:border-slate-800 rounded-xl transition-all group"
                      >
                        <div className="flex items-center gap-3 overflow-hidden">
                          <File className="h-4 w-4 text-indigo-400 flex-shrink-0" />
                          <div className="overflow-hidden">
                            <span className="text-sm font-semibold text-slate-200 block truncate">
                              {file.name}
                            </span>
                            <span className="text-xs text-slate-500 mt-0.5 block">
                              {formatBytes(file.size)}
                            </span>
                          </div>
                        </div>

                        <button
                          onClick={() => handleGuestDownload(file.id, file.name)}
                          className="p-2 hover:bg-slate-800 rounded-lg text-slate-400 hover:text-white transition-colors cursor-pointer"
                          title="Download shared file"
                        >
                          <Download className="h-4 w-4" />
                        </button>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="text-center py-10">
                    <p className="text-sm text-slate-400">This shared folder has no files inside.</p>
                  </div>
                )}
              </div>
            )}
            
          </div>
        )}
      </main>

      {/* Footer */}
      <footer className="relative z-10 border-t border-slate-900 py-6 text-center text-xs text-slate-600">
        <p>© {new Date().getFullYear()} CloudStore Storage Systems. Verification link processed safely.</p>
      </footer>
    </div>
  );
}
