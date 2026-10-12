import React, {useEffect,useState} from 'react';
import {createRoot} from 'react-dom/client';
function App(){
 const [v,setV]=useState({state:'LOADING'}),[busy,setBusy]=useState(false);
 const call=async(name)=>{setBusy(true);try{setV(await window.go.main.Probe[name]())}catch{setV({state:'BLOCKED',error:'runtime_error'})}finally{setBusy(false)}};
 useEffect(()=>{call('Status')},[]);
 return <main><h1>NDG MAS Phase 0</h1><p>人工测试目录，只读能力预验证。源文件路径和文件名不显示。</p><button disabled={busy} onClick={()=>call('Select')}>选择测试目录</button><button disabled={busy} onClick={()=>call('Scan')}>恢复授权并扫描</button><button disabled={busy} onClick={()=>call('BoundaryProbe')}>检查未授权测试文件边界</button><h2>{busy?'SCANNING':v.state}</h2><pre>{JSON.stringify(v,null,2)}</pre><p>Quarantine / Restore 未接入；Purge 无执行入口。</p></main>
}
createRoot(document.getElementById('root')).render(<App/>);
