import{h,add}from"./ui.js";
import{fixFromCheck,fixFromPlan,fixFromText,mount}from"./actions.js";

const PAL=["#22d3ee","#8b5cf6","#34d399","#fbbf24","#f87171","#3b82f6"];
let charts=[];
export const destroyCharts=()=>{charts.forEach(c=>c.destroy());charts=[]};
const css=v=>getComputedStyle(document.body).getPropertyValue(v).trim();
const tone=s=>s>=75?"#34d399":s>=50?"#fbbf24":"#f87171";
const grad=(color,a,b)=>ctx=>{const c=ctx.chart,ar=c.chartArea;if(!ar)return color;const g=c.ctx.createLinearGradient(0,ar.top,0,ar.bottom);g.addColorStop(0,color+a);g.addColorStop(1,color+b);return g};

function mkChart(parent,span,title,type,labels,datasets){
  const card=h("div","bz-card "+span);card.appendChild(h("h3",null,title));
  const wrap=h("div","bz-chart"),cv=document.createElement("canvas");wrap.appendChild(cv);card.appendChild(wrap);parent.appendChild(card);
  const mut=css("--mut"),gridc="rgba(148,163,184,.14)";
  const ds=datasets.map((d,i)=>{const c=PAL[i%PAL.length],dashed=/Trend|Recommended|forecast/i.test(d.name);
    if(type==="line")return{label:d.name,data:d.data,borderColor:c,backgroundColor:dashed?"transparent":grad(c,"88","00"),fill:!dashed,tension:.35,pointRadius:d.data.length>14?0:3,borderWidth:2.5,borderDash:dashed?[6,6]:[]};
    return{label:d.name,data:d.data,backgroundColor:grad(c,"ff","55"),borderRadius:8,maxBarThickness:30};});
  charts.push(new Chart(cv,{type,data:{labels,datasets:ds},options:{responsive:true,maintainAspectRatio:false,interaction:{mode:"index",intersect:false},plugins:{legend:{display:ds.length>1,labels:{color:mut,usePointStyle:true,boxWidth:8}},tooltip:{backgroundColor:"#0b1437",borderColor:"#22d3ee55",borderWidth:1,padding:10}},scales:{x:{grid:{display:false},ticks:{color:mut,maxTicksLimit:9},border:{display:false}},y:{beginAtZero:true,grid:{color:gridc},ticks:{color:mut},border:{display:false}}}}}));
}

function gauge(score){
  const R=54,C=2*Math.PI*R,col=tone(score),w=h("div","bz-gauge");
  w.innerHTML='<svg viewBox="0 0 140 140" width="170" height="170"><defs><linearGradient id="gg" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="'+col+'"/><stop offset="1" stop-color="#8b5cf6"/></linearGradient></defs><circle cx="70" cy="70" r="'+R+'" fill="none" stroke="rgba(148,163,184,.18)" stroke-width="12"/><circle cx="70" cy="70" r="'+R+'" fill="none" stroke="url(#gg)" stroke-width="12" stroke-linecap="round" stroke-dasharray="'+(C*score/100)+" "+C+'" transform="rotate(-90 70 70)"/></svg>';
  const mid=h("div");add(mid,h("b",null,String(score)),h("small",null,"out of 100"));w.appendChild(mid);return w;
}

function kpi(k){
  const d=h("div","bz-kpi");d.dataset.s=k.status||"";add(d,h("small",null,k.label),h("b",null,k.value));
  const m=/([+-]\d+%)/.exec(k.note||"");
  if(m){const up=m[1][0]==="+";d.appendChild(h("span","bz-chip "+(up?"up":"down"),(up?"\u25B2 ":"\u25BC ")+m[1].slice(1)));d.appendChild(h("span","bz-mut",(k.note||"").replace(m[1],"").trim()))}
  else d.appendChild(h("span","bz-mut",k.note||""));
  return d;
}

function catBars(parent,span,cats){
  const card=h("div","bz-card "+span);card.appendChild(h("h3",null,"Score by area"));
  const box=h("div","bz-bars");
  for(const[k,v]of Object.entries(cats||{}).sort((a,b)=>a[1]-b[1])){
    const row=h("div"),l=h("div","bz-bar-l");add(l,h("span",null,k),h("b",null,v+"%"));
    const bar=h("div","bz-bar"),i=h("i");i.style.width=v+"%";i.style.background="linear-gradient(90deg,"+tone(v)+",#8b5cf6)";bar.appendChild(i);add(row,l,bar);box.appendChild(row);}
  card.appendChild(box);parent.appendChild(card);
}

function donut(parent,span,checks){
  const c={pass:0,warn:0,fail:0};for(const x of checks)c[x.status]=(c[x.status]||0)+1;
  const card=h("div","bz-card "+span);card.appendChild(h("h3",null,"Check status"));
  const w=h("div","bz-donut"),cv=document.createElement("canvas");w.appendChild(cv);
  const mid=h("div");add(mid,h("b",null,String(checks.length)),h("small","bz-mut","checks"));w.appendChild(mid);card.appendChild(w);
  const lg=h("div","bz-legend");
  for(const[k,col]of[["pass","#34d399"],["warn","#fbbf24"],["fail","#f87171"]]){const s=h("span",null,c[k]+" "+k),i=h("i");i.style.background=col;s.prepend(i);lg.appendChild(s)}
  card.appendChild(lg);parent.appendChild(card);
  charts.push(new Chart(cv,{type:"doughnut",data:{labels:["Pass","Warn","Fail"],datasets:[{data:[c.pass,c.warn,c.fail],backgroundColor:["#34d399","#fbbf24","#f87171"],borderWidth:0,spacing:3,borderRadius:6}]},options:{responsive:true,maintainAspectRatio:false,cutout:"76%",plugins:{legend:{display:false}}}}));
}

function sec(box,title,sub){const s=h("div","bz-sec");s.appendChild(h("h2",null,title));if(sub)s.appendChild(h("span",null,sub));box.appendChild(s)}

function fixCard(parent,o,builder,r,item){
  const card=h("div","bz-fix"),hd=h("div","bz-fix-h"),left=h("div"),line=h("div");
  add(line,h("span","bz-pill "+o.cls,o.pill),h("b",null,o.title));left.appendChild(line);
  if(o.sub)left.appendChild(h("p",null,o.sub));
  if(o.body)left.appendChild(h("p",null,o.body));
  const btn=h("button","bz-btn","Open fix");add(hd,left,btn);card.appendChild(hd);
  const ws=h("div","bz-ws");ws.hidden=true;card.appendChild(ws);mount(btn,ws,builder,r,item);parent.appendChild(card);
}

function table(cols,rows){
  const w=h("div"),t=h("table","bz-table");w.style.overflowX="auto";
  const hd=t.createTHead().insertRow();for(const c of cols)hd.appendChild(h("th",null,c));
  const tb=t.createTBody();for(const r of rows){const tr=tb.insertRow();for(const c of r)tr.insertCell().textContent=c}
  w.appendChild(t);return w;
}

function monthPlan(box,r){
  const checks=(r.checks||[]).filter(c=>c.status!=="pass").sort((a,b)=>((a.status==="fail"?0:1)-(b.status==="fail"?0:1))||(b.impact-a.impact));
  let steps=[];
  if(r.ai&&(r.ai.plan||[]).length)steps=r.ai.plan.map(t=>({text:t}));
  else{for(const p of r.plan||[])steps.push({text:p.title,plan:p});for(const c of checks.slice(0,4))steps.push({text:c.fix,check:c})}
  const seen=new Set();steps=steps.filter(s=>{const k=s.text.toLowerCase();if(!s.text||seen.has(k))return false;seen.add(k);return true}).slice(0,8);
  if(!steps.length)return;
  sec(box,"30-day plan","Tick steps as you finish them. Each step opens the exact content to use.");
  const per=Math.ceil(steps.length/4);
  steps.forEach((s,i)=>{
    const wk=Math.floor(i/per)+1;
    if(i%per===0){const w=h("div","bz-week");w.appendChild(h("h4",null,"Week "+wk+" \u00b7 days "+((wk-1)*7+1)+"\u2013"+(wk*7)));w.id="wk"+wk;box.appendChild(w)}
    const host=box.querySelector("#wk"+wk),row=h("div","bz-step-row"),chk=h("input");chk.type="checkbox";
    const key="bz:"+(r.id||r.createdAt)+":"+i;try{chk.checked=localStorage.getItem(key)==="1"}catch(e){}
    chk.onchange=()=>{row.classList.toggle("done",chk.checked);try{localStorage.setItem(key,chk.checked?"1":"0")}catch(e){}};row.classList.toggle("done",chk.checked);
    const body=h("div"),t=h("div","t",s.text),btn=h("button","bz-btn ghost","Open fix");btn.style.marginTop="8px";
    const ws=h("div","bz-ws");ws.hidden=true;add(body,t,btn,ws);
    const builder=s.plan?fixFromPlan(s.plan,r):s.check?fixFromCheck(s.check,r):fixFromText(s.text,r);
    mount(btn,ws,builder,r,{text:s.text});add(row,chk,body);host.appendChild(row);
  });
}

export function renderReport(box,r,hist){
  destroyCharts();box.innerHTML="";box.hidden=false;
  const checks=r.checks||[],fails=checks.filter(c=>c.status==="fail").length,warns=checks.filter(c=>c.status==="warn").length;
  const grid=h("div","bz-grid");box.appendChild(grid);

  const hero=h("div","bz-card s5"),hh=h("div","bz-hero");
  const txt=h("div");txt.appendChild(h("h2",null,r.target));
  txt.appendChild(h("p",null,r.kind+" report \u00b7 "+new Date(r.createdAt).toLocaleString()));
  txt.appendChild(h("p",null,(r.ai&&r.ai.summary)||((r.score>=75?"Strong":r.score>=50?"Solid but improvable":"Needs attention")+": "+fails+" critical issue"+(fails===1?"":"s")+" and "+warns+" improvement"+(warns===1?"":"s")+" found. Open each fix below for the exact text or steps to use.")));
  const bd=h("div","bz-badges");for(const[k,v]of Object.entries(r.categories||{}))bd.appendChild(h("span","bz-badge",k+" "+v+"%"));
  const pr=h("button","bz-btn ghost","Print / save PDF");pr.onclick=()=>window.print();pr.style.marginTop="10px";
  add(txt,bd,pr);add(hh,gauge(r.score),txt);hero.appendChild(hh);grid.appendChild(hero);

  const kc=h("div","bz-card s7");kc.appendChild(h("h3",null,"At a glance"));
  const kg=h("div","bz-kpis");for(const k of r.kpis||[])kg.appendChild(kpi(k));kc.appendChild(kg);grid.appendChild(kc);

  catBars(grid,"s4",r.categories);
  const cs=r.charts||[];
  if(cs[0])mkChart(grid,"s8",cs[0].title,cs[0].type,cs[0].labels,cs[0].datasets);
  donut(grid,"s4",checks);
  for(const c of cs.slice(1))mkChart(grid,"s6",c.title,c.type,c.labels,c.datasets);
  const series=(hist||[]).filter(x=>x.kind===r.kind&&x.target===r.target).sort((a,b)=>new Date(a.createdAt)-new Date(b.createdAt));
  if(series.length>=2)mkChart(grid,"s6","Score trend for this target","line",series.map(x=>new Date(x.createdAt).toLocaleDateString()),[{name:"Score",data:series.map(x=>x.score)}]);

  const bad=checks.filter(c=>c.status!=="pass").sort((a,b)=>((a.status==="fail"?0:1)-(b.status==="fail"?0:1))||(b.impact-a.impact)).slice(0,8);
  sec(box,"Fix these first","Open a fix to get the exact text, code or steps, with a copy button.");
  if(!bad.length)box.appendChild(h("div","bz-card","Nothing urgent. Every check passed."));
  for(const c of bad)fixCard(box,{cls:c.status,pill:c.status==="fail"?"CRITICAL":"IMPROVE",title:c.name+" \u2014 "+c.value,sub:"Impact "+c.impact+"/5 \u00b7 "+c.category,body:"What to do: "+c.fix},fixFromCheck(c,r),r,{text:c.fix});

  const plan=[...(r.plan||[])].sort((a,b)=>(b.impact/Math.max(1,b.effort))-(a.impact/Math.max(1,a.effort))||b.impact-a.impact);
  if(plan.length){sec(box,"Growth plan","Highest payoff for the effort first.");
    for(const p of plan)fixCard(box,{cls:"plan",pill:"PLAN",title:p.title,sub:"Impact "+p.impact+"/5 \u00b7 Effort "+p.effort+"/5",body:p.why},fixFromPlan(p,r),r,{text:p.title})}

  monthPlan(box,r);

  if((r.tables||[]).length){sec(box,"Data tables");const g=h("div","bz-grid");box.appendChild(g);
    for(const t of r.tables){const c=h("div","bz-card s12");add(c,h("h3",null,t.title),table(t.columns,(t.rows||[]).map(x=>x.cells)));g.appendChild(c)}}

  sec(box,"All checks");
  const all=h("div","bz-card"),fl=h("div","bz-filter"),holder=h("div");
  const draw=f=>{holder.innerHTML="";holder.appendChild(table(["Status","Area","Check","Result"],checks.filter(c=>f==="all"||c.status===f).map(c=>[c.status.toUpperCase(),c.category,c.name,c.value])));
    fl.querySelectorAll("button").forEach(b=>b.classList.toggle("on",b.dataset.f===f))};
  for(const f of["all","fail","warn","pass"]){const b=h("button","bz-mini",f==="all"?"All":f);b.dataset.f=f;b.onclick=()=>draw(f);fl.appendChild(b)}
  add(all,fl,holder);box.appendChild(all);draw("all");
  if((r.notes||[]).length){const n=h("div","bz-card");n.style.marginTop="16px";n.appendChild(h("h3",null,"Notes"));for(const x of r.notes)n.appendChild(h("p","bz-mut",x));box.appendChild(n)}
}
