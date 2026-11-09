import{h,add}from"./ui.js";
const NAMES={adsperf:"Paid ads performance (Meta)",website:"Website and SEO audit",google:"Search, traffic and YouTube analytics",youtube:"YouTube channel",social_connected:"Social media accounts",competitors:"Competitor analysis",keywords:"Keyword research",ads:"Ads plan",app:"App store listing",social:"Social account (manual)"};
const tone=s=>s>=75?"#16a34a":s>=50?"#f59e0b":"#ef4444";
export function renderMarketing(box,hist,company,by){
  box.innerHTML="";box.hidden=false;
  const latest={};for(const r of hist||[])if(!latest[r.kind])latest[r.kind]=r;
  const reps=Object.values(latest);
  const wrap=h("div","rp-main");wrap.style.marginTop="18px";box.appendChild(wrap);
  const head=h("div","rp-card");
  add(head,h("h2",null,"Marketing performance report"),h("p","bz-mut",(company||"Your company")+" \u00b7 "+new Date().toLocaleDateString(undefined,{year:"numeric",month:"long",day:"numeric"})+(by?" \u00b7 Prepared by "+by:"")));
  wrap.appendChild(head);
  if(!reps.length){wrap.appendChild(h("div","rp-card","Run a few evaluations first (website, Google, YouTube, social, SEO or ads). This report assembles your latest results automatically."));return}
  const avg=Math.round(reps.reduce((a,r)=>a+r.score,0)/reps.length);
  const bad=[];for(const r of reps)for(const c of r.checks||[])if(c.status==="fail")bad.push({r,c});
  const sum=h("div","rp-card");sum.appendChild(h("h3",null,"Executive summary"));
  sum.appendChild(h("p",null,"Overall score "+avg+"/100 across "+reps.length+" area"+(reps.length===1?"":"s")+". "+bad.length+" critical issue"+(bad.length===1?"":"s")+" found. The highest-impact actions are listed below, each with ready-to-use content inside Blitz AI."));
  const bars=h("div","bz-bars");for(const r of reps){const row=h("div"),l=h("div","bz-bar-l");add(l,h("span",null,NAMES[r.kind]||r.kind),h("b",null,r.score+"%"));const bar=h("div","bz-bar"),i=h("i");i.style.width=r.score+"%";i.style.background=tone(r.score);bar.appendChild(i);add(row,l,bar);bars.appendChild(row)}
  sum.appendChild(bars);wrap.appendChild(sum);
  const acts=[];for(const r of reps){for(const p of r.plan||[])acts.push({t:p.title,w:p.why,s:(p.impact||1)/Math.max(1,p.effort||1),k:r.kind});for(const c of r.checks||[])if(c.status!=="pass")acts.push({t:c.name+": "+c.fix,w:"Impact "+c.impact+"/5",s:c.impact/3,k:r.kind})}
  acts.sort((a,b)=>b.s-a.s);
  const top=h("div","rp-card");top.appendChild(h("h3",null,"Priority actions for next month"));
  acts.slice(0,10).forEach((a,i)=>{const row=h("div","bz-step-row");add(row,h("div",null,(i+1)+"."),h("div",null,a.t));top.appendChild(row)});wrap.appendChild(top);
  for(const r of reps){
    const c=h("div","rp-card");c.appendChild(h("h3",null,(NAMES[r.kind]||r.kind)+": "+r.target));
    const k=h("div","rp-kpis");for(const x of(r.kpis||[]).slice(0,4)){const d=h("div","rp-kpi"),b=h("div","rp-kb");add(b,h("small",null,x.label),h("b",null,x.value),h("span","bz-mut rp-note",x.note||""));d.appendChild(b);k.appendChild(d)}c.appendChild(k);
    const fx=(r.checks||[]).filter(x=>x.status!=="pass").sort((a,b)=>b.impact-a.impact).slice(0,3);
    if(fx.length){c.appendChild(h("h4",null,"Top issues"));for(const x of fx)c.appendChild(h("p",null,x.name+" \u2014 "+x.value+". "+x.fix))}
    wrap.appendChild(c);
  }
  const pr=h("button","bz-btn","Print / save PDF");pr.onclick=()=>window.print();wrap.appendChild(pr);
}
