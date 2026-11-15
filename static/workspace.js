// Modes and company workspaces (see companies.go).
// ficx = analytics only · hybrid = analytics + CRM, shared · company = everything for one company.
import{h,add,api}from"./ui.js";
const LS=(k,v)=>{try{if(v===undefined)return localStorage.getItem(k)||"";if(v===null)localStorage.removeItem(k);else localStorage.setItem(k,v)}catch(e){}return""};
export const coId=()=>LS("ficx.co");
export const getMode=()=>LS("ficx.mode");
export function activeCompany(){try{return JSON.parse(LS("ficx.coinfo")||"null")}catch(e){return null}}
const MODES=[
 ["ficx","ficx Analytics","Fast analysis tools: website, social, YouTube, ads, SEO and reports. No CRM.","Best for a quick audit or a one-off report."],
 ["hybrid","Hybrid","Analytics and CRM together, shared across all your companies. The layout you know.","Best when you manage many accounts yourself."],
 ["company","Company workspace","Everything for ONE company in one place: analyze, optimize, publish, ads, messaging, outbound and CRM.","Best for a client or your own business."]];
export const modeName=m=>(MODES.find(x=>x[0]===m)||["","Choose mode"])[1];
const shortMode=m=>({ficx:"Analytics",hybrid:"Hybrid",company:"Company"}[m]||"Choose mode");

export async function loadWorkspace(){
  const w=await api("GET","/api/workspace");const list=w.companies||[];
  let id=coId();if(id&&!list.some(c=>c.id===id)){id="";LS("ficx.co",null)}
  const mode=w.mode||"hybrid";LS("ficx.mode",mode);
  if(mode==="company"&&!id&&list.length){const l=LS("ficx.lastco");id=list.some(c=>c.id===l)?l:list[0].id;LS("ficx.co",id)}
  const c=list.find(x=>x.id===id);LS("ficx.coinfo",c?JSON.stringify(c):null);
  return{mode,companies:list,active:c||null}}

function overlay(title,body,closable){
  const o=h("div","ws-ov"),b=h("div","ws-box");const hd=h("div","ws-hd");add(hd,h("h3",null,title));
  if(closable){const x=h("button","bz-mini","Close");x.onclick=()=>o.remove();hd.appendChild(x);o.onclick=e=>{if(e.target===o)o.remove()}}
  add(b,hd,body);o.appendChild(b);document.body.appendChild(o);return o}

// Instant switch: remember the choice, tell the server (so phone and web agree), then go.
export async function switchMode(ws,k){
  if(k===ws.mode)return;
  if(ws.mode==="company"&&coId())LS("ficx.lastco",coId());
  LS("ficx.mode",k);
  if(k==="company"){const list=ws.companies;if(!list.length){companyDialog(ws,null,"company");return}const l=LS("ficx.lastco");LS("ficx.co",list.some(c=>c.id===l)?l:list[0].id)}
  else LS("ficx.co",null);
  try{await Promise.race([api("POST","/api/workspace/mode",{mode:k}),new Promise(r=>setTimeout(r,900))])}catch(e){}
  if(k==="ficx")location.href="/dashboard";else if(k==="company")location.href="/crm";else location.reload()}

export function companyDialog(ws,co,goMode){
  const body=h("div","ws-form");const f={};
  const fld=(k,l,ph,v)=>{const i=h("input");i.placeholder=ph||"";i.value=v||"";f[k]=i;const r=h("label",null,l);r.appendChild(i);body.appendChild(r)};
  fld("name","Company name","e.g. Noor Dental",co&&co.name);fld("website","Website","example.com",co&&co.website);
  const dl=h("datalist");dl.id="ws-ind";for(const x of["Real estate","Clinic / healthcare","Education / training","E-commerce / retail","Restaurant / hospitality","Automotive","B2B software / IT","Agency","Recruitment","Fitness / wellness","Finance / insurance","Logistics / trading","Construction / interiors","Legal / consulting","Travel / events"])dl.appendChild(new Option(x));
  fld("industry","Industry","Choose or type",co&&co.industry);f.industry.setAttribute("list","ws-ind");body.appendChild(dl);
  let um;if(!co){const lb=h("label","ws-chk");um=h("input");um.type="checkbox";um.checked=!ws.companies.length;lb.append(um," Use the social accounts and ad accounts I already connected (no reconnecting)");body.appendChild(lb);
    body.appendChild(h("p","bz-mut","Leave it off if this company has its own Facebook, Instagram, TikTok, YouTube or Google accounts. You connect those once, inside the company."))}
  const err=h("p","bz-err");err.hidden=true;const go=h("button","bz-btn",co?"Save":"Create company workspace");
  go.onclick=async()=>{go.disabled=true;err.hidden=true;try{const r=await api("POST","/api/workspace/company",{id:co&&co.id,name:f.name.value,website:f.website.value,industry:f.industry.value,useMain:um?um.checked:false});
    LS("ficx.co",r.id);if(goMode){await api("POST","/api/workspace/mode",{mode:goMode});LS("ficx.mode",goMode)}
    location.href=goMode||!co?"/crm?an=website":location.pathname+location.search}catch(e){err.textContent=e.message;err.hidden=false;go.disabled=false}};
  add(body,err,go);return overlay(co?"Edit company":"New company",body,true)}

const SEG=[["ficx","Analytics","Socials, SEO, ads and reports","Social & marketing specialists"],["hybrid","Hybrid","Analytics and CRM together","Managers & team leads"],["company","Company workspace","One company, everything in one place","Owners, CEOs & agencies"]];

function companyControls(host,ws){
  if(ws.mode==="ficx")return;
  const sel=h("select","ws-sel");sel.title="Company";
  if(ws.mode!=="company")sel.appendChild(new Option("All companies",""));
  for(const c of ws.companies)sel.appendChild(new Option(c.name+(c.shared?" (team)":""),c.id));
  sel.value=coId();sel.onchange=()=>{LS("ficx.co",sel.value||null);location.reload()};host.appendChild(sel);
  const nb=h("button","bz-mini","+ New");nb.title="New company";nb.onclick=()=>companyDialog(ws,null,ws.mode==="company"?"company":"");host.appendChild(nb);
  if(ws.active){const eb=h("button","bz-mini","Edit");eb.onclick=()=>companyDialog(ws,ws.active);host.appendChild(eb);
    const db=h("button","bz-mini","\u00d7");db.title="Delete company";db.onclick=async()=>{if(!confirm("Delete "+ws.active.name+"? Accounts connected only to this company are disconnected. Its CRM records are kept and stay visible under All companies."))return;
      try{await api("POST","/api/workspace/company/delete",{id:ws.active.id});LS("ficx.co",null);location.reload()}catch(e){alert(e.message)}};host.appendChild(db)}}

function segment(ws){
  const g=h("div","ws-seg");g.setAttribute("role","tablist");
  for(const[k,t,d,who]of SEG){const b=h("button","ws-tab"+(ws.mode===k?" on":""));b.setAttribute("role","tab");b.setAttribute("aria-selected",ws.mode===k?"true":"false");b.title=d+" \u00b7 "+who;
    add(b,h("b",null,t),h("small",null,who));b.onclick=()=>switchMode(ws,k);g.appendChild(b)}
  return g}

// The top-most strip on every page: pick how you work, then the company.
export function mountTopBar(ws){
  if(document.getElementById("ws-top"))return;
  const bar=h("div","ws-top");bar.id="ws-top";add(bar,h("span","ws-lbl","View"),segment(ws));
  const r=h("div","ws-bar");companyControls(r,ws);bar.appendChild(r);document.body.prepend(bar);document.body.classList.add("ws-on")}

// Phone: the same controls inside the Menu tab.
export function workspaceBar(host,ws){host.innerHTML="";host.classList.add("ws-phone");host.appendChild(segment(ws));const r=h("div","ws-bar");companyControls(r,ws);host.appendChild(r)}

// Called first on every page. Returns the workspace state, or null when a redirect is under way.
export async function initWorkspace(page,{embed}={}){
  if(embed)return{mode:getMode()||"hybrid",companies:[],active:activeCompany(),embed:true};
  const ws=await loadWorkspace();
  if(ws.mode==="ficx"&&page==="crm"){location.replace("/dashboard");return null}
  if(ws.mode==="company"&&page==="dashboard"){location.replace("/crm");return null}
  if(!matchMedia("(max-width:760px)").matches)mountTopBar(ws);
  if(ws.mode==="company"&&!ws.companies.length)companyDialog(ws,null,"company");
  return ws}
