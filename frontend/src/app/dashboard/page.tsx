'use client';

import React, { useEffect, useState, useRef } from 'react';
import { useRouter } from 'next/navigation';
import { useAuthStore } from '../../store/authStore';
import { useNotificationStore } from '../../store/notificationStore';
import { useWebSocket } from '../../hooks/useWebSocket';
import { apiFetch } from '../../services/api';
import { 
  HardDrive, LogOut, Shield, User, File, Folder, 
  ChevronRight, Plus, Upload, Trash, Download, Edit3, 
  X, FolderPlus, ArrowLeft, Loader2, Play, Pause, 
  History, Share2, Copy, Check, Lock, Calendar,
  Bell, BellOff, Info, CheckCircle, AlertCircle, Search, Tag, FileText
} from 'lucide-react';

interface FolderData {
  id: string;
  name: string;
  parent_id: string | null;
  created_at: string;
  updated_at: string;
}

interface FileData {
  id: string;
  name: string;
  folder_id: string | null;
  size: number;
  mime_type: string;
  current_version: number;
  summary?: string;
  tags?: string[];
  created_at: string;
  updated_at: string;
}

interface BreadcrumbItem {
  id: string | null;
  name: string;
}

interface FileVersion {
  id: string;
  version_number: number;
  size: number;
  hash: string;
  created_at: string;
}

interface UploadTask {
  file: File;
  uploadId: string | null;
  status: 'hashing' | 'uploading' | 'paused' | 'merging' | 'completed' | 'failed';
  progress: number;
  hash: string | null;
  totalParts: number;
  activeRequests: XMLHttpRequest[];
}

export default function DashboardPage() {
  const router = useRouter();
  const user = useAuthStore((state) => state.user);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  const refreshToken = useAuthStore((state) => state.refreshToken);
  const clearAuth = useAuthStore((state) => state.clearAuth);

  // Initialize WebSockets for real-time events
  useWebSocket();

  // Notification States
  const notifications = useNotificationStore((state) => state.notifications);
  const markAsRead = useNotificationStore((state) => state.markAsRead);
  const clearAllNotifications = useNotificationStore((state) => state.clearAll);
  const [isNotificationsOpen, setIsNotificationsOpen] = useState(false);
  const unreadCount = notifications.filter(n => !n.read).length;

  // Core States
  const [folders, setFolders] = useState<FolderData[]>([]);
  const [files, setFiles] = useState<FileData[]>([]);
  const [currentFolderId, setCurrentFolderId] = useState<string | null>(null);
  const [breadcrumbs, setBreadcrumbs] = useState<BreadcrumbItem[]>([{ id: null, name: 'Root' }]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Search States
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState<FileData[]>([]);
  const [searching, setSearching] = useState(false);

  // Modals & Target States
  const [isFolderModalOpen, setIsFolderModalOpen] = useState(false);
  const [newFolderName, setNewFolderName] = useState('');
  const [renameTarget, setRenameTarget] = useState<{ id: string; type: 'file' | 'folder'; currentName: string } | null>(null);
  const [renameNewName, setRenameNewName] = useState('');

  // Version History Modal
  const [historyTarget, setHistoryTarget] = useState<FileData | null>(null);
  const [versions, setVersions] = useState<FileVersion[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);

  // Share Modal
  const [shareTarget, setShareTarget] = useState<FileData | null>(null);
  const [shareExpires, setShareExpires] = useState<string>('24'); // hours
  const [shareMaxDownloads, setShareMaxDownloads] = useState<string>('');
  const [sharePassword, setSharePassword] = useState<string>('');
  const [generatedShareLink, setGeneratedShareLink] = useState<string | null>(null);
  const [copiedLink, setCopiedLink] = useState(false);

  // Upload Progress
  const [activeUpload, setActiveUpload] = useState<UploadTask | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const CHUNK_SIZE = 2 * 1024 * 1024; // 2MB
  const CONCURRENCY_LIMIT = 3;

  useEffect(() => {
    if (!isAuthenticated) {
      router.push('/login');
    } else {
      fetchDirectory(currentFolderId);
    }
  }, [isAuthenticated, currentFolderId, router]);

  // Debounced search effect
  useEffect(() => {
    if (!searchQuery.trim()) {
      setSearchResults([]);
      setSearching(false);
      return;
    }

    setSearching(true);
    const delayDebounce = setTimeout(async () => {
      try {
        const response = await apiFetch(`/api/search?q=${encodeURIComponent(searchQuery)}`);
        const data = await response.json();
        if (response.ok) {
          setSearchResults(data || []);
        }
      } catch (err) {
        console.error("Search API failed", err);
      } finally {
        setSearching(false);
      }
    }, 350);

    return () => clearTimeout(delayDebounce);
  }, [searchQuery]);

  const handleLogout = async () => {
    try {
      if (refreshToken) {
        await apiFetch('/api/auth/logout', {
          method: 'POST',
          body: JSON.stringify({ refresh_token: refreshToken }),
        });
      }
    } catch (err) {
      console.error('Logout failed', err);
    } finally {
      clearAuth();
      router.push('/login');
    }
  };

  const fetchDirectory = async (folderId: string | null) => {
    setLoading(true);
    setError(null);
    try {
      const endpoint = folderId ? `/api/folders?parent_id=${folderId}` : '/api/folders';
      const response = await apiFetch(endpoint);
      const data = await response.json();

      if (!response.ok) {
        throw new Error(data.error || 'Failed to fetch directory items');
      }

      setFolders(data.folders || []);
      setFiles(data.files || []);
    } catch (err: any) {
      setError(err.message || 'An error occurred loading dashboard files');
    } finally {
      setLoading(false);
    }
  };

  const handleCreateFolder = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newFolderName.trim()) return;

    try {
      const response = await apiFetch('/api/folders', {
        method: 'POST',
        body: JSON.stringify({
          name: newFolderName,
          parent_id: currentFolderId,
        }),
      });

      if (!response.ok) {
        const data = await response.json();
        throw new Error(data.error || 'Failed to create folder');
      }

      setNewFolderName('');
      setIsFolderModalOpen(false);
      fetchDirectory(currentFolderId);
    } catch (err: any) {
      alert(err.message);
    }
  };

  const handleDeleteFolder = async (id: string) => {
    if (!confirm("Delete this folder? This will delete all subfolders and files inside it!")) return;

    try {
      const response = await apiFetch(`/api/folders/${id}`, {
        method: 'DELETE',
      });

      if (!response.ok) {
        const data = await response.json();
        throw new Error(data.error || 'Failed to delete folder');
      }

      fetchDirectory(currentFolderId);
    } catch (err: any) {
      alert(err.message);
    }
  };

  const handleDeleteFile = async (id: string) => {
    if (!confirm("Move this file to trash?")) return;

    try {
      const response = await apiFetch(`/api/files/${id}`, {
        method: 'DELETE',
      });

      if (!response.ok) {
        const data = await response.json();
        throw new Error(data.error || 'Failed to delete file');
      }

      fetchDirectory(currentFolderId);
      if (searchQuery.trim()) {
        // Refresh search results if deleting from search view
        setSearchResults(prev => prev.filter(f => f.id !== id));
      }
    } catch (err: any) {
      alert(err.message);
    }
  };

  const handleRename = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!renameTarget || !renameNewName.trim()) return;

    const endpoint = renameTarget.type === 'file' 
      ? `/api/files/${renameTarget.id}` 
      : `/api/folders/${renameTarget.id}`;

    try {
      const response = await apiFetch(endpoint, {
        method: 'PATCH',
        body: JSON.stringify({ name: renameNewName }),
      });

      if (!response.ok) {
        const data = await response.json();
        throw new Error(data.error || 'Failed to rename');
      }

      setRenameTarget(null);
      setRenameNewName('');
      fetchDirectory(currentFolderId);
      if (searchQuery.trim() && renameTarget.type === 'file') {
        setSearchResults(prev => prev.map(f => f.id === renameTarget.id ? { ...f, name: renameNewName } : f));
      }
    } catch (err: any) {
      alert(err.message);
    }
  };

  const handleDownloadFile = async (id: string, name: string) => {
    try {
      const response = await apiFetch(`/api/files/${id}/download`);
      const data = await response.json();

      if (!response.ok) {
        throw new Error(data.error || 'Failed to download file');
      }

      const link = document.createElement('a');
      link.href = data.download_url;
      link.setAttribute('download', name);
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
    } catch (err: any) {
      alert(err.message);
    }
  };

  // --- VERSION HISTORY LOGIC ---
  const openVersionHistory = async (file: FileData) => {
    setHistoryTarget(file);
    setLoadingVersions(true);
    setVersions([]);
    try {
      const response = await apiFetch(`/api/files/${file.id}/versions`);
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || 'Failed to load version history');
      setVersions(data || []);
    } catch (err: any) {
      alert(err.message);
      setHistoryTarget(null);
    } finally {
      setLoadingVersions(false);
    }
  };

  const handleRestoreVersion = async (versionNumber: number) => {
    if (!historyTarget) return;
    if (!confirm(`Are you sure you want to restore Version ${versionNumber} as the active file version?`)) return;

    try {
      const response = await apiFetch(`/api/files/${historyTarget.id}/versions/${versionNumber}/restore`, {
        method: 'POST',
      });
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || 'Failed to restore version');
      
      alert(data.message || 'Version restored successfully');
      setHistoryTarget(null);
      fetchDirectory(currentFolderId);
    } catch (err: any) {
      alert(err.message);
    }
  };

  const handleDownloadVersion = async (versionNumber: number, name: string) => {
    if (!historyTarget) return;
    try {
      const response = await apiFetch(`/api/files/${historyTarget.id}/versions/${versionNumber}/download`);
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || 'Failed to download version');

      const link = document.createElement('a');
      link.href = data.download_url;
      link.setAttribute('download', `v${versionNumber}-${name}`);
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
    } catch (err: any) {
      alert(err.message);
    }
  };

  // --- SHARING LOGIC ---
  const handleGenerateShare = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!shareTarget) return;

    try {
      const body: any = { file_id: shareTarget.id };
      
      if (shareExpires !== 'never') {
        body.expires_in_hours = parseInt(shareExpires);
      }
      if (shareMaxDownloads.trim() !== '') {
        body.max_downloads = parseInt(shareMaxDownloads);
      }
      if (sharePassword.trim() !== '') {
        body.password = sharePassword;
      }

      const response = await apiFetch('/api/shares', {
        method: 'POST',
        body: JSON.stringify(body),
      });
      const data = await response.json();

      if (!response.ok) throw new Error(data.error || 'Failed to create share link');

      setGeneratedShareLink(data.share_url);
    } catch (err: any) {
      alert(err.message);
    }
  };

  const copyShareLink = () => {
    if (!generatedShareLink) return;
    navigator.clipboard.writeText(generatedShareLink);
    setCopiedLink(true);
    setTimeout(() => setCopiedLink(false), 2000);
  };

  // --- UPLOAD LOGIC ---
  const calculateSHA256 = async (file: File): Promise<string> => {
    const sliceLimit = 10 * 1024 * 1024;
    const slice = file.slice(0, sliceLimit);
    const arrayBuffer = await slice.arrayBuffer();
    const hashBuffer = await crypto.subtle.digest('SHA-256', arrayBuffer);
    const hashArray = Array.from(new Uint8Array(hashBuffer));
    return hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
  };

  const handleFileUploadInit = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const selectedFiles = e.target.files;
    if (!selectedFiles || selectedFiles.length === 0) return;

    const file = selectedFiles[0];

    if (file.size < 5 * 1024 * 1024) {
      uploadSmallFileDirectly(file);
      return;
    }

    startChunkedUpload(file);
  };

  const uploadSmallFileDirectly = async (file: File) => {
    setActiveUpload({
      file,
      uploadId: null,
      status: 'uploading',
      progress: 0,
      hash: null,
      totalParts: 1,
      activeRequests: []
    });

    const formData = new FormData();
    formData.append('file', file);
    if (currentFolderId) {
      formData.append('folder_id', currentFolderId);
    }

    const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
    const xhr = new XMLHttpRequest();
    
    xhr.open('POST', `${API_URL}/api/files/upload`);
    const token = useAuthStore.getState().accessToken;
    if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`);

    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) {
        const percent = Math.round((event.loaded / event.total) * 100);
        setActiveUpload(prev => prev ? { ...prev, progress: percent } : null);
      }
    };

    xhr.onload = () => {
      const data = JSON.parse(xhr.responseText);
      if (xhr.status >= 200 && xhr.status < 300) {
        if (data.deduplicated) alert("File deduplicated! Loaded instantaneously.");
        fetchDirectory(currentFolderId);
        setActiveUpload(prev => prev ? { ...prev, status: 'completed', progress: 100 } : null);
      } else {
        alert(data.error || 'Upload failed');
        setActiveUpload(prev => prev ? { ...prev, status: 'failed' } : null);
      }
      setTimeout(() => setActiveUpload(null), 2000);
    };

    xhr.onerror = () => {
      alert('Upload network error');
      setActiveUpload(prev => prev ? { ...prev, status: 'failed' } : null);
      setTimeout(() => setActiveUpload(null), 2000);
    };

    xhr.send(formData);
  };

  const startChunkedUpload = async (file: File) => {
    setActiveUpload({
      file,
      uploadId: null,
      status: 'hashing',
      progress: 0,
      hash: null,
      totalParts: 0,
      activeRequests: []
    });

    try {
      const fileHash = await calculateSHA256(file);
      const totalParts = Math.ceil(file.size / CHUNK_SIZE);

      const initResponse = await apiFetch('/api/files/chunks/init', {
        method: 'POST',
        body: JSON.stringify({
          filename: file.name,
          total_size: file.size,
          mime_type: file.type || 'application/octet-stream',
          total_parts: totalParts,
          file_hash: fileHash,
          folder_id: currentFolderId
        })
      });

      const initData = await initResponse.json();
      if (!initResponse.ok) throw new Error(initData.error || 'Failed to initialize chunk session');

      const uploadId = initData.upload_id;

      setActiveUpload({
        file,
        uploadId,
        status: 'uploading',
        progress: 0,
        hash: fileHash,
        totalParts,
        activeRequests: []
      });

      await runChunkQueue(file, uploadId, totalParts, fileHash, []);
    } catch (err: any) {
      console.error(err);
      alert(err.message || 'Chunk initialization failed');
      setActiveUpload(null);
    }
  };

  const runChunkQueue = async (
    file: File, 
    uploadId: string, 
    totalParts: number, 
    fileHash: string,
    completedParts: number[]
  ) => {
    const activeReqs: XMLHttpRequest[] = [];
    setActiveUpload(prev => prev ? { ...prev, uploadId, totalParts, status: 'uploading', activeRequests: activeReqs } : null);

    const partsToUpload: number[] = [];
    for (let i = 1; i <= totalParts; i++) {
      if (!completedParts.includes(i)) partsToUpload.push(i);
    }

    if (partsToUpload.length === 0) {
      await triggerMerge(uploadId);
      return;
    }

    const chunkProgressMap = new Map<number, number>();
    completedParts.forEach(c => chunkProgressMap.set(c, 100));

    let cursor = 0;
    let failed = false;

    return new Promise<void>((resolve, reject) => {
      const uploadNext = () => {
        if (failed) return;

        if (cursor >= partsToUpload.length) {
          if (activeReqs.length === 0) {
            triggerMerge(uploadId).then(resolve).catch(reject);
          }
          return;
        }

        const partNum = partsToUpload[cursor];
        cursor++;

        const startBytes = (partNum - 1) * CHUNK_SIZE;
        const endBytes = Math.min(partNum * CHUNK_SIZE, file.size);
        const chunkBlob = file.slice(startBytes, endBytes);

        const formData = new FormData();
        formData.append('upload_id', uploadId);
        formData.append('chunk_number', String(partNum));
        formData.append('chunk', chunkBlob, `${file.name}.part${partNum}`);

        const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
        const xhr = new XMLHttpRequest();
        xhr.open('POST', `${API_URL}/api/files/chunks/upload`);
        
        const token = useAuthStore.getState().accessToken;
        if (token) xhr.setRequestHeader('Authorization', `Bearer ${token}`);

        activeReqs.push(xhr);

        xhr.upload.onprogress = (event) => {
          if (event.lengthComputable) {
            const partPercent = Math.round((event.loaded / event.total) * 100);
            chunkProgressMap.set(partNum, partPercent);
            
            let totalDone = 0;
            chunkProgressMap.forEach(val => totalDone += val);
            const overallProgress = Math.round(totalDone / totalParts);
            
            setActiveUpload(prev => prev && prev.status === 'uploading' ? { ...prev, progress: overallProgress } : prev);
          }
        };

        xhr.onload = () => {
          const idx = activeReqs.indexOf(xhr);
          if (idx !== -1) activeReqs.splice(idx, 1);

          if (xhr.status >= 200 && xhr.status < 300) {
            chunkProgressMap.set(partNum, 100);
            uploadNext();
          } else {
            failed = true;
            abortAll();
            alert(`Upload failed at chunk ${partNum}`);
            setActiveUpload(prev => prev ? { ...prev, status: 'failed' } : null);
            reject(new Error('Chunk upload failed'));
          }
        };

        xhr.onerror = () => {
          failed = true;
          abortAll();
          alert(`Network error during chunk ${partNum} upload.`);
          setActiveUpload(prev => prev ? { ...prev, status: 'failed' } : null);
          reject(new Error('Network error'));
        };

        xhr.send(formData);
      };

      const abortAll = () => {
        activeReqs.forEach(req => req.abort());
        activeReqs.length = 0;
      };

      for (let i = 0; i < Math.min(CONCURRENCY_LIMIT, partsToUpload.length); i++) {
        uploadNext();
      }
    });
  };

  const triggerMerge = async (uploadId: string) => {
    setActiveUpload(prev => prev ? { ...prev, status: 'merging', progress: 100 } : null);

    try {
      const response = await apiFetch('/api/files/chunks/merge', {
        method: 'POST',
        body: JSON.stringify({ upload_id: uploadId })
      });

      const data = await response.json();
      if (!response.ok) throw new Error(data.error || 'Failed to merge chunks');

      if (data.deduplicated) alert("File deduplicated! Finalized instantly.");

      fetchDirectory(currentFolderId);
      setActiveUpload(prev => prev ? { ...prev, status: 'completed' } : null);
      setTimeout(() => setActiveUpload(null), 2500);
    } catch (err: any) {
      alert(err.message || 'Integrity or merge error');
      setActiveUpload(prev => prev ? { ...prev, status: 'failed' } : null);
    }
  };

  const handlePauseUpload = () => {
    if (!activeUpload || activeUpload.status !== 'uploading') return;
    activeUpload.activeRequests.forEach(req => req.abort());
    setActiveUpload(prev => prev ? { ...prev, status: 'paused', activeRequests: [] } : null);
  };

  const handleResumeUpload = async () => {
    if (!activeUpload || activeUpload.status !== 'paused' || !activeUpload.uploadId) return;

    try {
      setActiveUpload(prev => prev ? { ...prev, status: 'uploading' } : null);
      const response = await apiFetch(`/api/files/chunks/status?upload_id=${activeUpload.uploadId}`);
      const data = await response.json();

      if (!response.ok) throw new Error(data.error || 'Failed to sync status');

      const uploadedChunks: number[] = data.uploaded_chunks || [];
      const total = activeUpload.totalParts;
      const startPercent = Math.round((uploadedChunks.length / total) * 100);
      setActiveUpload(prev => prev ? { ...prev, progress: startPercent } : null);

      await runChunkQueue(activeUpload.file, activeUpload.uploadId, total, activeUpload.hash || '', uploadedChunks);
    } catch (err: any) {
      alert(err.message || 'Failed to resume upload session');
      setActiveUpload(prev => prev ? { ...prev, status: 'failed' } : null);
    }
  };

  const handleCancelUpload = () => {
    if (activeUpload) activeUpload.activeRequests.forEach(req => req.abort());
    setActiveUpload(null);
  };

  const navigateToFolder = (folderId: string | null, folderName: string) => {
    if (folderId === null) {
      setBreadcrumbs([{ id: null, name: 'Root' }]);
    } else {
      const idx = breadcrumbs.findIndex(item => item.id === folderId);
      if (idx !== -1) {
        setBreadcrumbs(breadcrumbs.slice(0, idx + 1));
      } else {
        setBreadcrumbs([...breadcrumbs, { id: folderId, name: folderName }]);
      }
    }
    setCurrentFolderId(folderId);
    setSearchQuery(''); // Clear search on folder change
  };

  const formatBytes = (bytes: number, decimals = 2) => {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
  };

  if (!user) return null;

  const isSearching = searchQuery.trim() !== '';

  return (
    <div className="flex h-screen bg-slate-950 text-slate-100 overflow-hidden relative">
      {/* Sidebar */}
      <aside className="w-64 border-r border-slate-900 bg-slate-900/10 flex flex-col justify-between hidden md:flex">
        <div className="p-6">
          <div className="flex items-center gap-2 font-bold text-xl tracking-tight text-white mb-8">
            <HardDrive className="h-6 w-6 text-indigo-500" />
            <span>CloudStore</span>
          </div>
          <nav className="space-y-1">
            <button
              onClick={() => navigateToFolder(null, 'Root')}
              className={`flex items-center gap-3 w-full px-3 py-2.5 rounded-lg font-medium text-sm transition-colors cursor-pointer text-left ${
                currentFolderId === null && !isSearching
                  ? 'bg-indigo-600/15 text-indigo-400 border border-indigo-500/15' 
                  : 'text-slate-400 hover:bg-slate-900 hover:text-white'
              }`}
            >
              <Folder className="h-4 w-4" />
              All Files
            </button>
            <a href="#" className="flex items-center gap-3 px-3 py-2.5 rounded-lg text-slate-400 hover:bg-slate-900 hover:text-white font-medium text-sm transition-colors">
              <Shield className="h-4 w-4" />
              Shared Links
            </a>
          </nav>
        </div>

        {/* User Card */}
        <div className="p-4 border-t border-slate-900">
          <div className="flex items-center gap-3 px-3 py-2.5 rounded-xl bg-slate-900/50 border border-slate-800/50 mb-3">
            <div className="h-8 w-8 rounded-lg bg-indigo-600/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400">
              <User className="h-4 w-4" />
            </div>
            <div className="overflow-hidden">
              <p className="text-xs font-semibold text-white truncate">{user.email}</p>
              <span className="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium bg-indigo-400/10 text-indigo-400 border border-indigo-400/25 mt-0.5">
                {user.role}
              </span>
            </div>
          </div>
          <button
            onClick={handleLogout}
            className="flex items-center justify-center gap-2 w-full px-3 py-2 rounded-lg bg-slate-900 hover:bg-slate-800 border border-slate-800 text-slate-300 hover:text-white text-sm font-medium transition-colors cursor-pointer"
          >
            <LogOut className="h-4 w-4" />
            Sign Out
          </button>
        </div>
      </aside>

      {/* Main Content Area */}
      <main className="flex-1 flex flex-col overflow-hidden">
        {/* Header toolbar */}
        <header className="border-b border-slate-900 px-6 py-4 flex flex-col sm:flex-row gap-4 items-stretch sm:items-center justify-between bg-slate-950/40 backdrop-blur-md relative z-15">
          <div className="flex items-center gap-4 flex-1">
            <div className="flex items-center gap-2 text-sm text-slate-400 overflow-x-auto whitespace-nowrap scrollbar-none py-1 max-w-[200px] sm:max-w-xs">
              {breadcrumbs.map((item, idx) => (
                <React.Fragment key={idx}>
                  {idx > 0 && <ChevronRight className="h-3.5 w-3.5 flex-shrink-0 text-slate-600" />}
                  <button
                    onClick={() => navigateToFolder(item.id, item.name)}
                    className={`hover:text-white font-medium transition-colors cursor-pointer ${
                      idx === breadcrumbs.length - 1 && !isSearching ? 'text-white font-bold' : ''
                    }`}
                  >
                    {item.name}
                  </button>
                </React.Fragment>
              ))}
              {isSearching && (
                <>
                  <ChevronRight className="h-3.5 w-3.5 flex-shrink-0 text-slate-650" />
                  <span className="text-white font-bold">Search results</span>
                </>
              )}
            </div>

            {/* Premium debounced search bar */}
            <div className="relative flex-1 max-w-md hidden sm:block">
              <Search className="absolute left-3 top-2.5 h-4 w-4 text-slate-500" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full bg-slate-900 border border-slate-800 text-white pl-10 pr-4 py-2 rounded-lg text-sm placeholder-slate-500 focus:border-indigo-500 focus:outline-none transition-colors"
                placeholder="Search files by name, AI tags, or summaries..."
              />
              {searching && <Loader2 className="absolute right-3 top-2.5 h-4 w-4 animate-spin text-indigo-500" />}
            </div>
          </div>

          <div className="flex items-center gap-3 justify-between sm:justify-start">
            {/* Mobile search input */}
            <div className="relative flex-1 sm:hidden mr-2 max-w-[150px]">
              <Search className="absolute left-2.5 top-2 h-3.5 w-3.5 text-slate-500" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full bg-slate-900 border border-slate-800 text-white pl-8 pr-2 py-1.5 rounded-lg text-xs placeholder-slate-600 focus:outline-none"
                placeholder="Search..."
              />
            </div>

            {/* Real-time notification Bell */}
            <div className="relative">
              <button
                onClick={() => setIsNotificationsOpen(!isNotificationsOpen)}
                className="p-2 bg-slate-905 hover:bg-slate-900 border border-slate-900 rounded-lg text-slate-400 hover:text-white transition-colors relative cursor-pointer"
                title="Notifications"
              >
                <Bell className="h-4 w-4" />
                {unreadCount > 0 && (
                  <span className="absolute -top-1 -right-1 flex h-4 w-4 items-center justify-center rounded-full bg-indigo-600 text-[9px] font-bold text-white ring-2 ring-slate-950 animate-pulse">
                    {unreadCount}
                  </span>
                )}
              </button>

              {/* Notification Popover Dropdown */}
              {isNotificationsOpen && (
                <div className="absolute right-0 mt-2 w-80 bg-slate-900 border border-slate-800 rounded-2xl p-4 shadow-2xl z-30 animate-scale-in">
                  <div className="flex items-center justify-between border-b border-slate-800 pb-2 mb-3">
                    <span className="text-xs font-bold text-white uppercase tracking-wider">Live Activity Log</span>
                    {notifications.length > 0 && (
                      <button 
                        onClick={clearAllNotifications}
                        className="text-[10px] text-slate-400 hover:text-white transition-colors cursor-pointer font-semibold"
                      >
                        Clear All
                      </button>
                    )}
                  </div>
                  <div className="max-h-64 overflow-y-auto space-y-2 pr-1">
                    {notifications.length === 0 ? (
                      <div className="text-center py-6">
                        <BellOff className="h-6 w-6 text-slate-650 mx-auto mb-2" />
                        <p className="text-xs text-slate-400">No events logged yet.</p>
                      </div>
                    ) : (
                      notifications.map((n) => (
                        <div 
                          key={n.id} 
                          onClick={() => markAsRead(n.id)}
                          className={`p-2.5 rounded-lg border text-left cursor-pointer transition-colors ${
                            n.read 
                              ? 'bg-slate-950/20 border-slate-950/40 opacity-60' 
                              : 'bg-indigo-650/5 border-indigo-500/15 hover:bg-indigo-600/10'
                          }`}
                        >
                          <div className="flex items-start gap-2">
                            {n.type === 'success' && <CheckCircle className="h-3.5 w-3.5 text-emerald-400 flex-shrink-0 mt-0.5" />}
                            {n.type === 'error' && <AlertCircle className="h-3.5 w-3.5 text-red-400 flex-shrink-0 mt-0.5" />}
                            {n.type === 'info' && <Info className="h-3.5 w-3.5 text-indigo-400 flex-shrink-0 mt-0.5" />}
                            <div className="overflow-hidden">
                              <h4 className="text-xs font-bold text-white truncate">{n.title}</h4>
                              <p className="text-[11px] text-slate-300 mt-0.5 leading-relaxed">{n.message}</p>
                              <span className="text-[9px] text-slate-500 mt-1 block font-mono">
                                {new Date(n.timestamp).toLocaleTimeString()}
                              </span>
                            </div>
                          </div>
                        </div>
                      ))
                    )}
                  </div>
                </div>
              )}
            </div>

            <button
              onClick={() => setIsFolderModalOpen(true)}
              className="flex items-center justify-center gap-2 bg-slate-900 hover:bg-slate-800 border border-slate-800 text-white px-4 py-2 rounded-lg text-sm font-semibold transition-colors cursor-pointer"
            >
              <FolderPlus className="h-4 w-4" />
              New Folder
            </button>

            <button
              onClick={() => fileInputRef.current?.click()}
              className="flex items-center justify-center gap-2 bg-indigo-600 hover:bg-indigo-500 text-white px-4 py-2 rounded-lg text-sm font-semibold transition-all cursor-pointer shadow-lg shadow-indigo-500/10"
            >
              <Upload className="h-4 w-4" />
              Upload File
            </button>
            <input
              type="file"
              ref={fileInputRef}
              onChange={handleFileUploadInit}
              className="hidden"
            />
          </div>
        </header>

        {/* Upload status popup */}
        {activeUpload && (
          <div className="bg-slate-900 border border-slate-800 px-6 py-4 flex items-center justify-between gap-6 m-6 rounded-xl animate-fade-in relative z-20 shadow-2xl">
            <div className="flex-1 overflow-hidden">
              <div className="flex items-center justify-between gap-4">
                <p className="text-sm font-semibold text-white truncate">
                  {activeUpload.status === 'hashing' && `Calculating hash for ${activeUpload.file.name}...`}
                  {activeUpload.status === 'uploading' && `Uploading ${activeUpload.file.name}...`}
                  {activeUpload.status === 'paused' && `Paused uploading ${activeUpload.file.name}`}
                  {activeUpload.status === 'merging' && `Merging chunks for ${activeUpload.file.name}...`}
                  {activeUpload.status === 'completed' && `Successfully uploaded ${activeUpload.file.name}!`}
                  {activeUpload.status === 'failed' && `Failed to upload ${activeUpload.file.name}`}
                </p>
                <span className="text-xs font-bold text-indigo-400">{activeUpload.progress}%</span>
              </div>
              <div className="w-full bg-slate-950 rounded-full h-2 mt-2.5 border border-slate-800 overflow-hidden">
                <div 
                  className="bg-indigo-600 h-2 rounded-full transition-all duration-200" 
                  style={{ width: `${activeUpload.progress}%` }}
                ></div>
              </div>
            </div>
            
            {/* Control buttons */}
            <div className="flex items-center gap-2">
              {activeUpload.status === 'uploading' && (
                <button 
                  onClick={handlePauseUpload}
                  className="p-2 bg-slate-800 hover:bg-slate-750 border border-slate-700 rounded-lg text-slate-300 hover:text-white transition-colors cursor-pointer"
                  title="Pause Upload"
                >
                  <Pause className="h-4 w-4" />
                </button>
              )}
              {activeUpload.status === 'paused' && (
                <button 
                  onClick={handleResumeUpload}
                  className="p-2 bg-indigo-600 hover:bg-indigo-500 text-white transition-colors cursor-pointer"
                  title="Resume Upload"
                >
                  <Play className="h-4 w-4" />
                </button>
              )}
              {activeUpload.status !== 'completed' && activeUpload.status !== 'merging' && (
                <button 
                  onClick={handleCancelUpload}
                  className="p-2 bg-slate-800 hover:bg-red-500/20 border border-slate-700 hover:border-red-500/35 rounded-lg text-slate-400 hover:text-red-400 transition-colors cursor-pointer"
                  title="Cancel Upload"
                >
                  <X className="h-4 w-4" />
                </button>
              )}
            </div>
          </div>
        )}

        {/* File Browser Area */}
        <div className="flex-1 overflow-y-auto p-6 bg-slate-950">
          {error && (
            <div className="mb-6 rounded-lg bg-red-500/10 border border-red-500/30 p-4 text-sm text-red-400">
              {error}
            </div>
          )}

          {loading && (
            <div className="flex h-64 items-center justify-center">
              <Loader2 className="h-8 w-8 animate-spin text-indigo-500" />
            </div>
          )}

          {/* Render Search Results Grid if searching */}
          {!loading && isSearching ? (
            <div>
              <div className="flex items-center justify-between border-b border-slate-900 pb-3 mb-6">
                <h2 className="text-lg font-bold text-white flex items-center gap-2">
                  <Search className="h-4 w-4 text-indigo-400" /> Search Results for &ldquo;{searchQuery}&rdquo;
                </h2>
                <span className="text-xs text-slate-500 font-semibold">{searchResults.length} file(s) found</span>
              </div>

              {searchResults.length === 0 ? (
                <div className="flex flex-col items-center justify-center h-80 text-center">
                  <File className="h-10 w-10 text-slate-650 mb-3" />
                  <h3 className="text-sm font-semibold text-slate-350">No files matched your query</h3>
                  <p className="text-xs text-slate-500 mt-1">Check for spelling or try searching generic keywords.</p>
                </div>
              ) : (
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  {searchResults.map((file) => (
                    <div 
                      key={file.id}
                      className="flex flex-col justify-between p-5 bg-slate-900/30 hover:bg-slate-900/60 border border-slate-900 hover:border-slate-800 rounded-2xl group transition-all duration-200"
                    >
                      <div className="flex items-start gap-4 overflow-hidden">
                        <File className="h-8 w-8 text-indigo-400 flex-shrink-0 mt-1" />
                        <div className="overflow-hidden flex-1">
                          <span 
                            onClick={() => handleDownloadFile(file.id, file.name)}
                            className="text-base font-bold text-slate-200 group-hover:text-white cursor-pointer truncate block"
                          >
                            {file.name}
                          </span>
                          <span className="text-xs text-slate-500 mt-0.5 block">
                            {formatBytes(file.size)} • v{file.current_version} • {new Date(file.created_at).toLocaleDateString()}
                          </span>

                          {/* AI Tags rendering */}
                          {file.tags && file.tags.length > 0 && (
                            <div className="flex flex-wrap gap-1.5 mt-2.5">
                              {file.tags.map((tag, idx) => (
                                <span 
                                  key={idx}
                                  className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[10px] font-bold bg-indigo-500/10 text-indigo-400 border border-indigo-500/15"
                                >
                                  <Tag className="h-2 w-2" />
                                  {tag}
                                </span>
                              ))}
                            </div>
                          )}

                          {/* AI Bullet summary rendering */}
                          {file.summary && (
                            <div className="mt-3 bg-slate-950/40 border border-slate-900 p-3 rounded-xl">
                              <span className="text-[10px] font-bold text-indigo-400 uppercase tracking-wider flex items-center gap-1.5 mb-1.5">
                                <FileText className="h-3 w-3" />
                                AI Generated Summary
                              </span>
                              <p className="text-[11px] text-slate-400 leading-normal whitespace-pre-line">
                                {file.summary}
                              </p>
                            </div>
                          )}
                        </div>
                      </div>

                      <div className="flex justify-end gap-1 border-t border-slate-900/60 mt-4 pt-3 opacity-0 group-hover:opacity-100 transition-opacity">
                        <button
                          onClick={() => handleDownloadFile(file.id, file.name)}
                          className="p-1.5 hover:bg-slate-800 rounded-md text-slate-400 hover:text-white transition-colors cursor-pointer"
                          title="Download"
                        >
                          <Download className="h-4 w-4" />
                        </button>
                        <button
                          onClick={() => openVersionHistory(file)}
                          className="p-1.5 hover:bg-slate-800 rounded-md text-slate-400 hover:text-white transition-colors cursor-pointer"
                          title="History"
                        >
                          <History className="h-4 w-4" />
                        </button>
                        <button
                          onClick={() => {
                            setShareTarget(file);
                            setGeneratedShareLink(null);
                            setSharePassword('');
                            setShareMaxDownloads('');
                            setShareExpires('24');
                          }}
                          className="p-1.5 hover:bg-slate-800 rounded-md text-slate-400 hover:text-white transition-colors cursor-pointer"
                          title="Share"
                        >
                          <Share2 className="h-4 w-4" />
                        </button>
                        <button
                          onClick={() => handleDeleteFile(file.id)}
                          className="p-1.5 hover:bg-red-500/20 rounded-md text-slate-400 hover:text-red-400 transition-colors cursor-pointer"
                          title="Trash"
                        >
                          <Trash className="h-4 w-4" />
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          ) : (
            /* Render Standard Directory Grid if not searching */
            !loading && (
              <div className="space-y-6">
                {/* Folders List */}
                {folders.length > 0 && (
                  <div>
                    <h3 className="text-xs font-bold uppercase tracking-wider text-slate-500 mb-3">Folders</h3>
                    <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
                      {folders.map((folder) => (
                        <div 
                          key={folder.id}
                          className="flex items-center justify-between p-4 bg-slate-900/40 hover:bg-slate-900 border border-slate-900 hover:border-slate-800 rounded-xl group transition-all duration-200"
                        >
                          <div 
                            onClick={() => navigateToFolder(folder.id, folder.name)}
                            className="flex items-center gap-3 cursor-pointer overflow-hidden flex-1 py-1"
                          >
                            <Folder className="h-5 w-5 text-indigo-400 flex-shrink-0" />
                            <span className="text-sm font-semibold text-slate-200 group-hover:text-white truncate">
                              {folder.name}
                            </span>
                          </div>
                          <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity pl-2">
                            <button
                              onClick={() => {
                                setRenameTarget({ id: folder.id, type: 'folder', currentName: folder.name });
                                setRenameNewName(folder.name);
                              }}
                              className="p-1.5 hover:bg-slate-800 rounded-md text-slate-400 hover:text-white transition-colors cursor-pointer"
                              title="Rename"
                            >
                              <Edit3 className="h-3.5 w-3.5" />
                            </button>
                            <button
                              onClick={() => handleDeleteFolder(folder.id)}
                              className="p-1.5 hover:bg-red-500/20 rounded-md text-slate-400 hover:text-red-400 transition-colors cursor-pointer"
                              title="Delete"
                            >
                              <Trash className="h-3.5 w-3.5" />
                            </button>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Files List */}
                {files.length > 0 && (
                  <div>
                    <h3 className="text-xs font-bold uppercase tracking-wider text-slate-500 mb-3">Files</h3>
                    <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
                      {files.map((file) => (
                        <div 
                          key={file.id}
                          className="flex flex-col justify-between p-4 bg-slate-900/40 hover:bg-slate-900 border border-slate-900 hover:border-slate-800 rounded-xl group transition-all duration-200"
                        >
                          <div className="flex items-start justify-between gap-3 overflow-hidden">
                            <div 
                              onClick={() => handleDownloadFile(file.id, file.name)}
                              className="flex items-start gap-3 cursor-pointer overflow-hidden flex-1 py-1"
                            >
                              <File className="h-5 w-5 text-indigo-400 flex-shrink-0 mt-0.5" />
                              <div className="overflow-hidden">
                                <span className="text-sm font-semibold text-slate-200 group-hover:text-white block truncate">
                                  {file.name}
                                </span>
                                <span className="text-xs text-slate-500 mt-0.5 block">
                                  {formatBytes(file.size)}
                                </span>
                              </div>
                            </div>
                          </div>
                          
                          <div className="flex items-center justify-between border-t border-slate-900/50 mt-4 pt-3 opacity-0 group-hover:opacity-100 transition-opacity">
                            <span className="text-[10px] text-slate-500 font-mono uppercase">v{file.current_version}</span>
                            <div className="flex items-center gap-1">
                              <button
                                onClick={() => handleDownloadFile(file.id, file.name)}
                                className="p-1.5 hover:bg-slate-800 rounded-md text-slate-400 hover:text-white transition-colors cursor-pointer"
                                title="Download"
                              >
                                <Download className="h-3.5 w-3.5" />
                              </button>
                              <button
                                onClick={() => openVersionHistory(file)}
                                className="p-1.5 hover:bg-slate-800 rounded-md text-slate-400 hover:text-white transition-colors cursor-pointer"
                                title="Version History"
                              >
                                <History className="h-3.5 w-3.5" />
                              </button>
                              <button
                                onClick={() => {
                                  setShareTarget(file);
                                  setGeneratedShareLink(null);
                                  setSharePassword('');
                                  setShareMaxDownloads('');
                                  setShareExpires('24');
                                }}
                                className="p-1.5 hover:bg-slate-800 rounded-md text-slate-400 hover:text-white transition-colors cursor-pointer"
                                title="Share File"
                              >
                                <Share2 className="h-3.5 w-3.5" />
                              </button>
                              <button
                                onClick={() => {
                                  setRenameTarget({ id: file.id, type: 'file', currentName: file.name });
                                  setRenameNewName(file.name);
                                }}
                                className="p-1.5 hover:bg-slate-800 rounded-md text-slate-400 hover:text-white transition-colors cursor-pointer"
                                title="Rename"
                              >
                                <Edit3 className="h-3.5 w-3.5" />
                              </button>
                              <button
                                onClick={() => handleDeleteFile(file.id)}
                                className="p-1.5 hover:bg-red-500/20 rounded-md text-slate-400 hover:text-red-400 transition-colors cursor-pointer"
                                title="Trash"
                              >
                                <Trash className="h-3.5 w-3.5" />
                              </button>
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )
          )}
        </div>
      </main>

      {/* MODAL: Version History */}
      {historyTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm px-4">
          <div className="w-full max-w-2xl bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-2xl animate-scale-in">
            <div className="flex items-center justify-between mb-4 pb-3 border-b border-slate-800">
              <div>
                <h3 className="text-lg font-bold text-white">Version History</h3>
                <p className="text-xs text-slate-400 mt-0.5 truncate max-w-md">{historyTarget.name}</p>
              </div>
              <button 
                onClick={() => setHistoryTarget(null)}
                className="p-1 hover:bg-slate-800 rounded-md text-slate-400 hover:text-white transition-colors cursor-pointer"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            
            {loadingVersions ? (
              <div className="flex h-32 items-center justify-center">
                <Loader2 className="h-6 w-6 animate-spin text-indigo-500" />
              </div>
            ) : (
              <div className="max-h-96 overflow-y-auto space-y-2 pr-1">
                {versions.map((ver) => (
                  <div 
                    key={ver.id}
                    className={`flex items-center justify-between p-3 rounded-xl border transition-colors ${
                      ver.version_number === historyTarget.current_version
                        ? 'bg-indigo-600/5 border-indigo-500/30'
                        : 'bg-slate-950/40 border-slate-900 hover:border-slate-800'
                    }`}
                  >
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-bold text-white">Version {ver.version_number}</span>
                        {ver.version_number === historyTarget.current_version && (
                          <span className="text-[9px] font-bold bg-indigo-500 text-white px-1.5 py-0.5 rounded uppercase tracking-wider">
                            Active
                          </span>
                        )}
                      </div>
                      <div className="flex items-center gap-4 text-xs text-slate-400 mt-1">
                        <span>{formatBytes(ver.size)}</span>
                        <span>•</span>
                        <span>{new Date(ver.created_at).toLocaleString()}</span>
                      </div>
                    </div>

                    <div className="flex items-center gap-1">
                      <button
                        onClick={() => handleDownloadVersion(ver.version_number, historyTarget.name)}
                        className="p-2 hover:bg-slate-800 rounded-lg text-slate-400 hover:text-white transition-colors cursor-pointer"
                        title="Download version"
                      >
                        <Download className="h-4 w-4" />
                      </button>
                      {ver.version_number !== historyTarget.current_version && (
                        <button
                          onClick={() => handleRestoreVersion(ver.version_number)}
                          className="px-3 py-1.5 bg-slate-900 hover:bg-slate-800 border border-slate-800 text-xs text-indigo-400 hover:text-indigo-300 font-semibold rounded-lg transition-colors cursor-pointer"
                        >
                          Restore
                        </button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* MODAL: Share File */}
      {shareTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm px-4">
          <div className="w-full max-w-md bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-2xl animate-scale-in">
            <div className="flex items-center justify-between mb-4 pb-3 border-b border-slate-800">
              <div>
                <h3 className="text-lg font-bold text-white">Share Secure Link</h3>
                <p className="text-xs text-slate-400 mt-0.5 truncate max-w-xs">{shareTarget.name}</p>
              </div>
              <button 
                onClick={() => setShareTarget(null)}
                className="p-1 hover:bg-slate-800 rounded-md text-slate-400 hover:text-white transition-colors cursor-pointer"
              >
                <X className="h-5 w-5" />
              </button>
            </div>

            {!generatedShareLink ? (
              <form onSubmit={handleGenerateShare} className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-xs font-semibold text-slate-400 uppercase tracking-wider mb-1">Expires in</label>
                    <select
                      value={shareExpires}
                      onChange={(e) => setShareExpires(e.target.value)}
                      className="block w-full rounded-lg bg-slate-950 border border-slate-800 px-3 py-2 text-white text-sm focus:border-indigo-500 focus:outline-none transition-colors"
                    >
                      <option value="1">1 Hour</option>
                      <option value="24">24 Hours</option>
                      <option value="168">7 Days</option>
                      <option value="never">Never</option>
                    </select>
                  </div>
                  <div>
                    <label className="block text-xs font-semibold text-slate-400 uppercase tracking-wider mb-1">Max Downloads</label>
                    <input
                      type="number"
                      value={shareMaxDownloads}
                      onChange={(e) => setShareMaxDownloads(e.target.value)}
                      placeholder="Unlimited"
                      className="block w-full rounded-lg bg-slate-950 border border-slate-800 px-3 py-2 text-white text-sm focus:border-indigo-500 focus:outline-none transition-colors"
                    />
                  </div>
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-400 uppercase tracking-wider mb-1 flex items-center gap-1">
                    <Lock className="h-3 w-3" /> Link Password
                  </label>
                  <input
                    type="password"
                    value={sharePassword}
                    onChange={(e) => setSharePassword(e.target.value)}
                    placeholder="Optional plain password"
                    className="block w-full rounded-lg bg-slate-950 border border-slate-800 px-3 py-2 text-white text-sm focus:border-indigo-500 focus:outline-none transition-colors"
                  />
                </div>

                <div className="flex justify-end gap-3 pt-2">
                  <button
                    type="button"
                    onClick={() => setShareTarget(null)}
                    className="px-4 py-2 border border-slate-850 hover:bg-slate-800 rounded-lg text-sm text-slate-300 hover:text-white font-medium transition-colors cursor-pointer"
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 rounded-lg text-sm text-white font-semibold transition-colors cursor-pointer"
                  >
                    Generate Link
                  </button>
                </div>
              </form>
            ) : (
              <div className="space-y-4">
                <div>
                  <label className="block text-xs font-semibold text-slate-400 uppercase tracking-wider mb-1">Share Link Generated</label>
                  <div className="flex gap-2">
                    <input
                      type="text"
                      readOnly
                      value={generatedShareLink}
                      className="flex-1 rounded-lg bg-slate-950 border border-slate-800 px-3 py-2 text-indigo-400 text-sm focus:outline-none select-all font-mono"
                    />
                    <button
                      onClick={copyShareLink}
                      className="px-3 bg-indigo-600 hover:bg-indigo-500 rounded-lg text-white flex items-center justify-center transition-colors cursor-pointer"
                    >
                      {copiedLink ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                    </button>
                  </div>
                </div>

                <div className="rounded-lg bg-indigo-500/5 border border-indigo-500/10 p-3 flex gap-2">
                  <Info className="h-4 w-4 text-indigo-400 flex-shrink-0 mt-0.5" />
                  <p className="text-xs text-slate-400 leading-normal">
                    This link can be accessed by anyone without an account. {sharePassword && "Access requires the password you configured."}
                  </p>
                </div>

                <div className="flex justify-end pt-2">
                  <button
                    onClick={() => setShareTarget(null)}
                    className="px-4 py-2 bg-slate-900 hover:bg-slate-800 border border-slate-800 rounded-lg text-sm text-white font-semibold transition-colors cursor-pointer"
                  >
                    Close
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* MODAL: Create Folder */}
      {isFolderModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm px-4">
          <div className="w-full max-w-md bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-2xl animate-scale-in">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-bold text-white">Create New Folder</h3>
              <button 
                onClick={() => setIsFolderModalOpen(false)}
                className="p-1 hover:bg-slate-800 rounded-md text-slate-400 hover:text-white transition-colors cursor-pointer"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            <form onSubmit={handleCreateFolder} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-1">Folder Name</label>
                <input
                  type="text"
                  required
                  value={newFolderName}
                  onChange={(e) => setNewFolderName(e.target.value)}
                  className="block w-full rounded-lg bg-slate-950 border border-slate-800 px-3 py-2 text-white placeholder-slate-600 focus:border-indigo-500 focus:outline-none text-sm transition-colors"
                  placeholder="Documents, Images, Project Assets..."
                  autoFocus
                />
              </div>
              <div className="flex justify-end gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => setIsFolderModalOpen(false)}
                  className="px-4 py-2 border border-slate-850 hover:bg-slate-800 rounded-lg text-sm text-slate-300 hover:text-white font-medium transition-colors cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 rounded-lg text-sm text-white font-semibold transition-colors cursor-pointer"
                >
                  Create
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* MODAL: Rename */}
      {renameTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm px-4">
          <div className="w-full max-w-md bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-2xl animate-scale-in">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-lg font-bold text-white">Rename {renameTarget.type === 'file' ? 'File' : 'Folder'}</h3>
              <button 
                onClick={() => setRenameTarget(null)}
                className="p-1 hover:bg-slate-800 rounded-md text-slate-400 hover:text-white transition-colors cursor-pointer"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            <form onSubmit={handleRename} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-slate-300 mb-1">New Name</label>
                <input
                  type="text"
                  required
                  value={renameNewName}
                  onChange={(e) => setRenameNewName(e.target.value)}
                  className="block w-full rounded-lg bg-slate-950 border border-slate-800 px-3 py-2 text-white placeholder-slate-650 focus:border-indigo-500 focus:outline-none text-sm transition-colors"
                  placeholder="Enter new name"
                  autoFocus
                />
              </div>
              <div className="flex justify-end gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => setRenameTarget(null)}
                  className="px-4 py-2 border border-slate-850 hover:bg-slate-800 rounded-lg text-sm text-slate-300 hover:text-white font-medium transition-colors cursor-pointer"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 rounded-lg text-sm text-white font-semibold transition-colors cursor-pointer"
                >
                  Rename
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
