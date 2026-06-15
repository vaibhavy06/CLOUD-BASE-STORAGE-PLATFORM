import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import { AuthProvider } from "../components/AuthProvider";
import "./globals.css";

const geistSans = Geist({
	variable: "--font-geist-sans",
	subsets: ["latin"],
});

const geistMono = Geist_Mono({
	variable: "--font-geist-mono",
	subsets: ["latin"],
});

export const metadata: Metadata = {
	title: "CloudStore - Premium Distributed Cloud Storage",
	description: "Secure, lightning-fast, distributed cloud storage platform featuring chunked uploads, version control, public sharing, and local AI capabilities.",
	keywords: ["cloud storage", "distributed systems", "dropbox clone", "secure file sharing", "nextjs", "golang"],
};

export default function RootLayout({
	children,
}: Readonly<{
	children: React.ReactNode;
}>) {
	return (
		<html lang="en" className="h-full">
			<body className={`${geistSans.variable} ${geistMono.variable} min-h-full flex flex-col antialiased bg-slate-950 text-slate-100`}>
				<AuthProvider>
					{children}
				</AuthProvider>
			</body>
		</html>
	);
}
