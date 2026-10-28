import{initializeApp}from"https://www.gstatic.com/firebasejs/10.14.1/firebase-app.js";
import{getAuth,onAuthStateChanged,signOut}from"https://www.gstatic.com/firebasejs/10.14.1/firebase-auth.js";
const auth=getAuth(initializeApp(window.FB));
const $=i=>document.getElementById(i);
const h=(t,c,x)=>{const e=document.createElement(t);if(c)e.className=c;if(x!=null)e.textContent=x;return e};
const add=(p,...k)=>{for(const x of k)if(x)p.appendChild(x);return p};
const num=i=>parseFloat($(i).value)||0;
const PAL=["#f26a2e","#3b82f6","#1faa59","#a855f7","#e8a317","#e5484d"];
const STEPS={website:["Fetching your page…","Checking SEO tags…","Testing robots, sitemap and AI crawlers…","Running Google speed test (can take 30s)…","Asking AI what it recommends…","Writing your report…"],social:["Calculating engagement…","Comparing with benchmarks…","Building your projection…","Writing your growth plan…"],app:["Looking up your app…","Checking store listing…","Scoring reputation…","Writing your report…"]};
let tok,kind="website",charts=[],history=[],timer;

async function api(m,path,b){
  tok=await auth.currentUser.getIdToken();
  const r=await fetch(path,{method:m,headers:{Authorization:"Bearer "+tok,"Content-Type":"application/json"},body:b?JSON.stringify(b):undefined});
  if(!r.ok)throw new Error(await r.text());
  return r.json();
}

document.querySelectorAll(".tab").forEach(b=>b.onclick=()=>{
  kind=b.dataset.kind;
  document.querySelectorAll(".tab").forEach(x=>x.classList.toggle("on",x===b));
  document.querySelectorAll("#formbox>.form").forEach(f=>f.hidden=f.id!=="f-"+kind);
  $("msg").textContent="";
});
$("a-store").onchange=()=>{const a=$("a-store").value==="android";$("a-manual").hidden=!a;$("a-ios").hidden=a};

function live(){
  const f=num("s-followers"),box=$("live");
  if(!f){box.textContent="Live KPIs appear here as you type.";return}
  const BM={instagram:1,tiktok:2,youtube:.8,x:.25,linkedin:.8,facebook:.25},FQ={instagram:3,tiktok:5,youtube:1,x:10,linkedin:3,facebook:3};
  const pl=$("s-platform").value,m=BM[pl]||1,base=f<1e4?[4,2]:f<1e5?[2.5,1]:[1.5,.5];
  const good=base[0]*m,ok=base[1]*m,er=(num("s-likes")+num("s-comments")+num("s-shares"))/f*100,pw=num("s-posts")/4.3,prev=num("s-prev");
  const verdict=er>=good?"good":er>=ok?"okay":"low";
  let t="Engagement "+er.toFixed(2)+"% ("+verdict+", good \u2265 "+good.toFixed(1)+"%) \u00b7 "+pw.toFixed(1)+" posts/week (aim "+FQ[pl]+"+)";
  if(prev)t+=" \u00b7 30-day growth "+((f-prev)/prev*100).toFixed(1)+"%";
  if(num("s-views"))t+=" \u00b7 reach "+(num("s-views")/f*100).toFixed(0)+"%";
  box.textContent=t;
}
document.querySelectorAll("#f-social input,#f-social select").forEach(e=>e.addEventListener("input",live));
live();

function collect(){
  const ai=$("ai").checked,m=$("msg");m.textContent="";
  if(kind==="website"){const u=$("w-url").value.trim();if(!u){m.textContent="Enter a website address";return}return{kind,url:u,useAI:ai}}
  if(kind==="social"){
    if(!$("s-handle").value.trim()||!num("s-followers")){m.textContent="Enter the handle and follower count";return}
    return{kind,platform:$("s-platform").value,handle:$("s-handle").value.trim(),followers:num("s-followers"),prevFollowers:num("s-prev"),following:num("s-following"),posts30:num("s-posts"),likes:num("s-likes"),comments:num("s-comments"),shares:num("s-shares"),views:num("s-views"),bio:$("s-bio").value,captions:$("s-captions").value,goal:$("s-goal").value,useAI:ai};
  }
  const st=$("a-store").value;
  if(st==="ios"&&!$("a-query").value.trim()){m.textContent="Paste the App Store link or the app name";return}
  return{kind,store:st,query:$("a-query").value.trim(),title:$("a-title").value,rating:num("a-rating"),ratings:num("a-ratings"),daysSince:num("a-days"),descLen:num("a-desc"),shots:num("a-shots"),useAI:ai};
}

function busy(on){
  $("run").disabled=on;clearInterval(timer);
  if(!on){$("busy").textContent="";return}
  let i=0;const s=STEPS[kind];$("busy").textContent=s[0];
  timer=setInterval(()=>{i=Math.min(i+1,s.length-1);$("busy").textContent=s[i]},4000);
}

$("run").onclick=async()=>{
  const b=collect();if(!b)return;
  busy(true);
  try{const r=await api("POST","/api/evaluate",b);history.unshift(r);drawHistory();render(r);$("result").scrollIntoView({behavior:"smooth"})}
  catch(e){$("msg").textContent=e.message}
  busy(false);
};

function addChart(p,title,cfg){
  const w=h("div","card");add(w,h("h3",null,title));
  const cv=document.createElement("canvas");w.appendChild(cv);p.appendChild(w);
  cfg.options=Object.assign({responsive:true},cfg.options||{});
  charts.push(new Chart(cv,cfg));
}
const colorOf=s=>s>=75?"var(--ok)":s>=50?"var(--warn)":"var(--bad)";
const COL={pass:"var(--ok)",warn:"var(--warn)",fail:"var(--bad)"};

function list(title,items){
  const d=h("div");add(d,h("h3",null,title));
  const ul=h("ul");for(const x of items||[])ul.appendChild(h("li",null,x));
  d.appendChild(ul);return d;
}

function render(r){
  charts.forEach(c=>c.destroy());charts=[];
  Chart.defaults.color=getComputedStyle(document.body).color;
  const box=$("result");box.innerHTML="";box.hidden=false;

  const top=h("div","rtop"),ring=h("div","ring");
  ring.style.setProperty("--p",r.score);ring.style.setProperty("--c",colorOf(r.score));
  add(ring,h("b",null,r.score),h("small",null,"/100"));
  const t=h("div");add(t,h("h2",null,r.target),h("p","mut",r.kind+" evaluation \u00b7 "+new Date(r.createdAt).toLocaleString()));
  const pr=h("button","btn ghost","Print / save PDF");pr.onclick=()=>window.print();
  add(top,ring,t,pr);box.appendChild(top);

  const kg=h("div","kpis");
  for(const k of r.kpis||[]){const c=h("div","kpi");c.style.borderTopColor=COL[k.status]||"var(--line)";add(c,h("small",null,k.label),h("b",null,k.value),h("span","mut",k.note||""));kg.appendChild(c)}
  box.appendChild(kg);

  if(r.ai){
    const a=h("div","card");add(a,h("h3",null,"AI insights"),h("p",null,r.ai.summary));
    const cols=h("div","cols");add(cols,list("Strengths",r.ai.strengths),list("Improve first",r.ai.improvements),list("30-day plan",r.ai.plan),list("Ideas",r.ai.ideas));
    a.appendChild(cols);box.appendChild(a);
  }

  const cg=h("div","charts");box.appendChild(cg);
  const cats=Object.keys(r.categories||{});
  addChart(cg,"Score by category",{type:cats.length>2?"radar":"bar",data:{labels:cats,datasets:[{label:"Score",data:cats.map(k=>r.categories[k]),backgroundColor:"rgba(242,106,46,.25)",borderColor:"#f26a2e"}]},options:cats.length>2?{scales:{r:{min:0,max:100}}}:{scales:{y:{min:0,max:100}}}});
  for(const c of r.charts||[])addChart(cg,c.title,{type:c.type,data:{labels:c.labels,datasets:c.datasets.map((d,i)=>({label:d.name,data:d.data,backgroundColor:PAL[i%PAL.length]+"66",borderColor:PAL[i%PAL.length]}))}});
  const series=history.filter(x=>x.kind===r.kind&&x.target===r.target).sort((a,b)=>new Date(a.createdAt)-new Date(b.createdAt));
  if(series.length>=2)addChart(cg,"Score trend for this target",{type:"line",data:{labels:series.map(x=>new Date(x.createdAt).toLocaleDateString()),datasets:[{label:"Score",data:series.map(x=>x.score),borderColor:"#f26a2e",backgroundColor:"#f26a2e55"}]},options:{scales:{y:{min:0,max:100}}}});

  const bad=(r.checks||[]).filter(c=>c.status!=="pass").sort((a,b)=>((a.status==="fail"?0:1)-(b.status==="fail"?0:1))||(b.impact-a.impact)).slice(0,8);
  const fx=h("div","card");fx.appendChild(h("h3",null,"Fix these first"));
  if(!bad.length)fx.appendChild(h("p","mut","Nothing urgent \u2013 every check passed."));
  for(const c of bad){const row=h("div","fix"),body=h("div");
    add(body,h("b",null,c.name+" \u2014 "+c.value),h("p","mut","Impact "+c.impact+"/5 \u00b7 "+c.category),h("p",null,c.fix));
    add(row,h("span","pill "+c.status,c.status.toUpperCase()),body);fx.appendChild(row)}
  box.appendChild(fx);

  const all=h("div","card"),sel=h("select");
  for(const v of["all","fail","warn","pass"]){const o=h("option",null,v==="all"?"All checks":v);o.value=v;sel.appendChild(o)}
  add(all,h("h3",null,"All checks"),sel);
  const tb=h("table");all.appendChild(h("div")).appendChild(tb);
  const wrap=tb.parentElement;wrap.style.overflowX="auto";
  const draw=()=>{tb.innerHTML="";
    for(const c of (r.checks||[]).filter(c=>sel.value==="all"||c.status===sel.value)){
      const tr=tb.insertRow();
      add(tr.insertCell(),h("span","pill "+c.status,c.status));
      tr.insertCell().textContent=c.category;tr.insertCell().textContent=c.name;tr.insertCell().textContent=c.value;
    }};
  sel.onchange=draw;draw();box.appendChild(all);

  if((r.notes||[]).length){const n=h("div","card");add(n,list("Notes",r.notes));box.appendChild(n)}
}

function drawHistory(){
  const box=$("hist");box.innerHTML="";
  if(!history.length){box.appendChild(h("p","mut","No evaluations yet."));return}
  for(const r of history){
    const row=h("div","hrow");
    add(row,h("span","pill "+(r.score>=75?"pass":r.score>=50?"warn":"fail"),String(r.score)),h("b",null,r.target),h("span","mut",r.kind+" \u00b7 "+new Date(r.createdAt).toLocaleDateString()));
    row.onclick=()=>{render(r);$("result").scrollIntoView({behavior:"smooth"})};
    box.appendChild(row);
  }
}

onAuthStateChanged(auth,async u=>{
  if(!u){location.href="/";return}
  $("name").textContent=(u.displayName||"").split(" ")[0];
  try{history=await api("GET","/api/reports")}catch(e){$("msg").textContent=e.message}
  drawHistory();
});
$("out").onclick=()=>signOut(auth);
