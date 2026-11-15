// ficx phone app. On screens up to 760 px the CRM is replaced by a native-style app (not the desktop layout squeezed):
// bottom tabs (Home, Chats, CRM, Tasks, Menu) + a floating create button, card lists, full-screen record pages with
// call/WhatsApp/email actions and a stage picker, full-screen chat, bottom sheets, pull-to-refresh, offline banner.
// Patterns taken from the mobile apps of HubSpot (list tabs, tap-the-stage, quick-action icons, activity timeline),
// Zoho (header + action buttons + Overview/Details/Related tabs), Bitrix24 (bottom menu, timeline, call from CRM),
// Pipedrive (offline banner, agenda) and the TeamGram rebuild (floating create button, one-tap lists).
export function phoneApp(X){
  const {h,add,api,icon,S,can,ENT,load,label,money,ago,date,A,mk,stc,ownerName,formEl,nav,show,close,convertLead,modalTask,quick,SECTIONS,nameOf,members,logout}=X;
  document.body.classList.add("mapp-on");
  const root=h("div","ma");root.id="mapp";
  const stage=h("div","ma-stage"),tabbar=h("nav","ma-tabs"),fab=h("button","ma-fab","+");
  fab.type="button";fab.setAttribute("aria-label","Create");
  const off=h("div","ma-off","You are offline. Showing saved data; changes need a connection.");off.hidden=navigator.onLine;
  addEventListener("offline",()=>off.hidden=false);addEventListener("online",()=>off.hidden=true);
  add(root,off,stage,tabbar,fab);document.body.appendChild(root);

  const first=()=>(ownerName(S.me.me.uid)||"there").split(/\s+/)[0];
  const initials=n=>(String(n||"?").trim().split(/\s+/).map(x=>x[0]).slice(0,2).join("")||"?").toUpperCase();
  const today=()=>new Date().toISOString().slice(0,10);
  const ER=(p,m)=>{p.appendChild(h("p","ma-err",m))};

  // ---------------------------------------------------------------- toast + sheets
  function toast(t){const e=h("div","ma-toast",t);root.appendChild(e);setTimeout(()=>e.classList.add("in"),10);setTimeout(()=>{e.classList.remove("in");setTimeout(()=>e.remove(),250)},2200)}
  let ov=null;
  function closeSheet(){if(!ov)return;const o=ov;ov=null;o.classList.remove("in");setTimeout(()=>o.remove(),200)}
  function sheet(title,fill){closeSheet();ov=h("div","ma-ov");const sh=h("div","ma-sheet"),hd=h("div","ma-sh-h"),x=h("button","ma-x","×");x.type="button";x.onclick=closeSheet;
    add(hd,h("b",null,title),x);const b=h("div","ma-sh-b");add(sh,h("i","ma-grab"),hd,b);ov.appendChild(sh);ov.onclick=e=>{if(e.target===ov)closeSheet()};fill(b);root.appendChild(ov);requestAnimationFrame(()=>ov&&ov.classList.add("in"))}

  // ---------------------------------------------------------------- navigation stack
  // History: [exit][app base][pushed screens...]. Back closes sheets, then pops a screen, then goes Home from any
  // other tab, and only on Home (nothing open) does it leave the app.
  const stack=[];let tab="home",depth=0,silent=false;
  function chrome(){root.classList.toggle("deep",stack.length>1)}
  function mount(scr){stack.forEach(s=>s.root.hidden=true);stack.push(scr);stage.appendChild(scr.root);chrome()}
  function push(scr){history.pushState({ma:"s"},"");depth++;mount(scr)}
  function pop(){if(stack.length<2)return;const s=stack.pop();if(s.onclose)s.onclose();s.root.remove();const p=stack[stack.length-1];p.root.hidden=false;if(p.onshow)p.onshow();chrome()}
  function back(){if(stack.length>1)history.back()}
  addEventListener("popstate",e=>{
    if(silent){silent=false;return}
    if(ov){closeSheet();history.pushState({ma:"app"},"");return}
    if(stack.length>1){depth=Math.max(0,depth-1);pop();return}
    if(tab!=="home"){history.pushState({ma:"app"},"");go("home");return}
    if(e.state&&e.state.ma==="exit"){history.back();return}      // already on the exit marker: leave the app
    history.pushState({ma:"app"},"")}); // safety: stay in the app
  function resetTo(scr){while(stack.length){const s=stack.pop();if(s.onclose)s.onclose();s.root.remove()}mount(scr)}
  function screen(title,o={}){
    const r=h("section","ma-scr"),top=h("header","ma-top"),body=h("div","ma-body"),scr={root:r,top,body};
    if(o.back){const b=h("button","ma-back","‹");b.type="button";b.setAttribute("aria-label","Back");b.onclick=back;top.appendChild(b)}
    const t=h("h1","ma-title",title);top.appendChild(t);scr.title=t;scr.right=h("div","ma-right");top.appendChild(scr.right);add(r,top,body);
    if(!o.back)ptr(scr);return scr}
  function ptr(scr){let y=0,on=false,pull=0;const b=scr.body;
    b.addEventListener("touchstart",e=>{on=b.scrollTop<=0&&e.touches.length===1;y=e.touches[0].clientY;pull=0},{passive:true});
    b.addEventListener("touchmove",e=>{if(!on)return;pull=e.touches[0].clientY-y;scr.top.classList.toggle("pull",pull>90)},{passive:true});
    b.addEventListener("touchend",()=>{scr.top.classList.remove("pull");if(on&&pull>110&&scr.reload){for(const k of["leads","deals","contacts","companies","tasks"])S.data[k]=null;scr.reload()}on=false})}

  // ---------------------------------------------------------------- tabs
  const TABS=()=>[
    ["home","Home","trend",true],
    ["chats","Chats","send",can("leads","v")],
    ["crm","CRM","layers",["leads","deals","contacts","companies"].some(e=>can(e,"v"))],
    ["tasks","Tasks","check",can("tasks","v")],
    ["menu","Menu","grid",true]].filter(t=>t[3]);
  function drawTabs(){tabbar.innerHTML="";for(const[k,l,ic]of TABS()){const b=h("button","ma-tab"+(k===tab?" on":""));b.type="button";b.dataset.k=k;add(b,icon(ic,23),h("span",null,l));b.onclick=()=>go(k);tabbar.appendChild(b)}}
  function go(k){if(depth>0){const d=depth;depth=0;silent=true;history.go(-d)}tab=k;closeSheet();drawTabs();const fn={home,chats,crm,tasks,menu}[k];resetTo(fn());fab.hidden=!(k==="home"||k==="crm"||k==="tasks")||!creatable().length}
  const creatable=()=>["leads","deals","contacts","companies","tasks"].filter(e=>can(e,"c"));
  fab.onclick=()=>sheet("Create new",b=>{const g=h("div","ma-list");for(const e of creatable()){const r=h("button","ma-row");r.type="button";add(r,h("span","ma-dot "+e),h("b",null,"New "+ENT[e].one));r.onclick=()=>{closeSheet();if(e==="tasks")newTask();else record(e,null)};g.appendChild(r)}b.appendChild(g)});

  // ---------------------------------------------------------------- shared pieces
  const av=(name,c)=>{const a=h("span","ma-av",initials(name));if(c)a.style.background=c;else{let k=0;for(const ch of String(name||""))k=(k*31+ch.charCodeAt(0))%360;a.style.background=`hsl(${k} 50% 42%)`}return a};
  const stPill=(s)=>{const p=h("span","ma-pill",label(s));p.style.setProperty("--c",stc(s));return p};
  const phoneOf=(e,r)=>{if(r.phone)return r.phone;if(e==="deals"){const c=A(S.data.contacts).find(x=>x.id===r.contactId);return c&&c.phone||""}return""};
  const emailOf=(e,r)=>{if(r.email)return r.email;if(e==="deals"){const c=A(S.data.contacts).find(x=>x.id===r.contactId);return c&&c.email||""}return""};
  const clean=p=>String(p||"").replace(/[^+\d]/g,"");
  const waLink=(p,name)=>"https://wa.me/"+clean(p).replace(/^\+/,"").replace(/^00/,"")+"?text="+encodeURIComponent("Hi "+String(name||"").split(" ")[0]+", ");
  const logAct=(e,id,type,text)=>api("POST","/api/crm/activity",{relType:e,relId:id,type,text}).catch(()=>{});
  function circle(cls,ic,href,label2,fn){const a=h("a","ma-ic "+cls);a.href=href;if(href.startsWith("http")){a.target="_blank";a.rel="noopener"}a.setAttribute("aria-label",label2);a.appendChild(icon(ic,19));a.onclick=ev=>{ev.stopPropagation();fn&&fn()};return a}
  function subOf(e,r){return e==="deals"?[r.contactName,r.companyName].filter(Boolean).join(" · "):e==="leads"?[r.company,r.source&&label(r.source)].filter(Boolean).join(" · "):e==="contacts"?[r.title,r.companyName||nameOf("companies",r.companyId)].filter(Boolean).join(" · "):[r.industry,r.city].filter(Boolean).join(" · ")}
  function card(e,r){
    const key=ENT[e].key,s=key?r[key]:"",c=h("div","ma-card");
    const a=av(r.title||r.name,key?stc(s):null),main=h("div","ma-cm"),right=h("div","ma-cr");
    const l1=h("div","ma-c1");add(l1,h("b",null,r.title||r.name||"Untitled"));
    const sub=subOf(e,r);const l2=h("div","ma-c2",sub||" ");const l3=h("div","ma-c3");if(s)l3.appendChild(stPill(s));l3.appendChild(h("span",null,ago(r.updatedAt||r.createdAt)));
    add(main,l1,l2,l3);if(r.value)right.appendChild(h("b",null,money(r.value)));
    const ph=phoneOf(e,r);if(ph&&e!=="companies"){const q=h("div","ma-q");add(q,circle("call","phone","tel:"+clean(ph),"Call",()=>logAct(e,r.id,"call","Call started from the CRM ("+ph+")")),circle("wa","send",waLink(ph,r.name||r.title),"WhatsApp",()=>logAct(e,r.id,"note","WhatsApp chat opened")));right.appendChild(q)}
    add(c,a,main,right);c.onclick=()=>record(e,r);return c}
  const empty=(t,b)=>{const d=h("div","ma-empty");add(d,h("b",null,t),b?h("p",null,b):null);return d};

  // ---------------------------------------------------------------- HOME
  function home(){
    const scr=screen("");scr.root.classList.add("home");scr.top.innerHTML="";
    const lg=h("img","ma-logo");lg.src="/static/ficx-mark.svg";lg.alt="ficx";const hi=h("div","ma-hi");add(hi,h("small",null,S.me.workspace.name),h("h1",null,(new Date().getHours()<12?"Good morning, ":new Date().getHours()<18?"Good afternoon, ":"Good evening, ")+first()));
    const sb=h("button","ma-ibtn");sb.type="button";sb.setAttribute("aria-label","Search");sb.appendChild(icon("search",22));sb.onclick=globalSearch;add(scr.top,lg,hi,sb);
    scr.reload=async()=>{scr.body.innerHTML="";scr.body.appendChild(h("div","ma-load","Loading…"));
      const ents=["leads","deals","tasks","contacts","companies"].filter(e=>can(e,"v"));await Promise.all(ents.map(e=>load(e).catch(()=>[])));
      let k=null;try{k=(await api("GET","/api/crm/dashboard?days=30")).kpis}catch(e){}
      const b=scr.body;b.innerHTML="";
      if(k){const st=h("div","ma-kpis");for(const[l,v,sub]of[["Open pipeline",money(k.pipeline),""],["Won (30 days)",money(k.revenue),k.won+" deals"],["New leads",String(k.leads),"30 days"],["Open tasks",String(k.tasksOpen),k.tasksOverdue?k.tasksOverdue+" overdue":"all on time"]]){const c=h("div","ma-kpi");add(c,h("small",null,l),h("b",null,v),h("em",null,sub));st.appendChild(c)}b.appendChild(st)}
      const sc=h("div","ma-chips");for(const[e,l]of[["leads","Leads"],["deals","Deals"],["contacts","Contacts"],["companies","Companies"]])if(can(e,"v")){const c=h("button","ma-chip",l+" ("+A(S.data[e]).length+")");c.type="button";c.onclick=()=>{S.ma=S.ma||{};S.ma.crm=e;go("crm")};sc.appendChild(c)}b.appendChild(sc);
      const sec=(t,items,more)=>{if(!items.length)return;const s=h("section","ma-sec"),hd=h("div","ma-sec-h");hd.appendChild(h("h3",null,t));if(more){const m=h("button","ma-link","See all");m.type="button";m.onclick=more;hd.appendChild(m)}s.appendChild(hd);for(const x of items)s.appendChild(x);b.appendChild(s)};
      const t=today(),due=A(S.data.tasks).filter(x=>!x.done&&x.due&&x.due<=t&&(x.assigneeUid===S.me.me.uid||!x.assigneeUid)).sort((a,c)=>a.due<c.due?-1:1).slice(0,5);
      sec("Needs you today",due.map(taskRow),()=>go("tasks"));
      const nl=A(S.data.leads).filter(x=>x.status==="new").sort((a,c)=>new Date(c.createdAt)-new Date(a.createdAt)).slice(0,5);
      sec("New leads to contact",nl.map(x=>card("leads",x)),()=>{S.ma=S.ma||{};S.ma.crm="leads";S.ma.f="new";go("crm")});
      const hot=A(S.data.deals).filter(x=>!["won","lost"].includes(x.stage)).sort((a,c)=>(c.value||0)-(a.value||0)).slice(0,5);
      sec("Biggest open deals",hot.map(x=>card("deals",x)),()=>{S.ma=S.ma||{};S.ma.crm="deals";go("crm")});
      if(!b.querySelector(".ma-sec")&&!k)b.appendChild(empty("Nothing here yet","Leads, deals and tasks you add will show up on this screen. Tap + to start."))};
    scr.reload();return scr}

  function globalSearch(){
    const scr=screen("",{back:true});scr.top.innerHTML="";const b=h("button","ma-back","‹");b.type="button";b.onclick=back;
    const i=mk("input",{placeholder:"Search leads, deals, contacts…",type:"search",autocomplete:"off"});i.className="ma-search";add(scr.top,b,i);
    const out=h("div");scr.body.appendChild(out);push(scr);setTimeout(()=>i.focus(),60);let tm;
    i.oninput=()=>{clearTimeout(tm);tm=setTimeout(async()=>{const q=i.value.trim().toLowerCase();out.innerHTML="";if(q.length<2)return;
      const ents=["leads","deals","contacts","companies"].filter(e=>can(e,"v"));await Promise.all(ents.map(e=>load(e)));let n=0;
      for(const e of ents){const rs=A(S.data[e]).filter(x=>JSON.stringify(Object.values(x)).toLowerCase().includes(q)).slice(0,8);if(!rs.length)continue;out.appendChild(h("h4","ma-grp",label(ENT[e].one)+"s"));for(const r of rs){n++;out.appendChild(card(e,r))}}
      if(!n)out.appendChild(empty("No matches"))},200)}}

  // ---------------------------------------------------------------- CRM (Leads / Deals / Contacts / Companies)
  function crm(){
    const ents=["leads","deals","contacts","companies"].filter(e=>can(e,"v"));S.ma=S.ma||{};if(!ents.includes(S.ma.crm))S.ma.crm=ents.includes("deals")?"deals":ents[0];
    const scr=screen("CRM");const seg=h("div","ma-seg");scr.top.after(seg);
    const bar=h("div","ma-filter"),q=mk("input",{placeholder:"Search…",type:"search"});q.className="ma-search";const chips=h("div","ma-chips"),sum=h("div","ma-sum"),list=h("div","ma-list2");add(bar,q,chips);add(scr.body,bar,sum,list);
    let limit=40;
    const draw=()=>{const e=S.ma.crm,spec=ENT[e];seg.innerHTML="";for(const k of ents){const b=h("button","ma-segb"+(k===e?" on":""),label(ENT[k].one)+"s");b.type="button";b.onclick=()=>{if(S.ma.crm!==k){S.ma.crm=k;S.ma.f="all";limit=40;q.value="";scr.reload()}};seg.appendChild(b)}
      const items=A(S.data[e]),f=S.ma.f||"all",t=q.value.trim().toLowerCase(),key=spec.key;
      chips.innerHTML="";const cs=[["all","All"],["my","Mine"]];if(key)for(const s of spec.stages())cs.push([s,label(s)]);
      for(const[k,l]of cs){const n=k==="all"?items.length:k==="my"?items.filter(x=>x.ownerUid===S.me.me.uid).length:items.filter(x=>x[key]===k).length;const c=h("button","ma-chip"+(f===k?" on":""),l+" "+n);c.type="button";if(key&&k!=="all"&&k!=="my")c.style.setProperty("--c",stc(k));c.onclick=()=>{S.ma.f=k;limit=40;draw()};chips.appendChild(c)}
      const rs=items.filter(x=>(f==="all"||(f==="my"?x.ownerUid===S.me.me.uid:x[key]===f))&&(!t||JSON.stringify(Object.values(x)).toLowerCase().includes(t))).sort((a,c)=>new Date(c.updatedAt||c.createdAt||0)-new Date(a.updatedAt||a.createdAt||0));
      sum.textContent=rs.length+" "+(rs.length===1?spec.one:spec.one+"s")+(e==="deals"||e==="leads"?" · "+money(rs.reduce((a,x)=>a+(x.value||0),0)):"");
      list.innerHTML="";for(const r of rs.slice(0,limit))list.appendChild(card(e,r));
      if(rs.length>limit){const m=h("button","ma-more","Show more");m.type="button";m.onclick=()=>{limit+=40;draw()};list.appendChild(m)}
      if(!rs.length)list.appendChild(empty("Nothing here yet",can(e,"c")?"Tap + to add a "+spec.one+".":""))};
    q.oninput=()=>{limit=40;draw()};
    scr.reload=async()=>{const e=S.ma.crm;list.innerHTML="";list.appendChild(h("div","ma-load","Loading…"));try{await Promise.all([load(e),load("contacts"),load("companies")].map(p=>p.catch(()=>[])));draw()}catch(x){list.innerHTML="";ER(list,x.message)}};
    scr.onshow=()=>{S.data[S.ma.crm]?draw():scr.reload()};scr.reload();return scr}

  // ---------------------------------------------------------------- RECORD (full screen)
  async function record(e,rec,defaults){
    const spec=ENT[e],isNew=!rec;
    try{if(e==="deals"||e==="contacts"||e==="leads")await Promise.all([load("companies"),load("contacts")].map(p=>p.catch(()=>[])))}catch(x){}
    const vals=Object.assign({ownerUid:S.me.me.uid,assigneeUid:S.me.me.uid,source:"manual"},defaults||{},rec||{});
    if(e==="deals"&&isNew&&!vals.stage)vals.stage="new";if(e==="leads"&&isNew&&!vals.status)vals.status="new";
    const f=formEl(spec.fields(),vals);let cur=isNew?(vals[spec.key]||""):rec&&rec[spec.key];
    const scr=screen(isNew?"New "+spec.one:"",{back:true});const sv=h("button","ma-save",isNew?"Create":"Save");sv.type="button";
    const more=h("button","ma-ibtn","⋯");more.type="button";more.setAttribute("aria-label","More");
    if(isNew?can(e,"c"):can(e,"e"))scr.right.appendChild(sv);if(rec)scr.right.appendChild(more);
    const body=scr.body;let hdr=null,tabs=null,pane=null,dirty=false;
    const payload=()=>{const o=f.get();if(rec)o.id=rec.id;if(spec.key)o[spec.key]=cur;if(e==="deals"||e==="contacts"){o.companyName=nameOf("companies",o.companyId);if(e==="deals")o.contactName=nameOf("contacts",o.contactId)}return o};
    sv.onclick=async()=>{const o=payload();const nm=spec.fields()[0].k;if(!String(o[nm]||"").trim()){toast("Please fill in the "+spec.fields()[0].label.toLowerCase());return}sv.disabled=true;
      try{await api("POST","/api/crm/save?e="+e,o);S.data[e]=null;toast(isNew?"Created":"Saved");if(isNew){back()}else{Object.assign(rec,o);dirty=false;drawHdr();scr.title.textContent=rec.title||rec.name||"";show_("overview")}}catch(x){toast(x.message)}sv.disabled=false};
    const setStage=async s=>{if(e==="leads"&&s==="converted"){if(rec)convertLead(rec);return}let lr;if(s==="lost"&&e==="deals"){lr=prompt("Why was this deal lost? (optional)");if(lr===null)return}cur=s;drawHdr();
      if(rec&&can(e,"e")){try{const o={id:rec.id,[spec.key]:s};if(lr!=null)o.lostReason=lr;await api("POST","/api/crm/save?e="+e,o);rec[spec.key]=s;S.data[e]=null;toast("Moved to "+label(s))}catch(x){toast(x.message)}}};
    function drawHdr(){if(!hdr){hdr=h("div","ma-rh");body.prepend(hdr)}hdr.innerHTML="";const nm=(rec&&(rec.title||rec.name))||(f.get()[spec.fields()[0].k])||"";
      const top=h("div","ma-rh1");add(top,av(nm||"?",spec.key?stc(cur):null),(()=>{const d=h("div","ma-rht");add(d,h("h2",null,nm||"New "+spec.one),h("p",null,rec?subOf(e,rec):""));return d})());hdr.appendChild(top);
      const meta=h("div","ma-rm");if(spec.key){const sp=h("button","ma-stage-btn");sp.type="button";sp.style.setProperty("--c",stc(cur));sp.append(label(cur||"Choose stage")+" ▾");if(rec&&!can(e,"e"))sp.disabled=true;sp.onclick=()=>sheet("Stage",b=>{const g=h("div","ma-list");for(const s of spec.stages()){const r=h("button","ma-row"+(s===cur?" on":""));r.type="button";const d=h("span","ma-dot");d.style.background=stc(s);add(r,d,h("b",null,label(s)));r.onclick=()=>{closeSheet();setStage(s)};g.appendChild(r)}b.appendChild(g)});meta.appendChild(sp)}
      if(rec&&rec.value)meta.appendChild(h("b","ma-val",money(rec.value)));hdr.appendChild(meta);
      if(rec){const ph=phoneOf(e,rec),em=emailOf(e,rec),ac=h("div","ma-acts"),nm2=rec.name||rec.title;
        const A1=(ic,t,href,fn,dis)=>{const a=h(href?"a":"button","ma-act"+(dis?" dis":""));if(href){a.href=href;if(href.startsWith("http")){a.target="_blank";a.rel="noopener"}}else a.type="button";add(a,icon(ic,21),h("span",null,t));if(fn)a.onclick=fn;return a};
        ac.appendChild(A1("phone","Call",ph?"tel:"+clean(ph):null,()=>{ph?logAct(e,rec.id,"call","Call started from the CRM ("+ph+")"):toast("No phone number yet")},!ph));
        ac.appendChild(A1("send","WhatsApp",ph?waLink(ph,nm2):null,()=>{ph?logAct(e,rec.id,"note","WhatsApp chat opened"):toast("No phone number yet")},!ph));
        ac.appendChild(A1("send","Email",em?"mailto:"+em:null,()=>{em?logAct(e,rec.id,"email","Email opened from the CRM"):toast("No email yet")},!em));
        if(can("tasks","c"))ac.appendChild(A1("check","Task",null,()=>newTask({relType:e,relId:rec.id,relName:nm2||"",assigneeUid:rec.ownerUid||S.me.me.uid})));
        hdr.appendChild(ac);
        if(e==="leads"&&rec.status!=="converted"&&can("deals","c")&&can("leads","e")){const c=h("button","ma-convert","Convert to deal");c.type="button";c.onclick=()=>convertLead(rec);hdr.appendChild(c)}}}
    more.onclick=()=>sheet(label(spec.one),b=>{const g=h("div","ma-list");
      if(e==="leads"&&rec.email&&X.coldEmail){const r=h("button","ma-row");r.type="button";r.appendChild(h("b",null,"AI cold email"));r.onclick=()=>{closeSheet();X.coldEmail(rec)};g.appendChild(r)}
      if(can(e,"d")){const r=h("button","ma-row bad");r.type="button";r.appendChild(h("b",null,"Delete "+spec.one));r.onclick=async()=>{if(!confirm("Delete this "+spec.one+"?"))return;try{await api("POST","/api/crm/delete?e="+e,{id:rec.id});S.data[e]=null;closeSheet();back();toast("Deleted")}catch(x){toast(x.message)}};g.appendChild(r)}
      if(!g.children.length)g.appendChild(h("p","ma-mut","No more actions."));b.appendChild(g)});
    drawHdr();if(rec)scr.title.textContent=rec.title||rec.name||"";
    pane=h("div","ma-pane");
    function show_(k){if(tabs)[...tabs.children].forEach(b=>b.classList.toggle("on",b.dataset.k===k));pane.innerHTML="";
      if(k==="overview")overview();else if(k==="details"){pane.appendChild(f);f.classList.add("ma-form");f.oninput=()=>{dirty=true};if(rec&&e==="companies"){const cs=A(S.data.contacts).filter(c=>c.companyId===rec.id);pane.appendChild(h("p","ma-mut",cs.length?"Contacts: "+cs.map(c=>c.name).join(", "):"No contacts yet."))}}
      else activity()}
    function overview(){const rows=[];const cur2=rec||{};
      const push2=(l,v)=>{if(v)rows.push([l,v])};
      push2("Phone",phoneOf(e,cur2));push2("Email",emailOf(e,cur2));
      if(e==="leads"){push2("Company",cur2.company);push2("Job title",cur2.title);push2("Source",cur2.source&&label(cur2.source));push2("Campaign",cur2.campaign)}
      if(e==="deals"){push2("Contact",cur2.contactName);push2("Company",cur2.companyName);push2("Expected close",cur2.expectedClose);push2("Source",cur2.source&&label(cur2.source));push2("Lost reason",cur2.lostReason)}
      if(e==="contacts"){push2("Company",cur2.companyName||nameOf("companies",cur2.companyId));push2("Job title",cur2.title);push2("City",cur2.city)}
      if(e==="companies"){push2("Website",cur2.website);push2("Industry",cur2.industry);push2("City",cur2.city);push2("Size",cur2.size)}
      push2("Responsible",ownerName(cur2.ownerUid));push2("Created",date(cur2.createdAt));
      if(A(cur2.tags).length)push2("Tags",cur2.tags.join(", "));
      const c=h("div","ma-facts");for(const[l,v]of rows){const r=h("div","ma-fact");add(r,h("small",null,l),h("span",null,v));c.appendChild(r)}pane.appendChild(c);
      if(cur2.notes){const n=h("div","ma-note");add(n,h("small",null,"Comments"),h("p",null,cur2.notes));pane.appendChild(n)}
      const nx=h("div","ma-hint");nx.textContent="Tap Details to edit, or Activity to log a call, note or meeting.";pane.appendChild(nx)}
    async function activity(){let mode="note";
      if(can(e,"e")){const comp=h("div","ma-comp"),ch=h("div","ma-chips");
        const ta=mk("textarea",{rows:3,placeholder:"Write a note, log a call…"}),dd=mk("input",{type:"date"});dd.hidden=true;
        for(const[k,l]of[["note","Note"],["call","Call"],["meeting","Meeting"],["email","Email"],...(can("tasks","c")?[["task","Task"]]:[])]){const c=h("button","ma-chip"+(k===mode?" on":""),l);c.type="button";c.onclick=()=>{mode=k;[...ch.children].forEach(x=>x.classList.toggle("on",x===c));dd.hidden=k!=="task";ta.placeholder=k==="task"?"What needs to be done?":"Write here…"};ch.appendChild(c)}
        const go2=h("button","ma-btn","Save to timeline");go2.type="button";
        go2.onclick=async()=>{const t=ta.value.trim();if(!t)return;go2.disabled=true;try{if(mode==="task"){await api("POST","/api/crm/save?e=tasks",{title:t,due:dd.value,priority:"normal",assigneeUid:rec.ownerUid||S.me.me.uid,relType:e,relId:rec.id,relName:rec.name||rec.title||""});S.data.tasks=null}else await api("POST","/api/crm/activity",{relType:e,relId:rec.id,type:mode,text:t});ta.value="";toast("Added");draw()}catch(x){toast(x.message)}go2.disabled=false};
        add(comp,ch,ta,dd,go2);pane.appendChild(comp)}
      const tl=h("div","ma-tl");pane.appendChild(tl);
      async function draw(){tl.innerHTML="";try{const its=A((await api("GET","/api/crm/activities?relType="+e+"&relId="+rec.id)).items);for(const a of its){const d=h("div","ma-ev "+a.type);add(d,h("i"),(()=>{const x=h("div");add(x,h("p",null,a.text),h("small",null,label(a.type)+" · "+(a.byUid?ownerName(a.byUid):"System")+" · "+ago(a.at)));return x})());tl.appendChild(d)}if(!its.length)tl.appendChild(empty("No history yet"))}catch(x){ER(tl,x.message)}}
      draw()}
    if(isNew){body.appendChild(pane);show_("details")}
    else{tabs=h("div","ma-tabs2");for(const[k,l]of[["overview","Overview"],["details","Details"],...(e!=="tasks"?[["activity","Activity"]]:[])]){const b=h("button",null,l);b.type="button";b.dataset.k=k;b.onclick=()=>show_(k);tabs.appendChild(b)}body.append(tabs,pane);show_("overview")}
    push(scr)}

  function newTask(defaults){
    sheet("New task",b=>{const f=formEl([{k:"title",label:"Task",req:1,wide:1},{k:"due",label:"Deadline",type:"date"},{k:"priority",label:"Priority",type:"select",opts:[["low","Low"],["normal","Normal"],["high","High"]]},{k:"assigneeUid",label:"Responsible",type:"select",opts:ENT.tasks.fields()[3].opts}],Object.assign({priority:"normal",assigneeUid:S.me.me.uid,due:today()},defaults||{}));f.classList.add("ma-form");
      const go2=h("button","ma-btn","Create task");go2.type="button";go2.onclick=async()=>{const o=Object.assign({},defaults||{},f.get());if(!o.title.trim()){toast("Write what needs to be done");return}go2.disabled=true;try{await api("POST","/api/crm/save?e=tasks",o);S.data.tasks=null;closeSheet();toast("Task added");const t=stack[stack.length-1];t&&t.onshow&&t.onshow();t&&t.reload&&t.reload()}catch(x){toast(x.message);go2.disabled=false}};add(b,f,go2)})}

  // ---------------------------------------------------------------- TASKS
  function taskRow(k){const r=h("div","ma-task"+(k.done?" done":"")),c=h("button","ma-check"+(k.done?" on":""));c.type="button";c.setAttribute("aria-label","Complete");if(k.done)c.textContent="✓";c.disabled=!can("tasks","e");
    c.onclick=async ev=>{ev.stopPropagation();const n=!k.done;try{await api("POST","/api/crm/save?e=tasks",{id:k.id,done:n});k.done=n;S.data.tasks=null;c.classList.toggle("on",n);c.textContent=n?"✓":"";r.classList.toggle("done",n);toast(n?"Task completed":"Task reopened")}catch(x){toast(x.message)}};
    const m=h("div","ma-tm"),od=!k.done&&k.due&&k.due<today();add(m,h("b",null,k.title),h("small",od?"bad":null,[k.due?(k.due===today()?"Today":date(k.due)):"No deadline",k.relName,k.priority==="high"?"High priority":""].filter(Boolean).join(" · ")));
    add(r,c,m);r.onclick=()=>modalTask(k);return r}
  function tasks(){
    const scr=screen("Tasks");S.ma=S.ma||{};const m0=S.ma.tm||"today";const seg=h("div","ma-seg");scr.top.after(seg);const list=h("div","ma-list2");scr.body.appendChild(list);
    const draw=()=>{const t=today(),mode=S.ma.tm||"today";seg.innerHTML="";for(const[k,l]of[["today","Today"],["open","Open"],["overdue","Overdue"],["done","Done"]]){const b=h("button","ma-segb"+(k===mode?" on":""),l);b.type="button";b.onclick=()=>{S.ma.tm=k;draw()};seg.appendChild(b)}
      const mine=A(S.data.tasks).filter(x=>x.assigneeUid===S.me.me.uid||!x.assigneeUid||can("team","v"));
      const rs=mine.filter(x=>mode==="done"?x.done:mode==="today"?!x.done&&x.due&&x.due<=t:mode==="overdue"?!x.done&&x.due&&x.due<t:!x.done).sort((a,c)=>(a.due||"9")<(c.due||"9")?-1:1);
      list.innerHTML="";for(const r of rs)list.appendChild(taskRow(r));if(!rs.length)list.appendChild(empty(mode==="today"?"Nothing due today":"Nothing here","Tap + to add a task."))};
    scr.reload=async()=>{list.innerHTML="";list.appendChild(h("div","ma-load","Loading…"));try{await load("tasks");draw()}catch(x){list.innerHTML="";ER(list,x.message)}};
    scr.onshow=()=>{S.data.tasks?draw():scr.reload()};scr.reload();return scr}

  // ---------------------------------------------------------------- CHATS
  function chats(){
    const scr=screen("Chats");const list=h("div","ma-list2");scr.body.appendChild(list);let data=[];
    const sig=d=>d.map(c=>c.id+":"+A(c.msgs).length+":"+(c.status||"")).join("|");let last="";
    const draw=()=>{list.innerHTML="";for(const c of data){const r=h("div","ma-chat"),a=av((c.name||"?"),c.channel==="instagram"?"#d6249f":c.channel==="whatsapp"?"#25a35a":"#1877f2");const lm=A(c.msgs).slice(-1)[0]||{};const m=h("div","ma-cm");add(m,h("b",null,c.name||"Contact"),h("span",null,lm.t||""));const rt=h("div","ma-cr");add(rt,h("small",null,ago(lm.at)),h("em",null,label(c.channel)));add(r,a,m,rt);r.onclick=()=>conversation(c);list.appendChild(r)}
      if(!data.length)list.appendChild(empty("No conversations yet","They appear here as soon as someone messages or comments on a connected Page or Instagram account."))};
    scr.reload=async()=>{try{data=A((await api("GET","/api/crm/chats")).chats);last=sig(data);draw()}catch(x){list.innerHTML="";ER(list,x.message)}};
    const tm=setInterval(async()=>{if(!document.body.contains(scr.root)){clearInterval(tm);return}if(scr.root.hidden||document.hidden)return;try{const d=A((await api("GET","/api/crm/chats")).chats);if(sig(d)!==last){data=d;last=sig(d);draw()}}catch(x){}},5000);
    scr.onshow=()=>scr.reload();scr.reload();return scr}
  function conversation(c0){
    let c=c0;const scr=screen(c.name||"Contact",{back:true});scr.root.classList.add("chat");
    const sub=h("small","ma-sub",label(c.channel)+(c.flowName?" · Automation: "+c.flowName:""));scr.title.after(sub);
    const msgs=h("div","ma-msgs");scr.body.appendChild(msgs);
    const draw=()=>{msgs.innerHTML="";if(c.lastError)msgs.appendChild(h("p","ma-err",c.lastError));
      if(c.status==="human"&&can("leads","e")){const r=h("button","ma-convert","Resume automation");r.type="button";r.onclick=async()=>{try{await api("POST","/api/crm/chat/resume",{id:c.id});toast("Automation resumed");poll()}catch(x){toast(x.message)}};msgs.appendChild(r)}
      for(const x of A(c.msgs)){const b=h("div","ma-b "+x.d);add(b,h("span",null,x.t),h("small",null,ago(x.at)));msgs.appendChild(b)}msgs.scrollTop=msgs.scrollHeight};
    draw();
    if(can("leads","e")){const f=h("form","ma-composer"),i=mk("input",{placeholder:"Reply as your team",autocomplete:"off"}),sd=h("button","ma-send","↑");sd.type="submit";sd.setAttribute("aria-label","Send");add(f,i,sd);scr.root.appendChild(f);
      f.onsubmit=async ev=>{ev.preventDefault();const t=i.value.trim();if(!t)return;sd.disabled=true;try{await api("POST","/api/crm/chat/send",{id:c.id,text:t});i.value="";await poll()}catch(x){toast(x.message)}sd.disabled=false}}
    async function poll(){try{const d=A((await api("GET","/api/crm/chats")).chats),n=d.find(x=>x.id===c.id);if(n&&JSON.stringify(n.msgs)!==JSON.stringify(c.msgs)||(n&&n.status!==c.status)){c=n;draw()}}catch(x){}}
    const tm=setInterval(()=>{if(!document.body.contains(scr.root)){clearInterval(tm);return}if(!scr.root.hidden&&!document.hidden)poll()},4000);
    push(scr)}

  // ---------------------------------------------------------------- MENU
  const PRIMARY=new Set(["leads","deals","contacts","companies","tasks","chats"]);
  function menu(){
    const scr=screen("Menu");const b=scr.body;if(X.wsBar)b.appendChild(X.wsBar());
    const pc=h("div","ma-prof");add(pc,av(ownerName(S.me.me.uid)),(()=>{const d=h("div");add(d,h("b",null,ownerName(S.me.me.uid)),h("small",null,S.me.workspace.name+" · "+label(S.me.me.role)));return d})());b.appendChild(pc);
    for(const sec of SECTIONS){const its=sec.items.filter(i=>can(i[2],"v")&&!PRIMARY.has(i[0]));if(!its.length)continue;
      b.appendChild(h("h4","ma-grp",sec.t));const g=h("div","ma-menu");
      for(const[k,l,,ic]of its){const r=h("button","ma-mrow");r.type="button";add(r,(()=>{const s=h("span","ma-mi");s.appendChild(icon(ic,20));return s})(),h("b",null,l),h("span","ma-chev","›"));r.onclick=()=>legacy(k,l);g.appendChild(r)}b.appendChild(g)}
    b.appendChild(h("h4","ma-grp","Appearance"));const t=h("div","ma-seg in");for(const[k,l]of[["dark","Dark"],["slate","Slate"],["light","Light"]]){const x=h("button","ma-segb"+(document.documentElement.dataset.theme===k?" on":""),l);x.type="button";x.onclick=()=>{const o=document.querySelector('.bz-theme button[data-t="'+k+'"]');if(o)o.click();[...t.children].forEach(y=>y.classList.toggle("on",y===x))};t.appendChild(x)}b.appendChild(t);
    const al=h("a","ma-mrow solo");al.href="/dashboard";add(al,(()=>{const s=h("span","ma-mi");s.appendChild(icon("bars",20));return s})(),h("b",null,"Analytics home"),h("span","ma-chev","›"));b.appendChild(al);
    const lo=h("button","ma-mrow solo bad");lo.type="button";add(lo,(()=>{const x=h("span","ma-mi");x.appendChild(icon("userminus",20));return x})(),h("b",null,"Log out"),h("span","ma-chev","\u203A"));lo.onclick=()=>sheet("Log out of ficx?",bd=>{const g=h("div","ma-list"),y=h("button","ma-row bad");y.type="button";y.appendChild(h("b",null,"Yes, log me out"));y.onclick=()=>logout();const n=h("button","ma-row");n.type="button";n.appendChild(h("b",null,"Stay signed in"));n.onclick=closeSheet;add(g,y,n);bd.appendChild(g)});b.appendChild(lo);
    return scr}

  // Other sections (analytics, outbound, automations, team …) open inside the app with a native top bar.
  const mainEl=()=>document.querySelector(".bx-main");let mainHome=null;
  function legacy(k,l){
    const scr=screen(l,{back:true});scr.root.classList.add("leg");const m=mainEl();if(m&&!mainHome)mainHome=m.parentElement;
    if(m){scr.body.appendChild(m);m.hidden=false}scr.onclose=()=>{if(mainHome&&m)mainHome.appendChild(m)};
    S.view=k;push(scr);show(k)}

  // ---------------------------------------------------------------- start
  drawTabs();
  history.replaceState({ma:"exit"},"");history.pushState({ma:"app"},"");
  const api2={
    // used by crm.js: route desktop calls to the phone screens
    nav(k){if(k==="chats"||k==="tasks"){go(k);return true}if(["leads","deals","contacts","companies"].includes(k)){S.ma=S.ma||{};S.ma.crm=k;go("crm");return true}if(k==="dashboard"||k==="home"){go("home");return true}const it=SECTIONS.flatMap(s=>s.items).find(i=>i[0]===k);if(it){legacy(k,it[1]);return true}return false},
    record,sync(){}};
  go("home");
  return api2}
