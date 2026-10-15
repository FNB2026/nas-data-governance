import { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import DisabledNotice from "../components/DisabledNotice";
import LoadingState from "../components/LoadingState";
import { HttpAdapter, type Source, type Snapshot } from "./http";
import "../styles/tokens.css";
import "../styles.css";
const api = new HttpAdapter();
function WebApp() {
 const [password,setPassword]=useState(""); const [sources,setSources]=useState<Source[]>([]); const [alias,setAlias]=useState(""); const [status,setStatus]=useState<Snapshot>(); const [error,setError]=useState(""); const [busy,setBusy]=useState(false); const [signedIn,setSignedIn]=useState(false);
 async function action(fn:()=>Promise<unknown>){setBusy(true);setError("");try{await fn()}catch{setError("Request rejected. Check session and approved source; private details omitted.")}finally{setBusy(false)}}
 useEffect(()=>{if(!signedIn||!alias)return;let disposed=false,pending=false;const refresh=async()=>{if(pending)return;pending=true;try{const next=await api.status(alias);if(!disposed)setStatus(next)}catch{if(!disposed)setError("Status unavailable; verify session.")}finally{pending=false}};void refresh();const timer=setInterval(()=>void refresh(),2000);return()=>{disposed=true;clearInterval(timer)}},[signedIn,alias]);
 return <main className="page"><h1>NDG — Read-only Web</h1><DisabledNotice reason="Experimental local-source channel. Quarantine / Restore / Purge are disabled by the backend." hint="Source paths and filenames are omitted. Network sources, plans and full directory context are not supported in this first slice." />{!signedIn?<form onSubmit={e=>{e.preventDefault();void action(async()=>{await api.login(password);setPassword("");const list=await api.sources();setSources(list);setAlias(list[0]?.alias??"");setSignedIn(true)})}}><label>Operator credential <input aria-label="Operator credential" type="password" autoComplete="current-password" value={password} onChange={e=>setPassword(e.target.value)} /></label><button disabled={busy}>Sign in</button></form>:<section><label>Approved source <select value={alias} onChange={e=>setAlias(e.target.value)}>{sources.map(s=><option key={s.alias}>{s.alias}</option>)}</select></label><button disabled={busy} onClick={()=>void action(()=>api.scan(alias,false))}>Scan</button><button disabled={busy} onClick={()=>void action(()=>api.scan(alias,true))}>Resume</button><button disabled={busy} onClick={()=>void action(()=>api.cancel(alias))}>Cancel scan</button><button disabled={busy} onClick={()=>void action(async()=>{await api.logout();setSignedIn(false);setStatus(undefined)})}>Sign out</button><pre>{status?JSON.stringify(status,null,2):"No status available"}</pre></section>}{busy&&<LoadingState label="Waiting for server…"/>}{error&&<p role="alert">{error}</p>}</main>
}
createRoot(document.getElementById("root")!).render(<WebApp/>);
