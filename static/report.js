import{h,add,icon,api,copyBox}from"./ui.js";
import{fixFromCheck,fixFromPlan,fixFromText,mount,renderViral}from"./actions.js";

let charts=[];
export const destroyCharts=()=>{charts.forEach(c=>c.destroy());charts=[]};
const css=v=>getComputedStyle(document.documentElement).getPropertyValue(v).trim();
const pal=()=>(css("--pal")||"#2563eb,#60a5fa,#14b8a6,#7c3aed,#f59e0b,#ef4444").split(",").map(x=>x.trim());
const nf=v=>Number(v||0).toLocaleString();
const compact=v=>Intl.NumberFormat("en",{notation:"compact",maximumFractionDigits:1}).format(v);
const tone=s=>s>=75?"#16a34a":s>=50?"#f59e0b":"#ef4444";
const PAGE_ICON={overview:"grid",shortlong:"layers",videos:"film",tags:"tag",time:"cal",fixes:"check",seo:"search",technical:"zap",ai:"star",search:"search",traffic:"trend",youtube:"play",posts:"film",growth:"trend",reputation:"star",listing:"layers"};
const kpiIcon=l=>/watch|duration|time/i.test(l)?"clock":/new sub/i.test(l)?"userplus":/lost/i.test(l)?"userminus":/subscr|users|follow/i.test(l)?"users":/engag|like/i.test(l)?"thumb":/forecast|growth|trend/i.test(l)?"trend":/rating|score|ctr/i.test(l)?"star":/view|click|impress|session|reach/i.test(l)?"eye":"bars";

function spark(vals){
  const w=100,hh=38,min=Math.min(...vals),max=Math.max(...vals),rng=(max-min)||1;
  const pts=vals.map((v,i)=>[i/(vals.length-1)*w,hh-3-((v-min)/rng)*(hh-8)]);
  const d=pts.map((p,i)=>(i?"L":"M")+p[0].toFixed(1)+" "+p[1].toFixed(1)).join(" ");
  const el=document.createElement("div");el.className="rp-spark";
  el.innerHTML='<svg viewBox="0 0 '+w+" "+hh+'" preserveAspectRatio="none"><path d="'+d+" L"+w+" "+hh+" L0 "+hh+' Z" fill="var(--accent)" opacity=".12"/><path d="'+d+'" fill="none" stroke="var(--accent)" stroke-width="2" vector-effect="non-scaling-stroke"/></svg>';
  return el;
}
function kpiCard(k){
  const d=h("div","rp-kpi");d.dataset.s=k.status||"";
  const ic=h("div","rp-ico");ic.appendChild(icon(kpiIcon(k.label),22));
  const b=h("div","rp-kb");add(b,h("small",null,k.label),h("b",null,k.value));
  const m=/([+-]\d+%)/.exec(k.note||"");
  if(m){const up=m[1][0]==="+";const row=h("div","rp-kd");add(row,h("span","bz-chip "+(up?"up":"down"),(up?"\u25B2 ":"\u25BC ")+m[1].slice(1)),h("span","bz-mut",(k.note||"").replace(m[1],"").trim()));b.appendChild(row)}
  else if(k.note)b.appendChild(h("span","bz-mut rp-note",k.note));
  add(d,ic,b);if(k.spark&&k.spark.length>3)d.appendChild(spark(k.spark));return d;
}
const kpiRow=list=>{const g=h("div","rp-kpis");for(const k of list)g.appendChild(kpiCard(k));return g};

function chartCard(parent,c,cls){
  const card=h("div","rp-card "+(cls||""));card.appendChild(h("h3",null,c.title));
  const wrap=h("div","rp-chart"+(c.type==="doughnut"?" rp-donut":"")),cv=document.createElement("canvas");wrap.appendChild(cv);card.appendChild(wrap);parent.appendChild(card);
  const P=pal(),mut=css("--mut")||"#64748b",grid="rgba(148,163,184,.18)",dn=c.type==="doughnut"||c.type==="pie";
  const ds=c.datasets.map((d,i)=>{
    const col=P[i%P.length],prev=/previous|trend|recommended|forecast/i.test(d.name);
    if(dn)return{label:d.name,data:d.data,backgroundColor:P,borderWidth:0,spacing:3,borderRadius:6};
    if(c.type==="line")return{label:d.name,data:d.data,borderColor:prev?(i===0?col:P[1]):col,backgroundColor:prev?"transparent":(ctx=>{const a=ctx.chart.chartArea;if(!a)return col+"22";const g=ctx.chart.ctx.createLinearGradient(0,a.top,0,a.bottom);g.addColorStop(0,col+"55");g.addColorStop(1,col+"00");return g}),fill:!prev,tension:.35,pointRadius:d.data.length>16?0:3,pointHoverRadius:5,borderWidth:2.5,borderDash:prev&&i>0?[6,6]:[]};
    return{label:d.name,data:d.data,backgroundColor:col,borderRadius:8,maxBarThickness:34};
  });
  const tips=c.tips||[];
  charts.push(new Chart(cv,{type:c.type,data:{labels:c.labels,datasets:ds},options:{responsive:true,maintainAspectRatio:false,cutout:dn?"68%":undefined,interaction:{mode:"index",intersect:false},
    plugins:{legend:{display:dn||ds.length>1,position:dn?"bottom":"top",align:"end",labels:{color:mut,usePointStyle:true,boxWidth:8,padding:14}},
      tooltip:{backgroundColor:"#0f172a",titleColor:"#fff",bodyColor:"#e2e8f0",padding:10,cornerRadius:10,callbacks:{title:items=>tips[items[0].dataIndex]||items[0].label}}},
    scales:dn?{}:{x:{grid:{display:false},ticks:{color:mut,maxTicksLimit:10,maxRotation:0},border:{display:false}},y:{beginAtZero:true,grid:{color:grid},ticks:{color:mut,callback:v=>compact(v)},border:{display:false}}}}}));
}
function tableCard(t,cls){
  const card=h("div","rp-card "+(cls||""));card.appendChild(h("h3",null,t.title));
  const w=h("div","rp-scroll"),tb=h("table","bz-table"),hd=tb.createTHead().insertRow();
  for(const c of t.columns)hd.appendChild(h("th",null,c));
  const body=tb.createTBody();for(const r of t.rows||[]){const tr=body.insertRow();for(const c of r.cells)tr.insertCell().textContent=c}
  w.appendChild(tb);card.appendChild(w);return card;
}
function grid(main,cls){const g=h("div",cls||"rp-grid");main.appendChild(g);return g}
const fmtDur=s=>{s=Math.round(s);return Math.floor(s/60)+":"+String(s%60).padStart(2,"0")};

// ---------- real content (videos / posts / screenshots) ----------
function thumb(it,cls){
  const d=h("div",cls||"rp-thumb");
  if(it.thumb){const i=document.createElement("img");i.src=it.thumb;i.loading="lazy";i.alt="";i.referrerPolicy="no-referrer";i.onerror=()=>{i.remove()};d.appendChild(i)}
  return d;
}
function topContent(r){
  const items=[...(r.items||[])].filter(x=>x.metrics).sort((a,b)=>(b.metrics.views||b.metrics.engagement||0)-(a.metrics.views||a.metrics.engagement||0)).slice(0,5);
  if(!items.length)return null;
  const card=h("div","rp-card");card.appendChild(h("h3",null,"Top performing content"));
  const max=Math.max(...items.map(x=>x.metrics.views||x.metrics.engagement||1),1);
  items.forEach((it,i)=>{
    const v=it.metrics.views||it.metrics.engagement||0,row=h("div","rp-top");
    const body=h("div","rp-top-b"),bar=h("div","bz-bar"),f=h("i");f.style.width=Math.max(4,v/max*100)+"%";bar.appendChild(f);
    const t=h("a",null,it.title);t.href=it.url||"#";t.target="_blank";t.rel="noopener";
    add(body,t,bar);add(row,h("b",null,String(i+1)),thumb(it,"rp-thumb sm"),body,h("span","rp-num",nf(v)));card.appendChild(row);
  });
  return card;
}
function itemsPage(main,r,pageId){
  const items=(r.items||[]).filter(x=>x.page===pageId);
  if(!items.length)return;
  const card=h("div","rp-card");const hd=h("h3");hd.appendChild(h("span",null,items[0].kind==="screenshot"?"Store screenshots":"Content ("+items.length+")"));
  const tools=h("div","rp-tools");hd.appendChild(tools);card.appendChild(hd);
  const list=h("div","rp-scroll rp-tall");card.appendChild(list);
  let filter="all",sort="published";
  const kinds=[...new Set(items.map(x=>x.kind))];
  if(kinds.length>1){const sel=h("select");for(const k of["all",...kinds]){const o=h("option",null,k==="all"?"All":k[0].toUpperCase()+k.slice(1));o.value=k;sel.appendChild(o)}sel.onchange=()=>{filter=sel.value;draw()};tools.appendChild(sel)}
  if(items[0].metrics&&items[0].metrics.views!==undefined){const s2=h("select");for(const[v,l]of[["published","Newest"],["views","Most views"],["perDay","Views per day"],["engagement","Engagement"]]){const o=h("option",null,l);o.value=v;s2.appendChild(o)}s2.onchange=()=>{sort=s2.value;draw()};tools.appendChild(s2)}
  const draw=()=>{
    list.innerHTML="";
    let arr=items.filter(x=>filter==="all"||x.kind===filter);
    arr=arr.sort((a,b)=>sort==="published"?new Date(b.published)-new Date(a.published):((b.metrics||{})[sort]||0)-((a.metrics||{})[sort]||0));
    const max=Math.max(...arr.map(x=>(x.metrics||{}).views||0),1);
    for(const it of arr){
      const m=it.metrics||{},wrap=h("div","vitem"),row=h("div","vrow");
      const mid=h("div","vmid"),t=h("a",null,it.title||"(untitled)");t.href=it.url||"#";t.target="_blank";t.rel="noopener";t.className="vtitle";
      const badges=h("div","vbadges");
      if(it.kind&&it.kind!=="screenshot")badges.appendChild(h("span","bz-badge",it.kind));
      if(m.duration)badges.appendChild(h("span","bz-badge",fmtDur(m.duration)));
      if(it.published)badges.appendChild(h("span","bz-badge",new Date(it.published).toLocaleDateString()));
      add(mid,t,badges);
      if(m.views!==undefined){const bar=h("div","bz-bar"),f=h("i");f.style.width=Math.max(3,m.views/max*100)+"%";bar.appendChild(f);mid.appendChild(bar)}
      const st=[];if(m.views!==undefined)st.push(nf(m.views)+" views");if(m.likes!==undefined)st.push(nf(m.likes)+" likes");if(m.comments!==undefined)st.push(nf(m.comments)+" comments");if(m.perDay)st.push(m.perDay+"/day");if(m.engagement!==undefined)st.push(m.engagement+"% eng.");
      if(m.shares)st.push(nf(m.shares)+" shares/saves");
      mid.appendChild(h("div","vstats",st.join("  \u00b7  ")));
      const act=h("div","vact");
      if(r.kind==="youtube"||r.kind==="google"){
        const b=h("button","bz-btn","Optimize + viral tags"),ws=h("div","bz-ws");ws.hidden=true;
        mount(b,ws,fixFromPlan({auto:"optimize-video:"+it.id,title:it.title},r),r,{videoId:it.id,text:it.title});
        act.appendChild(b);add(wrap,row,ws);
      }else if(it.kind==="post"){
        const b=h("button","bz-btn ghost","Improve caption"),ws=h("div","bz-ws");ws.hidden=true;
        mount(b,ws,fixFromText("caption hashtags hook",r),r,{text:it.title});act.appendChild(b);add(wrap,row,ws);
      }else wrap.appendChild(row);
      add(row,thumb(it),mid,act);list.appendChild(wrap);
    }
  };
  draw();main.appendChild(card);
}

// ---------- fixes ----------
function fixCard(parent,o,builder,r,item){
  const card=h("div","bz-fix"),hd=h("div","bz-fix-h"),left=h("div"),line=h("div");
  add(line,h("span","bz-pill "+o.cls,o.pill),h("b",null,o.title));left.appendChild(line);
  if(o.sub)left.appendChild(h("p",null,o.sub));if(o.body)left.appendChild(h("p",null,o.body));
  const btn=h("button","bz-btn","Open fix");add(hd,left,btn);card.appendChild(hd);
  const ws=h("div","bz-ws");ws.hidden=true;card.appendChild(ws);mount(btn,ws,builder,r,item);parent.appendChild(card);
}
function checksCard(main,r,id){
  const cs=(r.checks||[]).filter(c=>c.page===id);if(!cs.length)return;
  const card=h("div","rp-card");card.appendChild(h("h3",null,"Checks in this area"));
  for(const c of cs.sort((a,b)=>(a.status==="pass")-(b.status==="pass")||b.impact-a.impact)){
    if(c.status==="pass"){const row=h("div","rp-chk");add(row,h("span","bz-pill pass","PASS"),h("span",null,c.name+" \u2014 "+c.value));card.appendChild(row)}
    else fixCard(card,{cls:c.status,pill:c.status==="fail"?"CRITICAL":"IMPROVE",title:c.name+" \u2014 "+c.value,sub:"Impact "+c.impact+"/5",body:"What to do: "+c.fix},fixFromCheck(c,r),r,{text:c.fix});
  }
  main.appendChild(card);
}
function monthPlan(box,r){
  const checks=(r.checks||[]).filter(c=>c.status!=="pass").sort((a,b)=>((a.status==="fail"?0:1)-(b.status==="fail"?0:1))||(b.impact-a.impact));
  let steps=[];
  if(r.ai&&(r.ai.plan||[]).length)steps=r.ai.plan.map(t=>({text:t}));
  else{for(const p of r.plan||[])steps.push({text:p.title,plan:p});for(const c of checks.slice(0,4))steps.push({text:c.fix,check:c})}
  const seen=new Set();steps=steps.filter(s=>{const k=(s.text||"").toLowerCase();if(!k||seen.has(k))return false;seen.add(k);return true}).slice(0,8);
  if(!steps.length)return;
  const card=h("div","rp-card");card.appendChild(h("h3",null,"30-day plan"));
  const per=Math.ceil(steps.length/4),hosts={};
  steps.forEach((s,i)=>{
    const wk=Math.floor(i/per)+1;
    if(!hosts[wk]){const w=h("div","bz-week");w.appendChild(h("h4",null,"Week "+wk+" \u00b7 days "+((wk-1)*7+1)+"\u2013"+(wk*7)));card.appendChild(w);hosts[wk]=w}
    const row=h("div","bz-step-row"),chk=h("input");chk.type="checkbox";
    const key="bz:"+(r.id||r.createdAt)+":"+i;try{chk.checked=localStorage.getItem(key)==="1"}catch(e){}
    row.classList.toggle("done",chk.checked);chk.onchange=()=>{row.classList.toggle("done",chk.checked);try{localStorage.setItem(key,chk.checked?"1":"0")}catch(e){}};
    const body=h("div"),t=h("div","t",s.text),btn=h("button","bz-btn ghost","Open fix"),ws=h("div","bz-ws");btn.style.marginTop="8px";ws.hidden=true;add(body,t,btn,ws);
    mount(btn,ws,s.plan?fixFromPlan(s.plan,r):s.check?fixFromCheck(s.check,r):fixFromText(s.text,r),r,{text:s.text});
    add(row,chk,body);hosts[wk].appendChild(row);
  });
  box.appendChild(card);
}
function list(title,items){const d=h("div");add(d,h("h4",null,title));const ul=h("ul");for(const x of items||[])ul.appendChild(h("li",null,x));d.appendChild(ul);return d}

// ---------- pages ----------
function overview(main,r,hist){
  const checks=r.checks||[],fails=checks.filter(c=>c.status==="fail").length,warns=checks.filter(c=>c.status==="warn").length;
  const top=h("div","rp-top2");main.appendChild(top);
  const hero=h("div","rp-card rp-hero"),ring=h("div","rp-ring");
  ring.style.setProperty("--p",r.score);ring.style.setProperty("--c",tone(r.score));
  const mid=h("div");add(mid,h("b",null,String(r.score)),h("small",null,"/ 100"));ring.appendChild(mid);
  const tx=h("div");add(tx,h("h2",null,r.target),h("p","bz-mut",r.kind.replace("_"," ")+" report \u00b7 "+new Date(r.createdAt).toLocaleString()),
    h("p",null,(r.ai&&r.ai.summary)||((r.score>=75?"Strong":r.score>=50?"Solid but improvable":"Needs attention")+": "+fails+" critical issue"+(fails===1?"":"s")+" and "+warns+" improvement"+(warns===1?"":"s")+" found. Open Fixes & plan for ready-to-use content.")));
  add(hero,ring,tx);
  const cats=h("div","rp-card");cats.appendChild(h("h3",null,"Score by area"));
  const bars=h("div","bz-bars");
  for(const[k,v]of Object.entries(r.categories||{}).sort((a,b)=>a[1]-b[1])){const row=h("div"),l=h("div","bz-bar-l");add(l,h("span",null,k),h("b",null,v+"%"));const bar=h("div","bz-bar"),i=h("i");i.style.width=v+"%";i.style.background=tone(v);bar.appendChild(i);add(row,l,bar);bars.appendChild(row)}
  cats.appendChild(bars);add(top,hero,cats);
  if((r.kpis||[]).length)main.appendChild(kpiRow(r.kpis));
  const g=grid(main);
  const oc=(r.charts||[]).filter(c=>c.page==="overview");
  oc.forEach((c,i)=>chartCard(g,c,i===0?"span2":""));
  const tc=topContent(r);if(tc)g.appendChild(tc);
  for(const t of(r.tables||[]).filter(t=>t.page==="overview"))g.appendChild(tableCard(t,"span2"));
}
function fixes(main,r){
  if(r.ai){
    const a=h("div","rp-card");add(a,h("h3",null,"AI insights"),h("p",null,r.ai.summary));
    const cols=h("div","cols");add(cols,list("Strengths",r.ai.strengths),list("Improve first",r.ai.improvements),list("Ideas",r.ai.ideas));a.appendChild(cols);main.appendChild(a);
  }
  const checks=r.checks||[];
  const bad=checks.filter(c=>c.status!=="pass").sort((a,b)=>((a.status==="fail"?0:1)-(b.status==="fail"?0:1))||(b.impact-a.impact)).slice(0,8);
  const fx=h("div","rp-card");fx.appendChild(h("h3",null,"Fix these first"));
  if(!bad.length)fx.appendChild(h("p","bz-mut","Nothing urgent. Every check passed."));
  for(const c of bad)fixCard(fx,{cls:c.status,pill:c.status==="fail"?"CRITICAL":"IMPROVE",title:c.name+" \u2014 "+c.value,sub:"Impact "+c.impact+"/5 \u00b7 "+c.category,body:"What to do: "+c.fix},fixFromCheck(c,r),r,{text:c.fix});
  main.appendChild(fx);
  const plan=[...(r.plan||[])].sort((a,b)=>(b.impact/Math.max(1,b.effort))-(a.impact/Math.max(1,a.effort))||b.impact-a.impact);
  if(plan.length){const pc=h("div","rp-card");pc.appendChild(h("h3",null,"Growth plan \u2013 highest payoff first"));
    for(const p of plan)fixCard(pc,{cls:"plan",pill:"PLAN",title:p.title,sub:"Impact "+p.impact+"/5 \u00b7 Effort "+p.effort+"/5",body:p.why},fixFromPlan(p,r),r,{text:p.title});main.appendChild(pc)}
  monthPlan(main,r);
  const all=h("div","rp-card");all.appendChild(h("h3",null,"All checks"));
  const fl=h("div","bz-filter"),holder=h("div","rp-scroll");
  const draw=f=>{holder.innerHTML="";const t=h("table","bz-table"),hd=t.createTHead().insertRow();for(const c of["Status","Area","Check","Result"])hd.appendChild(h("th",null,c));
    const b=t.createTBody();for(const c of checks.filter(c=>f==="all"||c.status===f)){const tr=b.insertRow();for(const v of[c.status.toUpperCase(),c.category,c.name,c.value])tr.insertCell().textContent=v}holder.appendChild(t);
    fl.querySelectorAll("button").forEach(x=>x.classList.toggle("on",x.dataset.f===f))};
  for(const f of["all","fail","warn","pass"]){const b=h("button","bz-mini",f==="all"?"All":f);b.dataset.f=f;b.onclick=()=>draw(f);fl.appendChild(b)}
  add(all,fl,holder);main.appendChild(all);draw("all");
  if((r.notes||[]).length){const n=h("div","rp-card");n.appendChild(h("h3",null,"Notes"));for(const x of r.notes)n.appendChild(h("p","bz-mut",x));main.appendChild(n)}
}
function viralCard(main,r){
  const card=h("div","rp-card");card.appendChild(h("h3",null,"Viral tag pack generator"));
  card.appendChild(h("p","bz-mut","Builds tiered tags (broad, niche, long-tail, question, trending), title options, hooks and thumbnail text. It reuses the tags and words that already work on your channel."));
  const f=h("div","form"),topic=h("input"),sel=h("select");
  topic.placeholder="Topic of your next video";topic.value=(r.meta||{}).topTitle||"";
  const o0=h("option",null,"Just generate (no video)");o0.value="";sel.appendChild(o0);
  for(const it of(r.items||[]).filter(x=>r.kind!=="social"&&x.kind!=="post"&&x.kind!=="screenshot").slice(0,30)){const o=h("option",null,it.title.slice(0,60));o.value=it.id;sel.appendChild(o)}
  const l1=h("label",null,"Topic"),l2=h("label",null,"Optimize an existing video (optional)");l1.appendChild(topic);l2.appendChild(sel);add(f,l1,l2);card.appendChild(f);
  const out=h("div"),b=h("button","bz-btn","Generate viral tag pack");
  b.onclick=async()=>{b.disabled=true;out.innerHTML="";out.appendChild(h("p","bz-mut","Writing…"));
    try{const res=await api("POST","/api/optimize",{platform:"youtube",videoId:sel.value,topic:topic.value+" | tags that already work: "+((r.meta||{}).topTags||"")+" | recent titles: "+((r.meta||{}).niche||""),goal:"Maximise reach and click-through"});
      out.innerHTML="";renderViral(out,res.result||{},res.current||{},sel.value)}catch(e){out.innerHTML="";out.appendChild(h("p","bz-err",e.message))}b.disabled=false};
  add(card,b,out);main.appendChild(card);
}
function generic(main,r,id){
  const k=(r.kpis||[]).filter(x=>x.page===id);if(k.length)main.appendChild(kpiRow(k));
  const cs=(r.charts||[]).filter(c=>c.page===id);
  if(cs.length){const g=grid(main);cs.forEach((c,i)=>chartCard(g,c,(cs.length%2===1&&i===0)||c.labels.length>16?"span2":""))}
  itemsPage(main,r,id);
  for(const t of(r.tables||[]).filter(t=>t.page===id))main.appendChild(tableCard(t));
  if(id==="tags")viralCard(main,r);
  checksCard(main,r,id);
}

export function renderReport(box,r,hist){
  destroyCharts();box.innerHTML="";box.hidden=false;
  const pages=(r.pages&&r.pages.length)?r.pages:[{id:"overview",title:"Overview"},{id:"fixes",title:"Fixes & plan"}];
  const shell=h("div","rp"),side=h("aside","rp-side"),main=h("div","rp-main");
  const brand=h("div","rp-brand");add(brand,h("b",null,r.target),h("small",null,r.kind.replace("_"," ")));side.appendChild(brand);
  const btns=[];
  const showPage=id=>{
    destroyCharts();main.innerHTML="";btns.forEach(b=>b.classList.toggle("on",b.dataset.p===id));
    try{sessionStorage.setItem("bz-page-"+r.kind,id)}catch(e){}
    if(id==="overview")overview(main,r,hist);else if(id==="fixes")fixes(main,r);else generic(main,r,id);
  };
  for(const p of pages){const b=h("button","rp-nav");b.dataset.p=p.id;b.appendChild(icon(PAGE_ICON[p.id]||"grid",18));b.appendChild(h("span",null,p.title));b.onclick=()=>showPage(p.id);side.appendChild(b);btns.push(b)}
  const pr=h("button","bz-btn ghost rp-print","Print / save PDF");pr.onclick=()=>window.print();side.appendChild(pr);
  add(shell,side,main);box.appendChild(shell);
  let start="overview";try{const s=sessionStorage.getItem("bz-page-"+r.kind);if(s&&pages.some(p=>p.id===s))start=s}catch(e){}
  showPage(start);
}
