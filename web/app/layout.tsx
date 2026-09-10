import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";
import "./workflow.css";
import "./authority.css";
import "./authority-technical.css";
const sans=Geist({variable:"--font-sans",subsets:["latin"]});const mono=Geist_Mono({variable:"--font-mono",subsets:["latin"]});
export const metadata:Metadata={title:{default:"HealthTrust Exchange",template:"%s · HealthTrust Exchange"},description:"Patient-controlled health information exchange with off-chain clinical data and verifiable trust.",openGraph:{title:"HealthTrust Exchange",description:"Secure health information exchange built around patient-controlled trust.",images:[{url:"/og.png",width:1200,height:630,alt:"HealthTrust Exchange connects trusted healthcare organizations"}]}};
export default function RootLayout({children}:{children:React.ReactNode}){return <html lang="en"><body className={`${sans.variable} ${mono.variable}`}>{children}</body></html>}
