import {notFound} from "next/navigation";
import {Dashboard} from "../../../components/dashboard";

const sections=new Set(["patients","records","referrals","check-in","tasks","journey","requests","audit","network","operations"]);

export default async function ApplicationSection({params}:{params:Promise<{section:string}>}){
  const{section}=await params;
  if(!sections.has(section))notFound();
  return <Dashboard/>;
}
