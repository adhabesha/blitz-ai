import{initializeApp}from"https://www.gstatic.com/firebasejs/10.14.1/firebase-app.js";
import{getAuth,onAuthStateChanged,signOut}from"https://www.gstatic.com/firebasejs/10.14.1/firebase-auth.js";
const auth=getAuth(initializeApp(window.FB));
const $=i=>document.getElementById(i);
const h=(t,c,x)=>{const e=document.createElement(t);if(c)e.className=c;if(x!=null)e.textContent=x;return e};
const add=(p,...k)=>{for(const x of k)if(x)p.appendChild(x);return p};
const num=i=>parseFloat($(i).value)||0;
const PAL=["#f26a2e","#3b82f6","#1faa59","#a855f7","#e8a317","#e5484d"];
const STEPS={website:["Fetching your page…","Checking SEO tags…","Testing robots, sitemap and AI crawlers…","Running Google speed test (can take 30s)…","Asking AI what it recommends…","Writing your report…"],youtube:["Finding the channel…","Reading your latest videos…","Calculating KPIs…","Building projections and plan…"],google:["Reading Search Console…","Reading Google Analytics…","Reading YouTube Analytics…","Forecasting trends…","Writing your report…"],social:["Calculating engagement…","Comparing with benchmarks…","Building your projection…"],app:["Looking up your app…","Checking store listing…","Scoring reputation…"]};
let tok,kind="website",charts=[],history=[],timer,gAssets=null;

async function api(m,path,b){
  tok=await auth.currentUser.getIdToken();
  const r=await fetch(path,{method:m,headers:{Authorization:"Bearer "+tok,"Content-Type":"application/json"},body:b?JSON.stringify(b):undefined});
  if(!r.ok)throw new Error(await r.text());
  return r.json();
}

function setKind(k){
  kind=k;
  document.querySelectorAll(".tab").forEach(x=>x.classList.toggle("on",x.dataset.kind===k));
  document.querySelectorAll("#formbox>.form").forEach(f=>f.hidden=f.id!=="f-"+k);
  $("runbox").hidden=(k==="optimizer");
  $("msg").textContent="";
  if(k==="google")loadGoogle();
}
document.querySelectorAll(".tab").forEach(b=>b.onclick=()=>setKind(b.dataset.kind));
$("a-store").onchange=()=>{const a=$("a-store").value==="android";$("a-manual").hidden=!a;$("a-ios").hidden=a};

function live(){
  const f=num("s-followers"),box=$("live");
  if(!f){box.textContent="Live KPIs appear here as you type.";return}
  const BM={instagram:1,tiktok:2,youtube:.8,x:.25,linkedin:.8,facebook:.25},FQ={instagram:3,tiktok:5,youtube:1,x:10,linkedin:3,facebook:3};
  const pl=$("s-platform").value,m=BM[pl]||1,base=f<1e4?[4,2]:f<1e5?[2.5,1]:[1.5,.5];
  const good=base[0]*m,ok=base[1]*m,er=(num("s-likes")+num("s-comments")+num("s-shares"))/f*100,pw=num("s-posts")/4.3,prev=num("s-prev");
  let t="Engagement "+er.toFixed(2)+"% ("+(er>=good?"good":er>=ok?"okay":"low")+", good \u2265 "+good.toFixed(1)+"%) \u00b7 "+pw.toFixed(1)+" posts/week (aim "+FQ[pl]+"+)";
  if(prev)t+=" \u00b7 30-day growth "+((f-prev)/prev*100).toFixed(1)+"%";
  if(num("s-views"))t+=" \u00b7 reach "+(num("s-views")/f*100).toFixed(0)+"%";
  box.textContent=t;
}
document.querySelectorAll("#f-social input,#f-social select").forEach(e=>e.addEventListener("input",live));
live();

async function loadGoogle(){
  try{
    const st=await api("GET","/api/google/status");
    $("g-setup").hidden=st.configured;$("g-connect").hidden=st.connected||!st.configured;$("g-connected").hidden=!st.connected;
    if(!st.connected)return;
    const a=await api("GET","/api/google/assets");gAssets=a;
    const fill=(id,items,val,lab)=>{const s=$(id);s.length=1;for(const x of items){const o=h("option",null,lab(x));o.value=val(x);s.appendChild(o)}};
    fill("g-site",a.sites||[],x=>x,x=>x);fill("g-prop",a.properties||[],x=>x.id,x=>x.name+" ("+x.id+")");
    $("g-chname").textContent=a.channel||"(no channel found)";
    $("g-errs").textContent=(a.errors||[]).length?"Some sources unavailable: "+a.errors.join(" | "):"";
  }catch(e){$("msg").textContent=e.message}
}
$("g-btn").onclick=async()=>{try{const r=await api("POST","/api/google/start");location.href=r.url}catch(e){$("msg").textContent=e.message}};
$("g-off").onclick=async()=>{if(!confirm("Disconnect Google?"))return;await api("POST","/api/google/disconnect");loadGoogle()};

function collect(){
  const ai=$("ai").checked,m=$("msg");m.textContent="";
  if(kind==="website"){const u=$("w-url").value.trim();if(!u){m.textContent="Enter a website address";return}return{kind,url:u,useAI:ai}}
  if(kind==="youtube"){const u=$("y-url").value.trim();if(!u){m.textContent="Paste a channel link or @handle";return}return{kind,query:u,useAI:ai}}
  if(kind==="google"){if(!$("g-site").value&&!$("g-prop").value&&!$("g-chan").checked){m.textContent="Choose a site, a GA4 property or your channel";return}return{kind,site:$("g-site").value,property:$("g-prop").value,channel:$("g-chan").checked,useAI:ai}}
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
  let i=0;const s=STEPS[kind]||["Working…"];$("busy").textContent=s[0];
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
function list(title,items){const d=h("div");add(d,h("h3",null,title));const ul=h("ul");for(const x of items||[])ul.appendChild(h("li",null,x));d.appendChild(ul);return d}
function table(cols,rows){const tb=h("table");const hd=tb.createTHead().insertRow();for(const c of cols)hd.appendChild(h("th",null,c));for(const r of rows){const tr=tb.insertRow();for(const c of r)tr.insertCell().textContent=c}const w=h("div");w.style.overflowX="auto";w.appendChild(tb);return w}

function render(r){
  charts.forEach(c=>c.destroy());charts=[];
  Chart.defaults.color=getComputedStyle(document.body).color;
  const box=$("result");box.innerHTML="";box.hidden=false;

  const top=h("div","rtop"),ring=h("div","ring");
  ring.style.setProperty("--p",r.score);ring.style.setProperty("--c",colorOf(r.score));
  add(ring,h("b",null,r.score),h("small",null,"/100"));
  const t=h("div");add(t,h("h2",null,r.target),h("p","mut",r.kind+" report \u00b7 "+new Date(r.createdAt).toLocaleString()));
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
  for(const c of r.charts||[])addChart(cg,c.title,{type:c.type,data:{labels:c.labels,datasets:c.datasets.map((d,i)=>({label:d.name,data:d.data,backgroundColor:PAL[i%PAL.length]+"66",borderColor:PAL[i%PAL.length],tension:.25}))}});
  const series=history.filter(x=>x.kind===r.kind&&x.target===r.target).sort((a,b)=>new Date(a.createdAt)-new Date(b.createdAt));
  if(series.length>=2)addChart(cg,"Score trend for this target",{type:"line",data:{labels:series.map(x=>new Date(x.createdAt).toLocaleDateString()),datasets:[{label:"Score",data:series.map(x=>x.score),borderColor:"#f26a2e",backgroundColor:"#f26a2e55"}]},options:{scales:{y:{min:0,max:100}}}});

  // Action plan with automation buttons
  if((r.plan||[]).length){
    const c=h("div","card");c.appendChild(h("h3",null,"Action plan \u2013 highest payoff first"));
    const plan=[...r.plan].sort((a,b)=>(b.impact/Math.max(1,b.effort))-(a.impact/Math.max(1,a.effort))||b.impact-a.impact);
    const tb=h("table");const hd=tb.createTHead().insertRow();for(const x of["Action","Why","Impact","Effort",""])hd.appendChild(h("th",null,x));
    plan.forEach((p,i)=>{const tr=tb.insertRow();tr.insertCell().textContent=(i+1)+". "+p.title;tr.insertCell().textContent=p.why;tr.insertCell().textContent=p.impact+"/5";tr.insertCell().textContent=p.effort+"/5";
      const cell=tr.insertCell();if(p.auto){const b=h("button","btn","Automate");b.onclick=()=>automate(p,r);cell.appendChild(b)}});
    const w=h("div");w.style.overflowX="auto";w.appendChild(tb);c.appendChild(w);box.appendChild(c);
  }
  for(const tb of r.tables||[]){const c=h("div","card");add(c,h("h3",null,tb.title),table(tb.columns,(tb.rows||[]).map(x=>x.cells)));box.appendChild(c)}

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
  const holder=h("div");holder.style.overflowX="auto";all.appendChild(holder);
  const draw=()=>{holder.innerHTML="";holder.appendChild(table(["Status","Category","Check","Result"],(r.checks||[]).filter(c=>sel.value==="all"||c.status===sel.value).map(c=>[c.status,c.category,c.name,c.value])))};
  sel.onchange=draw;draw();box.appendChild(all);
  if((r.notes||[]).length){const n=h("div","card");add(n,list("Notes",r.notes));box.appendChild(n)}
}

// ---------- Optimizer & publish ----------
function automate(p,r){
  setKind("optimizer");
  if(p.auto.startsWith("optimize-video:")){$("o-platform").value="youtube";$("o-vid").value=p.auto.split(":")[1];$("o-title").value="";$("o-desc").value="";$("o-tags").value="";$("o-topic").value="";}
  else{
    $("o-platform").value="website";$("o-vid").value="";
    const t=(r.tables||[]).find(x=>x.title.startsWith("Striking")||x.title.startsWith("High-ranking"));
    $("o-topic").value=t?(t.rows||[]).slice(0,5).map(x=>x.cells[0]).join(", "):"";
    $("o-title").value="";$("o-desc").value="";$("o-tags").value="";
  }
  $("formbox").scrollIntoView({behavior:"smooth"});
  $("o-go").click();
}
const splitTags=v=>v.split(",").map(x=>x.trim().replace(/^#/,"")).filter(Boolean);

$("o-go").onclick=async()=>{
  const m=$("o-msg");m.textContent="Generating…";$("o-go").disabled=true;
  try{
    const res=await api("POST","/api/optimize",{platform:$("o-platform").value,videoId:$("o-vid").value.trim(),title:$("o-title").value,description:$("o-desc").value,tags:splitTags($("o-tags").value),topic:$("o-topic").value,audience:$("o-aud").value,goal:$("o-goal").value});
    m.textContent="";renderOpt(res);
  }catch(e){m.textContent=e.message}
  $("o-go").disabled=false;
};

function field(label,el){const l=h("label",null,label);l.appendChild(el);return l}
function renderOpt(res){
  const o=res.result,cur=res.current||{},pl=$("o-platform").value,vid=$("o-vid").value.trim();
  const box=$("optres");box.innerHTML="";box.hidden=false;
  const c=h("div","card");c.appendChild(h("h3",null,"Optimized content (edit before using)"));
  const f=h("div","form");
  const ti=h("input");ti.value=(o.titles||[])[0]||cur.title||"";ti.maxLength=150;
  const de=h("textarea");de.rows=10;de.value=o.description||"";
  const tg=h("input");tg.value=(o.tags||[]).join(", ");
  const hs=h("input");hs.value=(o.hashtags||[]).join(" ");
  const tl=field("Title / opener",ti);tl.className="wide";const dl=field("Description / caption",de);dl.className="wide";
  add(f,tl,dl,field("Tags",tg),field("Hashtags",hs));c.appendChild(f);
  const alt=h("div");alt.appendChild(h("b",null,"Other title options (click to use):"));
  for(const t of(o.titles||[]).slice(1)){const b=h("div","hrow",t);b.onclick=()=>{ti.value=t};alt.appendChild(b)}
  c.appendChild(alt);
  const cp=h("button","btn ghost","Copy all");cp.onclick=()=>{navigator.clipboard.writeText(ti.value+"\n\n"+de.value+"\n\n"+hs.value+"\n\nTags: "+tg.value);cp.textContent="Copied"};
  const bar=h("div");bar.style.margin="12px 0";bar.appendChild(cp);
  const st=h("p","mut","");
  if(pl==="youtube"&&vid){
    const ap=h("button","btn","Apply to my YouTube video");ap.style.marginLeft="8px";
    ap.onclick=async()=>{ap.disabled=true;st.textContent="Updating video…";
      try{await api("POST","/api/youtube/apply",{videoId:vid,title:ti.value,description:de.value,tags:splitTags(tg.value)});st.textContent="Done. The video now uses the new title, description and tags."}
      catch(e){st.textContent=e.message}ap.disabled=false};
    bar.appendChild(ap);
  }
  c.appendChild(bar);c.appendChild(st);
  const cols=h("div","cols");add(cols,list("Hooks",o.hooks),list("Visual / thumbnail ideas",o.ideas),list("A/B testing tips",o.notes));c.appendChild(cols);
  box.appendChild(c);
  if(pl==="youtube")box.appendChild(uploadCard(ti,de,tg));
  else{const n=h("div","card");n.appendChild(h("p","mut","Automatic posting for this platform needs the platform's own approval of the app (Meta, TikTok, X). For now, copy the optimized text and publish it yourself, or use YouTube publishing above."));box.appendChild(n)}
  box.scrollIntoView({behavior:"smooth"});
}

function uploadCard(ti,de,tg){
  const c=h("div","card");c.appendChild(h("h3",null,"Publish a new video to YouTube"));
  c.appendChild(h("p","mut","Requires Connect Google. The file goes from your browser straight to YouTube. New API projects may be limited to private uploads until Google audits them; you can switch to public in YouTube Studio."));
  const f=h("div","form");
  const file=h("input");file.type="file";file.accept="video/*";
  const pv=h("select");for(const v of["private","unlisted","public"]){const o=h("option",null,v);o.value=v;pv.appendChild(o)}
  const when=h("input");when.type="datetime-local";
  add(f,field("Video file",file),field("Visibility",pv),field("Schedule (optional, publishes at this time)",when));c.appendChild(f);
  const st=h("p","mut","");const b=h("button","btn","Upload");
  b.onclick=async()=>{
    if(!file.files[0]){st.textContent="Choose a video file first";return}
    b.disabled=true;st.textContent="Starting upload…";
    try{
      const t=(await api("GET","/api/google/token")).accessToken,fl=file.files[0];
      const status={privacyStatus:pv.value,selfDeclaredMadeForKids:false};
      if(when.value){status.privacyStatus="private";status.publishAt=new Date(when.value).toISOString()}
      const init=await fetch("https://www.googleapis.com/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status",{method:"POST",headers:{Authorization:"Bearer "+t,"Content-Type":"application/json; charset=UTF-8","X-Upload-Content-Type":fl.type||"video/*","X-Upload-Content-Length":String(fl.size)},body:JSON.stringify({snippet:{title:ti.value,description:de.value,tags:splitTags(tg.value),categoryId:"22"},status})});
      if(!init.ok)throw new Error("Upload could not start ("+init.status+")");
      const loc=init.headers.get("Location");if(!loc)throw new Error("Upload location missing");
      await new Promise((res,rej)=>{const x=new XMLHttpRequest();x.open("PUT",loc);
        x.upload.onprogress=e=>{if(e.lengthComputable)st.textContent="Uploading… "+Math.round(e.loaded/e.total*100)+"%"};
        x.onload=()=>x.status<300?res():rej(new Error("Upload failed ("+x.status+")"));x.onerror=()=>rej(new Error("The browser blocked the upload. Upload manually in YouTube Studio."));x.send(fl)});
      st.textContent="Uploaded! Open YouTube Studio to check it (processing can take a few minutes).";
    }catch(e){st.textContent=e.message}
    b.disabled=false;
  };
  c.appendChild(b);c.appendChild(st);return c;
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
  const g=new URLSearchParams(location.search).get("google");
  if(g){setKind("google");$("msg").textContent=g==="ok"?"":"Google connection failed. Try again.";history.length;}
});
$("out").onclick=()=>signOut(auth);
