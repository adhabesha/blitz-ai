import{initializeApp}from"https://www.gstatic.com/firebasejs/10.14.1/firebase-app.js";
import{getAuth,onAuthStateChanged}from"https://www.gstatic.com/firebasejs/10.14.1/firebase-auth.js";
import{h,add,api,setAuth,copy,icon}from"./ui.js";

const auth=getAuth(initializeApp(window.FB));setAuth(auth);
const $=i=>document.getElementById(i);
const S={me:null,team:null,data:{},view:"dashboard",charts:[]};
const V=()=>$("view");
document.querySelectorAll(".bz-theme button").forEach(b=>b.onclick=()=>{document.documentElement.dataset.theme=b.dataset.t;try{localStorage.setItem("bz-theme",b.dataset.t)}catch(e){}});

const can=(m,a)=>((S.me.perms||{})[m]||"").includes(a);
const money=v=>{try{return new Intl.NumberFormat(undefined,{style:"currency",currency:S.me.workspace.currency||"USD",maximumFractionDigits:0}).format(v||0)}catch(e){return String(Math.round(v||0))}};
const num=v=>new Intl.NumberFormat().format(Math.round(v||0));
const date=t=>t?new Date(t).toLocaleDateString():"";
const ago=t=>{if(!t)return"";const s=(Date.now()-new Date(t))/1000;return s<60?"just now":s<3600?Math.floor(s/60)+" min ago":s<86400?Math.floor(s/3600)+" h ago":date(t)};
const label=s=>(s||"").replace(/_/g," ").replace(/^./,c=>c.toUpperCase());
const pill=s=>{const p=h("span","crm-pill "+s,label(s));return p};
const members=()=>(S.team?S.team.members:[]);
const ownerName=u=>{if(!u)return"Unassigned";const m=members().find(x=>x.uid===u);return m?m.name:"Former member"};
const SOURCES=["manual","csv_import","web_form","meta_lead_ad","fb_dm","fb_comment","ig_dm","ig_comment","referral","phone","event","other"];
const msg=(p,t,err)=>{let m=p.querySelector(".crm-msg");if(!m){m=h("div","crm-msg");p.appendChild(m)}m.textContent=t;m.className="crm-msg"+(err?" err":"")};

// ---------------------------------------------------------------- modal + forms
function modal(title,body,wide){
  const m=$("modal");m.innerHTML="";m.hidden=false;
  const b=h("div","crm-box");if(wide)b.style.width="min(980px,100%)";
  const hd=h("div","crm-row");hd.style.justifyContent="space-between";hd.style.marginTop="0";
  const x=h("button","bz-mini","Close");x.onclick=close;add(hd,h("h3",null,title),x);add(b,hd,body);m.appendChild(b);
  m.onclick=e=>{if(e.target===m)close()};
}
function close(){$("modal").hidden=true;$("modal").innerHTML=""}
function mk(tag,props){const e=document.createElement(tag);Object.assign(e,props||{});return e}
function formEl(fields,vals){
  const f=h("div","form"),inputs={};
  for(const d of fields){
    const l=h("label",d.wide?"wide":null,d.label+(d.req?" *":""));let i;
    if(d.type==="select"){i=mk("select");for(const[v,t]of d.opts){const o=mk("option",{value:v,textContent:t});i.appendChild(o)}}
    else if(d.type==="textarea"){i=mk("textarea",{rows:3})}
    else i=mk("input",{type:d.type==="tags"?"text":d.type||"text"});
    let v=vals&&vals[d.k];if(d.type==="tags"&&Array.isArray(v))v=v.join(", ");
    if(v!=null)i.value=v;l.appendChild(i);f.appendChild(l);inputs[d.k]=i;
  }
  f.get=()=>{const o={};for(const d of fields){let v=inputs[d.k].value;if(d.type==="number")v=parseFloat(v)||0;if(d.type==="tags")v=v.split(",").map(x=>x.trim()).filter(Boolean);o[d.k]=v}return o};
  f.inputs=inputs;return f;
}
const ownerOpts=()=>[["","Unassigned"],...members().filter(m=>m.status!=="disabled").map(m=>[m.uid,m.name])];
const sel=a=>a.map(x=>[x,label(x)]);

const ENT={
  leads:{one:"lead",fields:()=>[{k:"name",label:"Name",req:1},{k:"company",label:"Company"},{k:"email",label:"Email",type:"email"},{k:"phone",label:"Phone"},{k:"title",label:"Job title"},{k:"source",label:"Source",type:"select",opts:sel(SOURCES)},{k:"status",label:"Status",type:"select",opts:sel(S.me.leadStatus)},{k:"value",label:"Expected value",type:"number"},{k:"ownerUid",label:"Owner",type:"select",opts:ownerOpts()},{k:"campaign",label:"Campaign"},{k:"tags",label:"Tags (comma separated)",type:"tags"},{k:"notes",label:"Notes",type:"textarea",wide:1}]},
  deals:{one:"deal",fields:()=>[{k:"title",label:"Deal title",req:1,wide:1},{k:"stage",label:"Stage",type:"select",opts:sel(S.me.dealStage)},{k:"value",label:"Value",type:"number"},{k:"contactId",label:"Contact",type:"select",opts:[["","None"],...(S.data.contacts||[]).map(c=>[c.id,c.name])]},{k:"companyId",label:"Company",type:"select",opts:[["","None"],...(S.data.companies||[]).map(c=>[c.id,c.name])]},{k:"ownerUid",label:"Owner",type:"select",opts:ownerOpts()},{k:"expectedClose",label:"Expected close",type:"date"},{k:"source",label:"Source",type:"select",opts:sel(SOURCES)},{k:"lostReason",label:"Lost reason (if lost)"},{k:"notes",label:"Notes",type:"textarea",wide:1}]},
  contacts:{one:"contact",fields:()=>[{k:"name",label:"Name",req:1},{k:"email",label:"Email",type:"email"},{k:"phone",label:"Phone"},{k:"title",label:"Job title"},{k:"companyId",label:"Company",type:"select",opts:[["","None"],...(S.data.companies||[]).map(c=>[c.id,c.name])]},{k:"ownerUid",label:"Owner",type:"select",opts:ownerOpts()},{k:"city",label:"City"},{k:"tags",label:"Tags",type:"tags"},{k:"notes",label:"Notes",type:"textarea",wide:1}]},
  companies:{one:"company",fields:()=>[{k:"name",label:"Company name",req:1},{k:"website",label:"Website"},{k:"industry",label:"Industry"},{k:"phone",label:"Phone"},{k:"email",label:"Email",type:"email"},{k:"city",label:"City"},{k:"country",label:"Country"},{k:"size",label:"Size"},{k:"ownerUid",label:"Owner",type:"select",opts:ownerOpts()},{k:"tags",label:"Tags",type:"tags"},{k:"notes",label:"Notes",type:"textarea",wide:1}]},
  tasks:{one:"task",fields:()=>[{k:"title",label:"Task",req:1,wide:1},{k:"due",label:"Due date",type:"date"},{k:"priority",label:"Priority",type:"select",opts:sel(["low","normal","high"])},{k:"assigneeUid",label:"Assigned to",type:"select",opts:ownerOpts().slice(1)},{k:"notes",label:"Notes",type:"textarea",wide:1}]}
};
const load=async(e,force)=>{if(!force&&S.data[e])return S.data[e];if(!can(e,"v"))return S.data[e]=[];S.data[e]=(await api("GET","/api/crm/list?e="+e)).items;return S.data[e]};
const nameOf=(e,id)=>{const x=(S.data[e]||[]).find(r=>r.id===id);return x?x.name:""};

async function editRecord(e,rec,defaults){
  const spec=ENT[e],isNew=!rec;
  if(e==="deals"||e==="contacts"){await load("companies");await load("contacts")}
  const vals=Object.assign({ownerUid:S.me.me.uid,assigneeUid:S.me.me.uid},defaults||{},rec||{});
  const f=formEl(spec.fields(),vals),b=h("div");b.appendChild(f);
  const row=h("div","crm-row"),save=h("button","bz-btn",isNew?"Create":"Save");
  save.onclick=async()=>{
    const o=f.get();if(rec)o.id=rec.id;
    if(e==="deals"||e==="contacts"){o.companyName=nameOf("companies",o.companyId);if(e==="deals")o.contactName=nameOf("contacts",o.contactId)}
    save.disabled=true;try{await api("POST","/api/crm/save?e="+e,o);await load(e,true);close();show(S.view)}catch(x){msg(b,x.message,1);save.disabled=false}};
  if(isNew?can(e,"c"):can(e,"e"))row.appendChild(save);
  if(rec&&can(e,"d")){const d=h("button","bz-btn ghost","Delete");d.onclick=async()=>{if(!confirm("Delete this "+spec.one+"?"))return;try{await api("POST","/api/crm/delete?e="+e,{id:rec.id});await load(e,true);close();show(S.view)}catch(x){msg(b,x.message,1)}};row.appendChild(d)}
  if(e==="leads"&&rec&&rec.status!=="converted"&&can("deals","c")&&can("leads","e")){const c=h("button","bz-btn ghost","Convert to deal");c.onclick=()=>convertLead(rec);row.appendChild(c)}
  b.appendChild(row);
  if(rec&&e!=="tasks"){b.appendChild(h("h4",null,"History and notes"));const tl=h("div");b.appendChild(tl);timeline(e,rec,tl,b)}
  modal((isNew?"New ":"")+label(spec.one)+(rec?": "+(rec.name||rec.title||""):""),b,!!rec&&e!=="tasks");
}
async function timeline(e,rec,box,parent){
  box.innerHTML="";
  if(can(e,"e")){const r=h("div","crm-row");r.style.marginTop="0";const ty=mk("select"),tx=mk("input",{placeholder:"Write a note, log a call or a meeting"});ty.style.width="auto";for(const t of["note","call","email","meeting"])ty.appendChild(mk("option",{value:t,textContent:label(t)}));const a=h("button","bz-mini","Add");a.onclick=async()=>{if(!tx.value.trim())return;try{await api("POST","/api/crm/activity",{relType:e,relId:rec.id,type:ty.value,text:tx.value});timeline(e,rec,box,parent)}catch(x){msg(parent,x.message,1)}};tx.style.flex="1";add(r,ty,tx,a);box.appendChild(r)}
  try{const items=(await api("GET","/api/crm/activities?relType="+e+"&relId="+rec.id)).items;
    for(const a of items){const d=h("div","crm-act");d.innerHTML="";const t=h("div",null,a.text);const s=h("small",null,label(a.type)+" · "+ownerName(a.byUid)+(a.byUid?"":"System")+" · "+ago(a.at));add(d,t,s);box.appendChild(d)}
    if(!items.length)box.appendChild(h("p","bz-mut","No history yet."))}catch(x){}
}
function convertLead(l){
  const b=h("div"),f=formEl([{k:"dealTitle",label:"Deal title",wide:1},{k:"value",label:"Deal value",type:"number"},{k:"expectedClose",label:"Expected close",type:"date"}],{dealTitle:l.company||l.name,value:l.value||0});
  const mc=mk("input",{type:"checkbox",checked:true}),mo=mk("input",{type:"checkbox",checked:!!l.company});
  const l1=h("label",null," Also create a contact");l1.prepend(mc);const l2=h("label",null," Also create a company");l2.prepend(mo);
  const go=h("button","bz-btn","Convert");go.onclick=async()=>{go.disabled=true;try{const v=f.get();await api("POST","/api/crm/lead/convert",{id:l.id,dealTitle:v.dealTitle,value:v.value,expectedClose:v.expectedClose,makeContact:mc.checked,makeCompany:mo.checked});for(const e of["leads","deals","contacts","companies"])S.data[e]=null;close();nav("deals")}catch(x){msg(b,x.message,1);go.disabled=false}};
  add(b,f,l1,l2,h("div","crm-row"));b.lastChild.appendChild(go);modal("Convert lead to deal",b);
}

// ---------------------------------------------------------------- generic list view
function table(cols,rows,onRow){
  const w=h("div","crm-wrap"),t=h("table","crm-t"),hd=h("tr");
  for(const c of cols)hd.appendChild(h("th",null,c));t.appendChild(h("thead")).appendChild(hd);
  const tb=h("tbody");for(const r of rows){const tr=h("tr","row");for(const c of r.cells){const td=h("td");if(c instanceof Node)td.appendChild(c);else td.textContent=c==null?"":c;tr.appendChild(td)}if(onRow)tr.onclick=()=>onRow(r.rec);tb.appendChild(tr)}
  if(!rows.length){const tr=h("tr"),td=h("td","bz-mut","Nothing here yet.");td.colSpan=cols.length;tr.appendChild(td);tb.appendChild(tr)}
  t.appendChild(tb);w.appendChild(t);return w;
}
function csvDownload(name,cols,rows){
  const q=v=>'"'+String(v==null?"":v).replace(/"/g,'""')+'"';
  const t=[cols.map(q).join(",")].concat(rows.map(r=>r.map(q).join(","))).join("\n");
  const a=mk("a",{href:URL.createObjectURL(new Blob(["﻿"+t],{type:"text/csv"})),download:name});a.click();
}
function parseCSV(t){
  const rows=[];let r=[],c="",q=false;
  for(let i=0;i<t.length;i++){const ch=t[i];
    if(q){if(ch==='"'){if(t[i+1]==='"'){c+='"';i++}else q=false}else c+=ch}
    else if(ch==='"')q=true;else if(ch===","||ch===";"||ch==="\t"){r.push(c);c=""}
    else if(ch==="\n"||ch==="\r"){if(ch==="\r"&&t[i+1]==="\n")i++;r.push(c);c="";if(r.some(x=>x.trim()))rows.push(r);r=[]}
    else c+=ch}
  r.push(c);if(r.some(x=>x.trim()))rows.push(r);return rows;
}
const HDR={name:["name","full name","full_name","fullname","contact"],email:["email","e-mail","email address"],phone:["phone","phone number","mobile","tel","telephone"],company:["company","company name","organization"],title:["title","job title","position"],city:["city"],notes:["notes","message","comment"],value:["value","amount","budget"]};
function importLeads(){
  const b=h("div");b.appendChild(h("p","bz-mut","Upload a CSV with a header row. Columns recognised: name, email, phone, company, title, city, notes, value. Duplicates (same email or phone) are skipped."));
  const file=mk("input",{type:"file",accept:".csv,text/csv"}),src=mk("select");for(const s of SOURCES)src.appendChild(mk("option",{value:s,textContent:label(s)}));src.value="csv_import";
  const own=mk("select");for(const[v,t]of ownerOpts())own.appendChild(mk("option",{value:v,textContent:t}));
  const go=h("button","bz-btn","Import");
  go.onclick=async()=>{
    if(!file.files[0]){msg(b,"Choose a CSV file.",1);return}
    const rows=parseCSV(await file.files[0].text());if(rows.length<2){msg(b,"The file has no data rows.",1);return}
    const head=rows[0].map(x=>x.trim().toLowerCase()),idx={};
    for(const[k,al]of Object.entries(HDR)){const i=head.findIndex(x=>al.includes(x));if(i>=0)idx[k]=i}
    if(idx.name==null&&idx.email==null&&idx.phone==null){msg(b,"No name, email or phone column found in the header row.",1);return}
    const out=rows.slice(1,1001).map(r=>{const o={};for(const[k,i]of Object.entries(idx))o[k]=k==="value"?parseFloat(String(r[i]).replace(/[^0-9.]/g,""))||0:(r[i]||"").trim();return o});
    go.disabled=true;try{const r=await api("POST","/api/crm/leads/import",{rows:out,source:src.value,ownerUid:own.value});S.data.leads=null;msg(b,"Imported "+r.created+" leads. Skipped "+r.duplicates+" duplicates and "+r.invalid+" invalid rows.");setTimeout(()=>show("leads"),1200)}catch(x){msg(b,x.message,1);go.disabled=false}};
  const f=h("div","form");const l1=h("label",null,"CSV file");l1.appendChild(file);const l2=h("label",null,"Source");l2.appendChild(src);const l3=h("label",null,"Assign to");l3.appendChild(own);add(f,l1,l2,l3);add(b,f,go);modal("Import leads",b);
}
function listView(e,cols,rowFn,opts={}){
  const v=V();v.innerHTML="";const card=h("section","bz-card");v.appendChild(card);
  const bar=h("div","crm-bar"),q=mk("input",{placeholder:"Search…"});bar.appendChild(q);
  let filter=null;if(opts.filter){filter=mk("select");filter.appendChild(mk("option",{value:"",textContent:opts.filter.label}));for(const x of opts.filter.opts)filter.appendChild(mk("option",{value:x,textContent:label(x)}));bar.appendChild(filter)}
  bar.appendChild(h("span","sp"));
  if(can(e,"x")){const ex=h("button","bz-btn ghost","Export CSV");ex.onclick=()=>{const rs=cur();csvDownload(e+".csv",cols,rs.map(r=>r.cells.map(c=>c instanceof Node?c.textContent:c)))};bar.appendChild(ex)}
  if(e==="leads"&&can("leads","x")&&can("leads","c")){const im=h("button","bz-btn ghost","Import CSV");im.onclick=importLeads;bar.appendChild(im)}
  if(can(e,"c")){const n=h("button","bz-btn","+ New "+ENT[e].one);n.onclick=()=>editRecord(e,null);bar.appendChild(n)}
  card.appendChild(bar);const holder=h("div");card.appendChild(holder);
  const items=S.data[e];
  const cur=()=>{const t=q.value.toLowerCase();return items.filter(r=>(!filter||!filter.value||r[opts.filter.key]===filter.value)&&(!t||JSON.stringify(Object.values(r)).toLowerCase().includes(t))).map(r=>({cells:rowFn(r),rec:r}))};
  const draw=()=>{holder.innerHTML="";const rs=cur();holder.appendChild(table(cols,rs,r=>editRecord(e,r)));if(items.length>=2000)holder.appendChild(h("p","bz-mut","Showing the 2000 most recent records."))};
  q.oninput=draw;if(filter)filter.onchange=draw;draw();
}

// ---------------------------------------------------------------- views
const VIEWS={
  async leads(){await load("leads");listView("leads",["Name","Company","Contact","Source","Status","Value","Owner","Created"],l=>{const own=h("span",null,ownerName(l.ownerUid));if(!l.ownerUid&&can("leads","e")){const c=h("button","bz-mini","Claim");c.onclick=async ev=>{ev.stopPropagation();await api("POST","/api/crm/save?e=leads",{id:l.id,ownerUid:S.me.me.uid});S.data.leads=null;show("leads")};return[l.name,l.company,[l.email,l.phone].filter(Boolean).join(" · "),label(l.source),pill(l.status),l.value?money(l.value):"",c,date(l.createdAt)]}return[l.name,l.company,[l.email,l.phone].filter(Boolean).join(" · "),label(l.source),pill(l.status),l.value?money(l.value):"",ownerName(l.ownerUid),date(l.createdAt)]},{filter:{label:"All statuses",key:"status",opts:S.me.leadStatus}})},
  async contacts(){await load("contacts");await load("companies");listView("contacts",["Name","Company","Contact","Owner","Created"],c=>[c.name,c.companyName||nameOf("companies",c.companyId),[c.email,c.phone].filter(Boolean).join(" · "),ownerName(c.ownerUid),date(c.createdAt)])},
  async companies(){await load("companies");listView("companies",["Company","Industry","Website","Phone","Owner","Created"],c=>[c.name,c.industry,c.website,c.phone,ownerName(c.ownerUid),date(c.createdAt)])},
  async tasks(){
    await load("tasks");const v=V();v.innerHTML="";const card=h("section","bz-card");v.appendChild(card);
    const bar=h("div","crm-bar"),f=mk("select");for(const[a,b]of[["open","Open tasks"],["overdue","Overdue"],["done","Done"],["all","All"]])f.appendChild(mk("option",{value:a,textContent:b}));
    bar.appendChild(f);bar.appendChild(h("span","sp"));if(can("tasks","c")){const n=h("button","bz-btn","+ New task");n.onclick=()=>editRecord("tasks",null);bar.appendChild(n)}card.appendChild(bar);const hold=h("div");card.appendChild(hold);
    const today=new Date().toISOString().slice(0,10);
    const draw=()=>{hold.innerHTML="";const rs=S.data.tasks.filter(t=>f.value==="all"||(f.value==="done"?t.done:f.value==="open"?!t.done:!t.done&&t.due&&t.due<today)).sort((a,b)=>(a.due||"9")<(b.due||"9")?-1:1).map(t=>{const c=mk("input",{type:"checkbox",checked:!!t.done});c.onclick=async ev=>{ev.stopPropagation();try{await api("POST","/api/crm/save?e=tasks",{id:t.id,done:c.checked});S.data.tasks=null;show("tasks")}catch(x){alert(x.message)}};c.disabled=!can("tasks","e");return{cells:[c,t.title+(t.relName?" ("+t.relName+")":""),t.due?date(t.due):"",label(t.priority),ownerName(t.assigneeUid)],rec:t}});hold.appendChild(table(["","Task","Due","Priority","Assigned to"],rs,t=>editRecord("tasks",t)))};f.onchange=draw;draw()},
  async deals(){
    await Promise.all([load("deals"),load("contacts"),load("companies")]);const v=V();v.innerHTML="";const card=h("section","bz-card");v.appendChild(card);
    const bar=h("div","crm-bar"),tot=h("span","bz-mut");bar.appendChild(tot);bar.appendChild(h("span","sp"));if(can("deals","c")){const n=h("button","bz-btn","+ New deal");n.onclick=()=>editRecord("deals",null);bar.appendChild(n)}card.appendChild(bar);
    const kan=h("div","crm-kan");card.appendChild(kan);
    let open=0;for(const d of S.data.deals)if(d.stage!=="won"&&d.stage!=="lost")open+=d.value||0;tot.textContent="Open pipeline: "+money(open)+" · drag a card to change its stage";
    for(const st of S.me.dealStage){
      const col=h("div","crm-col"),items=S.data.deals.filter(d=>d.stage===st),sum=items.reduce((a,d)=>a+(d.value||0),0);
      col.appendChild(h("h4",null,label(st)));col.firstChild.appendChild(h("span","bz-mut",items.length+" · "+money(sum)));
      for(const d of items){const c=h("div","crm-card");c.draggable=can("deals","e");const b=h("b",null,d.title);add(c,b,h("div","m",(d.companyName||d.contactName||"")+(d.value?" · "+money(d.value):"")),h("div","m",ownerName(d.ownerUid)+(d.expectedClose?" · "+d.expectedClose:"")));
        c.ondragstart=e=>e.dataTransfer.setData("text/plain",d.id);c.onclick=()=>editRecord("deals",d);col.appendChild(c)}
      col.ondragover=e=>{e.preventDefault();col.classList.add("over")};col.ondragleave=()=>col.classList.remove("over");
      col.ondrop=async e=>{e.preventDefault();col.classList.remove("over");const id=e.dataTransfer.getData("text/plain"),d=S.data.deals.find(x=>x.id===id);if(!d||d.stage===st)return;
        const o={id,stage:st};if(st==="lost"){const r=prompt("Why was this deal lost? (optional)");if(r===null)return;o.lostReason=r}
        try{await api("POST","/api/crm/save?e=deals",o);S.data.deals=null;show("deals")}catch(x){alert(x.message)}};
      kan.appendChild(col)}
  },
  async dashboard(){
    const v=V();v.innerHTML="";const bar=h("div","crm-bar"),per=mk("select");for(const[a,b]of[["7","Last 7 days"],["30","Last 30 days"],["90","Last 90 days"],["365","Last 12 months"],["0","All time"]])per.appendChild(mk("option",{value:a,textContent:b}));per.value="30";bar.appendChild(per);v.appendChild(bar);const body=h("div");v.appendChild(body);
    const run=async()=>{
      body.innerHTML="";S.charts.forEach(c=>c.destroy());S.charts=[];let d;try{d=await api("GET","/api/crm/dashboard?days="+per.value)}catch(x){msg(body,x.message,1);return}
      const k=d.kpis,kp=h("div","crm-kpis");
      for(const[l,val]of[["Revenue won",money(k.revenue)],["Deals won",num(k.won)+" ("+k.winRate.toFixed(0)+"% win rate)"],["Open pipeline",money(k.pipeline)],["Weighted forecast",money(k.forecast)],["New leads",num(k.leads)],["Lead → deal",k.leadConv.toFixed(1)+"%"],["Average deal",money(k.avgDeal)],["Sales cycle",k.avgCycleDays?k.avgCycleDays.toFixed(0)+" days":"–"],["Open tasks",num(k.tasksOpen)+" ("+k.tasksOverdue+" overdue)"]]){const c=h("div","crm-kpi");add(c,h("span",null,l),h("b",null,val));kp.appendChild(c)}body.appendChild(kp);
      const g=h("div","crm-grid2");body.appendChild(g);
      const chart=(t,cfg)=>{const c=h("section","bz-card");c.appendChild(h("h3",null,t));const cv=mk("canvas");c.appendChild(cv);g.appendChild(c);S.charts.push(new Chart(cv,Object.assign({options:{responsive:true,plugins:{legend:{display:cfg.legend||false}}}},cfg)))};
      chart("Revenue won by month",{type:"bar",data:{labels:d.monthly.map(m=>m.month),datasets:[{label:"Revenue",data:d.monthly.map(m=>m.revenue),backgroundColor:"#22d3ee"}]}});
      chart("Pipeline by stage",{type:"bar",data:{labels:d.stages.map(s=>label(s.stage)),datasets:[{label:"Value",data:d.stages.map(s=>s.value),backgroundColor:"#8b5cf6"}]}});
      chart("New leads by month",{type:"line",data:{labels:d.monthly.map(m=>m.month),datasets:[{label:"Leads",data:d.monthly.map(m=>m.leads),borderColor:"#f26a2e",tension:.3}]}});
      chart("Leads by status",{type:"doughnut",legend:true,data:{labels:Object.keys(d.leadStatus).map(label),datasets:[{data:Object.values(d.leadStatus),backgroundColor:["#22d3ee","#fbbf24","#34d399","#f87171","#8b5cf6","#94a3b8"]}]}});
      const sc=h("section","bz-card");sc.appendChild(h("h3",null,"Revenue by lead source"));sc.appendChild(table(["Source","Leads","Converted","Won deals","Revenue","Open pipeline"],d.bySource.map(s=>({cells:[label(s.label),s.leads,s.convRate.toFixed(0)+"%",s.won,money(s.wonValue),money(s.pipeline)]}))));body.appendChild(sc);
      const oc=h("section","bz-card");oc.style.marginTop="14px";oc.appendChild(h("h3",null,"Team performance"));oc.appendChild(table(["Person","Leads","Won deals","Revenue","Open deals","Open pipeline"],d.byOwner.map(s=>({cells:[s.label,s.leads,s.won,money(s.wonValue),s.open,money(s.pipeline)]}))));body.appendChild(oc);
      try{const a=await api("GET","/api/crm/ads?days="+(per.value==="90"||per.value==="365"||per.value==="0"?90:30));
        const ac=h("section","bz-card");ac.style.marginTop="14px";ac.appendChild(h("h3",null,"Meta ads vs CRM revenue (last "+a.days+" days)"));
        if(a.connected)ac.appendChild(table(["Ad spend","Leads from lead ads","Cost per lead","Won deals","Revenue","ROAS"],[{cells:[(a.adCurrency||"")+" "+num(a.spend),a.leads,a.leads?num(a.cpl):"–",a.won,money(a.revenue),a.roas?a.roas.toFixed(2)+"x":"–"]}]));
        else ac.appendChild(h("p","bz-mut","Connect Facebook or Instagram under Connected social in Blitz AI to compare ad spend with CRM revenue."));body.appendChild(ac)}catch(x){}
    };per.onchange=run;run();
  },
  async team(){
    S.team=await api("GET","/api/crm/team");const v=V();v.innerHTML="";const T=S.team;
    const mc=h("section","bz-card");mc.appendChild(h("h3",null,"People"));
    const dn=id=>(T.departments.find(d=>d.id===id)||{}).name||"–";
    mc.appendChild(table(["Name","Email","Role","Department","Sees","Status"],T.members.map(m=>({cells:[m.name,m.email||"",label(m.role),dn(m.dept),m.scope?label(m.scope==="dept"?"department":m.scope):"default",m.status==="disabled"?"Off":"Active"],rec:m})),m=>can("team","e")&&m.uid!==S.me.me.uid&&m.role!=="owner"?memberEdit(m):null));
    if(can("team","c")){const r=h("div","crm-row"),b=h("button","bz-btn","+ Invite person");b.onclick=inviteForm;r.appendChild(b);mc.appendChild(r)}
    v.appendChild(mc);
    if(T.invites&&T.invites.length){const ic=h("section","bz-card");ic.style.marginTop="14px";ic.appendChild(h("h3",null,"Pending invitations"));ic.appendChild(h("p","bz-mut","They join automatically when they sign in to Blitz AI with that email address (they must verify it)."));
      ic.appendChild(table(["Email","Role","Department",""],T.invites.map(i=>{const c=h("button","bz-mini","Cancel");c.onclick=async()=>{await api("POST","/api/crm/invite/cancel",{email:i.email});show("team")};return{cells:[i.email,label(i.role),dn(i.dept),can("team","d")?c:""]}})));v.appendChild(ic)}
    const dc=h("section","bz-card");dc.style.marginTop="14px";dc.appendChild(h("h3",null,"Company structure (departments)"));
    const tree=(p,depth)=>{for(const d of T.departments.filter(x=>(x.parentId||"")===p)){const r=h("div","crm-row");r.style.marginLeft=depth*22+"px";const cnt=T.members.filter(m=>m.dept===d.id).length;r.appendChild(h("b",null,d.name));r.appendChild(h("span","bz-mut",cnt+" people"+(d.headUid?" · head: "+ownerName(d.headUid):"")));if(can("team","e")){const e=h("button","bz-mini","Edit");e.onclick=()=>deptForm(d);r.appendChild(e)}dc.appendChild(r);tree(d.id,depth+1)}};
    tree("",0);if(!T.departments.length)dc.appendChild(h("p","bz-mut","No departments yet. Add Sales, Marketing, Support… and put people in them."));
    if(can("team","c")){const r=h("div","crm-row"),b=h("button","bz-btn ghost","+ Add department");b.onclick=()=>deptForm();r.appendChild(b);dc.appendChild(r)}v.appendChild(dc);
    const rc=h("section","bz-card");rc.style.marginTop="14px";rc.appendChild(h("h3",null,"What each role can do"));
    const mods=["leads","deals","contacts","companies","tasks","reports","team","integrations"],P={owner:"vcedx",admin:"vcedx",manager:"vcedx",marketing:"vcex",sales:"vce",support:"vce",viewer:"v"};
    rc.appendChild(table(["Role","What they can do"],S.me.roles.map(r=>({cells:[r.label,r.desc]}))));
    rc.appendChild(h("p","bz-mut","Records visible to a person: Owner and Administrator see everything. Others see their own records, or their department's, or everything, as set per person. Unassigned leads can be claimed by anyone who can edit leads."));v.appendChild(rc);
    if(can("team","e")){const sc=h("section","bz-card");sc.style.marginTop="14px";sc.appendChild(h("h3",null,"Workspace settings"));const w=S.me.workspace,f=formEl([{k:"name",label:"Company name"},{k:"currency",label:"Currency (3 letters)"},{k:"autoAssign",label:"Auto-assign new inbound leads",type:"select",opts:[["no","No, leave unassigned"],["yes","Yes, round-robin to Sales and Managers"]]}],{name:w.name,currency:w.currency,autoAssign:w.autoAssign?"yes":"no"});const b=h("button","bz-btn","Save settings");b.onclick=async()=>{const o=f.get();try{await api("POST","/api/crm/settings",{name:o.name,currency:o.currency,autoAssign:o.autoAssign==="yes"});S.me=await api("GET","/api/crm/me");msg(sc,"Saved.")}catch(x){msg(sc,x.message,1)}};add(sc,f,b);v.appendChild(sc)}
  },
  async sources(){
    const v=V();v.innerHTML="";let I;try{I=await api("GET","/api/crm/integrations")}catch(x){v.appendChild(h("p","bz-err",x.message));return}
    const codeBox=(t,val)=>{const c=h("div");c.appendChild(h("h4",null,t));const b=h("div","crm-code",val);c.appendChild(b);const r=h("div","crm-row");r.style.marginTop="6px";const cp=h("button","bz-mini","Copy");cp.onclick=()=>copy(val,cp);r.appendChild(cp);c.appendChild(r);return c};
    const a=h("section","bz-card");a.appendChild(h("h3",null,"1. Website form and webhook"));a.appendChild(h("p","bz-mut","Every submission becomes a lead with the source Web form. Use the hosted page, embed it on your site, or post JSON from any form tool, Zapier or Make."));
    add(a,codeBox("Hosted contact form (share this link)",I.formUrl),codeBox("Embed on your website",I.embed),codeBox("Webhook URL (POST name, email, phone, company, message)",I.webhookUrl),codeBox("Test it",I.curl));
    if(can("integrations","e")){const r=h("div","crm-row"),b=h("button","bz-btn ghost","Rotate secret link");b.onclick=async()=>{if(confirm("The old form and webhook links will stop working. Continue?")){await api("POST","/api/crm/integrations/rotate",{});show("sources")}};r.appendChild(b);a.appendChild(r)}v.appendChild(a);
    const m=h("section","bz-card");m.style.marginTop="14px";m.appendChild(h("h3",null,"2. Meta: lead ads, messages and comments"));
    m.appendChild(h("p","bz-mut","Uses the Facebook account you connected under Connected social in Blitz AI. Lead forms are pulled with Sync. Messages and comments arrive live once the webhook below is added in your Meta app. Messages and comments from the public need Meta app review (the leads_retrieval, pages_messaging and pages_read_engagement permissions); before review they work for app admins and testers."));
    if(I.metaPages.length)m.appendChild(table(["Connected page","Type","Last sync"],I.metaPages.map(p=>({cells:[p.name,label(p.kind),p.lastSync?ago(p.lastSync):"never"]}))));
    if(can("integrations","e")){const r=h("div","crm-row"),c=h("button","bz-btn","Connect my Pages"),s=h("button","bz-btn ghost","Sync lead forms now");
      c.onclick=async()=>{c.disabled=true;try{const x=await api("POST","/api/crm/meta/connect",{});msg(m,"Connected "+x.pages+" Page(s)."+((x.warnings||[]).length?" Notes: "+x.warnings.join(" | "):""));setTimeout(()=>show("sources"),1500)}catch(e){msg(m,e.message,1);c.disabled=false}};
      s.onclick=async()=>{s.disabled=true;msg(m,"Reading your lead forms…");try{const x=await api("POST","/api/crm/meta/sync",{});msg(m,"Added "+x.added+" new leads (checked "+x.checked+")."+((x.warnings||[]).length?" Notes: "+x.warnings.join(" | "):""));S.data.leads=null}catch(e){msg(m,e.message,1)}s.disabled=false};add(r,c,s);m.appendChild(r)}
    const wh=I.metaWebhook;add(m,codeBox("Meta webhook callback URL",wh.url),codeBox("Verify token",wh.verifyToken));
    m.appendChild(h("p","bz-mut",wh.appSecretSet?"In your Meta app: Webhooks > Page (and Instagram) > Callback URL and Verify token above > subscribe to leadgen, messages and feed (comments for Instagram).":"FACEBOOK_APP_SECRET is not set on the server, so the live webhook is off. Sync still works."));
    v.appendChild(m);
    const c=h("section","bz-card");c.style.marginTop="14px";c.appendChild(h("h3",null,"3. Add by hand or import"));c.appendChild(h("p","bz-mut","Use Leads > + New lead, or Leads > Import CSV. Duplicates are detected by email and phone."));v.appendChild(c);
  }
};
function memberEdit(m){
  const b=h("div"),f=formEl([{k:"role",label:"Role",type:"select",opts:S.me.roles.filter(r=>r.key!=="owner").map(r=>[r.key,r.label])},{k:"dept",label:"Department",type:"select",opts:[["","None"],...S.team.departments.map(d=>[d.id,d.name])]},{k:"scope",label:"Records they can see",type:"select",opts:[["default","Default for the role"],["own","Only their own"],["dept","Their department"],["all","Everything"]]},{k:"status",label:"Access",type:"select",opts:[["active","Active"],["disabled","Turned off"]]}],{role:m.role,dept:m.dept,scope:m.scope||"default",status:m.status||"active"});
  const s=h("button","bz-btn","Save"),r=h("button","bz-btn ghost","Remove from workspace");
  s.onclick=async()=>{try{await api("POST","/api/crm/member/save",Object.assign({uid:m.uid},f.get()));close();show("team")}catch(x){msg(b,x.message,1)}};
  r.onclick=async()=>{if(!confirm("Remove "+m.name+"? They lose access to the CRM."))return;try{await api("POST","/api/crm/member/remove",{uid:m.uid});close();show("team")}catch(x){msg(b,x.message,1)}};
  const row=h("div","crm-row");add(row,s,can("team","d")?r:null);add(b,f,row);modal(m.name,b);
}
function inviteForm(){
  const b=h("div"),f=formEl([{k:"email",label:"Email address",type:"email",req:1,wide:1},{k:"role",label:"Role",type:"select",opts:S.me.roles.filter(r=>r.key!=="owner"&&r.key!==S.me.me.role&&(S.me.me.role==="owner"||r.key!=="admin")).map(r=>[r.key,r.label])},{k:"dept",label:"Department",type:"select",opts:[["","None"],...S.team.departments.map(d=>[d.id,d.name])]},{k:"scope",label:"Records they can see",type:"select",opts:[["","Default for the role"],["own","Only their own"],["dept","Their department"],["all","Everything"]]}],{role:"sales"});
  const go=h("button","bz-btn","Send invitation");
  go.onclick=async()=>{try{const r=await api("POST","/api/crm/invite",f.get());b.innerHTML="";b.appendChild(h("p",null,"Invitation saved. Ask them to open the link below and sign in or create an account with exactly this email address. They join this workspace automatically."));const c=h("div","crm-code",r.joinUrl);b.appendChild(c);const cp=h("button","bz-btn","Copy link");cp.onclick=()=>copy(r.joinUrl,cp);b.appendChild(cp);S.team=null}catch(x){msg(b,x.message,1)}};
  add(b,f,go);modal("Invite a team member",b);
}
function deptForm(d){
  const b=h("div"),f=formEl([{k:"name",label:"Department name",req:1},{k:"parentId",label:"Part of",type:"select",opts:[["","Top level"],...S.team.departments.filter(x=>!d||x.id!==d.id).map(x=>[x.id,x.name])]},{k:"headUid",label:"Head of department",type:"select",opts:ownerOpts()}],d?{name:d.name,parentId:d.parentId,headUid:d.headUid}:{});
  const s=h("button","bz-btn",d?"Save":"Create"),x=h("button","bz-btn ghost","Delete");
  s.onclick=async()=>{try{await api("POST","/api/crm/dept/save",Object.assign({id:d?d.id:""},f.get()));close();show("team")}catch(e){msg(b,e.message,1)}};
  x.onclick=async()=>{if(!confirm("Delete this department?"))return;try{await api("POST","/api/crm/dept/delete",{id:d.id});close();show("team")}catch(e){msg(b,e.message,1)}};
  const row=h("div","crm-row");add(row,s,d&&can("team","d")?x:null);add(b,f,row);modal(d?"Edit department":"New department",b);
}

// ---------------------------------------------------------------- shell
const MENU=[["dashboard","Revenue","reports","trend"],["leads","Leads","leads","userplus"],["deals","Deals","deals","layers"],["contacts","Contacts","contacts","users"],["companies","Companies","companies","grid"],["tasks","Tasks","tasks","check"],["sources","Lead sources","integrations","zap"],["team","Team & access","team","share"]];
const SUB={dashboard:"Revenue, pipeline and what is working",leads:"Everyone who showed interest",deals:"Your sales pipeline",contacts:"People you do business with",companies:"Organisations",tasks:"Follow-ups and to-dos",sources:"Where new leads come from",team:"Company structure, roles and access"};
function nav(k){S.view=k;document.querySelectorAll("#nav .bz-nav").forEach(b=>b.classList.toggle("on",b.dataset.k===k));$("title").textContent=(MENU.find(m=>m[0]===k)||[0,"CRM"])[1];$("sub").textContent=SUB[k]||"";show(k)}
async function show(k){S.charts.forEach(c=>c.destroy());S.charts=[];try{if(!S.team)S.team=await api("GET","/api/crm/team");await VIEWS[k]()}catch(x){V().innerHTML="";V().appendChild(h("p","bz-err",x.message))}}
function boot(){
  const me=S.me;$("who").textContent=me.workspace.name+" · "+label(me.me.role);
  const n=$("nav");n.innerHTML="";let first=null;
  for(const[k,l,m,ic]of MENU){if(!can(m,"v"))continue;first=first||k;const b=h("button","bz-nav",l);b.dataset.k=k;b.prepend(icon(ic,18));b.onclick=()=>nav(k);n.appendChild(b)}
  if(!first){V().innerHTML="<p class='bz-mut'>Your role has no modules yet. Ask an administrator.</p>";return}
  nav(first);
}
function gate(st,me){
  const v=V();v.innerHTML="";const c=h("section","bz-card");v.appendChild(c);
  if(st==="none"){c.appendChild(h("h3",null,"Set up your company CRM"));c.appendChild(h("p","bz-mut","Create a workspace for your company. You become the owner and can then invite your team, build departments and decide who can see what. If a colleague invited you, sign in with the invited email address and refresh this page."));
    const f=formEl([{k:"name",label:"Company name",req:1},{k:"me",label:"Your name"}],{});const b=h("button","bz-btn","Create workspace");b.onclick=async()=>{b.disabled=true;try{await api("POST","/api/crm/workspace",f.get());start()}catch(x){msg(c,x.message,1);b.disabled=false}};add(c,f,b)}
  else if(st==="unverified")c.appendChild(h("p",null,"An invitation is waiting for "+me.email+". Verify your email address (check your inbox for the Blitz AI verification email), sign in again and refresh this page."));
  else if(st==="disabled")c.appendChild(h("p","bz-err","Your access to this workspace was turned off by an administrator."));
}
async function start(){
  try{const me=await api("GET","/api/crm/me");S.me=me;if(me.state!=="ok"){gate(me.state,me);return}S.team=await api("GET","/api/crm/team");boot()}catch(x){V().innerHTML="";V().appendChild(h("p","bz-err",x.message))}
}
onAuthStateChanged(auth,u=>{if(!u){location.href="/app";return}start()});
