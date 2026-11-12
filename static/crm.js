import{initializeApp}from"https://www.gstatic.com/firebasejs/10.14.1/firebase-app.js";
import{getAuth,onAuthStateChanged}from"https://www.gstatic.com/firebasejs/10.14.1/firebase-auth.js";
import{h,add,api,setAuth,copy,icon}from"./ui.js";

const auth=getAuth(initializeApp(window.FB));setAuth(auth);
const $=i=>document.getElementById(i);
const S={me:null,team:null,data:{},view:"dashboard",sub:{leads:"kanban",deals:"kanban"},charts:[],q:{},chat:null};
const V=()=>$("view");
document.querySelectorAll(".bz-theme button").forEach(b=>b.onclick=()=>{document.documentElement.dataset.theme=b.dataset.t;try{localStorage.setItem("bz-theme",b.dataset.t)}catch(e){}});

const can=(m,a)=>((S.me.perms||{})[m]||"").includes(a);
const money=v=>{try{return new Intl.NumberFormat(undefined,{style:"currency",currency:S.me.workspace.currency||"USD",maximumFractionDigits:0}).format(v||0)}catch(e){return String(Math.round(v||0))}};
const num=v=>new Intl.NumberFormat().format(Math.round(v||0));
const date=t=>t?new Date(t).toLocaleDateString():"";
const ago=t=>{if(!t)return"";const s=(Date.now()-new Date(t))/1000;return s<60?"just now":s<3600?Math.floor(s/60)+" min ago":s<86400?Math.floor(s/3600)+" h ago":date(t)};
const label=s=>(s||"").replace(/_/g," ").replace(/^./,c=>c.toUpperCase());
const pill=s=>h("span","crm-pill "+s,label(s));
const A=x=>Array.isArray(x)?x:[];
const members=()=>A(S.team&&S.team.members);
const ownerName=u=>{if(!u)return"Unassigned";const m=members().find(x=>x.uid===u);return m?m.name:"Former member"};
const STC={new:"#2fc6f6",contacted:"#3e8bf1",qualified:"#8b5cf6",proposal:"#fbbf24",negotiation:"#f26a2e",won:"#34d399",converted:"#34d399",unqualified:"#f87171",lost:"#f87171"};
const stc=s=>STC[s]||"#2fc6f6";
const SOURCES=["manual","csv_import","web_form","meta_lead_ad","fb_dm","fb_comment","ig_dm","ig_comment","referral","phone","event","other"];
const msg=(p,t,err)=>{let m=p.querySelector(".crm-msg");if(!m){m=h("div","crm-msg");p.appendChild(m)}m.textContent=t;m.className="crm-msg"+(err?" err":"")};
const mk=(tag,props)=>Object.assign(document.createElement(tag),props||{});
function avatar(uid){const n=ownerName(uid),s=h("span","bx-av",uid?n.split(/\s+/).map(x=>x[0]).slice(0,2).join("").toUpperCase():"?");let k=0;for(const c of uid||"")k=(k*31+c.charCodeAt(0))%360;s.style.background=uid?`hsl(${k} 55% 45%)`:"#94a3b8";s.title=n;return s}

// ---------------------------------------------------------------- modal + forms
function modal(title,body,wide){const m=$("modal");m.innerHTML="";m.hidden=false;const b=h("div","crm-box");if(wide)b.style.width="min(980px,100%)";const hd=h("div","crm-row");hd.style.cssText="justify-content:space-between;margin:0 0 12px";const x=h("button","bz-mini","Close");x.onclick=close;add(hd,h("h3",null,title),x);add(b,hd,body);m.appendChild(b);m.onclick=e=>{if(e.target===m)close()}}
function close(){$("modal").hidden=true;$("modal").innerHTML="";$("slider").hidden=true;$("slider").innerHTML=""}
function formEl(fields,vals){
  const f=h("div","form"),inputs={};
  for(const d of fields){const l=h("label",d.wide?"wide":null,d.label+(d.req?" *":""));let i;
    if(d.type==="select"){i=mk("select");for(const[v,t]of d.opts)i.appendChild(mk("option",{value:v,textContent:t}))}
    else if(d.type==="textarea")i=mk("textarea",{rows:d.rows||3});else i=mk("input",{type:d.type==="tags"?"text":d.type||"text"});
    let v=vals&&vals[d.k];if(d.type==="tags"&&Array.isArray(v))v=v.join(", ");if(v!=null)i.value=v;l.appendChild(i);f.appendChild(l);inputs[d.k]=i}
  f.get=()=>{const o={};for(const d of fields){let v=inputs[d.k].value;if(d.type==="number")v=parseFloat(v)||0;if(d.type==="tags")v=v.split(",").map(x=>x.trim()).filter(Boolean);o[d.k]=v}return o};f.inputs=inputs;return f}
const ownerOpts=()=>[["","Unassigned"],...members().filter(m=>m.status!=="disabled").map(m=>[m.uid,m.name])];
const sel=a=>A(a).map(x=>[x,label(x)]);
const co=()=>[["","None"],...A(S.data.companies).map(c=>[c.id,c.name])];
const ct=()=>[["","None"],...A(S.data.contacts).map(c=>[c.id,c.name])];
const ENT={
  leads:{one:"lead",key:"status",stages:()=>S.me.leadStatus,fields:()=>[{k:"name",label:"Name",req:1},{k:"company",label:"Company"},{k:"email",label:"Email",type:"email"},{k:"phone",label:"Phone"},{k:"title",label:"Job title"},{k:"source",label:"Source",type:"select",opts:sel(SOURCES)},{k:"value",label:"Expected value",type:"number"},{k:"ownerUid",label:"Responsible",type:"select",opts:ownerOpts()},{k:"campaign",label:"Campaign"},{k:"tags",label:"Tags",type:"tags"},{k:"notes",label:"Comments",type:"textarea",wide:1}]},
  deals:{one:"deal",key:"stage",stages:()=>S.me.dealStage,fields:()=>[{k:"title",label:"Deal name",req:1,wide:1},{k:"value",label:"Amount",type:"number"},{k:"contactId",label:"Contact",type:"select",opts:ct()},{k:"companyId",label:"Company",type:"select",opts:co()},{k:"ownerUid",label:"Responsible",type:"select",opts:ownerOpts()},{k:"expectedClose",label:"Expected close",type:"date"},{k:"source",label:"Source",type:"select",opts:sel(SOURCES)},{k:"lostReason",label:"Lost reason"},{k:"notes",label:"Description",type:"textarea",wide:1}]},
  contacts:{one:"contact",fields:()=>[{k:"name",label:"Name",req:1},{k:"email",label:"Email",type:"email"},{k:"phone",label:"Phone"},{k:"title",label:"Job title"},{k:"companyId",label:"Company",type:"select",opts:co()},{k:"ownerUid",label:"Responsible",type:"select",opts:ownerOpts()},{k:"city",label:"City"},{k:"tags",label:"Tags",type:"tags"},{k:"notes",label:"Comments",type:"textarea",wide:1}]},
  companies:{one:"company",fields:()=>[{k:"name",label:"Company name",req:1},{k:"website",label:"Website"},{k:"industry",label:"Industry"},{k:"phone",label:"Phone"},{k:"email",label:"Email",type:"email"},{k:"city",label:"City"},{k:"country",label:"Country"},{k:"size",label:"Size"},{k:"ownerUid",label:"Responsible",type:"select",opts:ownerOpts()},{k:"tags",label:"Tags",type:"tags"},{k:"notes",label:"Comments",type:"textarea",wide:1}]},
  tasks:{one:"task",fields:()=>[{k:"title",label:"Task",req:1,wide:1},{k:"due",label:"Deadline",type:"date"},{k:"priority",label:"Priority",type:"select",opts:sel(["low","normal","high"])},{k:"assigneeUid",label:"Responsible",type:"select",opts:ownerOpts().slice(1)},{k:"notes",label:"Description",type:"textarea",wide:1}]}
};
const load=async(e,force)=>{if(!force&&S.data[e])return S.data[e];if(!can(e,"v"))return S.data[e]=[];S.data[e]=A((await api("GET","/api/crm/list?e="+e)).items);return S.data[e]};
const nameOf=(e,id)=>{const x=A(S.data[e]).find(r=>r.id===id);return x?x.name:""};
const refresh=()=>show(S.view);
const quick=async(e,o)=>{await api("POST","/api/crm/save?e="+e,o);S.data[e]=null};

// ---------------------------------------------------------------- record slider (Bitrix-style card)
async function openRecord(e,rec,defaults){
  const spec=ENT[e],isNew=!rec;
  if(e==="deals"||e==="contacts"){await Promise.all([load("companies"),load("contacts")])}
  if(e==="tasks"){const m=$("modal");}
  const vals=Object.assign({ownerUid:S.me.me.uid,assigneeUid:S.me.me.uid,source:"manual"},defaults||{},rec||{});
  if(e==="deals"&&isNew&&!vals.stage)vals.stage="new";if(e==="leads"&&isNew&&!vals.status)vals.status="new";
  const f=formEl(spec.fields(),vals);
  const sl=$("slider");sl.innerHTML="";sl.hidden=false;sl.onclick=ev=>{if(ev.target===sl)close()};
  const P=h("div","bx-panel");sl.appendChild(P);
  const head=h("div","bx-ph"),ttl=h("h2",null,(isNew?"New "+spec.one:(rec.name||rec.title||""))),acts=h("div","crm-row");acts.style.margin="0";
  const save=h("button","bz-btn",isNew?"Create":"Save");
  const clean=()=>{const o=f.get();if(rec)o.id=rec.id;if(spec.key)o[spec.key]=cur;if(e==="deals"||e==="contacts"){o.companyName=nameOf("companies",o.companyId);if(e==="deals")o.contactName=nameOf("contacts",o.contactId)}return o};
  let cur=isNew?(vals[spec.key]||""):rec[spec.key];
  save.onclick=async()=>{const o=clean();save.disabled=true;try{await api("POST","/api/crm/save?e="+e,o);S.data[e]=null;close();refresh()}catch(x){msg(body,x.message,1);save.disabled=false}};
  if(isNew?can(e,"c"):can(e,"e"))acts.appendChild(save);
  if(e==="leads"&&rec&&rec.status!=="converted"&&can("deals","c")&&can("leads","e")){const c=h("button","bz-btn ghost","Convert to deal");c.onclick=()=>convertLead(rec);acts.appendChild(c)}
  if(rec&&can(e,"d")){const d=h("button","bz-btn ghost","Delete");d.onclick=async()=>{if(!confirm("Delete this "+spec.one+"?"))return;try{await api("POST","/api/crm/delete?e="+e,{id:rec.id});S.data[e]=null;close();refresh()}catch(x){msg(body,x.message,1)}};acts.appendChild(d)}
  const x=h("button","bz-mini","✕ Close");x.onclick=close;acts.appendChild(x);add(head,ttl,acts);P.appendChild(head);
  if(spec.key){const bar=h("div","bx-stages");const drawBar=()=>{bar.innerHTML="";const st=spec.stages();const idx=st.indexOf(cur);st.forEach((s,i)=>{const b=h("button","bx-stage"+(i<=idx&&idx>=0?" done":""),label(s));b.style.setProperty("--c",stc(idx>=0?cur:s));b.onclick=async()=>{if(e==="leads"&&s==="converted"){if(rec)convertLead(rec);return}let lr;if(s==="lost"&&e==="deals"){lr=prompt("Why was this deal lost? (optional)");if(lr===null)return}cur=s;drawBar();if(rec&&can(e,"e")){try{const o={id:rec.id,[spec.key]:s};if(lr!=null)o.lostReason=lr;await api("POST","/api/crm/save?e="+e,o);rec[spec.key]=s;S.data[e]=null;tl&&timeline(e,rec,tl,body)}catch(er){msg(body,er.message,1)}}};bar.appendChild(b)})};drawBar();P.appendChild(bar)}
  const body=h("div","bx-pb");P.appendChild(body);
  const left=h("div"),sec=h("div","bx-sec");sec.appendChild(h("h3",null,e==="deals"?"About the deal":"About"));sec.appendChild(f);left.appendChild(sec);
  if(rec&&e==="companies"){const lk=h("div","bx-sec");lk.appendChild(h("h3",null,"Linked"));const cs=A(S.data.contacts).filter(c=>c.companyId===rec.id);lk.appendChild(h("p",null,cs.length?"Contacts: "+cs.map(c=>c.name).join(", "):"No contacts yet."));left.appendChild(lk)}
  body.appendChild(left);
  let tl=null;
  if(rec&&e!=="tasks"){const right=h("div"),comp=h("div","bx-sec");right.appendChild(comp);let mode="note";
    const tb=h("div","tb"),ta=mk("textarea",{rows:3,placeholder:"Write a comment, log a call or meeting…"}),dd=mk("input",{type:"date"});dd.hidden=true;
    const bw=h("div","bx-comp");const mkTab=(k,l)=>{const b=h("button",null,l);b.onclick=()=>{mode=k;[...tb.children].forEach(c=>c.classList.toggle("on",c===b));dd.hidden=k!=="task";ta.placeholder=k==="task"?"What needs to be done?":"Write here…"};if(k==="note")b.classList.add("on");tb.appendChild(b)};
    for(const[k,l]of[["note","Comment"],["call","Call"],["meeting","Meeting"],["email","Email"],...(can("tasks","c")?[["task","Task"]]:[])])mkTab(k,l);
    const go=h("button","bz-btn","Send");go.style.alignSelf="flex-start";
    go.onclick=async()=>{const t=ta.value.trim();if(!t)return;go.disabled=true;try{if(mode==="task")await api("POST","/api/crm/save?e=tasks",{title:t,due:dd.value,priority:"normal",assigneeUid:rec.ownerUid||S.me.me.uid,relType:e,relId:rec.id,relName:rec.name||rec.title||""}),S.data.tasks=null;else await api("POST","/api/crm/activity",{relType:e,relId:rec.id,type:mode,text:t});ta.value="";timeline(e,rec,tl,body)}catch(er){msg(comp,er.message,1)}go.disabled=false};
    if(can(e,"e"))add(bw,tb,ta,dd,go);else bw.appendChild(h("p","bz-mut","You can view history but not add to it."));comp.appendChild(bw);
    const hs=h("div","bx-sec");hs.appendChild(h("h3",null,"History"));tl=h("div","bx-tl");hs.appendChild(tl);right.appendChild(hs);body.appendChild(right);timeline(e,rec,tl,body)}
}
async function timeline(e,rec,box,parent){
  box.innerHTML="";
  try{const items=A((await api("GET","/api/crm/activities?relType="+e+"&relId="+rec.id)).items);
    for(const a of items){const d=h("div","crm-act "+a.type);add(d,h("div",null,a.text),h("small",null,label(a.type)+" · "+(a.byUid?ownerName(a.byUid):"System")+" · "+ago(a.at)));box.appendChild(d)}
    if(!items.length)box.appendChild(h("p","bz-mut","No history yet."))}catch(x){}
}
function convertLead(l){
  const b=h("div"),f=formEl([{k:"dealTitle",label:"Deal name",wide:1},{k:"value",label:"Amount",type:"number"},{k:"expectedClose",label:"Expected close",type:"date"}],{dealTitle:l.name,value:l.value||0});
  const mc=mk("input",{type:"checkbox",checked:true}),mo=mk("input",{type:"checkbox",checked:!!l.company});
  const l1=h("label",null," Also create a contact");l1.prepend(mc);const l2=h("label",null," Also create a company ("+(l.company||"none on this lead")+")");l2.prepend(mo);
  const go=h("button","bz-btn","Convert");go.onclick=async()=>{go.disabled=true;try{const v=f.get();await api("POST","/api/crm/lead/convert",{id:l.id,dealTitle:v.dealTitle,value:v.value,expectedClose:v.expectedClose,makeContact:mc.checked,makeCompany:mo.checked});for(const e of["leads","deals","contacts","companies"])S.data[e]=null;close();S.sub.deals="kanban";nav("deals")}catch(x){msg(b,x.message,1);go.disabled=false}};
  add(b,f,l1,l2,h("div","crm-row"));b.lastChild.appendChild(go);modal("Convert lead to deal",b);
}

// ---------------------------------------------------------------- shared list helpers
function csvDownload(name,cols,rows){const q=v=>'"'+String(v==null?"":v).replace(/"/g,'""')+'"';const t=[cols.map(q).join(",")].concat(rows.map(r=>r.map(q).join(","))).join("\n");mk("a",{href:URL.createObjectURL(new Blob(["﻿"+t],{type:"text/csv"})),download:name}).click()}
function parseCSV(t){const rows=[];let r=[],c="",q=false;for(let i=0;i<t.length;i++){const ch=t[i];if(q){if(ch==='"'){if(t[i+1]==='"'){c+='"';i++}else q=false}else c+=ch}else if(ch==='"')q=true;else if(ch===","||ch===";"||ch==="\t"){r.push(c);c=""}else if(ch==="\n"||ch==="\r"){if(ch==="\r"&&t[i+1]==="\n")i++;r.push(c);c="";if(r.some(x=>x.trim()))rows.push(r);r=[]}else c+=ch}r.push(c);if(r.some(x=>x.trim()))rows.push(r);return rows}
const HDR={name:["name","full name","full_name","fullname","contact"],email:["email","e-mail","email address"],phone:["phone","phone number","mobile","tel","telephone"],company:["company","company name","organization"],title:["title","job title","position"],city:["city"],notes:["notes","message","comment"],value:["value","amount","budget"]};
function importLeads(){
  const b=h("div");b.appendChild(h("p","bz-mut","Upload a CSV with a header row. Columns recognised: name, email, phone, company, title, city, notes, value. Duplicates (same email or phone) are skipped."));
  const file=mk("input",{type:"file",accept:".csv,text/csv"}),src=mk("select");for(const s of SOURCES)src.appendChild(mk("option",{value:s,textContent:label(s)}));src.value="csv_import";
  const own=mk("select");for(const[v,t]of ownerOpts())own.appendChild(mk("option",{value:v,textContent:t}));const go=h("button","bz-btn","Import");
  go.onclick=async()=>{if(!file.files[0]){msg(b,"Choose a CSV file.",1);return}const rows=parseCSV(await file.files[0].text());if(rows.length<2){msg(b,"The file has no data rows.",1);return}
    const head=rows[0].map(x=>x.trim().toLowerCase()),idx={};for(const[k,al]of Object.entries(HDR)){const i=head.findIndex(x=>al.includes(x));if(i>=0)idx[k]=i}
    if(idx.name==null&&idx.email==null&&idx.phone==null){msg(b,"No name, email or phone column found in the header row.",1);return}
    const out=rows.slice(1,1001).map(r=>{const o={};for(const[k,i]of Object.entries(idx))o[k]=k==="value"?parseFloat(String(r[i]).replace(/[^0-9.]/g,""))||0:(r[i]||"").trim();return o});
    go.disabled=true;try{const r=await api("POST","/api/crm/leads/import",{rows:out,source:src.value,ownerUid:own.value});S.data.leads=null;msg(b,"Imported "+r.created+" leads. Skipped "+r.duplicates+" duplicates and "+r.invalid+" invalid rows.");setTimeout(()=>{close();refresh()},1200)}catch(x){msg(b,x.message,1);go.disabled=false}};
  const f=h("div","form");for(const[l,el]of[["CSV file",file],["Source",src],["Assign to",own]]){const lb=h("label",null,l);lb.appendChild(el);f.appendChild(lb)}add(b,f,go);modal("Import leads",b)}

function setHead(title,sub,acts,tabs){
  $("title").textContent=title;$("sub").textContent=sub||"";const a=$("acts");a.innerHTML="";for(const x of A(acts))a.appendChild(x);
  const t=$("tabs");t.innerHTML="";t.hidden=!tabs;if(tabs)for(const[k,l,on,fn]of tabs){const b=h("button",on?"on":"",l);b.onclick=fn;t.appendChild(b)}}
const newBtn=e=>{if(!can(e,"c"))return null;const b=h("button","bz-btn","+ Create");b.onclick=()=>openRecord(e,null);return b};
function toolbar(e,state,redraw){
  const bar=h("div","crm-bar"),q=mk("input",{placeholder:"Search…",value:state.q||""}),chips=h("div","bx-chips");
  const mine=e==="tasks"?"assigneeUid":"ownerUid";
  for(const[k,l]of[["all","All"],["my","Mine"],["free","Unassigned"]]){if(k==="free"&&e==="tasks")continue;const c=h("button","bx-chip"+((state.f||"all")===k?" on":""),l);c.onclick=()=>{state.f=k;[...chips.children].forEach(x=>x.classList.toggle("on",x===c));redraw()};chips.appendChild(c)}
  q.oninput=()=>{state.q=q.value;redraw()};add(bar,chips,q);
  const pass=r=>{const f=state.f||"all";if(f==="my"&&r[mine]!==S.me.me.uid)return false;if(f==="free"&&r[mine])return false;const t=(state.q||"").toLowerCase();return !t||JSON.stringify(Object.values(r)).toLowerCase().includes(t)};
  return{bar,pass}}

// ---------------------------------------------------------------- kanban
function kanban(e){
  const spec=ENT[e],key=spec.key,items=S.data[e],st=S.q[e]=S.q[e]||{};const{bar,pass}=toolbar(e,st,draw);
  const v=V();v.innerHTML="";v.appendChild(bar);const fun=h("div","bx-funnel"),kan=h("div","crm-kan");v.append(fun,kan);
  const stages=spec.stages();for(const s of stages){const i=h("i");i.style.background=stc(s);fun.appendChild(i)}
  function draw(){kan.innerHTML="";const list=items.filter(pass);
    for(const s of stages){const col=h("div","crm-col"),its=list.filter(d=>d[key]===s),sum=its.reduce((a,d)=>a+(d.value||0),0);
      const hd=h("div","bx-colh");hd.style.background=stc(s);add(hd,h("b",null,label(s)),h("span","tot",money(sum)),h("small",null,its.length+" "+(its.length===1?spec.one:spec.one+"s")));col.appendChild(hd);
      if(can(e,"c")&&s!=="converted"){const q=h("button","bx-quick","+");q.onclick=()=>{const w=h("div","bx-quickf"),i=mk("input",{placeholder:e==="deals"?"Deal name":"Lead name"}),ok=h("button","bz-mini","Add");w.append(i,ok);q.replaceWith(w);i.focus();const go=async()=>{if(!i.value.trim())return;try{await quick(e,{[e==="deals"?"title":"name"]:i.value.trim(),[key]:s,ownerUid:S.me.me.uid,source:"manual"});await load(e,true);show(e)}catch(x){alert(x.message)}};ok.onclick=go;i.onkeydown=ev=>{if(ev.key==="Enter")go()}};col.appendChild(q)}
      for(const d of its){const c=h("div","crm-card");c.style.setProperty("--st",stc(s));c.draggable=can(e,"e");
        const t=h("span","t",d.title||d.name);c.appendChild(t);if(d.value)c.appendChild(h("div","amt",money(d.value)));
        const sub=e==="deals"?[d.contactName,d.companyName].filter(Boolean).join(" · "):[d.company,d.source&&label(d.source)].filter(Boolean).join(" · ");if(sub)c.appendChild(h("div","m",sub));
        const ft=h("div","ft");add(ft,h("span",null,e==="deals"&&d.expectedClose?d.expectedClose:date(d.createdAt)),avatar(d.ownerUid));c.appendChild(ft);
        c.ondragstart=ev=>ev.dataTransfer.setData("text/plain",d.id);c.onclick=()=>openRecord(e,d);col.appendChild(c)}
      col.ondragover=ev=>{ev.preventDefault();col.classList.add("over")};col.ondragleave=()=>col.classList.remove("over");
      col.ondrop=async ev=>{ev.preventDefault();col.classList.remove("over");const id=ev.dataTransfer.getData("text/plain"),d=items.find(x=>x.id===id);if(!d||d[key]===s)return;
        if(e==="leads"&&s==="converted"){convertLead(d);return}const o={id,[key]:s};if(s==="lost"&&e==="deals"){const r=prompt("Why was this deal lost? (optional)");if(r===null)return;o.lostReason=r}
        try{await api("POST","/api/crm/save?e="+e,o);S.data[e]=null;await load(e,true);show(e)}catch(x){alert(x.message)}};
      kan.appendChild(col)}}
  draw()}

function grid(e,cols,rowFn,opts={}){
  const items=S.data[e],st=S.q[e]=S.q[e]||{};const{bar,pass}=toolbar(e,st,draw);const v=V();v.innerHTML="";
  if(opts.filter){const sel2=mk("select");sel2.appendChild(mk("option",{value:"",textContent:opts.filter.label}));for(const x of opts.filter.opts)sel2.appendChild(mk("option",{value:x,textContent:label(x)}));sel2.value=st.sf||"";sel2.onchange=()=>{st.sf=sel2.value;draw()};bar.appendChild(sel2)}
  bar.appendChild(h("span","sp"));
  if(can(e,"x")){const ex=h("button","bz-mini","Export CSV");ex.onclick=()=>{const rs=cur();csvDownload(e+".csv",cols,rs.map(r=>rowFn(r).map(c=>c instanceof Node?c.textContent:c)))};bar.appendChild(ex)}
  if(e==="leads"&&can("leads","x")&&can("leads","c")){const im=h("button","bz-mini","Import CSV");im.onclick=importLeads;bar.appendChild(im)}
  v.appendChild(bar);const bulk=h("div"),hold=h("div");v.append(bulk,hold);const sel3=new Set();
  const cur=()=>items.filter(r=>pass(r)&&(!st.sf||r[opts.filter.key]===st.sf));
  function drawBulk(){bulk.innerHTML="";if(!sel3.size)return;const b=h("div","bx-bulk");b.appendChild(h("b",null,sel3.size+" selected"));
    if(can(e,"e")&&e!=="companies"||can(e,"e")){const o=mk("select");for(const[v2,t]of ownerOpts().slice(1))o.appendChild(mk("option",{value:v2,textContent:"Assign to "+t}));const a=h("button","bz-mini","Assign");a.onclick=async()=>{for(const id of sel3)await api("POST","/api/crm/save?e="+e,{id,[e==="tasks"?"assigneeUid":"ownerUid"]:o.value});S.data[e]=null;await load(e,true);show(e)};add(b,o,a)}
    if(can(e,"d")){const d=h("button","bz-mini","Delete");d.onclick=async()=>{if(!confirm("Delete "+sel3.size+" records?"))return;for(const id of sel3)await api("POST","/api/crm/delete?e="+e,{id});S.data[e]=null;await load(e,true);show(e)};b.appendChild(d)}
    bulk.appendChild(b)}
  function draw(){hold.innerHTML="";const rs=cur(),w=h("div","crm-wrap"),t=h("table","crm-t"),hd=h("tr"),all=mk("input",{type:"checkbox"});
    all.onclick=()=>{rs.forEach(r=>all.checked?sel3.add(r.id):sel3.delete(r.id));draw();drawBulk()};const th0=h("th");th0.appendChild(all);hd.appendChild(th0);
    for(const c of cols)hd.appendChild(h("th",null,c));t.appendChild(h("thead")).appendChild(hd);const tb=h("tbody");
    for(const r of rs){const tr=h("tr","row"),cb=mk("input",{type:"checkbox",checked:sel3.has(r.id)});cb.onclick=ev=>{ev.stopPropagation();cb.checked?sel3.add(r.id):sel3.delete(r.id);drawBulk()};const td0=h("td");td0.appendChild(cb);tr.appendChild(td0);
      rowFn(r).forEach((c,i)=>{const td=h("td");if(c instanceof Node)td.appendChild(c);else if(i===0&&!opts.noLink){const a=h("a","ln",c==null?"":c);td.appendChild(a)}else td.textContent=c==null?"":c;tr.appendChild(td)});tr.onclick=()=>openRecord(e,r);tb.appendChild(tr)}
    if(!rs.length){const tr=h("tr"),td=h("td","bz-mut","Nothing here yet.");td.colSpan=cols.length+1;tr.appendChild(td);tb.appendChild(tr)}
    t.appendChild(tb);w.appendChild(t);hold.appendChild(w)}
  draw()}

// ---------------------------------------------------------------- views
const lv=(e,l)=>[["kanban","Kanban"],["list","List"]].map(([k,t])=>[k,t,S.sub[e]===k,()=>{S.sub[e]=k;show(e)}]);
const VIEWS={
  async leads(){await load("leads");setHead("Leads","Everyone who showed interest",[newBtn("leads")],lv("leads"));
    if(S.sub.leads==="kanban")return kanban("leads");
    grid("leads",["Lead","Company","Contact","Source","Status","Amount","Responsible","Created"],l=>[l.name,l.company,[l.email,l.phone].filter(Boolean).join(" · "),label(l.source),pill(l.status),l.value?money(l.value):"",(()=>{const sp=h("span");sp.append(avatar(l.ownerUid)," ",ownerName(l.ownerUid));return sp})(),date(l.createdAt)],{filter:{label:"All statuses",key:"status",opts:S.me.leadStatus}})},
  async deals(){await Promise.all([load("deals"),load("contacts"),load("companies")]);setHead("Deals","Your sales pipeline",[newBtn("deals")],lv("deals"));
    if(S.sub.deals==="kanban")return kanban("deals");
    grid("deals",["Deal","Stage","Amount","Contact","Company","Responsible","Close date"],d=>[d.title,pill(d.stage),d.value?money(d.value):"",d.contactName,d.companyName,(()=>{const sp=h("span");sp.append(avatar(d.ownerUid)," ",ownerName(d.ownerUid));return sp})(),d.expectedClose],{filter:{label:"All stages",key:"stage",opts:S.me.dealStage}})},
  async contacts(){await Promise.all([load("contacts"),load("companies")]);setHead("Contacts","People you do business with",[newBtn("contacts")]);grid("contacts",["Name","Company","Contact","Responsible","Created"],c=>[c.name,c.companyName||nameOf("companies",c.companyId),[c.email,c.phone].filter(Boolean).join(" · "),ownerName(c.ownerUid),date(c.createdAt)])},
  async companies(){await load("companies");setHead("Companies","Organisations",[newBtn("companies")]);grid("companies",["Company","Industry","Website","Phone","Responsible","Created"],c=>[c.name,c.industry,c.website,c.phone,ownerName(c.ownerUid),date(c.createdAt)])},
  async tasks(){await load("tasks");setHead("Activities","Tasks, follow-ups and to-dos",[newBtn("tasks")]);const st=S.q.tasks=S.q.tasks||{};const today=new Date().toISOString().slice(0,10);
    const v=V();v.innerHTML="";const bar=h("div","crm-bar"),chips=h("div","bx-chips");const mode=st.m||"open";
    for(const[a,b]of[["open","Open"],["overdue","Overdue"],["mine","Mine"],["done","Completed"],["all","All"]]){const c=h("button","bx-chip"+(mode===a?" on":""),b);c.onclick=()=>{st.m=a;show("tasks")};chips.appendChild(c)}bar.appendChild(chips);v.appendChild(bar);
    const rs=S.data.tasks.filter(t=>mode==="all"||(mode==="done"?t.done:mode==="open"?!t.done:mode==="mine"?!t.done&&t.assigneeUid===S.me.me.uid:!t.done&&t.due&&t.due<today)).sort((a,b)=>(a.due||"9")<(b.due||"9")?-1:1);
    const w=h("div","crm-wrap"),t=h("table","crm-t");t.innerHTML="<thead><tr><th></th><th>Task</th><th>Deadline</th><th>Priority</th><th>Responsible</th></tr></thead>";const tb=h("tbody");
    for(const k of rs){const tr=h("tr","row"),c=mk("input",{type:"checkbox",checked:!!k.done});c.disabled=!can("tasks","e");c.onclick=async ev=>{ev.stopPropagation();try{await api("POST","/api/crm/save?e=tasks",{id:k.id,done:c.checked});S.data.tasks=null;show("tasks")}catch(x){alert(x.message)}};
      const td0=h("td");td0.appendChild(c);tr.appendChild(td0);const t1=h("td");t1.appendChild(h("a","ln",k.title));if(k.relName)t1.appendChild(h("small","bz-mut"," · "+k.relName));tr.appendChild(t1);
      const dtd=h("td",null,k.due?date(k.due):"");if(!k.done&&k.due&&k.due<today)dtd.style.color="var(--bad)";tr.appendChild(dtd);tr.appendChild(h("td",null,label(k.priority)));const o=h("td");o.append(avatar(k.assigneeUid)," ",ownerName(k.assigneeUid));tr.appendChild(o);tr.onclick=()=>modalTask(k);tb.appendChild(tr)}
    if(!rs.length){const tr=h("tr"),td=h("td","bz-mut","Nothing here.");td.colSpan=5;tr.appendChild(td);tb.appendChild(tr)}t.appendChild(tb);w.appendChild(t);v.appendChild(w)},
  async dashboard(){
    setHead("Revenue","Sales analytics: revenue, pipeline and what is working");const v=V();v.innerHTML="";const bar=h("div","crm-bar"),per=mk("select");
    for(const[a,b]of[["7","Last 7 days"],["30","Last 30 days"],["90","Last 90 days"],["365","Last 12 months"],["0","All time"]])per.appendChild(mk("option",{value:a,textContent:b}));per.value="30";bar.appendChild(per);v.appendChild(bar);const body=h("div");v.appendChild(body);
    const run=async()=>{body.innerHTML="";S.charts.forEach(c=>c.destroy());S.charts=[];let d;try{d=await api("GET","/api/crm/dashboard?days="+per.value)}catch(x){msg(body,x.message,1);return}
      const k=d.kpis,kp=h("div","crm-kpis");
      for(const[l,val]of[["Revenue won",money(k.revenue)],["Deals won",num(k.won)+" ("+k.winRate.toFixed(0)+"% win rate)"],["Open pipeline",money(k.pipeline)],["Weighted forecast",money(k.forecast)],["New leads",num(k.leads)],["Lead → deal",k.leadConv.toFixed(1)+"%"],["Average deal",money(k.avgDeal)],["Sales cycle",k.avgCycleDays?k.avgCycleDays.toFixed(0)+" days":"–"],["Open tasks",num(k.tasksOpen)+" ("+k.tasksOverdue+" overdue)"]]){const c=h("div","crm-kpi");add(c,h("span",null,l),h("b",null,val));kp.appendChild(c)}body.appendChild(kp);
      const g=h("div","crm-grid2");body.appendChild(g);
      const chart=(t,cfg)=>{const c=h("section","bz-card");c.appendChild(h("h3",null,t));const cv=mk("canvas");c.appendChild(cv);g.appendChild(c);S.charts.push(new Chart(cv,Object.assign({options:{responsive:true,plugins:{legend:{display:!!cfg.legend}}}},cfg)))};
      const fc=h("section","bz-card");fc.appendChild(h("h3",null,"Sales funnel"));const fn=h("div","bx-fun");const mx=Math.max(1,...A(d.stages).map(s=>s.count||0));
      for(const s of A(d.stages)){const r=h("div"),bb=h("i");bb.style.cssText="width:"+Math.max(1,(s.count||0)/mx*100)+"%;background:"+stc(s.stage);add(r,h("span",null,label(s.stage)),bb,h("b",null,(s.count||0)+" · "+money(s.value)));fn.appendChild(r)}fc.appendChild(fn);g.appendChild(fc);
      chart("Revenue won by month",{type:"bar",data:{labels:A(d.monthly).map(m=>m.month),datasets:[{label:"Revenue",data:A(d.monthly).map(m=>m.revenue),backgroundColor:"#22d3ee"}]}});
      chart("New leads by month",{type:"line",data:{labels:A(d.monthly).map(m=>m.month),datasets:[{label:"Leads",data:A(d.monthly).map(m=>m.leads),borderColor:"#f26a2e",tension:.3}]}});
      chart("Leads by status",{type:"doughnut",legend:true,data:{labels:Object.keys(d.leadStatus||{}).map(label),datasets:[{data:Object.values(d.leadStatus||{}),backgroundColor:["#22d3ee","#fbbf24","#34d399","#f87171","#8b5cf6","#94a3b8"]}]}});
      const tbl=(t,cols,rows)=>{const c=h("section","bz-card");c.style.marginBottom="14px";c.appendChild(h("h3",null,t));const w=h("div","crm-wrap"),tb=h("table","crm-t");tb.innerHTML="<thead><tr>"+cols.map(x=>"<th>"+x+"</th>").join("")+"</tr></thead>";const bd=h("tbody");for(const r of rows){const tr=h("tr");for(const x of r)tr.appendChild(h("td",null,x));bd.appendChild(tr)}if(!rows.length)bd.innerHTML="<tr><td class='bz-mut'>No data yet.</td></tr>";tb.appendChild(bd);w.appendChild(tb);c.appendChild(w);body.appendChild(c)};
      tbl("Revenue by lead source",["Source","Leads","Converted","Won deals","Revenue","Open pipeline"],A(d.bySource).map(s=>[label(s.label),s.leads,s.convRate.toFixed(0)+"%",s.won,money(s.wonValue),money(s.pipeline)]));
      tbl("Team performance",["Person","Leads","Won deals","Revenue","Open deals","Open pipeline"],A(d.byOwner).map(s=>[s.label,s.leads,s.won,money(s.wonValue),s.open,money(s.pipeline)]));
      try{const a=await api("GET","/api/crm/ads?days="+(per.value==="90"||per.value==="365"||per.value==="0"?90:30));
        if(a.connected)tbl("Meta ads vs CRM revenue (last "+a.days+" days)",["Ad spend","Leads from lead ads","Cost per lead","Won deals","Revenue","ROAS"],[[(a.adCurrency||"")+" "+num(a.spend),a.leads,a.leads?num(a.cpl):"–",a.won,money(a.revenue),a.roas?a.roas.toFixed(2)+"x":"–"]]);
        else{const c=h("section","bz-card");c.appendChild(h("p","bz-mut","Connect Facebook or Instagram under Connected social in Blitz AI to compare ad spend with CRM revenue."));body.appendChild(c)}}catch(x){}};
    per.onchange=run;run()},
  async chats(){
    setHead("Chats","Messenger and Instagram conversations, live");const v=V();v.innerHTML="";let data;try{data=A((await api("GET","/api/crm/chats")).chats)}catch(x){v.appendChild(h("p","bz-err",x.message));return}
    if(!data.length){const c=h("section","bz-card");c.appendChild(h("p",null,"No conversations yet. They appear here as soon as someone messages or comments on a connected Page or Instagram account. Connect your Pages first under Lead sources."));v.appendChild(c);return}
    const box=h("div","bx-chat"),list=h("div","bx-cl"),pane=h("div","bx-ct");box.append(list,pane);v.appendChild(box);
    const open=c=>{S.chat=c.id;list.childNodes.forEach(n=>n.classList.toggle("on",n.dataset.id===c.id));pane.innerHTML="";const hd=h("div","bx-ph");hd.style.position="static";const s=h("span",null,label(c.status||"open"));hd.append(h("h2",null,(c.name||"Contact")+" · "+label(c.channel)),pill(c.status||"open"));
      if(c.flowName)hd.appendChild(h("small","bz-mut","Automation: "+c.flowName));if(c.status==="human"&&can("leads","e")){const r=h("button","bz-mini","Resume automation");r.onclick=async()=>{await api("POST","/api/crm/chat/resume",{id:c.id});show("chats")};hd.appendChild(r)}pane.appendChild(hd);
      if(c.lastError)pane.appendChild(h("p","crm-msg err",c.lastError));
      const m=h("div","bx-cm");for(const x of A(c.msgs)){const b=h("div","bx-b "+x.d);b.append(x.t,h("small",null,ago(x.at)));m.appendChild(b)}pane.appendChild(m);m.scrollTop=m.scrollHeight;
      if(can("leads","e")){const f=h("div","bx-cf"),i=mk("input",{placeholder:"Reply as your team (pauses automations for 24 h)"}),sd=h("button","bz-btn","Send");const go=async()=>{if(!i.value.trim())return;sd.disabled=true;try{await api("POST","/api/crm/chat/send",{id:c.id,text:i.value});show("chats")}catch(x){msg(pane,x.message,1);sd.disabled=false}};sd.onclick=go;i.onkeydown=ev=>{if(ev.key==="Enter")go()};f.append(i,sd);pane.appendChild(f)}};
    for(const c of data){const it=h("div","bx-ci");it.dataset.id=c.id;const a=h("span","bx-av",(c.name||"?")[0].toUpperCase());a.style.background=c.channel==="instagram"?"#d6249f":"#1877f2";const t=h("div");add(t,h("b",null,c.name||"Contact"),h("small",null,(A(c.msgs).slice(-1)[0]||{}).t||""));add(it,a,t);it.onclick=()=>open(c);list.appendChild(it)}
    open(data.find(c=>c.id===S.chat)||data[0])},
  async automations(){
    setHead("Automations","Reply to comments and messages automatically, and capture leads",[(()=>{if(!can("integrations","c"))return null;const b=h("button","bz-btn","+ New automation");b.onclick=()=>flowEditor(null);return b})()]);
    const v=V();v.innerHTML="";let fl;try{fl=A((await api("GET","/api/crm/flows")).flows)}catch(x){v.appendChild(h("p","bz-err",x.message));return}
    if(!fl.length){const c=h("section","bz-card");c.appendChild(h("h3",null,"Start from a template"));c.appendChild(h("p","bz-mut","Pick one, adjust the wording and switch it on."));const r=h("div","crm-row");for(const[k,t]of Object.entries(TEMPLATES)){const b=h("button","bz-btn ghost",t.name);b.onclick=()=>flowEditor(null,t);r.appendChild(b)}c.appendChild(r);v.appendChild(c)}
    const g=h("div","bx-flows");v.appendChild(g);
    for(const f of fl){const c=h("div","bx-flow"),top=h("div","crm-row");top.style.cssText="margin:0;justify-content:space-between";const sw=h("button","bx-sw"+(f.active?" on":""));sw.title="Switch on/off";sw.disabled=!can("integrations","e");
      sw.onclick=async()=>{await api("POST","/api/crm/flow/save",Object.assign({},f,{active:!f.active}));show("automations")};add(top,h("h4",null,f.name),sw);
      const st=f.stats||{};add(c,top,h("p",null,TRIG[f.trigger]+(A(f.keywords).length?": “"+A(f.keywords).join("”, “")+"”":"")+" · "+label(f.channel==="both"?"Facebook + Instagram":f.channel)),h("p",null,A(f.steps).length+" step(s) · triggered "+(st.triggered||0)+" · completed "+(st.completed||0)+" · contacts captured "+(st.leads||0)));
      if(can("integrations","e")){const r=h("div","crm-row");r.style.margin="0";const e=h("button","bz-mini","Edit");e.onclick=()=>flowEditor(f);r.appendChild(e);if(can("integrations","d")){const d=h("button","bz-mini","Delete");d.onclick=async()=>{if(confirm("Delete this automation?")){await api("POST","/api/crm/flow/delete",{id:f.id});show("automations")}};r.appendChild(d)}c.appendChild(r)}g.appendChild(c)}
    const n=h("section","bz-card");n.style.marginTop="14px";n.innerHTML="<h3>How it works</h3><p class='bz-mut'>Connect your Pages under Lead sources first. When someone comments a keyword or messages you, the automation replies, can ask for their name, phone or email and saves the answers into the lead. Replying yourself in Chats pauses automations for that person for 24 hours. Facebook and Instagram only allow automated replies within 24 hours of the person's message or comment.</p>";v.appendChild(n)},
  async sources(){
    setHead("Lead sources","Where new leads come from");const v=V();v.innerHTML="";let I;try{I=await api("GET","/api/crm/integrations")}catch(x){v.appendChild(h("p","bz-err",x.message));return}
    const codeBox=(t,val)=>{const c=h("div");c.appendChild(h("h4",null,t));c.appendChild(h("div","crm-code",val));const r=h("div","crm-row");r.style.marginTop="6px";const cp=h("button","bz-mini","Copy");cp.onclick=()=>copy(val,cp);r.appendChild(cp);c.appendChild(r);return c};
    const a=h("section","bz-card");a.appendChild(h("h3",null,"1. Website form and webhook"));a.appendChild(h("p","bz-mut","Every submission becomes a lead with the source Web form. Use the hosted page, embed it on your site, or post JSON from any form tool, Zapier or Make."));
    add(a,codeBox("Hosted contact form (share this link)",I.formUrl),codeBox("Embed on your website",I.embed),codeBox("Webhook URL (POST name, email, phone, company, message)",I.webhookUrl),codeBox("Test it",I.curl));
    if(can("integrations","e")){const r=h("div","crm-row"),b=h("button","bz-btn ghost","Rotate secret link");b.onclick=async()=>{if(confirm("The old form and webhook links will stop working. Continue?")){await api("POST","/api/crm/integrations/rotate",{});show("sources")}};r.appendChild(b);a.appendChild(r)}v.appendChild(a);
    const m=h("section","bz-card");m.style.marginTop="14px";m.appendChild(h("h3",null,"2. Meta: lead ads, messages and comments"));
    m.appendChild(h("p","bz-mut","Uses the Facebook account you connected under Connected social in Blitz AI. Lead forms are pulled with Sync. Messages and comments arrive live once the webhook below is added in your Meta app, and drive the Automations and Chats screens. For the public (not just app admins and testers) Meta needs app review for leads_retrieval, pages_messaging, pages_manage_engagement and instagram_manage_messages. Set CRM chat automation permissions to 1 in Admin and reconnect Facebook to request them."));
    if(A(I.metaPages).length)m.appendChild(h("div","crm-wrap")).appendChild((()=>{const t=h("table","crm-t");t.innerHTML="<thead><tr><th>Connected page</th><th>Type</th><th>Last sync</th></tr></thead>";const b=h("tbody");for(const p of I.metaPages){const tr=h("tr");for(const x of[p.name,label(p.kind),p.lastSync?ago(p.lastSync):"never"])tr.appendChild(h("td",null,x));b.appendChild(tr)}t.appendChild(b);return t})());
    if(can("integrations","e")){const r=h("div","crm-row"),c=h("button","bz-btn","Connect my Pages"),s=h("button","bz-btn ghost","Sync lead forms now");
      c.onclick=async()=>{c.disabled=true;try{const x=await api("POST","/api/crm/meta/connect",{});msg(m,"Connected "+x.pages+" Page(s)."+(A(x.warnings).length?" Notes: "+x.warnings.join(" | "):""));setTimeout(()=>show("sources"),1500)}catch(e){msg(m,e.message,1);c.disabled=false}};
      s.onclick=async()=>{s.disabled=true;msg(m,"Reading your lead forms…");try{const x=await api("POST","/api/crm/meta/sync",{});msg(m,"Added "+x.added+" new leads (checked "+x.checked+")."+(A(x.warnings).length?" Notes: "+x.warnings.join(" | "):""));S.data.leads=null}catch(e){msg(m,e.message,1)}s.disabled=false};add(r,c,s);m.appendChild(r)}
    const wh=I.metaWebhook;add(m,codeBox("Meta webhook callback URL",wh.url),codeBox("Verify token",wh.verifyToken));
    m.appendChild(h("p","bz-mut",wh.appSecretSet?"In your Meta app: Webhooks > Page (and Instagram) > paste the callback URL and verify token > subscribe to leadgen, messages and feed (comments for Instagram).":"FACEBOOK_APP_SECRET is not set on the server, so the live webhook is off. Sync still works."));v.appendChild(m);
    const lg=h("section","bz-card");lg.style.marginTop="14px";lg.appendChild(h("h3",null,"Meta activity log (what Meta has sent us)"));
    const L=A(I.hookLog);if(!L.length)lg.appendChild(h("p","bz-mut","Nothing received from Meta yet. If you commented or sent a message and this stays empty: switch your Meta app to Live, subscribe the webhook fields, and make sure the Instagram account allows access to messages (Instagram > Settings > Messages and story replies > Message controls > Connected tools)."));
    else{const t=h("table","crm-t");t.innerHTML="<thead><tr><th>When</th><th>What</th><th>Result</th></tr></thead>";const b=h("tbody");for(const x of L){const tr=h("tr");for(const y of[ago(x.at),x.kind,x.detail])tr.appendChild(h("td",null,y));b.appendChild(tr)}t.appendChild(b);const w=h("div","crm-wrap");w.appendChild(t);lg.appendChild(w)}
    v.appendChild(lg);
    const c=h("section","bz-card");c.style.marginTop="14px";c.appendChild(h("h3",null,"3. Add by hand or import"));c.appendChild(h("p","bz-mut","Use Leads > + Create, or Leads > List > Import CSV. Duplicates are detected by email and phone."));v.appendChild(c)},
  async team(){
    S.team=await api("GET","/api/crm/team");setHead("Team & access","Company structure, roles and access");const v=V();v.innerHTML="";const T=S.team;T.members=A(T.members);T.departments=A(T.departments);T.invites=A(T.invites);
    const dn=id=>(T.departments.find(d=>d.id===id)||{}).name||"–";
    const tbl=(cols,rows,onRow)=>{const w=h("div","crm-wrap"),t=h("table","crm-t");t.innerHTML="<thead><tr>"+cols.map(x=>"<th>"+x+"</th>").join("")+"</tr></thead>";const b=h("tbody");for(const r of rows){const tr=h("tr",onRow&&onRow(r.rec)?"row":"");for(const x of r.cells){const td=h("td");if(x instanceof Node)td.appendChild(x);else td.textContent=x==null?"":x;tr.appendChild(td)}if(onRow&&onRow(r.rec))tr.onclick=()=>onRow(r.rec)();b.appendChild(tr)}if(!rows.length)b.innerHTML="<tr><td class='bz-mut'>Nothing here yet.</td></tr>";t.appendChild(b);w.appendChild(t);return w};
    const mc=h("section","bz-card");mc.appendChild(h("h3",null,"People"));
    mc.appendChild(tbl(["Name","Email","Role","Department","Sees","Status"],T.members.map(m=>({cells:[m.name,m.email||"",label(m.role),dn(m.dept),m.scope?label(m.scope==="dept"?"department":m.scope):"default",m.status==="disabled"?"Off":"Active"],rec:m})),m=>can("team","e")&&m.uid!==S.me.me.uid&&m.role!=="owner"?()=>memberEdit(m):null));
    if(can("team","c")){const r=h("div","crm-row"),b=h("button","bz-btn","+ Invite person");b.onclick=inviteForm;r.appendChild(b);mc.appendChild(r)}v.appendChild(mc);
    if(T.invites.length){const ic=h("section","bz-card");ic.style.marginTop="14px";ic.appendChild(h("h3",null,"Pending invitations"));ic.appendChild(h("p","bz-mut","They join automatically when they sign in to Blitz AI with that email address."));
      ic.appendChild(tbl(["Email","Role","Department","Email sent",""],T.invites.map(i=>{const r=h("span");const rs=h("button","bz-mini","Resend email");rs.onclick=async()=>{rs.disabled=true;try{const x=await api("POST","/api/crm/invite/resend",{email:i.email});showInviteResult(x,i.email)}catch(e){alert(e.message)}rs.disabled=false};const c=h("button","bz-mini","Cancel");c.onclick=async()=>{await api("POST","/api/crm/invite/cancel",{email:i.email});show("team")};if(can("team","c"))r.appendChild(rs);r.append(" ");if(can("team","d"))r.appendChild(c);return{cells:[i.email,label(i.role),dn(i.dept),i.emailedAt?ago(i.emailedAt):"not yet",r]}}),()=>null));v.appendChild(ic)}
    const dc=h("section","bz-card");dc.style.marginTop="14px";dc.appendChild(h("h3",null,"Company structure (departments)"));
    const tree=(p,depth)=>{for(const d of T.departments.filter(x=>(x.parentId||"")===p)){const r=h("div","crm-row");r.style.marginLeft=depth*22+"px";const cnt=T.members.filter(m=>m.dept===d.id).length;r.appendChild(h("b",null,d.name));r.appendChild(h("span","bz-mut",cnt+" people"+(d.headUid?" · head: "+ownerName(d.headUid):"")));if(can("team","e")){const e=h("button","bz-mini","Edit");e.onclick=()=>deptForm(d);r.appendChild(e)}dc.appendChild(r);tree(d.id,depth+1)}};
    tree("",0);if(!T.departments.length)dc.appendChild(h("p","bz-mut","No departments yet. Add Sales, Marketing, Support… and put people in them."));
    if(can("team","c")){const r=h("div","crm-row"),b=h("button","bz-btn ghost","+ Add department");b.onclick=()=>deptForm();r.appendChild(b);dc.appendChild(r)}v.appendChild(dc);
    const rc=h("section","bz-card");rc.style.marginTop="14px";rc.appendChild(h("h3",null,"What each role can do"));rc.appendChild(tbl(["Role","What they can do"],A(S.me.roles).map(r=>({cells:[r.label,r.desc]})),()=>null));
    rc.appendChild(h("p","bz-mut","Owner and Administrator see all records. Others see their own, their department's, or everything, as set per person. Unassigned leads can be claimed by anyone who can edit leads."));v.appendChild(rc);
    if(can("team","e")){const sc=h("section","bz-card");sc.style.marginTop="14px";sc.appendChild(h("h3",null,"Workspace settings"));const w=S.me.workspace,f=formEl([{k:"name",label:"Company name"},{k:"currency",label:"Currency (3 letters)"},{k:"autoAssign",label:"Auto-assign new inbound leads",type:"select",opts:[["no","No, leave unassigned"],["yes","Yes, round-robin to Sales and Managers"]]}],{name:w.name,currency:w.currency,autoAssign:w.autoAssign?"yes":"no"});const b=h("button","bz-btn","Save settings");b.onclick=async()=>{const o=f.get();try{await api("POST","/api/crm/settings",{name:o.name,currency:o.currency,autoAssign:o.autoAssign==="yes"});S.me=await api("GET","/api/crm/me");msg(sc,"Saved.")}catch(x){msg(sc,x.message,1)}};add(sc,f,b);v.appendChild(sc)}}
};
const TRIG={comment:"Comment on a post",dm_keyword:"Keyword in a message",welcome:"First message (welcome)",default:"Default reply"};
const TEMPLATES={
  magnet:{name:"Comment → DM (lead magnet)",trigger:"comment",keywords:["info","price","guide"],commentReply:"Thanks {{first_name}}! Check your DMs \u{1F4E9}",tag:"comment-lead",steps:[{type:"message",text:"Hi {{first_name}}! Thanks for your interest. Here is what you asked for."},{type:"ask",field:"phone",text:"What's the best phone number to reach you on?"},{type:"ask",field:"email",text:"And your email address?"},{type:"message",text:"Perfect, our team will contact you shortly!"}]},
  welcome:{name:"Welcome message",trigger:"welcome",keywords:[],steps:[{type:"message",text:"Hi {{first_name}}, welcome! How can we help you today?"},{type:"ask",field:"note",text:"Tell us briefly what you are looking for."},{type:"message",text:"Thank you! A team member will reply soon."}]},
  price:{name:"Keyword: price",trigger:"dm_keyword",keywords:["price","cost","how much"],steps:[{type:"message",text:"Great question! Prices depend on your needs."},{type:"ask",field:"phone",text:"Leave your number and we'll call you with a quote."},{type:"message",text:"Thanks! We'll be in touch."}]}
};
function flowEditor(f,tpl){
  const d=Object.assign({name:"",active:true,channel:"both",trigger:"comment",match:"contains",keywords:[],postId:"",commentReply:"",tag:"",steps:[{type:"message",text:""}]},tpl||{},f||{});d.steps=A(d.steps).map(s=>Object.assign({},s));
  const sl=$("slider");sl.innerHTML="";sl.hidden=false;sl.onclick=ev=>{if(ev.target===sl)close()};const P=h("div","bx-panel");sl.appendChild(P);
  const hd=h("div","bx-ph");const save=h("button","bz-btn","Save"),x=h("button","bz-mini","✕ Close");x.onclick=close;add(hd,h("h2",null,f?"Edit automation":"New automation"),save,x);P.appendChild(hd);
  const body=h("div");body.style.padding="16px 20px 30px";P.appendChild(body);
  const s1=h("div","bx-sec");s1.appendChild(h("h3",null,"1. When should it start?"));const tg=h("div","bx-trig");
  const info={comment:"Someone comments on a post or reel (optionally with a keyword)",dm_keyword:"Someone sends a message containing your keyword",welcome:"Someone messages you for the first time",default:"Any message no other automation answers"};
  const drawT=()=>{tg.innerHTML="";for(const k of Object.keys(TRIG)){const b=h("button",d.trigger===k?"on":"");add(b,h("b",null,TRIG[k]),h("small",null,info[k]));b.onclick=()=>{d.trigger=k;drawT();ex.hidden=false;kwl.hidden=k==="welcome"||k==="default";pl.hidden=k!=="comment";cl.hidden=k!=="comment"};tg.appendChild(b)}};drawT();s1.appendChild(tg);
  const ex=h("div","form");const nm=mk("input",{value:d.name,placeholder:"e.g. Spring offer"}),ch=mk("select"),ky=mk("input",{value:A(d.keywords).join(", "),placeholder:"price, info, guide (leave empty = any comment)"}),mt=mk("select"),po=mk("input",{value:d.postId,placeholder:"Optional: post ID to limit it to one post"}),cr=mk("input",{value:d.commentReply,placeholder:"Optional public reply, e.g. Thanks! Check your DMs"}),tgI=mk("input",{value:d.tag,placeholder:"Optional tag added to the lead"});
  for(const[v2,t]of[["both","Facebook + Instagram"],["facebook","Facebook only"],["instagram","Instagram only"]])ch.appendChild(mk("option",{value:v2,textContent:t}));ch.value=d.channel;for(const[v2,t]of[["contains","Message contains the keyword"],["exact","Message is exactly the keyword"]])mt.appendChild(mk("option",{value:v2,textContent:t}));mt.value=d.match;
  const L=(t,el,cls)=>{const l=h("label",cls,t);l.appendChild(el);ex.appendChild(l);return l};L("Automation name",nm,"wide");L("Channel",ch);const kwl=L("Keywords (comma separated)",ky);L("Keyword matching",mt);const pl=L("Only this post (ID)",po);const cl=L("Public reply to the comment",cr,"wide");L("Tag the lead",tgI);kwl.hidden=d.trigger==="welcome"||d.trigger==="default";pl.hidden=d.trigger!=="comment";cl.hidden=d.trigger!=="comment";s1.appendChild(ex);body.appendChild(s1);
  const s2=h("div","bx-sec");s2.appendChild(h("h3",null,"2. What should it send?"));const stp=h("div");s2.appendChild(stp);
  const drawS=()=>{stp.innerHTML="";d.steps.forEach((s,i)=>{const b=h("div","bx-step "+s.type);const top=h("div","crm-row");top.style.cssText="margin:0 0 8px;justify-content:space-between";top.appendChild(h("b",null,(i+1)+". "+(s.type==="ask"?"Ask a question and save the answer":"Send a message")));const rm=h("button","bz-mini","Remove");rm.onclick=()=>{d.steps.splice(i,1);drawS()};top.appendChild(rm);b.appendChild(top);
    const ta=mk("textarea",{rows:2,value:s.text,placeholder:s.type==="ask"?"The question to ask":"Message text. Use {{first_name}} to personalise."});ta.oninput=()=>s.text=ta.value;b.appendChild(ta);
    if(s.type==="ask"){const fs=mk("select");for(const[v2,t]of[["phone","Save as phone number"],["email","Save as email"],["name","Save as name"],["note","Save as a note"]])fs.appendChild(mk("option",{value:v2,textContent:t}));fs.value=s.field||"note";fs.onchange=()=>s.field=fs.value;fs.style.marginTop="8px";b.appendChild(fs)}
    else{const r=h("div","crm-row"),bt=mk("input",{value:s.btnTitle||"",placeholder:"Button text (optional)",maxLength:20}),bu=mk("input",{value:s.btnUrl||"",placeholder:"Button link https://…"});bt.oninput=()=>s.btnTitle=bt.value;bu.oninput=()=>s.btnUrl=bu.value;bu.style.flex="1";r.append(bt,bu);b.appendChild(r)}stp.appendChild(b)});
    const r=h("div","crm-row"),a1=h("button","bz-btn ghost","+ Message"),a2=h("button","bz-btn ghost","+ Ask a question");a1.onclick=()=>{d.steps.push({type:"message",text:""});drawS()};a2.onclick=()=>{d.steps.push({type:"ask",field:"phone",text:""});drawS()};r.append(a1,a2);stp.appendChild(r)};drawS();body.appendChild(s2);
  const s3=h("div","bx-sec"),ac=mk("input",{type:"checkbox",checked:!!d.active}),al=h("label",null," Switch this automation on");al.prepend(ac);s3.appendChild(al);body.appendChild(s3);
  save.onclick=async()=>{const o={id:f?f.id:"",name:nm.value,active:ac.checked,channel:ch.value,trigger:d.trigger,match:mt.value,keywords:ky.value.split(",").map(x=>x.trim()).filter(Boolean),postId:po.value,commentReply:cr.value,tag:tgI.value,steps:d.steps};save.disabled=true;try{await api("POST","/api/crm/flow/save",o);close();show("automations")}catch(e){msg(body,e.message,1);save.disabled=false}};
}
function modalTask(k){const f=formEl(ENT.tasks.fields(),k),b=h("div");b.appendChild(f);const row=h("div","crm-row"),s=h("button","bz-btn","Save");s.onclick=async()=>{const o=f.get();o.id=k.id;try{await api("POST","/api/crm/save?e=tasks",o);S.data.tasks=null;close();show("tasks")}catch(x){msg(b,x.message,1)}};if(can("tasks","e"))row.appendChild(s);if(can("tasks","d")){const d=h("button","bz-btn ghost","Delete");d.onclick=async()=>{if(!confirm("Delete this task?"))return;await api("POST","/api/crm/delete?e=tasks",{id:k.id});S.data.tasks=null;close();show("tasks")};row.appendChild(d)}b.appendChild(row);modal("Task",b)}
function showInviteResult(r,email){const b=h("div");
  b.appendChild(h("p",null,r.emailed?"✅ Invitation email sent to "+email+".":"⚠️ The email was not sent."+(r.emailError?" "+r.emailError:"")));
  b.appendChild(h("p","bz-mut","You can also send this link yourself. They must sign in or create an account with exactly "+email+" and they join automatically."));b.appendChild(h("div","crm-code",r.joinUrl));
  const row=h("div","crm-row"),cp=h("button","bz-btn","Copy link");cp.onclick=()=>copy(r.joinUrl,cp);const ml=h("a","bz-btn ghost","Open in my email app");ml.href="mailto:"+encodeURIComponent(email)+"?subject="+encodeURIComponent("Join our CRM")+"&body="+encodeURIComponent("You are invited to our CRM. Sign in with this email address ("+email+"): "+r.joinUrl);row.append(cp,ml);b.appendChild(row);
  modal("Invitation",b);}
function memberEdit(m){
  const b=h("div"),f=formEl([{k:"role",label:"Role",type:"select",opts:A(S.me.roles).filter(r=>r.key!=="owner").map(r=>[r.key,r.label])},{k:"dept",label:"Department",type:"select",opts:[["","None"],...S.team.departments.map(d=>[d.id,d.name])]},{k:"scope",label:"Records they can see",type:"select",opts:[["default","Default for the role"],["own","Only their own"],["dept","Their department"],["all","Everything"]]},{k:"status",label:"Access",type:"select",opts:[["active","Active"],["disabled","Turned off"]]}],{role:m.role,dept:m.dept,scope:m.scope||"default",status:m.status||"active"});
  const s=h("button","bz-btn","Save"),r=h("button","bz-btn ghost","Remove from workspace");
  s.onclick=async()=>{try{await api("POST","/api/crm/member/save",Object.assign({uid:m.uid},f.get()));close();show("team")}catch(x){msg(b,x.message,1)}};
  r.onclick=async()=>{if(!confirm("Remove "+m.name+"?"))return;try{await api("POST","/api/crm/member/remove",{uid:m.uid});close();show("team")}catch(x){msg(b,x.message,1)}};
  const row=h("div","crm-row");add(row,s,can("team","d")?r:null);add(b,f,row);modal(m.name,b)}
function inviteForm(){
  const b=h("div"),f=formEl([{k:"email",label:"Email address",type:"email",req:1,wide:1},{k:"role",label:"Role",type:"select",opts:A(S.me.roles).filter(r=>r.key!=="owner"&&r.key!==S.me.me.role&&(S.me.me.role==="owner"||r.key!=="admin")).map(r=>[r.key,r.label])},{k:"dept",label:"Department",type:"select",opts:[["","None"],...S.team.departments.map(d=>[d.id,d.name])]},{k:"scope",label:"Records they can see",type:"select",opts:[["","Default for the role"],["own","Only their own"],["dept","Their department"],["all","Everything"]]}],{role:"sales"});
  const go=h("button","bz-btn","Send invitation");go.onclick=async()=>{go.disabled=true;try{const o=f.get(),r=await api("POST","/api/crm/invite",o);S.team=null;showInviteResult(r,o.email.trim().toLowerCase())}catch(x){msg(b,x.message,1);go.disabled=false}};
  add(b,f,go);modal("Invite a team member",b)}
function deptForm(d){
  const b=h("div"),f=formEl([{k:"name",label:"Department name",req:1},{k:"parentId",label:"Part of",type:"select",opts:[["","Top level"],...S.team.departments.filter(x=>!d||x.id!==d.id).map(x=>[x.id,x.name])]},{k:"headUid",label:"Head of department",type:"select",opts:ownerOpts()}],d?{name:d.name,parentId:d.parentId,headUid:d.headUid}:{});
  const s=h("button","bz-btn",d?"Save":"Create"),x=h("button","bz-btn ghost","Delete");
  s.onclick=async()=>{try{await api("POST","/api/crm/dept/save",Object.assign({id:d?d.id:""},f.get()));close();show("team")}catch(e){msg(b,e.message,1)}};
  x.onclick=async()=>{if(!confirm("Delete this department?"))return;try{await api("POST","/api/crm/dept/delete",{id:d.id});close();show("team")}catch(e){msg(b,e.message,1)}};
  const row=h("div","crm-row");add(row,s,d&&can("team","d")?x:null);add(b,f,row);modal(d?"Edit department":"New department",b)}

// ---------------------------------------------------------------- shell
const SECTIONS=[
 {t:"CRM",items:[["dashboard","Revenue","reports","trend"],["leads","Leads","leads","userplus"],["deals","Deals","deals","layers"],["contacts","Contacts","contacts","users"],["companies","Companies","companies","grid"],["tasks","Activities","tasks","check"]]},
 {t:"Engage",items:[["chats","Chats","leads","send"],["automations","Automations","integrations","zap"],["sources","Lead sources","integrations","link"]]},
 {t:"Analytics",fold:true,open:true,items:[["an:website","Website","reports","globe",{k:"website",sub:"Speed, SEO and fixes for any website"}],["an:youtube","YouTube","reports","play",{k:"youtube",sub:"Channel stats and growth plan"}],["an:google","Connected Google","reports","link",{k:"google",sub:"Connected Google analytics and search data"}],["an:socialc","Connected social","reports","share",{k:"socialc",sub:"Facebook, Instagram and TikTok: posts, views, fixes"}],["an:social","Other social (manual)","reports","users",{k:"social",sub:"Manual social profile analysis"}],["an:app","App","reports","phone",{k:"app",sub:"Store listing and app growth"}],["an:runads","Run ads","reports","zap",{k:"runads",sub:"Launch and manage paid campaigns"}],["an:adcreative","Ad creative","reports","film",{k:"adcreative",sub:"Scripts, briefs and finished ads"}],["an:seo","SEO & Ads","reports","search",{k:"seo",sub:"Keywords, competitors and market position"}],["an:mreport","Marketing report","reports","bars",{k:"mreport",sub:"All your results in one marketing report",b:"AI"}],["an:publish","Publish to all","reports","send",{k:"publish",sub:"Post to every connected account"}],["an:optimizer","Optimizer & Publish","reports","star",{k:"optimizer",sub:"Optimise and publish videos"}]]},
 {t:"Admin",items:[["team","Team & access","team","share"]]}
];
const MENU=SECTIONS.flatMap(x=>x.items);
let frameReady=false;
function nav(k){S.view=k;document.querySelectorAll("#nav .bz-nav").forEach(b=>b.classList.toggle("on",b.dataset.k===k));const g=document.querySelector('#nav .bz-nav.on');if(g&&g.parentElement.classList.contains("shut")){g.parentElement.classList.remove("shut");g.parentElement.previousElementSibling.classList.remove("shut")}close();show(k)}
async function show(k){S.charts.forEach(c=>c.destroy());S.charts=[];const an=k.startsWith("an:");$("anwrap").hidden=!an;$("view").hidden=an;
  if(an){const it=MENU.find(m=>m[0]===k),o=it[4];setHead(it[1],o.sub);$("tabs").hidden=true;const f=$("anframe");
    if(!frameReady){frameReady=true;const ex=new URLSearchParams(location.search);const keep=["google","social","p","why"].filter(x=>ex.get(x)).map(x=>"&"+x+"="+encodeURIComponent(ex.get(x))).join("");f.src="/dashboard?embed=1&kind="+o.k+keep;if(keep)history.replaceState(null,"","/crm")}else f.contentWindow.postMessage({bzKind:o.k},location.origin);window.scrollTo(0,0);return}
  try{if(!S.team)S.team=await api("GET","/api/crm/team");await VIEWS[k]()}catch(x){V().innerHTML="";V().appendChild(h("p","bz-err",x.message))}}
window.addEventListener("message",e=>{if(e.origin===location.origin&&e.data&&e.data.bzH)$("anframe").style.height=Math.max(e.data.bzH,innerHeight-150)+"px"});
function globalSearch(){
  const i=$("gsearch"),r=$("gres");let tm;
  i.oninput=()=>{clearTimeout(tm);tm=setTimeout(async()=>{const q=i.value.trim().toLowerCase();if(q.length<2){r.hidden=true;return}
    const ents=["leads","deals","contacts","companies"].filter(e=>can(e,"v"));await Promise.all(ents.map(e=>load(e)));r.innerHTML="";let n=0;
    for(const e of ents)for(const x of S.data[e]){if(n>=12)break;if(JSON.stringify(Object.values(x)).toLowerCase().includes(q)){n++;const d=h("div");add(d,h("span",null,x.name||x.title),h("small",null,label(ENT[e].one)));d.onclick=()=>{r.hidden=true;i.value="";openRecord(e,x)};r.appendChild(d)}}
    if(!n)r.appendChild(h("div","bz-mut","No matches"));r.hidden=false},250)};
  document.addEventListener("click",ev=>{if(!ev.target.closest(".bx-search"))r.hidden=true})}
function boot(){
  const me=S.me;$("who").textContent=me.workspace.name+" \u00b7 "+label(me.me.role);const n=$("nav");n.innerHTML="";let first=null;
  for(const sec of SECTIONS){const its=sec.items.filter(i=>can(i[2],"v"));if(!its.length)continue;
    const sh=sec.fold&&!sec.open;const hd=h("div","bx-sec-h"+(sec.fold?" fold"+(sh?" shut":""):""));add(hd,h("span","tx",sec.t),sec.fold?h("span","car","\u25BC"):null);n.appendChild(hd);const grp=h("div","bx-grp"+(sh?" shut":""));n.appendChild(grp);
    if(sec.fold)hd.onclick=()=>{hd.classList.toggle("shut");grp.classList.toggle("shut")};
    for(const[k,l,m,ic,o]of its){if(!first&&!k.startsWith("an:"))first=k;const b=h("button","bz-nav");b.dataset.k=k;b.title=l;b.append(icon(ic,18),h("span",null,l));if(o&&o.b)b.appendChild(h("span","bdg",o.b));b.onclick=()=>nav(k);grp.appendChild(b)}}
  try{if(localStorage.getItem("bz-rc")==="1")document.body.classList.add("rc")}catch(e){}
  $("collapse").onclick=()=>{const c=document.body.classList.toggle("rc");try{localStorage.setItem("bz-rc",c?"1":"0")}catch(e){}};
  const pm=$("plusm");pm.innerHTML="";for(const e of["leads","deals","contacts","companies","tasks"])if(can(e,"c")){const b=h("button",null,label(ENT[e].one));b.onclick=()=>{pm.hidden=true;openRecord(e,null)};pm.appendChild(b)}
  $("plus").onclick=ev=>{ev.stopPropagation();pm.hidden=!pm.hidden};document.addEventListener("click",()=>pm.hidden=true);globalSearch();
  if(!first&&!MENU.some(m=>can(m[2],"v"))){V().innerHTML="<p class='bz-mut'>Your role has no modules yet. Ask an administrator.</p>";return}
  const want=new URLSearchParams(location.search).get("an"),wk=MENU.find(m=>m[0]==="an:"+want);nav(wk?wk[0]:(first||"an:website"))}
function gate(st,me){
  setHead("CRM","");$("nav").innerHTML="";const v=V();v.innerHTML="";const c=h("section","bz-card");v.appendChild(c);
  if(st==="none"){c.appendChild(h("h3",null,"Set up your company CRM"));c.appendChild(h("p","bz-mut","Create a workspace for your company. You become the owner and can then invite your team, build departments and decide who can see what. If a colleague invited you, sign in with the invited email address and refresh this page."));
    const f=formEl([{k:"name",label:"Company name",req:1},{k:"me",label:"Your name"}],{});const b=h("button","bz-btn","Create workspace");b.onclick=async()=>{b.disabled=true;try{await api("POST","/api/crm/workspace",f.get());start()}catch(x){msg(c,x.message,1);b.disabled=false}};add(c,f,b)}
  else if(st==="unverified")c.appendChild(h("p",null,"An invitation is waiting for "+me.email+". Verify your email address (check your inbox for the Blitz AI verification email), sign in again and refresh this page."));
  else if(st==="disabled")c.appendChild(h("p","bz-err","Your access to this workspace was turned off by an administrator."))}
async function start(){try{const me=await api("GET","/api/crm/me");S.me=me;if(me.state!=="ok"){gate(me.state,me);return}S.team=await api("GET","/api/crm/team");boot()}catch(x){V().innerHTML="";V().appendChild(h("p","bz-err",x.message))}}
onAuthStateChanged(auth,u=>{if(!u){location.href="/app";return}start()});
