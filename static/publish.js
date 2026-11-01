import{h,add,api}from"./ui.js";

const TYPES={
  short:{label:"Short / Reel",hint:"Vertical video (9:16), up to about 3 minutes. Goes to YouTube Shorts, TikTok, Facebook Reels and Instagram Reels.",accept:"video/mp4,video/quicktime",plats:["youtube","tiktok","facebook","instagram"]},
  video:{label:"Normal video",hint:"Landscape or square video. Goes to YouTube, Facebook and Instagram (Instagram publishes all videos as Reels).",accept:"video/mp4,video/quicktime",plats:["youtube","facebook","instagram"]},
  image:{label:"Image",hint:"JPG works everywhere (Instagram does not accept PNG through its API). Goes to Facebook and Instagram.",accept:"image/jpeg,image/png",plats:["facebook","instagram"]}
};
const PN={youtube:"YouTube",facebook:"Facebook Page",instagram:"Instagram",tiktok:"TikTok"};
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
let auth,conn={},type="short",plats=new Set(),file=null,ui={};

export function mountPublish(root,a){
  auth=a;root.innerHTML="";
  const w=h("div","wide");root.appendChild(w);
  w.appendChild(h("p","bz-mut","One upload, many platforms. Pick what you are posting, choose the platforms, and Blitz AI sends it to each connected account."));
  ui.seg=h("div","seg3");
  for(const[k,t]of Object.entries(TYPES)){const b=h("button",null,t.label);b.dataset.t=k;b.onclick=()=>{type=k;file=null;ui.file.value="";ui.prev.innerHTML="";drawType()};ui.seg.appendChild(b)}
  ui.hint=h("p","bz-mut");
  ui.file=h("input");ui.file.type="file";
  ui.prev=h("div","pub-prev");
  ui.file.onchange=()=>{file=ui.file.files[0]||null;ui.prev.innerHTML="";if(file){const u=URL.createObjectURL(file);const el=document.createElement(type==="image"?"img":"video");el.src=u;if(type!=="image"){el.controls=true;el.muted=true}ui.prev.appendChild(el)}};
  ui.title=h("input");ui.title.maxLength=100;ui.title.placeholder="Title (YouTube and Facebook)";
  ui.cap=h("textarea");ui.cap.rows=6;ui.cap.maxLength=4000;ui.cap.placeholder="Caption / description. Add hashtags at the end.";
  ui.topic=h("input");ui.topic.placeholder="What is it about? (for AI caption)";
  ui.ai=h("button","bz-btn ghost","Write title, caption and hashtags with AI");
  ui.ai.onclick=async()=>{
    if(!ui.topic.value.trim()){ui.msg.textContent="Type what the post is about first.";return}
    ui.ai.disabled=true;ui.msg.textContent="Writing…";
    try{const r=await api("POST","/api/optimize",{platform:type==="image"?"instagram":"youtube",topic:ui.topic.value});const o=r.result||{};
      ui.title.value=(o.titles||[])[0]||ui.title.value;ui.cap.value=((o.description||"")+((o.hashtags||[]).length?"\n\n"+o.hashtags.slice(0,5).join(" "):"")).trim();ui.msg.textContent="Review and edit the text before publishing."}
    catch(e){ui.msg.textContent=e.message}ui.ai.disabled=false};
  ui.plats=h("div","chips");
  ui.when=h("input");ui.when.type="datetime-local";ui.when.onchange=drawPlats;
  ui.priv=h("select");for(const v of["private","unlisted","public"]){const o=h("option",null,v==="private"?"private (safest)":v);o.value=v;ui.priv.appendChild(o)}
  const lab=(t,el)=>{const l=h("label",null,t);l.appendChild(el);return l};
  const f=h("div","form");add(f,lab("Title",ui.title),lab("YouTube visibility",ui.priv),lab("Schedule (YouTube and Facebook only, optional)",ui.when));
  ui.go=h("button","bz-btn","Publish");ui.go.onclick=submit;
  ui.msg=h("p","bz-mut");ui.bar=h("div","pubbar");ui.barI=h("i");ui.bar.appendChild(ui.barI);ui.bar.hidden=true;ui.jobs=h("div","pub-jobs");
  add(w,ui.seg,ui.hint,lab("File",ui.file),ui.prev,f,lab("Caption / description",ui.cap),lab("Topic for AI (optional)",ui.topic),ui.ai,h("h4",null,"Post to"),ui.plats,ui.go,ui.bar,ui.msg,ui.jobs);
  drawType();
  return refresh;
}
export async function refresh(){
  try{const g=await api("GET","/api/google/status"),s=await api("GET","/api/social/status"),p=s.providers||{};
    conn={youtube:!!g.connected,facebook:!!(p.facebook||{}).connected,instagram:!!(p.instagram||{}).connected,tiktok:!!(p.tiktok||{}).connected}}catch(e){conn={}}
  drawPlats();
}
function drawType(){
  const t=TYPES[type];
  ui.seg.querySelectorAll("button").forEach(b=>b.classList.toggle("on",b.dataset.t===type));
  ui.hint.textContent=t.hint;ui.file.accept=t.accept;
  plats=new Set(t.plats.filter(p=>conn[p]));
  drawPlats();
}
function drawPlats(){
  const t=TYPES[type],scheduled=!!ui.when.value;
  ui.plats.innerHTML="";
  for(const p of["youtube","facebook","instagram","tiktok"]){
    const okType=t.plats.includes(p),okSched=!(scheduled&&(p==="instagram"||p==="tiktok")),connected=!!conn[p];
    const usable=okType&&okSched&&connected;
    if(!usable)plats.delete(p);
    const b=h("button","chip"+(plats.has(p)?" on":"")+(usable?"":" off"),PN[p]+(!okType?" (not for this type)":!connected?" (connect first)":!okSched?" (can't schedule)":""));
    b.onclick=()=>{if(!usable)return;if(plats.has(p))plats.delete(p);else plats.add(p);drawPlats()};
    ui.plats.appendChild(b);
  }
}
async function submit(){
  ui.msg.textContent="";
  if(!file){ui.msg.textContent="Choose a file first.";return}
  const list=[...plats];if(!list.length){ui.msg.textContent="Choose at least one platform that is connected.";return}
  if(file.size>300*1024*1024){ui.msg.textContent="The file is larger than 300 MB. Compress it first.";return}
  const fd=new FormData();
  fd.append("type",type);fd.append("title",ui.title.value);fd.append("caption",ui.cap.value);fd.append("privacy",ui.priv.value);fd.append("platforms",list.join(","));
  if(ui.when.value)fd.append("schedule",new Date(ui.when.value).toISOString());
  fd.append("file",file);
  ui.go.disabled=true;ui.bar.hidden=false;ui.barI.style.width="0";ui.jobs.innerHTML="";
  try{
    const tok=await auth.currentUser.getIdToken();
    const id=await new Promise((res,rej)=>{
      const x=new XMLHttpRequest();x.open("POST","/api/publish");x.setRequestHeader("Authorization","Bearer "+tok);
      x.upload.onprogress=e=>{if(e.lengthComputable){const p=Math.round(e.loaded/e.total*100);ui.barI.style.width=p+"%";ui.msg.textContent="Uploading to Blitz AI… "+p+"%"}};
      x.onload=()=>{if(x.status<300){try{res(JSON.parse(x.responseText).id)}catch(e){rej(new Error("Unexpected reply"))}}else rej(new Error(x.responseText||("Upload failed ("+x.status+")")))};
      x.onerror=()=>rej(new Error("The connection dropped while uploading. Try again, or use a smaller file."));x.send(fd)});
    ui.msg.textContent="Publishing to each platform… keep this page open.";
    for(let i=0;i<900;i++){
      let d;try{d=await api("GET","/api/publish/status?id="+id)}catch(e){await sleep(4000);continue}
      render(d);if(d.done)break;await sleep(3000);
    }
    ui.msg.textContent="Finished.";
  }catch(e){ui.msg.textContent=e.message}
  ui.go.disabled=false;
}
function render(d){
  ui.jobs.innerHTML="";
  for(const it of d.items){
    const row=h("div","pub-row"),st=h("span","bz-pill "+(it.state==="done"?"pass":it.state==="failed"?"fail":"warn"),it.state.toUpperCase());
    const body=h("div");body.appendChild(h("b",null,PN[it.platform]||it.platform));
    if(it.message)body.appendChild(h("p","bz-mut",it.message));
    if(it.url){const a=h("a",null,"Open post");a.href=it.url;a.target="_blank";a.rel="noopener";body.appendChild(a)}
    add(row,st,body);ui.jobs.appendChild(row);
  }
}
