import Link from "next/link";
import { HardDrive, Shield, Share2, Layers, Cpu, CheckCircle } from "lucide-react";

export default function Home() {
	return (
		<div className="flex flex-col min-h-screen bg-slate-950 text-slate-100 overflow-hidden relative">
			{/* Decorative grid background */}
			<div className="absolute inset-0 bg-[linear-gradient(to_right,#0f172a_1px,transparent_1px),linear-gradient(to_bottom,#0f172a_1px,transparent_1px)] bg-[size:4rem_4rem] [mask-image:radial-gradient(ellipse_60%_50%_at_50%_0%,#000_70%,transparent_100%)]"></div>

			{/* Top glow */}
			<div className="absolute top-0 left-1/2 -translate-x-1/2 w-[600px] h-[300px] bg-indigo-500/10 rounded-full blur-[120px] pointer-events-none"></div>

			{/* Header */}
			<header className="relative z-10 border-b border-slate-900 bg-slate-950/50 backdrop-blur-md px-6 py-4 flex items-center justify-between max-w-7xl mx-auto w-full">
				<div className="flex items-center gap-2 font-bold text-xl tracking-tight text-white">
					<HardDrive className="h-6 w-6 text-indigo-500" />
					<span>CloudStore</span>
				</div>
				<div className="flex items-center gap-4">
					<Link href="/login" className="text-sm font-medium text-slate-300 hover:text-white transition-colors">
						Sign In
					</Link>
					<Link href="/register" className="text-sm font-semibold text-white bg-indigo-600 hover:bg-indigo-500 px-4 py-2 rounded-lg transition-all shadow-md shadow-indigo-500/10">
						Get Started
					</Link>
				</div>
			</header>

			{/* Hero Section */}
			<main className="flex-1 relative z-10 max-w-7xl mx-auto px-6 py-20 flex flex-col items-center justify-center text-center">
				<div className="inline-flex items-center gap-2 bg-slate-900 border border-slate-800 rounded-full px-4 py-1.5 text-xs text-indigo-400 mb-6 font-semibold animate-pulse">
					<Cpu className="h-3.5 w-3.5" />
					Powered by Local AI & Distributed Storage
				</div>
				
				<h1 className="text-5xl md:text-7xl font-extrabold text-transparent bg-clip-text bg-gradient-to-r from-white via-slate-100 to-indigo-400 leading-tight tracking-tight max-w-4xl">
					Secure, Distributed Cloud Storage for Your Files
				</h1>
				
				<p className="mt-6 text-lg md:text-xl text-slate-400 max-w-2xl leading-relaxed">
					Experience lightning-fast object storage with local AI-powered document intelligence. Instantly upload, search, version, and share your files securely on a highly optimized distributed storage engine.
				</p>

				<div className="mt-10 flex flex-col sm:flex-row gap-4 justify-center w-full max-w-md">
					<Link href="/register" className="flex-1 bg-indigo-600 hover:bg-indigo-500 text-white font-semibold py-3 px-6 rounded-xl transition-all shadow-lg shadow-indigo-500/20 text-center">
						Create Free Account
					</Link>
					<Link href="/login" className="flex-1 bg-slate-900 hover:bg-slate-800 border border-slate-800 text-white font-semibold py-3 px-6 rounded-xl transition-colors text-center">
						Access Console
					</Link>
				</div>

				{/* Feature Grid */}
				<div className="mt-28 grid grid-cols-1 md:grid-cols-4 gap-6 w-full text-left">
					<div className="bg-slate-900/40 border border-slate-900 p-6 rounded-2xl backdrop-blur-sm">
						<div className="h-10 w-10 bg-indigo-600/10 rounded-xl flex items-center justify-center text-indigo-400 mb-4 border border-indigo-500/10">
							<Shield className="h-5 w-5" />
						</div>
						<h3 className="font-bold text-lg text-white">Bank-Grade Security</h3>
						<p className="mt-2 text-slate-400 text-sm">Full RBAC middleware control, Redis session validation, and hash check deduplication protect your data.</p>
					</div>

					<div className="bg-slate-900/40 border border-slate-900 p-6 rounded-2xl backdrop-blur-sm">
						<div className="h-10 w-10 bg-indigo-600/10 rounded-xl flex items-center justify-center text-indigo-400 mb-4 border border-indigo-500/10">
							<Layers className="h-5 w-5" />
						</div>
						<h3 className="font-bold text-lg text-white">Chunked & Resumable</h3>
						<p className="mt-2 text-slate-400 text-sm">Split huge files into chunks dynamically, upload concurrently, and resume seamlessly after disconnects.</p>
					</div>

					<div className="bg-slate-900/40 border border-slate-900 p-6 rounded-2xl backdrop-blur-sm">
						<div className="h-10 w-10 bg-indigo-600/10 rounded-xl flex items-center justify-center text-indigo-400 mb-4 border border-indigo-500/10">
							<Share2 className="h-5 w-5" />
						</div>
						<h3 className="font-bold text-lg text-white">Smart Expiring Links</h3>
						<p className="mt-2 text-slate-400 text-sm">Create public or password-protected share links, specify custom limits, and track downloads in real time.</p>
					</div>

					<div className="bg-slate-900/40 border border-slate-900 p-6 rounded-2xl backdrop-blur-sm">
						<div className="h-10 w-10 bg-indigo-600/10 rounded-xl flex items-center justify-center text-indigo-400 mb-4 border border-indigo-500/10">
							<Cpu className="h-5 w-5" />
						</div>
						<h3 className="font-bold text-lg text-white">Local Ollama AI</h3>
						<p className="mt-2 text-slate-400 text-sm">Automatic Tesseract OCR text extraction, PDF document summarization, and tag generation run locally for free.</p>
					</div>
				</div>
			</main>

			{/* Footer */}
			<footer className="relative z-10 border-t border-slate-900 py-8 text-center text-sm text-slate-500">
				<p>© {new Date().getFullYear()} CloudStore Storage Systems. Built using Next.js & Golang.</p>
			</footer>
		</div>
	);
}
