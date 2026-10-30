import{initializeApp}from"https://www.gstatic.com/firebasejs/10.14.1/firebase-app.js";
import{getAuth,onAuthStateChanged,signOut}from"https://www.gstatic.com/firebasejs/10.14.1/firebase-auth.js";
import{$,h,add,icon,setAuth,api,copyBox}from"./ui.js";
import{renderReport,destroyCharts}from"./report.js";

const auth=getAuth(initializeApp(window.FB));setAuth(auth);
const STEPS={website:["Fetching your page…","Checking SEO tags…","Testing robots, sitemap and AI crawlers…","Running Google speed test (can take 30s)…","Asking AI what it recommends…","Writing your report…"],youtube:["Finding the channel…","Reading your latest videos…","Calculating KPIs…","Building projections and plan…"],google:["Reading your Google data…","Forecasting trends…","Writing your report…"],social:["Calculating engagement…","Comparing with benchmarks…","Building your projection…"],app:["Looking up your app…","Checking store listing…","Scoring reputation…"]};
let kind="website",hist=[],timer,current=null,gA=null;

// theme
const setTheme=t=>{document.documentElement.dataset.theme=t;try{localStorage.setItem("bz-theme",t)}catch(e){}document.querySelectorAll(".bz-theme button").forEach(b=>b.classList.toggle("on",b.dataset.t===t));if(current)show(current,false)};
document.querySelectorAll(".bz-theme button").forEach(b=>b.onclick=()=>setTheme(b.dataset.t));
setTheme(document.documentElement.dataset.theme||"dark");
document.querySelectorAll(".bz-nav").forEach(b=>b.prepend(icon(b.dataset.icon,18)));

function setKind(k){
  kind=k;
  document.querySelectorAll(".bz-nav").forEach(x=>x.classList.toggle("on",x.dataset.kind===k));
  document.querySelectorAll("#formbox>.form").forEach(f=>f.hidden=f.id!=="f-"+k);
  $("runbox").hidden=(k==="optimizer"||k==="google");
  $("msg").textContent="";
  if(k==="google")loadGoogle(true).catch(e=>{$("msg").textContent=e.message});
}
document.querySelectorAll(".bz-nav").forEach(b=>b.onclick=()=>setKind(b.dataset.kind));
$("a-store").onchange=()=>{const a=$("a-store").value==="android";$("a-manual").hidden=!a;$("a-ios").hidden=a};
const num=i=>parseFloat($(i).value)||0;

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
document.querySelectorAll("#f-social input,#f-social select").forEach(e=>e.addEventListener("input",live));live();

// ---------- Connected Google: one-click connect & analyze ----------
async function loadGoogle(force){
  const st=await api("GET","/api/google/status");
  $("g-setup").hidden=st.configured;$("g-connect").hidden=st.connected||!st.configured;$("g-connected").hidden=!st.connected;
  if(!st.connected){gA=null;return null}
  if(gA&&!force)return gA;
  const a=await api("GET","/api/google/assets");gA=a;
  const fill=(id,items,val,lab)=>{const s=$(id);s.length=1;for(const x of items){const o=h("option",null,lab(x));o.value=val(x);s.appendChild(o)}if(items.length)s.selectedIndex=1};
  fill("g-site",a.sites||[],x=>x,x=>x);fill("g-prop",a.properties||[],x=>x.id,x=>x.name+" ("+x.id+")");
  $("g-chan").checked=!!a.channel;$("g-chname").textContent=a.channel||"(no channel found)";
  const src=$("g-src");src.innerHTML="";
  for(const t of[a.channel?"YouTube: "+a.channel:"YouTube: none found",(a.sites||[]).length+" Search Console site(s)",(a.properties||[]).length+" GA4 propert"+((a.properties||[]).length===1?"y":"ies")])src.appendChild(h("span","bz-src",t));
  $("g-errs").textContent=(a.errors||[]).length?"Some sources unavailable: "+a.errors.join(" | "):"";
  return a;
}
async function autoAnalyze(){
  const ol=$("g-steps");ol.hidden=false;
  const S=[["Connected to Google","done"],["Finding your channel, Search Console sites and Analytics properties","active"],["Analyzing your data","wait"],["Ready","wait"]];
  const draw=()=>{ol.innerHTML="";for(const s of S){const li=h("li","bz-step "+s[1]);add(li,h("i"),h("span",null,s[0]));ol.appendChild(li)}};
  draw();
  try{
    const a=await loadGoogle(false);
    if(!a)throw new Error("Not connected yet. Click Connect & Analyze.");
    const found=[a.channel?"1 YouTube channel":null,(a.sites||[]).length?a.sites.length+" Search Console site(s)":null,(a.properties||[]).length?a.properties.length+" GA4 property(ies)":null].filter(Boolean);
    if(!found.length){S[1]=["Nothing to analyze: this Google account has no YouTube channel, verified Search Console site or GA4 property.","fail"];draw();return}
    S[1]=["Found "+found.join(", "),"done"];S[2][1]="active";draw();
    const r=await api("POST","/api/evaluate",{kind:"google",site:$("g-site").value,property:$("g-prop").value,channel:$("g-chan").checked,useAI:$("ai").checked});
    hist.unshift(r);drawHistory();S[2][1]="done";S[3]=["Ready \u2013 your report is below","done"];draw();show(r,true);
  }catch(e){const i=S.findIndex(s=>s[1]==="active");if(i>=0){S[i][1]="fail";S[i][0]=e.message}draw()}
}
$("g-btn").onclick=async()=>{try{const r=await api("POST","/api/google/start");location.href=r.url}catch(e){$("msg").textContent=e.message}};
$("g-run").onclick=autoAnalyze;
$("g-off").onclick=async()=>{if(!confirm("Disconnect Google?"))return;await api("POST","/api/google/disconnect");gA=null;$("g-steps").hidden=true;loadGoogle(true)};

// ---------- standard evaluations ----------
function collect(){
  const ai=$("ai").checked,m=$("msg");m.textContent="";
  if(kind==="website"){const u=$("w-url").value.trim();if(!u){m.textContent="Enter a website address";return}return{kind,url:u,useAI:ai}}
  if(kind==="youtube"){const u=$("y-url").value.trim();if(!u){m.textContent="Paste a channel link or @handle";return}return{kind,query:u,useAI:ai}}
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
function show(r,scroll){current=r;renderReport($("result"),r,hist);if(scroll)$("result").scrollIntoView({behavior:"smooth"})}
$("run").onclick=async()=>{
  const b=collect();if(!b)return;
  busy(true);
  try{const r=await api("POST","/api/evaluate",b);hist.unshift(r);drawHistory();show(r,true)}
  catch(e){$("msg").textContent=e.message}
  busy(false);
};

// ---------- Optimizer & publish ----------
const splitTags=v=>v.split(",").map(x=>x.trim().replace(/^#/,"")).filter(Boolean);
$("o-go").onclick=async()=>{
  const m=$("o-msg");m.textContent="Generating…";$("o-go").disabled=true;
  try{
    const res=await api("POST","/api/optimize",{platform:$("o-platform").value,videoId:$("o-vid").value.trim(),title:$("o-title").value,description:$("o-desc").value,tags:splitTags($("o-tags").value),topic:$("o-topic").value,audience:$("o-aud").value,goal:$("o-goal").value});
    m.textContent="";renderOpt(res);
  }catch(e){m.textContent=e.message}
  $("o-go").disabled=false;
};
function renderOpt(res){
  const o=res.result,cur=res.current||{},pl=$("o-platform").value,vid=$("o-vid").value.trim();
  const box=$("optres");box.innerHTML="";box.hidden=false;
  const c=h("div","bz-card");c.style.marginTop="16px";c.appendChild(h("h3",null,"Optimized content (edit before using)"));
  const t=copyBox("Title / opener",(o.titles||[])[0]||cur.title||"",true),d=copyBox("Description / caption",o.description||"",true),g=copyBox("Tags",(o.tags||[]).join(", "),true),hs=copyBox("Hashtags",(o.hashtags||[]).join(" "),true);
  add(c,t,d,g,hs);
  if((o.titles||[]).length>1){c.appendChild(h("h4",null,"Other title options"));for(const x of o.titles.slice(1))c.appendChild(copyBox("Option",x))}
  for(const[label,items]of[["Hooks",o.hooks],["Visual / thumbnail ideas",o.ideas],["A/B testing tips",o.notes]])if((items||[]).length)c.appendChild(copyBox(label,items.join("\n")));
  const st=h("p","bz-mut");
  if(pl==="youtube"&&vid){const ap=h("button","bz-btn","Apply to my YouTube video");
    ap.onclick=async()=>{ap.disabled=true;st.textContent="Updating video…";
      try{await api("POST","/api/youtube/apply",{videoId:vid,title:t.ta.value,description:d.ta.value,tags:splitTags(g.ta.value)});st.textContent="Done. The video now uses the new title, description and tags."}
      catch(e){st.textContent=e.message}ap.disabled=false};add(c,ap,st)}
  box.appendChild(c);
  if(pl==="youtube")box.appendChild(uploadCard(t,d,g));
  else{const n=h("div","bz-card");n.style.marginTop="16px";n.appendChild(h("p","bz-mut","Automatic posting for this platform needs the platform's own approval of the app (Meta, TikTok, X). For now copy the text above and publish it yourself, or use YouTube publishing."));box.appendChild(n)}
  box.scrollIntoView({behavior:"smooth"});
}
function uploadCard(t,d,g){
  const c=h("div","bz-card");c.style.marginTop="16px";c.appendChild(h("h3",null,"Publish a new video to YouTube"));
  c.appendChild(h("p","bz-mut","Requires Connected Google. The file goes from your browser straight to YouTube. New API projects may be limited to private uploads until Google audits them; you can switch to public in YouTube Studio."));
  const f=h("div","form"),file=h("input"),pv=h("select"),when=h("input");file.type="file";file.accept="video/*";when.type="datetime-local";
  for(const v of["private","unlisted","public"]){const o=h("option",null,v);o.value=v;pv.appendChild(o)}
  const lab=(x,el)=>{const l=h("label",null,x);l.appendChild(el);return l};
  add(f,lab("Video file",file),lab("Visibility",pv),lab("Schedule (optional)",when));c.appendChild(f);
  const st=h("p","bz-mut"),b=h("button","bz-btn","Upload");
  b.onclick=async()=>{
    if(!file.files[0]){st.textContent="Choose a video file first";return}
    b.disabled=true;st.textContent="Starting upload…";
    try{
      const tk=(await api("GET","/api/google/token")).accessToken,fl=file.files[0],status={privacyStatus:pv.value,selfDeclaredMadeForKids:false};
      if(when.value){status.privacyStatus="private";status.publishAt=new Date(when.value).toISOString()}
      const init=await fetch("https://www.googleapis.com/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status",{method:"POST",headers:{Authorization:"Bearer "+tk,"Content-Type":"application/json; charset=UTF-8","X-Upload-Content-Type":fl.type||"video/*","X-Upload-Content-Length":String(fl.size)},body:JSON.stringify({snippet:{title:t.ta.value,description:d.ta.value,tags:splitTags(g.ta.value),categoryId:"22"},status})});
      if(!init.ok)throw new Error("Upload could not start ("+init.status+")");
      const loc=init.headers.get("Location");if(!loc)throw new Error("Upload location missing");
      await new Promise((res,rej)=>{const x=new XMLHttpRequest();x.open("PUT",loc);
        x.upload.onprogress=e=>{if(e.lengthComputable)st.textContent="Uploading… "+Math.round(e.loaded/e.total*100)+"%"};
        x.onload=()=>x.status<300?res():rej(new Error("Upload failed ("+x.status+")"));x.onerror=()=>rej(new Error("The browser blocked the upload. Upload manually in YouTube Studio."));x.send(fl)});
      st.textContent="Uploaded! Open YouTube Studio to check it (processing can take a few minutes).";
    }catch(e){st.textContent=e.message}
    b.disabled=false;
  };
  add(c,b,st);return c;
}

function drawHistory(){
  const box=$("hist");box.innerHTML="";
  if(!hist.length){box.appendChild(h("p","bz-mut","No evaluations yet."));return}
  for(const r of hist){
    const row=h("div","bz-hist");
    add(row,h("span","bz-score "+(r.score>=75?"pass":r.score>=50?"warn":"fail"),String(r.score)),h("b",null,r.target),h("span","bz-mut",r.kind+" \u00b7 "+new Date(r.createdAt).toLocaleDateString()));
    row.onclick=()=>show(r,true);box.appendChild(row);
  }
}

onAuthStateChanged(auth,async u=>{
  if(!u){location.href="/";return}
  $("name").textContent=(u.displayName||"").split(" ")[0];
  try{hist=await api("GET","/api/reports")}catch(e){$("msg").textContent=e.message}
  drawHistory();
  const g=new URLSearchParams(location.search).get("google");
  if(g==="ok"){setKind("google");history.replaceState(null,"","/dashboard");autoAnalyze()}
  else if(g){setKind("google");$("msg").textContent="Google connection failed. Try Connect & Analyze again."}
});
$("out").onclick=()=>signOut(auth);
