"use client";
import Link from "next/link";
import {motion,useReducedMotion,useScroll,useTransform} from "framer-motion";
import {ArrowLeft,ArrowRight,CheckCircle2,Database,FileHeart,GitCommitHorizontal,HeartHandshake,Hospital,KeyRound,LockKeyhole,ShieldCheck,UserRound} from "lucide-react";
import {useRef} from "react";

const scenes=[
 {n:"01",title:"Clinical record created",body:"Doctor A creates a focused clinical record at Hospital A.",icon:FileHeart,testid:"story-record"},
 {n:"02",title:"Data stays off-chain",body:"The clinical record remains in Hospital A. Only a deterministic integrity commitment enters the trust layer.",icon:Database,testid:"story-offchain"},
 {n:"03",title:"Patient referred",body:"Hospital A creates a referral to Hospital B, recorded as a verifiable audit event.",icon:Hospital,testid:"story-referral"},
 {n:"04",title:"Doctor B requests access",body:"The record remains locked while the request waits for the patient.",icon:LockKeyhole,testid:"story-request"},
 {n:"05",title:"Patient controls access",body:"Patient P reviews who is asking, which record is involved, and the requesting hospital.",icon:UserRound,testid:"story-patient"},
 {n:"06",title:"Consent granted",body:"An exact permission activates the secure path—without moving the record onto the blockchain.",icon:HeartHandshake,testid:"story-consent"},
 {n:"07",title:"Authorized retrieval",body:"Hospital A validates the peer, doctor, consent, and single-use proof before returning the record.",icon:KeyRound,testid:"story-retrieval"},
 {n:"08",title:"Integrity verified",body:"The stored record still matches its blockchain commitment and source organization.",icon:ShieldCheck,testid:"story-integrity"},
 {n:"09",title:"Every step is auditable",body:"Commitment, referral, consent, access, and revocation form a readable trust timeline.",icon:GitCommitHorizontal,testid:"story-audit"},
];

export function DemoStory(){
 const ref=useRef<HTMLElement|null>(null);const reduce=useReducedMotion();
 const{scrollYProgress}=useScroll({target:ref,offset:["start start","end end"]});
 const progress=useTransform(scrollYProgress,[0,1],["0%","100%"]);
 const recordOpacity=useTransform(scrollYProgress,[0,.08,.18],[0,0,1]);
 const commitmentOpacity=useTransform(scrollYProgress,[.12,.25],[0,1]);
 const referralX=useTransform(scrollYProgress,[.22,.4],["-45%","45%"]);
 const accessOpacity=useTransform(scrollYProgress,[.38,.58],[.2,1]);
 const auditOpacity=useTransform(scrollYProgress,[.7,.9],[0,1]);
 return <main className="demo-page"><motion.div className="demo-progress" aria-hidden style={reduce?{width:"100%"}:{width:progress}}/><nav className="nav demo-nav"><Link href="/" className="brand"><span className="brand-mark"><ShieldCheck size={19}/></span>HealthTrust Exchange</Link><Link href="/app" className="button small">Open interactive demo <ArrowRight size={16}/></Link></nav><header className="demo-hero"><Link href="/" className="back"><ArrowLeft size={15}/> Overview</Link><span className="eyebrow">The complete trust journey</span><h1>One record. Two hospitals.<br/><em>Patient-controlled trust.</em></h1><p>Scroll through the real HealthTrust workflow. Every scene remains understandable with motion disabled.</p></header><section ref={ref} className="story" data-testid="scroll-story"><aside className="story-sticky"><div className="story-canvas" aria-label="Hospital trust workflow visualization"><div className="story-hospital"><span>A</span><b>Hospital A</b><small>Record source</small></div><motion.div className="story-path" style={reduce?{scaleX:1}:{scaleX:scrollYProgress}}/><motion.div className="referral-token" style={reduce?{x:0}:{x:referralX}}><Hospital/><small>Referral</small></motion.div><div className="story-patient"><UserRound/><b>Patient P</b><small>Controls consent</small></div><div className="story-hospital"><span>B</span><b>Hospital B</b><small>Authorized care</small></div><motion.div className="offchain" style={reduce?undefined:{opacity:recordOpacity}}><Database/><div><b>Clinical data</b><small>Off-chain · Hospital A</small></div></motion.div><motion.div className="onchain" style={reduce?undefined:{opacity:commitmentOpacity}}><ShieldCheck/><div><b>Integrity commitment</b><small>On-chain · trust layer</small></div></motion.div><motion.div className="access-path" style={reduce?undefined:{opacity:accessOpacity}}><KeyRound/> Secure retrieval</motion.div><motion.div className="audit-signal" style={reduce?undefined:{opacity:auditOpacity}}><GitCommitHorizontal/> Audit trail verified</motion.div></div></aside><div className="story-copy"><div className="progress-rail"><motion.span style={reduce?{scaleY:1}:{scaleY:scrollYProgress}}/></div>{scenes.map((scene,index)=><motion.article key={scene.n} data-testid={scene.testid} className="scene" initial={reduce?false:{opacity:.25,y:28}} whileInView={{opacity:1,y:0}} viewport={{amount:.5}} transition={{duration:.4}}><span className="scene-number">{scene.n}</span><scene.icon/><h2>{scene.title}</h2><p>{scene.body}</p>{index===1&&<div className="clarity"><Database/>Clinical content never enters the blockchain.<CheckCircle2/></div>}{index===8&&<Audit/>}</motion.article>)}</div></section><footer className="demo-cta"><span className="eyebrow">Ready to explore</span><h2>Experience each role in Demo Mode.</h2><Link href="/app" className="button">Open interactive demo <ArrowRight size={17}/></Link></footer></main>
}

function Audit(){return <div className="audit-mini">{["Clinical record committed","Referral created","Patient granted access","Clinical record accessed","Patient revoked access"].map((event,index)=><div key={event}><span>{index<4?<CheckCircle2/>:<LockKeyhole/>}</span><p>{event}<small>Block {index+1} · verified</small></p></div>)}</div>}
